package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handsomezhuzhu/fastproxy/config"
	"github.com/handsomezhuzhu/fastproxy/logx"
	"github.com/handsomezhuzhu/fastproxy/probe"
	"github.com/handsomezhuzhu/fastproxy/proxy"
)

// TestCheckUpstreamsEndpoint pins the single probe entry point:
// POST /api/upstreams/check {"ids":[...]}.
func TestCheckUpstreamsEndpoint(t *testing.T) {
	s := testServer(t)

	// Probes only an upstream bound to an upstream proxy would need a live
	// server; a direct unreachable target fails fast and still yields a result.
	rec := s.do(t, http.MethodPost, "/api/upstreams/check", `{"ids":["missing-id"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("check status %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Results []struct {
			ID        string `json:"id"`
			OK        bool   `json:"ok"`
			LatencyMS int64  `json:"latency_ms"`
			ExitIP    string `json:"exit_ip"`
			Error     string `json:"error"`
		} `json:"results"`
		OK     int `json:"ok"`
		Failed int `json:"failed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 1 || resp.Results[0].ID != "missing-id" || resp.Results[0].Error != "不存在" {
		t.Fatalf("unexpected results: %+v", resp)
	}
	if resp.OK != 0 || resp.Failed != 1 {
		t.Fatalf("counters: ok=%d failed=%d", resp.OK, resp.Failed)
	}
	// Every result carries all fixed keys, even when empty.
	raw := rec.Body.String()
	for _, key := range []string{`"latency_ms":`, `"exit_ip":`, `"error":`} {
		if !strings.Contains(raw, key) {
			t.Fatalf("result missing %s: %s", key, raw)
		}
	}

	// Empty body / {} / omitted ids all mean "all upstreams" (none here).
	for _, body := range []string{`{}`, `{"ids":[]}`} {
		rec := s.do(t, http.MethodPost, "/api/upstreams/check", body)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d %s", body, rec.Code, rec.Body.String())
		}
		var r2 struct {
			Results []any `json:"results"`
		}
		json.Unmarshal(rec.Body.Bytes(), &r2)
		if len(r2.Results) != 0 {
			t.Fatalf("%s: want no results, got %v", body, r2.Results)
		}
	}
}

// TestSubscriptionAPIDuplicateURL checks the API refuses a second
// subscription with the same decrypted URL and keeps exactly one row.
func TestSubscriptionAPIDuplicateURL(t *testing.T) {
	s := testServer(t)
	const body = `{"name":"first","url":"https://example.com/sub?token=same","enabled":false,"interval_sec":600}`
	if rec := s.do(t, http.MethodPost, "/api/subscriptions", body); rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	dup := `{"name":"second","url":"https://example.com/sub?token=same","enabled":false,"interval_sec":600}`
	rec := s.do(t, http.MethodPost, "/api/subscriptions", dup)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "已存在") {
		t.Fatalf("duplicate should be rejected: %d %s", rec.Code, rec.Body.String())
	}
	list := s.do(t, http.MethodGet, "/api/subscriptions", "")
	var out struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Total != 1 || len(out.Items) != 1 || out.Items[0].Name != "first" {
		t.Fatalf("duplicate create must not add a row: %+v", out)
	}
}

// TestImportAPIDeduplicates checks POST /api/upstreams/import is idempotent
// for the same node text.
func TestImportAPIDeduplicates(t *testing.T) {
	s := testServer(t)
	const text = `{"text":"http://u:p@1.1.1.1:8080#dup"}`
	first := s.do(t, http.MethodPost, "/api/upstreams/import", text)
	if first.Code != http.StatusOK {
		t.Fatalf("first import: %d %s", first.Code, first.Body.String())
	}
	var r1 struct {
		Added int `json:"added"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &r1); err != nil {
		t.Fatal(err)
	}
	if r1.Added != 1 {
		t.Fatalf("first import should add one node: %s", first.Body.String())
	}
	second := s.do(t, http.MethodPost, "/api/upstreams/import", text)
	if second.Code != http.StatusOK {
		t.Fatalf("second import: %d %s", second.Code, second.Body.String())
	}
	var r2 struct {
		Added      int      `json:"added"`
		Duplicates []string `json:"duplicates"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &r2); err != nil {
		t.Fatal(err)
	}
	if r2.Added != 0 || len(r2.Duplicates) != 1 {
		t.Fatalf("second import must report a duplicate: %s", second.Body.String())
	}
}

