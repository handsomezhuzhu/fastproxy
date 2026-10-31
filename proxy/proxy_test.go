package proxy

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/handsomezhuzhu/fastproxy/config"
	"github.com/handsomezhuzhu/fastproxy/logx"
)

// --- networking helpers ---

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// newTarget starts an HTTP server that replies with a fixed body.
func newTarget(t *testing.T, body string) string {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	t.Cleanup(ts.Close)
	return strings.TrimPrefix(ts.URL, "http://")
}

// fakeUpstream is a minimal HTTP proxy (CONNECT + absolute-form GET).
type fakeUpstream struct{ ln net.Listener }

func newFakeHTTPUpstream(t *testing.T, user, pass string) *fakeUpstream {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeUpstream{ln: ln}
	go f.serve(user, pass)
	t.Cleanup(func() { ln.Close() })
	return f
}

func (f *fakeUpstream) serve(user, pass string) {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handle(conn, user, pass)
	}
}

func (f *fakeUpstream) handle(conn net.Conn, user, pass string) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	if user != "" || pass != "" {
		u, p, ok := parseProxyAuth(req.Header.Get("Proxy-Authorization"))
		if !ok || u != user || p != pass {
			writePlainResponse(conn, http.StatusProxyAuthRequired)
			return
		}
	}
	if req.Method == http.MethodConnect {
		if _, err := conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
			return
		}
		up, err := net.Dial("tcp", req.Host)
		if err != nil {
			return
		}
		defer up.Close()
		cc := &clientConn{br: br, Conn: conn}
		fin := make(chan struct{}, 2)
		go func() { io.Copy(up, cc); fin <- struct{}{} }()
		go func() { io.Copy(cc, up); fin <- struct{}{} }()
		<-fin
		conn.Close()
		up.Close()
		return
	}
	resp, err := (&http.Transport{}).RoundTrip(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	resp.Close = true
	resp.Write(conn)
}

// serveFakeSocks5Upstream answers SOCKS5 CONNECTs and pipes raw bytes.
func serveFakeSocks5Upstream(ln net.Listener, user, pass string) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go handleFakeSocks5(conn, user, pass)
	}
}

func handleFakeSocks5(conn net.Conn, user, pass string) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	head := make([]byte, 2)
	if _, err := io.ReadFull(br, head); err != nil {
		return
	}
	methods := make([]byte, head[1])
	if _, err := io.ReadFull(br, methods); err != nil {
		return
	}
	if user != "" || pass != "" {
		if _, err := conn.Write([]byte{5, 2}); err != nil {
			return
		}
		ver := make([]byte, 2)
		if _, err := io.ReadFull(br, ver); err != nil {
			return
		}
		u := make([]byte, ver[1])
		if _, err := io.ReadFull(br, u); err != nil {
			return
		}
		pl := make([]byte, 1)
		if _, err := io.ReadFull(br, pl); err != nil {
			return
		}
		p := make([]byte, pl[0])
		if _, err := io.ReadFull(br, p); err != nil {
			return
		}
		if string(u) != user || string(p) != pass {
			conn.Write([]byte{1, 1})
			return
		}
		conn.Write([]byte{1, 0})
	} else {
		conn.Write([]byte{5, 0})
	}

	req := make([]byte, 4)
	if _, err := io.ReadFull(br, req); err != nil {
		return
	}
	var host string
	switch req[3] {
	case 1:
		b := make([]byte, 4)
		if _, err := io.ReadFull(br, b); err != nil {
			return
		}
		host = net.IP(b).String()
	case 4:
		b := make([]byte, 16)
		if _, err := io.ReadFull(br, b); err != nil {
			return
		}
		host = net.IP(b).String()
	case 3:
		l := make([]byte, 1)
		if _, err := io.ReadFull(br, l); err != nil {
			return
		}
		b := make([]byte, l[0])
		if _, err := io.ReadFull(br, b); err != nil {
			return
		}
		host = string(b)
	default:
		return
	}
	pb := make([]byte, 2)
	if _, err := io.ReadFull(br, pb); err != nil {
		return
	}
	port := int(binary.BigEndian.Uint16(pb))
	up, err := net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		conn.Write([]byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer up.Close()
	conn.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
	cc := &clientConn{br: br, Conn: conn}
	fin := make(chan struct{}, 2)
	go func() { io.Copy(up, cc); fin <- struct{}{} }()
	go func() { io.Copy(cc, up); fin <- struct{}{} }()
	<-fin
	up.Close()
}

