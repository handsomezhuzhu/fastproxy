package proxy

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"testing"
	"time"

	sscore "github.com/shadowsocks/go-shadowsocks2/core"
)

// --- Shadowsocks server double (mirrors go-shadowsocks2's server loop) ---

// startFakeSS starts a minimal Shadowsocks AEAD TCP server: every accepted
// connection is unwrapped with the given cipher, the target address is read,
// a real TCP connection is dialed and bytes are piped both ways.
func startFakeSS(t *testing.T, method, password string) net.Listener {
	t.Helper()
	// go-shadowsocks2 keeps the salt replay filter in a process-global
	// singleton: the client here "contacts the same node" as its own server
	// double, so every salt would look replayed. The library documents
	// SHADOWSOCKS_SF_CAPACITY<=0 as the switch to disable it (this variable
	// is only ever set from tests, never from production code).
	t.Setenv("SHADOWSOCKS_SF_CAPACITY", "-1")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			raw, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				// A fresh cipher per connection: PickCipher carries a node-wide
				// salt replay filter that would reject later salts.
				ciph, err := sscore.PickCipher(method, nil, password)
				if err != nil {
					raw.Close()
					return
				}
				// Close the cipher conn (not raw): AEAD keeps buffered data
				// and closing raw first would truncate the reply mid-write.
				shadow := ciph.StreamConn(raw)
				defer shadow.Close()
				target, err := readAddr(shadow)
				if err != nil {
					return
				}
				up, err := net.Dial("tcp", target)
				if err != nil {
					return
				}
				defer up.Close()
				pipeBoth(shadow, up)
			}()
		}
	}()
	return ln
}

// pipeBoth copies bytes in both directions and returns when either side ends
// (suitable for AEAD streams, whose Close flushes the final records).
func pipeBoth(a, b net.Conn) {
	done := make(chan struct{}, 2)
	go func() { io.Copy(a, b); done <- struct{}{} }()
	go func() { io.Copy(b, a); done <- struct{}{} }()
	<-done
	a.Close()
	b.Close()
}

// --- VLESS server double ---

// fakeVLESS is a minimal VLESS server (TCP transport only). It validates the
// request header the same way a real server does, dials the requested target
// and replies with [0x00 version][addonsLen][addons] before relaying bytes.
type fakeVLESS struct {
	ln       net.Listener
	uuid     []byte
	addons   []byte
	rejected chan struct{} // one token per rejected request
	tlsConf  *tls.Config   // nil = plain TCP
}

func startFakeVLESS(t *testing.T, uuid string, tlsConf *tls.Config) *fakeVLESS {
	t.Helper()
	rawUUID, err := parseVlessUUID(uuid)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	f := &fakeVLESS{ln: ln, uuid: rawUUID, tlsConf: tlsConf,
		addons: []byte{0xde, 0xad}, rejected: make(chan struct{}, 8)}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.handle(c)
		}
	}()
	return f
}

func (f *fakeVLESS) addr() string { return f.ln.Addr().String() }

func (f *fakeVLESS) handle(c net.Conn) {
	defer c.Close()
	conn := c
	if f.tlsConf != nil {
		tc := tls.Server(c, f.tlsConf)
		tc.SetDeadline(time.Now().Add(10 * time.Second))
		if err := tc.Handshake(); err != nil {
			return
		}
		tc.SetDeadline(time.Time{})
		conn = tc
	}
	br := bufio.NewReader(conn)

	head := make([]byte, 18) // version + 16B uuid + addonsLen
	if _, err := io.ReadFull(br, head); err != nil {
		return
	}
	if head[0] != 0 || !bytes.Equal(head[1:17], f.uuid) {
		f.rejected <- struct{}{}
		return
	}
	if n := int(head[17]); n > 0 {
		if _, err := io.CopyN(io.Discard, br, int64(n)); err != nil {
			return
		}
	}
	var cmdPort [3]byte
	if _, err := io.ReadFull(br, cmdPort[:]); err != nil {
		return
	}
	if cmdPort[0] != vlessCmdTCP {
		f.rejected <- struct{}{}
		return
	}
	// VLESS header carries the port; the address part is ATYP + ADDR only. Read
	// the address bytes and append the header port to get "host:port".
	var atyp [1]byte
	if _, err := io.ReadFull(br, atyp[:]); err != nil {
		f.rejected <- struct{}{}
		return
	}
	var addrBytes []byte
	switch atyp[0] {
	case atypIPv4:
		addrBytes = make([]byte, 1+4)
	case atypIPv6:
		addrBytes = make([]byte, 1+16)
	case atypDomain:
		var l [1]byte
		if _, err := io.ReadFull(br, l[:]); err != nil {
			f.rejected <- struct{}{}
			return
		}
		addrBytes = make([]byte, 1+1+int(l[0]))
		addrBytes[1] = l[0]
	default:
		f.rejected <- struct{}{}
		return
	}
	addrBytes[0] = atyp[0]
	if _, err := io.ReadFull(br, addrBytes[1:]); err != nil {
		f.rejected <- struct{}{}
		return
	}
	addr, err := readAddr(io.MultiReader(bytes.NewReader(addrBytes), bytes.NewReader(cmdPort[1:])))
	if err != nil {
		f.rejected <- struct{}{}
		return
	}

	up, err := net.Dial("tcp", addr)
	if err != nil {
		return
	}
	defer up.Close()
	if _, err := conn.Write(append([]byte{0x00, byte(len(f.addons))}, f.addons...)); err != nil {
		return
	}
	// The client->server direction must read through br: it may already hold
	// pipelined payload bytes (e.g. probe requests written before the reply).
	// The client half-closes after its request, so io.Copy(up, br) ends at EOF.
	go io.Copy(up, br)
	io.Copy(conn, up)
}

// --- self-signed certificate for the TLS variant ---

func testCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "fastproxy-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost", "sni.test"},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}
