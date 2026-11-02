package admin

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func mustNets(t *testing.T, spec string) []*net.IPNet {
	t.Helper()
	nets, err := ParseTrustedProxies(spec)
	if err != nil {
		t.Fatal(err)
	}
	return nets
}

func TestParseTrustedProxies(t *testing.T) {
	nets := mustNets(t, " 127.0.0.1/32, 172.16.0.0/12 ,::1")
	if len(nets) != 3 {
		t.Fatalf("got %d nets", len(nets))
	}
	if !nets[0].Contains(net.ParseIP("127.0.0.1")) {
		t.Fatal("loopback not matched")
	}
	if !nets[1].Contains(net.ParseIP("172.20.1.2")) || nets[1].Contains(net.ParseIP("10.1.2.3")) {
		t.Fatal("172.16.0.0/12 not matched")
	}
	if !nets[2].Contains(net.ParseIP("::1")) {
		t.Fatal("bare IPv6 not matched")
	}
	if got := mustNets(t, ""); len(got) != 0 {
		t.Fatal("empty list should trust nobody")
	}
	for _, bad := range []string{"not-an-ip", "10.0.0.0/33", "1.2.3.4/abc"} {
		if _, err := ParseTrustedProxies(bad); err == nil || !strings.Contains(err.Error(), "TRUSTED_PROXIES") {
			t.Fatalf("%q: expected clear error, got %v", bad, err)
		}
	}
}

// TestClientInfoForwardedFor covers the right-most-untrusted XFF rule and the
// trusted-proxy gate on X-Forwarded-Proto.
func TestClientInfoForwardedFor(t *testing.T) {
	mask := testServer(t)
	mask.SetTrustedProxies(mustNets(t, "127.0.0.1/32,10.0.0.0/8,172.16.0.0/12"))
	bare := testServer(t) // no trusted proxies at all

	cases := []struct {
		name       string
		srv        *Server
		remoteAddr string
		xff        []string
		xfp        string
		wantAddr   string
		wantProto  string
	}{
		{"no headers", mask, "203.0.113.5:5555", nil, "", "203.0.113.5", "http"},
		{"untrusted peer sends XFF (ignored)", mask, "203.0.113.5:5555",
			[]string{"1.1.1.1"}, "https", "203.0.113.5", "http"},
		{"trusted proxy, single hop", mask, "127.0.0.1:44321",
			[]string{"198.51.100.7"}, "https", "198.51.100.7", "https"},
		{"trusted chain (proxy appends)", mask, "127.0.0.1:44321",
			[]string{"198.51.100.7, 10.1.2.3"}, "https", "198.51.100.7", "https"},
		{"client spoofs extra hops on the left", mask, "127.0.0.1:44321",
			[]string{"6.6.6.6, 198.51.100.7", "10.1.2.3"}, "", "198.51.100.7", "http"},
		{"all hops trusted falls back to peer", mask, "127.0.0.1:44321",
			[]string{"10.1.2.3, 172.16.0.9"}, "", "127.0.0.1", "http"},
		{"garbage hop stops the walk", mask, "127.0.0.1:44321",
			[]string{"198.51.100.7, not-an-ip"}, "", "127.0.0.1", "http"},
		{"promoted LAN proxy inside the chain", mask, "172.16.3.4:9000",
			[]string{"198.51.100.9, 172.16.0.9"}, "https", "198.51.100.9", "https"},
		{"peer outside every trusted CIDR", mask, "192.0.2.5:9000",
			[]string{"4.4.4.4"}, "https", "192.0.2.5", "http"},
		{"no trusted proxies: loopback ignored", bare, "127.0.0.1:1234",
			[]string{"1.1.1.1"}, "https", "127.0.0.1", "http"},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = c.remoteAddr
		for _, v := range c.xff {
			r.Header.Add("X-Forwarded-For", v)
		}
		if c.xfp != "" {
			r.Header.Set("X-Forwarded-Proto", c.xfp)
		}
		info := c.srv.clientInfo(r)
		if info.addr != c.wantAddr || info.proto != c.wantProto {
			t.Errorf("%s: got addr=%q proto=%q, want %q/%q", c.name, info.addr, info.proto, c.wantAddr, c.wantProto)
		}
	}
}

// TestSecureCookieDecision covers the Secure flag: direct TLS, trusted proxy
// with XFP=https, and everything else (no Secure).
func TestSecureCookieDecision(t *testing.T) {
	s := testServer(t)
	s.SetTrustedProxies(mustNets(t, "127.0.0.1/32"))
	if err := s.store.SetAdminPasswordHash(mustHash(t, "pw123456")); err != nil {
		t.Fatal(err)
	}

	login := func(remote string, headers map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"password":"pw123456"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = remote
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec
	}
	sessionCookie := func(rec *httptest.ResponseRecorder) *http.Cookie {
		for _, c := range rec.Result().Cookies() {
			if c.Name == cookieName {
				return c
			}
		}
		return nil
	}

	// 1. Plain HTTP from a non-proxy peer: no Secure.
	c := sessionCookie(login("203.0.113.10:5555", nil))
	if c == nil || c.Secure {
		t.Fatalf("plain http cookie should not be Secure: %+v", c)
	}
	// 2. Trusted proxy forwarding https: Secure.
	c = sessionCookie(login("127.0.0.1:5000", map[string]string{
		"X-Forwarded-For": "198.51.100.7", "X-Forwarded-Proto": "https"}))
	if c == nil || !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("trusted https cookie: %+v", c)
	}
	// 3. Untrusted peer claiming https: no Secure (claim ignored).
	c = sessionCookie(login("203.0.113.10:5555", map[string]string{"X-Forwarded-Proto": "https"}))
	if c == nil || c.Secure {
		t.Fatalf("untrusted https claim must be ignored: %+v", c)
	}
	// 4. Direct TLS: Secure.
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"password":"pw123456"}`))
	req.Header.Set("Content-Type", "application/json")
	req.TLS = &tls.ConnectionState{Version: tls.VersionTLS13}
	req.RemoteAddr = "203.0.113.10:5555"
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if c := sessionCookie(rec); c == nil || !c.Secure {
		t.Fatalf("direct TLS cookie should be Secure: %+v", c)
	}
}