// --- store/manager builder ---

type env struct {
	store *config.Store
	mgr   *Manager
}

func newEnv(t *testing.T) *env {
	t.Helper()
	store, err := config.Open(filepath.Join(t.TempDir(), "fastproxy.db"), freePort(t))
	if err != nil {
		t.Fatal(err)
	}
	env := &env{store: store, mgr: NewManager(store, logx.New("", 100, logx.LevelWarn, false))}
	t.Cleanup(func() {
		env.mgr.Close()
		env.store.Close()
	})
	return env
}

func (env *env) sync(t *testing.T) {
	t.Helper()
	if err := env.mgr.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
}

func (env *env) addUpstream(t *testing.T, typ, host, user, pass string) *config.Upstream {
	t.Helper()
	u, err := env.store.UpsertUpstream("", &config.Upstream{
		Name: "up-" + host, Type: typ, Host: host, Username: user, Password: pass,
	})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func (env *env) addPort(t *testing.T, port int, upstreamID string) *config.Port {
	t.Helper()
	p, err := env.store.UpsertPort("", &config.Port{Port: port, DefaultUpstream: upstreamID, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (env *env) addAccount(t *testing.T, user, pass, portID, upstreamID string) *config.Account {
	t.Helper()
	a, err := env.store.UpsertAccount("", &config.Account{
		Username: user, Password: pass, PortID: portID, UpstreamID: upstreamID, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// --- client helpers ---

func proxyAuthHeader(u, p string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(u+":"+p))
}

// connectTunnel performs an inbound CONNECT and returns the established conn.
func connectTunnel(t *testing.T, port int, auth string, target string) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := net.DialTimeout("tcp",
		net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: %s\r\n\r\n",
		target, target, auth)
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		conn.Close()
		t.Fatalf("CONNECT failed: %s", resp.Status)
	}
	return conn, br
}

// tunnelGet sends a plain HTTP GET over an established tunnel.
func tunnelGet(t *testing.T, conn net.Conn, br *bufio.Reader, target string) string {
	t.Helper()
	fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", target)
	// Read through the buffered reader first to honour bytes already read.
	body, err := io.ReadAll(io.MultiReader(br, conn))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// --- tests ---

func TestHTTPConnectTunnelViaHTTPUpstream(t *testing.T) {
	tAddr := newTarget(t, "hello-from-target")
	up := newFakeHTTPUpstream(t, "upuser", "uppass")
	env := newEnv(t)
	u := env.addUpstream(t, config.UpstreamHTTP, up.ln.Addr().String(), "upuser", "uppass")
	p := env.addPort(t, freePort(t), u.ID)
	env.addAccount(t, "alice", "wonder", p.ID, "")
	env.sync(t)

	conn, br := connectTunnel(t, p.Port, proxyAuthHeader("alice", "wonder"), tAddr)
	defer conn.Close()
	if body := tunnelGet(t, conn, br, tAddr); !strings.Contains(body, "hello-from-target") {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestPlainHTTPViaHTTPUpstream(t *testing.T) {
	tURL := "http://" + newTarget(t, "hello-from-target")
	up := newFakeHTTPUpstream(t, "", "")
	env := newEnv(t)
	u := env.addUpstream(t, config.UpstreamHTTP, up.ln.Addr().String(), "", "")
	p := env.addPort(t, freePort(t), u.ID)
	env.addAccount(t, "alice", "wonder", p.ID, "")
	env.sync(t)

	proxyURL := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort("127.0.0.1", strconv.Itoa(p.Port)),
		User:   url.UserPassword("alice", "wonder"),
	}
	resp, err := (&http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   10 * time.Second,
	}).Get(tURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(string(body), "hello-from-target") {
		t.Fatalf("bad response: %s %q", resp.Status, body)
	}
}

func TestSocks5InboundViaHTTPUpstream(t *testing.T) {
	tAddr := newTarget(t, "hello-from-target")
	up := newFakeHTTPUpstream(t, "upuser", "uppass")
	env := newEnv(t)
	u := env.addUpstream(t, config.UpstreamHTTP, up.ln.Addr().String(), "upuser", "uppass")
	p := env.addPort(t, freePort(t), u.ID)
	env.addAccount(t, "alice", "wonder", "", "") // global account
	env.sync(t)

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p.Port)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := Socks5Handshake(conn, "alice", "wonder", tAddr, 10*time.Second); err != nil {
		t.Fatalf("socks5 handshake: %v", err)
	}
	fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", tAddr)
	body, _ := io.ReadAll(conn)
	if !strings.Contains(string(body), "hello-from-target") {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestConnectViaSocks5Upstream(t *testing.T) {
	tAddr := newTarget(t, "hello-from-target")
	upSocks := freePort(t)
	upLn, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(upSocks)))
	if err != nil {
		t.Fatal(err)
	}
	go serveFakeSocks5Upstream(upLn, "su", "sp")
	t.Cleanup(func() { upLn.Close() })

	env := newEnv(t)
	u := env.addUpstream(t, config.UpstreamSocks5, upLn.Addr().String(), "su", "sp")
	p := env.addPort(t, freePort(t), u.ID)
	env.addAccount(t, "alice", "wonder", p.ID, "")
	env.sync(t)

	conn, br := connectTunnel(t, p.Port, proxyAuthHeader("alice", "wonder"), tAddr)
	defer conn.Close()
	if body := tunnelGet(t, conn, br, tAddr); !strings.Contains(body, "hello-from-target") {
		t.Fatalf("unexpected body: %q", body)
	}
}

// TestAccountOverridesUpstream proves user-based routing: the port default is
// a dead upstream, but the account's override works.
func TestAccountOverridesUpstream(t *testing.T) {
	tAddr := newTarget(t, "hello-from-target")
	upSocks := freePort(t)
	upLn, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(upSocks)))
	if err != nil {
		t.Fatal(err)
	}
	go serveFakeSocks5Upstream(upLn, "su", "sp")
	t.Cleanup(func() { upLn.Close() })

	env := newEnv(t)
	dead := env.addUpstream(t, config.UpstreamHTTP, "127.0.0.1:1", "", "")
	good := env.addUpstream(t, config.UpstreamSocks5, upLn.Addr().String(), "su", "sp")
	p := env.addPort(t, freePort(t), dead.ID) // default = dead upstream
	env.addAccount(t, "alice", "wonder", p.ID, good.ID)
	env.sync(t)

	conn, br := connectTunnel(t, p.Port, proxyAuthHeader("alice", "wonder"), tAddr)
	defer conn.Close()
	if body := tunnelGet(t, conn, br, tAddr); !strings.Contains(body, "hello-from-target") {
		t.Fatalf("account override did not route to the good upstream: %q", body)
	}
}

// TestHotReload verifies that account changes apply without a listener restart.
func TestHotReload(t *testing.T) {
	up := newFakeHTTPUpstream(t, "", "")
	env := newEnv(t)
	u := env.addUpstream(t, config.UpstreamHTTP, up.ln.Addr().String(), "", "")
	p := env.addPort(t, freePort(t), u.ID)
	env.addAccount(t, "alice", "wonder", p.ID, "")
	env.sync(t)

	env.mgr.mu.Lock()
	e1 := env.mgr.entries[p.ID]
	env.mgr.mu.Unlock()
	if e1 == nil {
		t.Fatal("entry missing after sync")
	}

	env.addAccount(t, "bob", "secret", p.ID, "")
	env.sync(t)

	env.mgr.mu.Lock()
	e2 := env.mgr.entries[p.ID]
	env.mgr.mu.Unlock()
	if e1 != e2 {
		t.Fatal("listener should not restart on account changes")
	}

	// bob can now authenticate; a disabled account fails.
	if _, err := env.store.SetAccountEnabled(
		env.store.Accounts()[1].ID, false); err != nil {
		t.Fatal(err)
	}
	env.sync(t)

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p.Port)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "GET http://example.com/ HTTP/1.1\r\nHost: example.com\r\nProxy-Authorization: %s\r\n\r\n",
		proxyAuthHeader("bob", "secret"))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("disabled account unexpectedly allowed: %s", resp.Status)
	}
}

