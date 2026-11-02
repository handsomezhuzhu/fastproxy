package admin

import (
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

// testServer builds a panel over a fresh store (full port range), the real
// proxy manager and prober; nothing is listening unless a test starts it.
func testServer(t *testing.T) *Server {
	t.Helper()
	store, err := config.OpenWithPortRange(filepath.Join(t.TempDir(), "fp.db"), 7788,
		config.PortRange{Min: 1, Max: 65535})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	logger := logx.New("", 100, logx.LevelError, false)
	t.Cleanup(logger.Close)
	mgr := proxy.NewManager(store, logger)
	t.Cleanup(func() { mgr.Close() })
	return NewServer(store, mgr, logger, probe.NewRunner(store, logger), "test")
}

// do performs an authenticated JSON request against the panel.
func (s *Server) do(t *testing.T, method, target, body string, hdr ...string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	r.AddCookie(s.newCookie())
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, r)
	return rec
}
