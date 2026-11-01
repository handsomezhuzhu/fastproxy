package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handsomezhuzhu/fastproxy/config"
	"github.com/handsomezhuzhu/fastproxy/logx"
	"github.com/handsomezhuzhu/fastproxy/probe"
	"github.com/handsomezhuzhu/fastproxy/proxy"
)

// TestParseUpstreamEndpoint covers POST /api/upstreams/parse: the panel's
// "paste a share link and auto-fill the form" helper.
func TestParseUpstreamEndpoint(t *testing.T) {
	store, err := config.Open(filepath.Join(t.TempDir(), "fp.db"), 7788)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	logger := logx.New("", 100, logx.LevelError, false)
	mgr := proxy.NewManager(store, logger)
	srv := NewServer(store, mgr, logger, probe.NewRunner(store, logger), "test")

	post := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/upstreams/parse", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(srv.newCookie())
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}

	// Unauthenticated requests are rejected.
	unauth := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/upstreams/parse",
		strings.NewReader(`{"text":"vless://x@h:443"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(unauth, req)
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated parse: status %d", unauth.Code)
	}

	rec := post(`{"text":"vless://b831381d-6324-4d53-ad4f-8cda48b30811@r.example.com:443?security=reality&type=ws&sni=www.microsoft.com&pbk=PBKEY&sid=a1b2&fp=chrome&path=%2Fws&host=cdn.example.com&flow=xtls-rprx-vision#HK"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("parse status %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Proxy map[string]any `json:"proxy"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	p := resp.Proxy
	want := map[string]any{
		"name": "HK", "type": "vless", "host": "r.example.com:443",
		"uuid": "b831381d-6324-4d53-ad4f-8cda48b30811", "tls": true,
		"sni": "www.microsoft.com", "security": "reality", "pbk": "PBKEY", "sid": "a1b2",
		"flow": "xtls-rprx-vision", "fingerprint": "chrome", "transport": "ws",
		"path": "/ws", "host_header": "cdn.example.com",
	}
	for k, v := range want {
		if p[k] != v {
			t.Fatalf("proxy[%s] = %v, want %v (all: %v)", k, p[k], v, p)
		}
	}

	// Parse failures answer 400 with a Chinese reason.
	for _, body := range []string{
		`{"text":""}`,
		`{"text":"garbage"}`,
		`{"text":"vless://nope"}`,
	} {
		if rec := post(body); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d", body, rec.Code)
		}
	}
	rec = post(`{"text":"vmess://%%%"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "error") {
		t.Fatalf("bad vmess: status %d body %s", rec.Code, rec.Body.String())
	}
}