func TestWrongHTTPAuthRejected(t *testing.T) {
	up := newFakeHTTPUpstream(t, "", "")
	env := newEnv(t)
	u := env.addUpstream(t, config.UpstreamHTTP, up.ln.Addr().String(), "", "")
	p := env.addPort(t, freePort(t), u.ID)
	env.addAccount(t, "alice", "wonder", p.ID, "")
	env.sync(t)

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p.Port)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "GET http://example.com/ HTTP/1.1\r\nHost: example.com\r\nProxy-Authorization: %s\r\n\r\n",
		proxyAuthHeader("alice", "wrong"))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("expected 407, got %s", resp.Status)
	}
}

func TestUnknownUserRejected(t *testing.T) {
	up := newFakeHTTPUpstream(t, "", "")
	env := newEnv(t)
	u := env.addUpstream(t, config.UpstreamHTTP, up.ln.Addr().String(), "", "")
	p := env.addPort(t, freePort(t), u.ID)
	env.addAccount(t, "alice", "wonder", p.ID, "")
	env.sync(t)

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p.Port)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "GET http://example.com/ HTTP/1.1\r\nHost: example.com\r\nProxy-Authorization: %s\r\n\r\n",
		proxyAuthHeader("nobody", "whatever"))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("expected 407, got %s", resp.Status)
	}
}

