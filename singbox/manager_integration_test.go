package singbox

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/sagernet/sing-box"

	"github.com/handsomezhuzhu/fastproxy/config"
)

const testUUID = "b831381d-6324-4d53-ad4f-8cda48b30811"

// staticSource is a fixed upstream pool for Manager tests.
type staticSource struct {
	mu  sync.Mutex
	ups []*config.Upstream
}

func (s *staticSource) Upstreams() []*config.Upstream {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*config.Upstream(nil), s.ups...)
}

func (s *staticSource) set(ups ...*config.Upstream) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ups = ups
}

func freePort(t *testing.T) uint16 {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	return uint16(ln.Addr().(*net.TCPAddr).Port)
}

// --- sing-box server side (the "remote node" under test) ---

type serverSpec struct {
	typ         string // vless | vmess
	security    string // "" | "tls" | "reality"
	flow        string
	publicKey   string
	privateKey  string
	shortID     string
	certPEM     string
	keyPEM      string
	transport   string // "" | "ws" | "grpc"
	path        string
	host        string
	serviceName string
	tlsTweak    string // "insecure_client" | "" — test-only hazard notes
}

// startServer starts a sing-box instance exposing one inbound of the given
// protocol on a free loopback port and returns its address and port.
func startServer(t *testing.T, spec serverSpec) (string, uint16) {
	t.Helper()
	port := freePort(t)
	raw := map[string]any{
		"type":        spec.typ,
		"listen":      "127.0.0.1",
		"listen_port": port,
	}
	switch spec.typ {
	case "vless":
		raw["users"] = []map[string]any{{"uuid": testUUID, "flow": spec.flow}}
	case "vmess":
		raw["users"] = []map[string]any{{"uuid": testUUID}}
	}
	if spec.security == "tls" {
		raw["tls"] = map[string]any{
			"enabled":     true,
			"certificate": []string{spec.certPEM},
			"key":         []string{spec.keyPEM},
		}
	}
	if spec.security == "reality" {
		raw["tls"] = map[string]any{
			"enabled": true,
			"reality": map[string]any{
				"enabled":     true,
				"handshake":   map[string]any{"server": "127.0.0.1", "server_port": int(realHandshakePort(t))},
				"private_key": spec.privateKey,
				"short_id":    []string{spec.shortID},
			},
		}
	}
	if spec.transport != "" {
		tr := map[string]any{"type": spec.transport}
		switch spec.transport {
		case "ws":
			tr["path"] = spec.path
			if spec.host != "" {
				tr["headers"] = map[string]any{"Host": spec.host}
			}
		case "grpc":
			tr["service_name"] = spec.serviceName
		}
		raw["transport"] = tr
	}
	cfg, err := boxOptionsFromJSON(t, map[string]any{
		"log":       map[string]any{"level": "warn"},
		"inbounds":  []any{raw},
		"outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}},
	})
	if err != nil {
		t.Fatalf("server config: %v", err)
	}
	instance, err := box.New(box.Options{Options: *cfg, Context: serverRegistries().context(context.Background())})
	if err != nil {
		t.Fatalf("new server box: %v", err)
	}
	if err := instance.Start(); err != nil {
		instance.Close()
		t.Fatalf("start server box: %v", err)
	}
	t.Cleanup(func() { _ = instance.Close() })
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port))), port
}

// realHandshakePort is a loopback listener kept open so REALITY's handshake
// target (a real site such as www.microsoft.com in production) accepts the
// disguised connection.
var (
	realHandshakeOnce sync.Once
	realHandshakeNum  uint16
)

func realHandshakePort(t *testing.T) uint16 {
	realHandshakeOnce.Do(func() {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			panic(err)
		}
		realHandshakeNum = uint16(ln.Addr().(*net.TCPAddr).Port)
		go func() {
			for {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				go func(c net.Conn) {
					io.Copy(io.Discard, c)
					c.Close()
				}(conn)
			}
		}()
	})
	return realHandshakeNum
}

// generateRealityKeyPair returns the base64 (raw URL) X25519 private/public
// key pair REALITY servers and clients exchange.
func generateRealityKeyPair(t *testing.T) (privateKey, publicKey string) {
	t.Helper()
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding
	return enc.EncodeToString(key.Bytes()), enc.EncodeToString(key.PublicKey().Bytes())
}

// --- client side helpers ---

func newManager(t *testing.T, ups ...*config.Upstream) (*Manager, *staticSource) {
	t.Helper()
	allowInsecure = true // trust the throwaway self-signed test certificates
	t.Cleanup(func() { allowInsecure = false })
	src := &staticSource{}
	src.set(ups...)
	m := New(src, nil)
	m.UseRegistry(serverRegistries())
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("manager start: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m, src
}

// httpThrough dials the httptest server through the manager and returns the
// response body.
func httpThrough(t *testing.T, m *Manager, up config.Upstream, addr string) string {
	t.Helper()
	body, err := m.tryDial(up, addr)
	if err != nil {
		t.Fatalf("dial %s: %v", up.Name, err)
	}
	return body
}

// tryDial performs the HTTP exchange behind httpThrough without failing the
// test (callers may turn a routing error into a skip).
func (m *Manager) tryDial(up config.Upstream, addr string) (string, error) {
	conn, err := m.Dial(up, addr, 5*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	req, err := http.NewRequest("GET", "http://"+addr+"/hello", nil)
	if err != nil {
		return "", err
	}
	req.Close = true
	if err := req.Write(conn); err != nil {
		return "", err
	}
	resp, err := http.ReadResponse(bufioReader(conn), req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func TestNeedsSingBox(t *testing.T) {
	cases := []struct {
		up   config.Upstream
		want bool
	}{
		{config.Upstream{Type: config.UpstreamVmess}, true},
		{config.Upstream{Type: config.UpstreamVless}, false},
		{config.Upstream{Type: config.UpstreamVless, Security: "reality"}, true},
		{config.Upstream{Type: config.UpstreamVless, Flow: "xtls-rprx-vision"}, true},
		{config.Upstream{Type: config.UpstreamVless, Transport: "ws"}, true},
		{config.Upstream{Type: config.UpstreamVless, Transport: "grpc"}, true},
		{config.Upstream{Type: config.UpstreamVless, Security: "tls", Transport: "tcp"}, false},
		{config.Upstream{Type: config.UpstreamHTTP}, false},
	}
	for i, c := range cases {
		if got := NeedsSingBox(c.up); got != c.want {
			t.Fatalf("case %d (%+v): got %v want %v", i, c.up, got, c.want)
		}
	}
}