// TestRemovedProbeEndpoints checks the legacy routes are gone: only the
// unified check endpoint exists and bulk no longer accepts action=check.
func TestRemovedProbeEndpoints(t *testing.T) {
	s := testServer(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/upstreams/x1/check"},
		{http.MethodPost, "/api/upstreams/x1/test"},
		{http.MethodPost, "/api/upstreams/check-all"},
	} {
		if rec := s.do(t, tc.method, tc.path, `{}`); rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s %s should be gone, got %d: %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
	rec := s.do(t, http.MethodPost, "/api/upstreams/bulk", `{"ids":["x"],"action":"check"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "upstreams/check") {
		t.Fatalf("bulk check should be rejected with a hint, got %d %s", rec.Code, rec.Body.String())
	}
}

// TestUpstreamAPIRoundTrip exercises create/update/parse through the API and
// checks the TLS-free JSON shape (security only).
func TestUpstreamAPIRoundTrip(t *testing.T) {
	s := testServer(t)
	rec := s.do(t, http.MethodPost, "/api/upstreams", `{"name":"JP","type":"vless",
		"host":"j.example.com:443","uuid":"b831381d-6324-4d53-ad4f-8cda48b30811",
		"security":"tls","sni":"cdn.example.com"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"tls":`) {
		t.Fatalf("create response still exposes tls: %s", rec.Body.String())
	}
	var created struct {
		Saved struct {
			ID       string `json:"id"`
			Security string `json:"security"`
		} `json:"saved"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Saved.ID == "" || created.Saved.Security != "tls" {
		t.Fatalf("created upstream: %+v", created.Saved)
	}

	// A legacy {"tls":true} field alone no longer turns TLS on: security wins.
	rec = s.do(t, http.MethodPut, "/api/upstreams/"+created.Saved.ID, `{"name":"JP","type":"vless",
		"host":"j.example.com:443","uuid":"b831381d-6324-4d53-ad4f-8cda48b30811","tls":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	var updated struct {
		Saved struct {
			Security string `json:"security"`
		} `json:"saved"`
	}
	json.Unmarshal(rec.Body.Bytes(), &updated)
	if updated.Saved.Security != "none" {
		t.Fatalf("legacy tls flag must be ignored, security=%q", updated.Saved.Security)
	}

	// Data endpoint: port_range plus no tls key anywhere.
	rec = s.do(t, http.MethodGet, "/api/data", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"port_range":{"min":1,"max":65535}`) {
		t.Fatalf("data: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"tls":`) {
		t.Fatalf("/api/data still exposes tls: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"allow_direct"`) {
		t.Fatalf("/api/data still exposes allow_direct: %s", rec.Body.String())
	}
}

// fakeReloader is a minimal SingBoxReloader for status/reload tests.
type fakeReloader struct {
	err    error
	status map[string]any
}

func (f *fakeReloader) Dial(config.Upstream, string, time.Duration) (net.Conn, error) {
	return nil, errors.New("fake")
}
func (f *fakeReloader) Reload(context.Context) error { return f.err }
func (f *fakeReloader) LastError() error             { return f.err }
func (f *fakeReloader) Status() map[string]any       { return f.status }

// TestSingBoxStatusEndpoint covers GET /api/singbox/status with and without an
// installed manager, and the error surfaced in /api/data.
func TestSingBoxStatusEndpoint(t *testing.T) {
	s := testServer(t)
	rec := s.do(t, http.MethodGet, "/api/singbox/status", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"running":false`) {
		t.Fatalf("no manager: %d %s", rec.Code, rec.Body.String())
	}

	s.SetSingBox(&fakeReloader{status: map[string]any{
		"running": true, "reload_count": 2,
		"upstreams": []map[string]any{{"id": "x", "name": "X", "port": 20000}},
		"logs":      []string{"line"},
	}})
	rec = s.do(t, http.MethodGet, "/api/singbox/status", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"reload_count":2`) {
		t.Fatalf("with manager: %d %s", rec.Code, rec.Body.String())
	}

	// A failing manager is reported through /api/data without breaking it.
	s.SetSingBox(&fakeReloader{err: errors.New("boom"), status: map[string]any{"running": false}})
	rec = s.do(t, http.MethodGet, "/api/data", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"singbox_error":"boom"`) {
		t.Fatalf("data with error: %d %s", rec.Code, rec.Body.String())
	}
	t.Cleanup(func() { proxy.SingBoxDialer = nil })
}

// TestPortCreationHonoursRange checks that out-of-range ports are refused with
// a PORT_RANGE hint while boundaries pass.
func TestPortCreationHonoursRange(t *testing.T) {
	// Default PORT_RANGE (10001-10050) for this test.
	store, err := config.Open(filepath.Join(t.TempDir(), "pr.db"), 7788)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	logger := logx.New("", 100, logx.LevelError, false)
	mgr := proxy.NewManager(store, logger)
	t.Cleanup(func() { mgr.Close() })
	s := NewServer(store, mgr, logger, probe.NewRunner(store, logger), "test")

	rec := s.do(t, http.MethodPost, "/api/ports", `{"port":10001,"remark":"edge-low"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("low boundary: %d %s", rec.Code, rec.Body.String())
	}
	rec = s.do(t, http.MethodPost, "/api/ports", `{"port":10050,"remark":"edge-high"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("high boundary: %d %s", rec.Code, rec.Body.String())
	}
	rec = s.do(t, http.MethodPost, "/api/ports", `{"port":10051}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "PORT_RANGE") {
		t.Fatalf("out of range: %d %s", rec.Code, rec.Body.String())
	}
}
