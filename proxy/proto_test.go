package proxy

import (
	"io"
	"net"
	"testing"

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

