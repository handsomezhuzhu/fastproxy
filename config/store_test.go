package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handsomezhuzhu/fastproxy/logx"
)

func tempStore(t *testing.T, adminPort int) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "fastproxy.db"), adminPort)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func upstreamFixture(name string) *Upstream {
	return &Upstream{Name: name, Type: UpstreamHTTP, Host: "10.0.0.1:8080"}
}

func portFixture(port int, upID string) *Port {
	return &Port{Port: port, DefaultUpstream: upID, Enabled: true}
}

func accountFixture(user, pass, portID, upID string) *Account {
	return &Account{Username: user, Password: pass, PortID: portID, UpstreamID: upID, Enabled: true}
}

func TestOpenCreatesSchemaAndDefaults(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "fastproxy.db"), 7788)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if len(s.Upstreams())+len(s.Ports())+len(s.Accounts()) != 0 {
		t.Fatal("expected empty database")
	}
	if s.SessionSecret() == "" {
		t.Fatal("session secret should be generated")
	}
	if s.AdminPasswordHash() != "" {
		t.Fatal("admin password should be unset initially")
	}
	if s.Settings() != DefaultSettings() {
		t.Fatalf("default settings mismatch: %+v", s.Settings())
	}
	if _, err := os.Stat(filepath.Join(dir, "fastproxy.db")); err != nil {
		t.Fatalf("database file not created: %v", err)
	}
}

func TestLegacyJSONMigration(t *testing.T) {
	dir := t.TempDir()
	legacy := `{
      "admin_password_hash": "pbkdf2-sha256$120000$aa$bb",
      "session_secret": "secret-123",
      "upstreams": [{"id":"u1","name":"旧上游","type":"http","host":"1.1.1.1:8080","username":"u","password":"p"}],
      "ports": [{"id":"p1","port":10201,"default_upstream":"u1","enabled":true,"remark":"旧端口"}],
      "accounts": [{"id":"a1","username":"alice","password":"wonder","port_id":"p1","upstream_id":"","enabled":true}]
    }`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(filepath.Join(dir, "fastproxy.db"), 7788)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if got := len(s.Upstreams()); got != 1 {
		t.Fatalf("expected 1 upstream after migration, got %d", got)
	}
	if got := s.Upstreams()[0]; got.Name != "旧上游" || got.Username != "u" {
		t.Fatalf("upstream migration mismatch: %+v", got)
	}
	if got := len(s.Ports()); got != 1 || s.Ports()[0].Port != 10201 {
		t.Fatalf("port migration mismatch: %+v", s.Ports())
	}
	if got := len(s.Accounts()); got != 1 || s.Accounts()[0].Username != "alice" {
		t.Fatalf("account migration mismatch: %+v", s.Accounts())
	}
	if s.SessionSecret() != "secret-123" {
		t.Fatalf("session secret not migrated: %q", s.SessionSecret())
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json.migrated")); err != nil {
		t.Fatalf("legacy file should be renamed: %v", err)
	}

	// Reopening must not duplicate migrated rows.
	s2, err := Open(filepath.Join(dir, "fastproxy.db"), 7788)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if got := len(s2.Upstreams()); got != 1 {
		t.Fatalf("migration duplicated rows: %d upstreams", got)
	}
}

