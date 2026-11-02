package proxy

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/handsomezhuzhu/fastproxy/config"
)

const testUUID = "b831381d-6324-4d53-ad4f-8cda48b30811"

// tunnelGetThrough dials the tunnel to target via up, sends a plain HTTP GET
// and returns the whole response body.
func tunnelGetThrough(t *testing.T, up config.Upstream, target string) string {
	t.Helper()
	conn, err := DialTunnel(up, target, 5*time.Second)
	if err != nil {
		t.Fatalf("DialTunnel(%s): %v", up.Type, err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", target)
	// half-close so servers waiting for client EOF can finish (like probe does)
	if hc, ok := conn.(interface{ CloseWrite() error }); ok {
		hc.CloseWrite()
	}
	body, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body)
}

// --- Shadowsocks ---

func TestShadowsocksTunnel(t *testing.T) {
	for _, method := range []string{"aes-256-gcm", "aes-128-gcm", "chacha20-ietf-poly1305"} {
		t.Run(method, func(t *testing.T) {
			target := newTarget(t, "hello-from-ss")
			ln := startFakeSS(t, method, "ss-secret")
			up := config.Upstream{Type: config.UpstreamShadowsocks, Host: ln.Addr().String(),
				Method: method, Password: "ss-secret"}
			if body := tunnelGetThrough(t, up, target); !strings.Contains(body, "hello-from-ss") {
				t.Fatalf("unexpected body: %q", body)
			}
		})
	}
}

func TestShadowsocksWrongPasswordFails(t *testing.T) {
	target := newTarget(t, "secret-body")
	ln := startFakeSS(t, "aes-256-gcm", "right")
	up := config.Upstream{Type: config.UpstreamShadowsocks, Host: ln.Addr().String(),
		Method: "aes-256-gcm", Password: "wrong"}
	conn, err := DialTunnel(up, target, 3*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	conn.Write([]byte("GET / HTTP/1.1\r\n\r\n"))
	// The server cannot authenticate the address block and drops the
	// connection: the client must observe EOF/timeout, never the body.
	buf, _ := io.ReadAll(conn)
	if strings.Contains(string(buf), "secret-body") {
		t.Fatalf("wrong password leaked traffic: %q", buf)
	}
}

func TestShadowsocksUnsupportedMethodError(t *testing.T) {
	up := config.Upstream{Type: config.UpstreamShadowsocks, Host: "127.0.0.1:1",
		Method: "rc4-md5", Password: "x"}
	_, err := DialTunnel(up, "example.com:80", time.Second)
	if err == nil || !strings.Contains(err.Error(), "暂不支持") {
		t.Fatalf("expected unsupported-method error, got %v", err)
	}
}

// --- VLESS / VMess dispatch (both go through the embedded sing-box) ---
//
// The VLESS end-to-end path is covered by the singbox package (a real
// sing-box inbound is started and dialled through the manager); here only the
// dispatcher contract of this package is pinned: vless/vmess never open a
// native connection and report a clear error when the hook is missing.

func TestDialTunnelVlessVmessRequireSingBox(t *testing.T) {
	if SingBoxDialer != nil {
		t.Fatal("test assumes no sing-box hook is installed")
	}
	for _, up := range []config.Upstream{
		{Type: config.UpstreamVless, Host: "127.0.0.1:1", UUID: testUUID},
		{Type: config.UpstreamVless, Host: "127.0.0.1:1", UUID: testUUID, Security: config.SecurityTLS, SNI: "a.example.com"},
		{Type: config.UpstreamVless, Host: "127.0.0.1:1", UUID: testUUID,
			Security: config.SecurityReality, PBK: "pk", SID: "01"},
		{Type: config.UpstreamVmess, Host: "127.0.0.1:1", UUID: testUUID},
	} {
		_, err := DialTunnel(up, "example.com:80", time.Second)
		if !errors.Is(err, ErrVmessUnsupported) {
			t.Fatalf("%s/%s: expected ErrVmessUnsupported, got %v", up.Type, up.Security, err)
		}
	}
}

func TestDialTunnelUsesSingBoxDialer(t *testing.T) {
	var got config.Upstream
	var gotAddr string
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	SingBoxDialer = func(up config.Upstream, addr string, timeout time.Duration) (net.Conn, error) {
		got, gotAddr = up, addr
		return client, nil
	}
	t.Cleanup(func() { SingBoxDialer = nil })

	up := config.Upstream{Type: config.UpstreamVless, ID: "x", Host: "h:443", UUID: testUUID}
	conn, err := DialTunnel(up, "example.com:443", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if got.ID != "x" || gotAddr != "example.com:443" {
		t.Fatalf("dialer received %+v / %q", got, gotAddr)
	}

	// A nil connection from the hook is an error, never a silent success.
	SingBoxDialer = func(config.Upstream, string, time.Duration) (net.Conn, error) { return nil, nil }
	if _, err := DialTunnel(up, "example.com:443", time.Second); err == nil {
		t.Fatal("nil connection should be rejected")
	}
}

// TestDialTunnelShadowsocksAndHTTP pins the remaining native dispatchers.
func TestDialTunnelShadowsocksAndHTTP(t *testing.T) {
	target := newTarget(t, "dispatch-ok")
	ssLn := startFakeSS(t, "aes-256-gcm", "pw")
	ss := config.Upstream{Type: config.UpstreamShadowsocks, Host: ssLn.Addr().String(),
		Method: "aes-256-gcm", Password: "pw"}
	if body := tunnelGetThrough(t, ss, target); !strings.Contains(body, "dispatch-ok") {
		t.Fatalf("shadowsocks dispatch: %q", body)
	}
	// The default branch is the HTTP CONNECT dial: it must fail against a
	// listener that is not an HTTP proxy.
	if _, err := DialTunnel(config.Upstream{Type: config.UpstreamHTTP, Host: ssLn.Addr().String()}, target, time.Second); err == nil {
		t.Fatal("http CONNECT against a shadowsocks server should fail")
	}
}

// --- inbound end-to-end through the manager ---

// TestInboundConnectViaShadowsocksUpstream proves the full path: inbound
// CONNECT on a managed port -> account -> Shadowsocks upstream -> target.
func TestInboundConnectViaShadowsocksUpstream(t *testing.T) {
	target := newTarget(t, "hello-from-ss-inbound")
	ln := startFakeSS(t, "chacha20-ietf-poly1305", "pw")
	env := newEnv(t)
	u, err := env.store.UpsertUpstream("", &config.Upstream{
		Name: "ss", Type: config.UpstreamShadowsocks, Host: ln.Addr().String(),
		Method: "chacha20-ietf-poly1305", Password: "pw",
	})
	if err != nil {
		t.Fatal(err)
	}
	p := env.addPort(t, freePort(t))
	env.addAccount(t, "alice", "wonder", p.ID, u.ID)
	env.sync(t)

	conn, br := connectTunnel(t, p.Port, proxyAuthHeader("alice", "wonder"), target)
	defer conn.Close()
	if body := tunnelGet(t, conn, br, target); !strings.Contains(body, "hello-from-ss-inbound") {
		t.Fatalf("unexpected body: %q", body)
	}
}
