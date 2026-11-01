//go:build with_utls

package singbox

import (
	"crypto/tls"
	"net"
	"net/http"
	"testing"

	"github.com/handsomezhuzhu/fastproxy/config"
)

// newTLSHandshakeTarget starts a real TLS 1.3 server. REALITY forwards the
// client's ClientHello to this target and relays its ServerHello, so a plain
// discard listener (the old test setup) can never complete a handshake.
func newTLSHandshakeTarget(t *testing.T) uint16 {
	t.Helper()
	certPEM, keyPEM := selfSignedPEM(t)
	cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		t.Fatalf("tls key pair: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("reality-handshake-target"))
	})}
	tlsLn := tls.NewListener(ln, &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	})
	go srv.Serve(tlsLn)
	t.Cleanup(func() { _ = srv.Close() })
	return uint16(ln.Addr().(*net.TCPAddr).Port)
}

// TestManagerVLESSReality proves the embedded REALITY client can authenticate
// against a REALITY server built from the same sing-box version, then carry a
// real HTTP request through the tunnel.
func TestManagerVLESSReality(t *testing.T) {
	handshakePort := newTLSHandshakeTarget(t)
	privateKey, publicKey := generateRealityKeyPair(t)
	const shortID = "0123456789abcdef"

	addr, _ := startServer(t, serverSpec{
		typ:           "vless",
		security:      "reality",
		privateKey:    privateKey,
		shortID:       shortID,
		handshakePort: handshakePort,
		serverNames:   []string{"localhost"},
	})
	waitPortReady(t, addr)

	u := &config.Upstream{
		ID:          "vl-reality",
		Name:        "vless-reality",
		Type:        config.UpstreamVless,
		Host:        addr,
		UUID:        testUUID,
		Security:    config.SecurityReality,
		PBK:         publicKey,
		SID:         shortID,
		SNI:         "localhost",
		Fingerprint: "chrome",
	}
	m, _ := newManager(t, u)
	target := newHTTPTarget(t)
	body, err := m.tryDial(*u, target)
	if err != nil {
		t.Fatalf("REALITY tunnel: %v", err)
	}
	if body != "hello-through-singbox" {
		t.Fatalf("unexpected body %q", body)
	}
}