func TestUpstreamCRUD(t *testing.T) {
	s := tempStore(t, 7788)
	saved, err := s.UpsertUpstream("", upstreamFixture("美国家宽"))
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID == "" || saved.Type != UpstreamHTTP {
		t.Fatalf("unexpected upstream: %+v", saved)
	}

	if _, err := s.UpsertPort("", portFixture(10201, saved.ID)); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUpstream(saved.ID); err == nil {
		t.Fatal("expected delete to fail while referenced by a port")
	}

	if _, err := s.UpsertUpstream("", &Upstream{Name: "x", Type: "ftp", Host: "1.2.3.4:21"}); err == nil {
		t.Fatal("expected type error")
	}
	if _, err := s.UpsertUpstream("", &Upstream{Name: "x", Host: "no-port"}); err == nil {
		t.Fatal("expected host error")
	}

	updated, err := s.UpsertUpstream(saved.ID, &Upstream{Name: "改名", Type: "socks5", Host: "2.2.2.2:1080"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "改名" || updated.Type != "socks5" {
		t.Fatalf("update mismatch: %+v", updated)
	}

	spare, err := s.UpsertUpstream("", upstreamFixture("备用"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUpstream(spare.ID); err != nil {
		t.Fatalf("delete spare: %v", err)
	}
	if err := s.DeleteUpstream("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPortValidation(t *testing.T) {
	s := tempStore(t, 7788)
	up, err := s.UpsertUpstream("", upstreamFixture("u"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertPort("", portFixture(10201, up.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertPort("", portFixture(10201, up.ID)); err == nil {
		t.Fatal("expected duplicate port error")
	}
	if _, err := s.UpsertPort("", portFixture(7788, up.ID)); err == nil {
		t.Fatal("expected admin-port conflict")
	}
	if _, err := s.UpsertPort("", portFixture(10202, "missing")); err == nil {
		t.Fatal("expected missing upstream error")
	}
	if _, err := s.UpsertPort("", portFixture(10202, "")); err != nil {
		t.Fatalf("direct port should be allowed: %v", err)
	}

	first := s.Ports()[0]
	if _, err := s.SetPortEnabled(first.ID, false); err != nil {
		t.Fatal(err)
	}
	if p := s.getPortLocked(first.ID); p == nil || p.Enabled {
		t.Fatalf("disable not applied: %+v", p)
	}

	// Deleting a port unbinds its accounts.
	acc := accountFixture("tom", "pw", first.ID, "")
	if _, err := s.UpsertAccount("", acc); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePort(first.ID); err != nil {
		t.Fatal(err)
	}
	for _, a := range s.Accounts() {
		if a.PortID == first.ID {
			t.Fatalf("account still bound to deleted port: %+v", a)
		}
	}
}

func TestAccountValidationAndUniqueness(t *testing.T) {
	s := tempStore(t, 7788)
	up, err := s.UpsertUpstream("", upstreamFixture("u"))
	if err != nil {
		t.Fatal(err)
	}
	port, err := s.UpsertPort("", portFixture(10201, up.ID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertAccount("", accountFixture("tom", "pw", port.ID, "")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertAccount("", accountFixture("tom", "pw2", port.ID, "")); err == nil {
		t.Fatal("expected duplicate username error")
	}
	if _, err := s.UpsertAccount("", accountFixture("", "pw", port.ID, "")); err == nil {
		t.Fatal("expected empty username error")
	}
	if _, err := s.UpsertAccount("", accountFixture("jerry", "pw", "missing-port", "")); err == nil {
		t.Fatal("expected missing port error")
	}
	if _, err := s.UpsertAccount("", accountFixture("jerry", "pw", "", "missing-up")); err == nil {
		t.Fatal("expected missing upstream error")
	}
	if _, err := s.UpsertAccount("", accountFixture("jerry", "pw", "", "")); err != nil {
		t.Fatalf("global account rejected: %v", err)
	}

	// Username update conflict is rejected.
	tom := s.Accounts()[0]
	if tom.Username != "tom" {
		t.Fatalf("unexpected first account: %+v", tom)
	}
	if _, err := s.UpsertAccount(tom.ID, accountFixture("jerry", "pw", "", "")); err == nil {
		t.Fatal("expected conflict when renaming to an existing username")
	}
}

func TestSettingsValidation(t *testing.T) {
	s := tempStore(t, 7788)
	next := DefaultSettings()
	next.AllowDirect = false
	next.LogLevel = "debug"
	next.LogRetentionDays = 3
	if _, err := s.UpdateSettings(next); err != nil {
		t.Fatal(err)
	}
	if s.Settings().AllowDirect || s.Settings().LogRetentionDays != 3 {
		t.Fatalf("settings not persisted: %+v", s.Settings())
	}

	if _, err := s.UpsertPort("", portFixture(10201, "")); err == nil {
		t.Fatal("expected direct port rejection")
	}
	up, err := s.UpsertUpstream("", upstreamFixture("u"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertPort("", portFixture(10201, up.ID)); err != nil {
		t.Fatalf("bound port should be allowed: %v", err)
	}

	bad := DefaultSettings()
	bad.LogLevel = "verbose"
	if _, err := s.UpdateSettings(bad); err == nil {
		t.Fatal("expected log level validation error")
	}
	bad = DefaultSettings()
	bad.LogRetentionDays = 0
	if _, err := s.UpdateSettings(bad); err == nil {
		t.Fatal("expected retention validation error")
	}
}

func TestImportUpstreamsDedupe(t *testing.T) {
	s := tempStore(t, 7788)
	added, dups := s.ImportUpstreams([]*Upstream{
		{Name: "a", Type: "http", Host: "1.1.1.1:8080"},
		{Name: "b", Type: "socks5", Host: "2.2.2.2:1080", Username: "u", Password: "p"},
	})
	if added != 2 || len(dups) != 0 {
		t.Fatalf("first import: added=%d dups=%v", added, dups)
	}
	added, dups = s.ImportUpstreams([]*Upstream{
		{Name: "a2", Type: "http", Host: "1.1.1.1:8080"},
		{Name: "b2", Type: "socks5", Host: "2.2.2.2:1080", Username: "u", Password: "p"},
	})
	if added != 0 || len(dups) != 2 {
		t.Fatalf("second import: added=%d dups=%v", added, dups)
	}
	added, _ = s.ImportUpstreams([]*Upstream{
		{Name: "c", Type: "socks5", Host: "2.2.2.2:1080", Username: "u", Password: "p2"},
	})
	if added != 1 {
		t.Fatalf("expected 1 added for different password, got %d", added)
	}
}

func TestLogPersistenceAndPrune(t *testing.T) {
	s := tempStore(t, 7788)
	entries := []struct {
		port   int
		user   string
		level  string
		status string
	}{
		{10201, "alice", "info", ""},
		{10201, "bob", "warn", "auth-failed"},
		{10202, "alice", "info", ""},
	}
	for _, e := range entries {
		if err := s.InsertLog(testEntry(e.port, e.user, e.level, e.status)); err != nil {
			t.Fatal(err)
		}
	}

	all, err := s.QueryLogs(testFilter(10, 0, "", "debug"))
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 logs, got %d", len(all))
	}
	// Newest first.
	if all[0].Port != 10202 {
		t.Fatalf("expected newest first, got %+v", all[0])
	}

	byPort, _ := s.QueryLogs(testFilter(10, 10201, "", "debug"))
	if len(byPort) != 2 {
		t.Fatalf("port filter: expected 2, got %d", len(byPort))
	}
	byUser, _ := s.QueryLogs(testFilter(10, 0, "alice", "debug"))
	if len(byUser) != 2 {
		t.Fatalf("account filter: expected 2, got %d", len(byUser))
	}
	warns, _ := s.QueryLogs(testFilter(10, 0, "", "warn"))
	if len(warns) != 1 || warns[0].Status != "auth-failed" {
		t.Fatalf("level filter mismatch: %+v", warns)
	}

	// Age the rows beyond the retention window, then prune.
	if _, err := s.db.Exec(`UPDATE logs SET ts = ?`, time.Now().AddDate(0, 0, -10).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	n, err := s.PruneLogs(7)
	if err != nil || n != 3 {
		t.Fatalf("prune: n=%d err=%v", n, err)
	}
	left, _ := s.QueryLogs(testFilter(10, 0, "", "debug"))
	if len(left) != 0 {
		t.Fatalf("expected empty logs after prune, got %d", len(left))
	}
}

func testEntry(port int, user, level, status string) logx.Entry {
	return logx.Entry{
		Level: level, Port: port, Account: user, Status: status,
		Proto: "http", Target: "example.com:443",
		BytesIn: 10, BytesOut: 20, DurationMS: 5,
	}
}

func testFilter(limit, port int, account, level string) logx.Filter {
	return logx.Filter{Limit: limit, Port: port, Account: account, Level: level}
}

func TestParseUpstream(t *testing.T) {
	cases := []struct {
		raw     string
		typ     string
		host    string
		user    string
		pass    string
		wantErr bool
	}{
		{"1.2.3.4:8080", "http", "1.2.3.4:8080", "", "", false},
		{"http://1.2.3.4:8080", "http", "1.2.3.4:8080", "", "", false},
		{"http://u:p@1.2.3.4:8080", "http", "1.2.3.4:8080", "u", "p", false},
		{"socks5://u:p@h:1080", "socks5", "h:1080", "u", "p", false},
		{"socks://h:1080", "socks5", "h:1080", "", "", false},
		{"http://u:p@ss:w@1.2.3.4:8080", "http", "1.2.3.4:8080", "u", "p@ss:w", false},
		{"http://u@1.2.3.4:8080", "http", "1.2.3.4:8080", "u", "", false},
		{"socks5://${VAR}:${PW}@host:1080", "socks5", "host:1080", "${VAR}", "${PW}", false},
		{"ftp://1.2.3.4:21", "", "", "", "", true},
		{"http://1.2.3.4", "", "", "", "", true},
		{"", "", "", "", "", true},
	}
	for _, c := range cases {
		typ, host, user, pass, err := ParseUpstream(c.raw)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseUpstream(%q): expected error", c.raw)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseUpstream(%q): %v", c.raw, err)
			continue
		}
		if typ != c.typ || host != c.host || user != c.user || pass != c.pass {
			t.Errorf("ParseUpstream(%q) = (%q,%q,%q,%q), want (%q,%q,%q,%q)",
				c.raw, typ, host, user, pass, c.typ, c.host, c.user, c.pass)
		}
	}
}

func TestSecretEnvInterpolation(t *testing.T) {
	t.Setenv("FP_TEST_UP_PASS", "s3cret")
	s := tempStore(t, 7788)

	u, err := s.UpsertUpstream("", &Upstream{
		Name: "u", Type: UpstreamHTTP, Host: "1.2.3.4:8080",
		Username: "u1", Password: "${FP_TEST_UP_PASS}",
	})
	if err != nil {
		t.Fatal(err)
	}
	if u.Password != "s3cret" {
		t.Fatalf("upstream password not expanded: %q", u.Password)
	}
	// Missing env var rejected.
	if _, err := s.UpsertUpstream("", &Upstream{
		Name: "u2", Type: UpstreamHTTP, Host: "1.2.3.4:8080", Password: "${FP_TEST_MISSING}",
	}); err == nil {
		t.Fatal("expected missing-env error")
	}
	// Default syntax.
	a, err := s.UpsertAccount("", &Account{
		Username: "bob", Password: "${FP_TEST_NONE:-fallback}", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.Password != "fallback" {
		t.Fatalf("default syntax not applied: %q", a.Password)
	}
}

func TestOpenFailsOnMissingSecretRef(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"upstreams":[{"id":"u1","name":"u","type":"http","host":"1.2.3.4:8080","password":"${FP_TEST_NONE_XYZ}"}],"ports":[],"accounts":[]}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(dir, "fastproxy.db"), 7788); err == nil {
		t.Fatal("expected startup error for missing secret env")
	}
}

func TestUpstreamProbeState(t *testing.T) {
	s := tempStore(t, 7788)
	u, err := s.UpsertUpstream("", upstreamFixture("probe"))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Upstreams()[0]; got.Health != "unknown" {
		t.Fatalf("expected unknown health, got %q", got.Health)
	}
	if err := s.RecordUpstreamCheck(u.ID, "ok", 123, "203.0.113.7", ""); err != nil {
		t.Fatal(err)
	}
	got := s.getUpstreamLocked(u.ID)
	if got.Health != "ok" || got.LatencyMS != 123 || got.ExitIP != "203.0.113.7" || got.LastCheckAt == 0 {
		t.Fatalf("probe state not persisted: %+v", got)
	}
	// A failed probe keeps the last known exit IP.
	if err := s.RecordUpstreamCheck(u.ID, "fail", 0, "", "dial timeout"); err != nil {
		t.Fatal(err)
	}
	got = s.getUpstreamLocked(u.ID)
	if got.Health != "fail" || got.LastError != "dial timeout" || got.ExitIP != "203.0.113.7" {
		t.Fatalf("failure state mismatch: %+v", got)
	}
	// Changing the endpoint identity resets probe state.
	updated, err := s.UpsertUpstream(u.ID, &Upstream{Name: "probe", Type: "socks5", Host: "2.2.2.2:1080"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Health != "unknown" || updated.ExitIP != "" || updated.LatencyMS != 0 {
		t.Fatalf("probe state should reset on endpoint change: %+v", updated)
	}
}

func TestEmptyCollectionsEncodeAsArrays(t *testing.T) {
	s := tempStore(t, 7788)
	if s.Upstreams() == nil || s.Ports() == nil || s.Accounts() == nil {
		t.Fatal("empty collections must be non-nil slices so the API returns [] instead of null")
	}
	b, err := json.Marshal(map[string]any{
		"upstreams": s.Upstreams(),
		"ports":     s.Ports(),
		"accounts":  s.Accounts(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "null") {
		t.Fatalf("collections serialized as null: %s", b)
	}
}

func TestMissingEntity(t *testing.T) {
	s := tempStore(t, 7788)
	if _, err := s.UpsertPort("nosuch", portFixture(10201, "")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if err := s.DeleteAccount("nosuch"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if err := s.DeletePort("nosuch"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if err := s.DeleteUpstream("nosuch"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if !strings.Contains(s.Path(), "fastproxy.db") {
		t.Fatalf("unexpected path: %s", s.Path())
	}
}
