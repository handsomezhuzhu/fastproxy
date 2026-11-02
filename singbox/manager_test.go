package singbox

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/handsomezhuzhu/fastproxy/config"
)

// selfSignedPEM returns a fresh self-signed certificate for
// 127.0.0.1/localhost as inline PEM strings for sing-box inbounds.
func selfSignedPEM(t *testing.T) (certPEM, keyPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "fastproxy-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	return certPEM, keyPEM
}

// TestManagerVLESSPlainTLS is the baseline end-to-end check: a local sing-box
// vless inbound (TLS, self-signed certificate) behind the manager's loopback
// SOCKS inbound, reaching an httptest target over the plain TCP transport.
//
// This is the only VLESS path FastProxy has: the native client is gone, so
// every VLESS upstream (plain TCP included) must route through sing-box.
func TestManagerVLESSPlainTLS(t *testing.T) {
	certPEM, keyPEM := selfSignedPEM(t)
	for _, transport := range []string{"", "ws"} {
		t.Run("transport="+transport, func(t *testing.T) {
			spec := serverSpec{
				typ: "vless", security: "tls", certPEM: certPEM, keyPEM: keyPEM,
				transport: transport, path: "/fp", host: "cdn.example.com",
			}
			addr, _ := startServer(t, spec)
			waitPortReady(t, addr)

			u := &config.Upstream{
				ID: "vl-tls-" + transport, Name: "vless-tls-" + transport,
				Type: config.UpstreamVless, Host: addr, UUID: testUUID,
				Security: config.SecurityTLS, Transport: transport,
				Path: spec.path, HostHeader: spec.host, SNI: "localhost",
			}
			m, _ := newManager(t, u)
			target := newHTTPTarget(t)
			body, err := m.tryDial(*u, target)
			if err != nil {
				t.Fatalf("plain-TLS VLESS must be dialable through sing-box: %v", err)
			}
			if body != "hello-through-singbox" {
				t.Fatalf("unexpected body %q", body)
			}
		})
	}
}

// TestManagerVLESSNoTLS covers security=none (no TLS at all).
func TestManagerVLESSNoTLS(t *testing.T) {
	addr, _ := startServer(t, serverSpec{typ: "vless"})
	waitPortReady(t, addr)
	u := &config.Upstream{
		ID: "vl-none", Name: "vless-none", Type: config.UpstreamVless,
		Host: addr, UUID: testUUID, Security: config.SecurityNone,
	}
	m, _ := newManager(t, u)
	target := newHTTPTarget(t)
	if body := httpThrough(t, m, *u, target); body != "hello-through-singbox" {
		t.Fatalf("unexpected body %q", body)
	}
}

// TestManagerVMessWebSocket covers VMess over WS (the native stack has no
// VMess at all).
func TestManagerVMessWebSocket(t *testing.T) {
	addr, _ := startServer(t, serverSpec{typ: "vmess", transport: "ws", path: "/vm"})
	waitPortReady(t, addr)

	up := &config.Upstream{
		ID: "vm-ws", Name: "vmess-ws", Type: config.UpstreamVmess, Host: addr,
		UUID: testUUID, Transport: "ws", Path: "/vm", VMessSecurity: "auto",
	}
	m, _ := newManager(t, up)
	target := newHTTPTarget(t)
	if body := httpThrough(t, m, *up, target); body != "hello-through-singbox" {
		t.Fatalf("unexpected body %q", body)
	}
}

// TestManagerReload verifies that the embedded box is rebuilt from the store,
// keeps port assignments and stops when nothing needs it.
func TestManagerReload(t *testing.T) {
	src := &staticSource{}
	m := New(src, nil)
	ctx := context.Background()
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if m.Running() {
		t.Fatal("an empty pool must not start a box")
	}
	if _, err := m.Dial(config.Upstream{ID: "nope", Name: "nope"}, "1.2.3.4:80", time.Second); err == nil {
		t.Fatal("dial of an unknown upstream must fail")
	}

	up := &config.Upstream{ID: "x1", Name: "x1", Type: config.UpstreamVmess, Host: "127.0.0.1:1", UUID: testUUID}
	src.set(up)
	if err := m.Reload(ctx); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !m.Running() {
		t.Fatal("box should be running")
	}
	port, ok := m.Ports()["x1"]
	if !ok || port < portStart {
		t.Fatalf("expected a loopback port for x1, got %v", m.Ports())
	}
	if err := m.Reload(ctx); err != nil {
		t.Fatalf("second reload: %v", err)
	}
	if got := m.Ports()["x1"]; got != port {
		t.Fatalf("port changed across reload: %d -> %d", port, got)
	}
	src.set()
	if err := m.Reload(ctx); err != nil {
		t.Fatalf("empty reload: %v", err)
	}
	if m.Running() {
		t.Fatalf("box should stop when no upstream needs it: %v", m.Ports())
	}
}