func TestSocks5WrongPasswordRejected(t *testing.T) {
	up := newFakeHTTPUpstream(t, "", "")
	env := newEnv(t)
	u := env.addUpstream(t, config.UpstreamHTTP, up.ln.Addr().String(), "", "")
	p := env.addPort(t, freePort(t), u.ID)
	env.addAccount(t, "alice", "wonder", p.ID, "")
	env.sync(t)

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p.Port)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// Auth fails during the SOCKS5 handshake, before any tunnel is dialed.
	err = Socks5Handshake(conn, "alice", "nope", "127.0.0.1:9", 10*time.Second)
	if err == nil || !strings.Contains(err.Error(), "账号或密码错误") {
		t.Fatalf("expected auth failure, got %v", err)
	}
}

func TestSocks5RequiresAuth(t *testing.T) {
	up := newFakeHTTPUpstream(t, "", "")
	env := newEnv(t)
	u := env.addUpstream(t, config.UpstreamHTTP, up.ln.Addr().String(), "", "")
	p := env.addPort(t, freePort(t), u.ID)
	env.addAccount(t, "alice", "wonder", p.ID, "")
	env.sync(t)

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p.Port)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// Client offers only the no-auth method; server must refuse (0xFF).
	conn.Write([]byte{5, 1, 0})
	rep := make([]byte, 2)
	if _, err := io.ReadFull(conn, rep); err != nil {
		t.Fatal(err)
	}
	if rep[0] != 5 || rep[1] != 0xFF {
		t.Fatalf("expected no-acceptable-method reply, got % x", rep)
	}
}

// TestPortDisableStopsListener ensures disabling a port syncs the listener.
func TestPortDisableStopsListener(t *testing.T) {
	up := newFakeHTTPUpstream(t, "", "")
	env := newEnv(t)
	u := env.addUpstream(t, config.UpstreamHTTP, up.ln.Addr().String(), "", "")
	p := env.addPort(t, freePort(t), u.ID)
	env.addAccount(t, "alice", "wonder", p.ID, "")
	env.sync(t)

	if _, err := env.store.SetPortEnabled(p.ID, false); err != nil {
		t.Fatal(err)
	}
	env.sync(t)

	conn, err := net.DialTimeout("tcp",
		net.JoinHostPort("127.0.0.1", strconv.Itoa(p.Port)), time.Second)
	if err == nil {
		conn.Close()
		t.Fatal("listener should be closed after disable")
	}
}
