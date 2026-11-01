package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
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

// --- VLESS ---

func TestVlessTunnelPlainTCP(t *testing.T) {
	target := newTarget(t, "hello-from-vless")
	srv := startFakeVLESS(t, testUUID, nil)
	up := config.Upstream{Type: config.UpstreamVless, Host: srv.addr(), UUID: testUUID}
	if body := tunnelGetThrough(t, up, target); !strings.Contains(body, "hello-from-vless") {
		t.Fatalf("unexpected body: %q", body)
	}
}

// TestVlessTunnelWithTLS runs the real client handshake against a local
// self-signed server, trusting that certificate only through testRoots
// (production code has no InsecureSkipVerify path).
func TestVlessTunnelWithTLS(t *testing.T) {
	cert := testCert(t)
	target := newTarget(t, "hello-from-vless-tls")
	srv := startFakeVLESS(t, testUUID, &tls.Config{Certificates: []tls.Certificate{cert}})

	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	testRoots = pool
	t.Cleanup(func() { testRoots = nil })

	up := config.Upstream{Type: config.UpstreamVless, Host: srv.addr(), UUID: testUUID,
		TLS: true, SNI: "sni.test"}
	if body := tunnelGetThrough(t, up, target); !strings.Contains(body, "hello-from-vless-tls") {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestVlessTLSHandshakeFailsWithUntrustedCert(t *testing.T) {
	// Without the throwaway root, the real client must reject the certificate:
	// proves certificate verification is on.
	target := newTarget(t, "nope")
	srv := startFakeVLESS(t, testUUID, &tls.Config{Certificates: []tls.Certificate{testCert(t)}})
	up := config.Upstream{Type: config.UpstreamVless, Host: srv.addr(), UUID: testUUID,
		TLS: true, SNI: "sni.test"}
	if _, err := DialTunnel(up, target, 3*time.Second); err == nil ||
		!strings.Contains(err.Error(), "TLS 握手失败") {
		t.Fatalf("expected TLS verification failure with an untrusted certificate, got %v", err)
	}
}

func TestVlessWrongUUIDRejected(t *testing.T) {
	target := newTarget(t, "nope")
	srv := startFakeVLESS(t, testUUID, nil)
	up := config.Upstream{Type: config.UpstreamVless, Host: srv.addr(),
		UUID: "11111111-2222-3333-4444-555555555555"}
	_, err := DialTunnel(up, target, 2*time.Second)
	if err == nil {
		t.Fatal("expected rejection for a mismatched uuid")
	}
	select {
	case <-srv.rejected:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not see the request as rejected")
	}
}

// TestDialTunnelDispatchesByType pins the dispatcher contract used by every
// inbound path and the prober.
func TestDialTunnelDispatchesByType(t *testing.T) {
	target := newTarget(t, "dispatch-ok")

	// shadowsocks (typed through the store → resolveUpstream path is covered
	// separately; here the struct itself is exercised).
	ssLn := startFakeSS(t, "aes-256-gcm", "pw")
	ss := config.Upstream{Type: config.UpstreamShadowsocks, Host: ssLn.Addr().String(),
		Method: "aes-256-gcm", Password: "pw"}
	if body := tunnelGetThrough(t, ss, target); !strings.Contains(body, "dispatch-ok") {
		t.Fatalf("shadowsocks dispatch: %q", body)
	}

	vl := startFakeVLESS(t, testUUID, nil)
	v := config.Upstream{Type: config.UpstreamVless, Host: vl.addr(), UUID: testUUID}
	if body := tunnelGetThrough(t, v, target); !strings.Contains(body, "dispatch-ok") {
		t.Fatalf("vless dispatch: %q", body)
	}

	// vmess is rejected before any network I/O.
	if _, err := DialTunnel(config.Upstream{Type: config.UpstreamVmess, Host: "127.0.0.1:1",
		UUID: testUUID}, target, time.Second); !errors.Is(err, ErrVmessUnsupported) {
		t.Fatalf("vmess should return ErrVmessUnsupported, got %v", err)
	}

	// socks5 and http still route through their own paths (HTTPS upstream is
	// covered by the existing suite; here just assert the default branch is
	// the CONNECT one, i.e. it fails against a non-HTTP listener).
	if _, err := DialTunnel(config.Upstream{Type: config.UpstreamHTTP, Host: vl.addr()}, target, time.Second); err == nil {
		t.Fatal("http CONNECT against a VLESS server should fail")
	}
}

// --- VMess ---

func TestVmessReturnsClearError(t *testing.T) {
	up := config.Upstream{Type: config.UpstreamVmess, Host: "127.0.0.1:1", UUID: testUUID}
	_, err := DialTunnel(up, "example.com:80", time.Second)
	if err == nil || !strings.Contains(err.Error(), "vmess 暂未支持") {
		t.Fatalf("expected explicit vmess error, got %v", err)
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

// TestInboundPlainHTTPViaVlessUpstream covers the plain-HTTP path (origin
// form request through a tunnel-only upstream).
func TestInboundPlainHTTPViaVlessUpstream(t *testing.T) {
	tAddr := newTarget(t, "hello-from-vless-plain")
	srv := startFakeVLESS(t, testUUID, nil)
	env := newEnv(t)
	u, err := env.store.UpsertUpstream("", &config.Upstream{
		Name: "vl", Type: config.UpstreamVless, Host: srv.addr(), UUID: testUUID,
	})
	if err != nil {
		t.Fatal(err)
	}
	p := env.addPort(t, freePort(t))
	env.addAccount(t, "alice", "wonder", p.ID, u.ID)
	env.sync(t)

	proxyURL := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort("127.0.0.1", strconv.Itoa(p.Port)),
		User:   url.UserPassword("alice", "wonder"),
	}
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   10 * time.Second,
	}
	resp, err := client.Get("http://" + tAddr + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(string(body), "hello-from-vless-plain") {
		t.Fatalf("bad response: %s %q", resp.Status, body)
	}
}
