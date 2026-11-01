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

func portFixture(port int) *Port {
	return &Port{Port: port, Enabled: true}
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

func TestPortCreateGeneratesDefaultAccount(t *testing.T) {
	s := tempStore(t, 7788)
	port, err := s.UpsertPort("", portFixture(10201))
	if err != nil {
		t.Fatal(err)
	}
	def := s.GetDefaultAccount(port.ID)
	if def == nil {
		t.Fatal("default account not generated")
	}
	if def.Username != "default-10201" || def.PortID != port.ID || !def.IsDefault || def.UpstreamID != "" {
		t.Fatalf("default account mismatch: %+v", def)
	}
	if def.Password == "" {
		t.Fatal("default account should get a random password")
	}
	// Recreating the same port does not duplicate the default account.
	if _, err := s.UpsertPort(port.ID, portFixture(10201)); err != nil {
		t.Fatal(err)
	}
	if n := len(s.Accounts()); n != 1 {
		t.Fatalf("expected 1 account, got %d", n)
	}
}

func TestAccountNeedsPortAndDefaultIsProtected(t *testing.T) {
	s := tempStore(t, 7788)
	port, err := s.UpsertPort("", portFixture(10201))
	if err != nil {
		t.Fatal(err)
	}
	def := s.GetDefaultAccount(port.ID)

	// Account without a port is rejected.
	if _, err := s.UpsertAccount("", accountFixture("tom", "pw", "", "")); !strings.Contains(err.Error(), "必须绑定端口") {
		t.Fatalf("expected port-required error, got %v", err)
	}
	if _, err := s.UpsertAccount("", accountFixture("tom", "pw", "missing", "")); err == nil {
		t.Fatal("expected missing port error")
	}
	if _, err := s.UpsertAccount("", accountFixture("tom", "pw", port.ID, "")); err != nil {
		t.Fatalf("valid account rejected: %v", err)
	}

	// Default account: port cannot change, delete is rejected; upstream/username/password editable.
	if _, err := s.UpsertAccount(def.ID, accountFixture(def.Username, "pw2", "some-other-port", "")); err == nil {
		t.Fatal("expected default account port-change rejection")
	}
	if err := s.DeleteAccount(def.ID); err == nil || !strings.Contains(err.Error(), "默认账户不可删除") {
		t.Fatalf("expected default delete rejection, got %v", err)
	}
	up, err := s.UpsertUpstream("", upstreamFixture("up"))
	if err != nil {
		t.Fatal(err)
	}
	updated, err := s.UpsertAccount(def.ID, accountFixture("renamed", "pw-new", def.PortID, up.ID))
	if err != nil {
		t.Fatalf("default account update failed: %v", err)
	}
	if !updated.IsDefault || updated.UpstreamID != up.ID || updated.Username != "renamed" {
		t.Fatalf("default update mismatch: %+v", updated)
	}
}

func TestDeletePortCascadeAndBlock(t *testing.T) {
	s := tempStore(t, 7788)
	port, err := s.UpsertPort("", portFixture(10201))
	if err != nil {
		t.Fatal(err)
	}
	// With only the default account bound, deletion succeeds and cascades it.
	if err := s.DeletePort(port.ID); err != nil {
		t.Fatalf("delete blocked unexpectedly: %v", err)
	}
	if def := s.GetDefaultAccount(port.ID); def != nil {
		t.Fatal("default account should be cascaded with the port")
	}
	if len(s.Ports()) != 0 {
		t.Fatal("port should be deleted")
	}

	// A non-default account blocks deletion.
	p2, err := s.UpsertPort("", portFixture(10202))
	if err != nil {
		t.Fatal(err)
	}
	acc, err := s.UpsertAccount("", accountFixture("tom", "pw", p2.ID, ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePort(p2.ID); err == nil || !strings.Contains(err.Error(), "请先删除绑定账户") {
		t.Fatalf("expected block error, got %v", err)
	}
	if err := s.DeleteAccount(acc.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePort(p2.ID); err != nil {
		t.Fatalf("delete should succeed after cleaning accounts: %v", err)
	}
}

func TestUpstreamCRUDAndReferences(t *testing.T) {
	s := tempStore(t, 7788)
	saved, err := s.UpsertUpstream("", upstreamFixture("美国家宽"))
	if err != nil {
		t.Fatal(err)
	}

	port, err := s.UpsertPort("", portFixture(10201))
	if err != nil {
		t.Fatal(err)
	}
	// Reference only exists through accounts now.
	if _, err := s.UpsertAccount("", accountFixture("refuser", "pw", port.ID, saved.ID)); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUpstream(saved.ID); !strings.Contains(err.Error(), "被账户") {
		t.Fatalf("expected reference rejection, got %v", err)
	}

	// Bad inputs rejected.
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
	if _, err := s.UpsertPort("", portFixture(7788)); err == nil {
		t.Fatal("expected admin-port conflict")
	}
	if _, err := s.UpsertPort("", portFixture(10201)); err == nil {
		t.Fatal("expected duplicate port error")
	}
}

func TestAccountValidationAndUniqueness(t *testing.T) {
	s := tempStore(t, 7788)
	port, err := s.UpsertPort("", portFixture(10201))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertAccount("", accountFixture("tom", "pw", port.ID, "")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertAccount("", accountFixture("tom", "pw2", port.ID, "")); err == nil {
		t.Fatal("expected duplicate username error")
	}
	if _, err := s.UpsertAccount("", accountFixture("jerry", "pw", port.ID, "missing-up")); err == nil {
		t.Fatal("expected missing upstream error")
	}
	acc := s.Accounts()[1] // accounts[0] is the port's default account
	if acc.Username != "tom" {
		t.Fatalf("unexpected account list: %+v", s.Accounts())
	}
	if _, err := s.UpsertAccount(acc.ID, accountFixture("jerry", "pw", port.ID, "")); err != nil {
		t.Fatalf("rename rejected: %v", err)
	}
	// "jerry" now exists, so a brand new account with the same name must fail.
	if _, err := s.UpsertAccount("", accountFixture("jerry", "other", port.ID, "")); err == nil {
		t.Fatal("expected conflict when reusing the renamed username")
	}
	// "tom" is free again after the rename.
	if _, err := s.UpsertAccount("", accountFixture("tom", "fresh", port.ID, "")); err != nil {
		t.Fatalf("tom should be reusable after rename: %v", err)
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
	blob, err := json.Marshal(map[string]any{"ports": s.Ports(), "accounts": s.Accounts()})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), "s3cret") {
		t.Fatalf("raw env reference leaked into serialized data: %s", blob)
	}

	if _, err := s.UpsertUpstream("", &Upstream{
		Name: "u2", Type: UpstreamHTTP, Host: "1.2.3.4:8080", Password: "${FP_TEST_MISSING}",
	}); err == nil {
		t.Fatal("expected missing-env error")
	}

	port, err := s.UpsertPort("", portFixture(10201))
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.UpsertAccount("", &Account{
		Username: "bob", Password: "${FP_TEST_NONE:-fallback}", PortID: port.ID,
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.Password != "fallback" {
		t.Fatalf("default syntax not applied: %q", a.Password)
	}
}

func TestEmptyCollectionsEncodeAsArrays(t *testing.T) {
	s := tempStore(t, 7788)
	if s.Upstreams() == nil || s.Ports() == nil || s.Accounts() == nil {
		t.Fatal("empty collections must be non-nil slices")
	}
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

	points := s.QueryHourlyMetrics(24)
	if len(points) != 24 {
		t.Fatalf("expected 24 buckets, got %d", len(points))
	}
	var connsTotal int64
	for _, p := range points {
		connsTotal += p.Conns
	}
	if connsTotal != 3 {
		t.Fatalf("hourly aggregation mismatch, conns=%d", connsTotal)
	}

	if _, err := s.db.Exec(`UPDATE logs SET ts = ?`, time.Now().AddDate(0, 0, -10).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PruneLogs(7); err != nil {
		t.Fatal(err)
	}
	left, _ := s.QueryLogs(testFilter(10, 0, "", "debug"))
	if len(left) != 0 {
		t.Fatalf("expected empty logs after prune, got %d", len(left))
	}
}

func TestBulkOperationsAndPaging(t *testing.T) {
	s := tempStore(t, 7788)

	// setup: 1 upstream + 2 ports + 3 accounts (1 default each)
	up, err := s.UpsertUpstream("", upstreamFixture("main"))
	if err != nil {
		t.Fatal(err)
	}
	p1, err := s.UpsertPort("", portFixture(10201))
	if err != nil {
		t.Fatal(err)
	}
	p2, err := s.UpsertPort("", portFixture(10202))
	if err != nil {
		t.Fatal(err)
	}
	a1, err := s.UpsertAccount("", accountFixture("u1", "pw", p1.ID, up.ID))
	if err != nil {
		t.Fatal(err)
	}
	a2, err := s.UpsertAccount("", accountFixture("u2", "pw", p1.ID, ""))
	if err != nil {
		t.Fatal(err)
	}
	// BulkAccounts disable.
	updated, problems := s.BulkAccounts([]string{a1.ID, a2.ID}, "disable")
	if updated != 2 || len(problems) != 0 {
		t.Fatalf("disable: updated=%d problems=%v", updated, problems)
	}
	for _, a := range s.Accounts() {
		if !a.IsDefault && a.Enabled {
			t.Fatalf("disable not applied: %+v", a)
		}
	}
	// BulkAccounts enable + delete (default blocked).
	updated, problems = s.BulkAccounts([]string{s.GetDefaultAccount(p1.ID).ID, a1.ID, a2.ID, "nope"}, "delete")
	if updated != 2 || len(problems) != 2 {
		t.Fatalf("delete: updated=%d problems=%v", updated, problems)
	}
	joined := ""
	for _, p := range problems {
		joined += p.Reason + ";"
	}
	if !strings.Contains(joined, "默认账户不可删除") || !strings.Contains(joined, "不存在") {
		t.Fatalf("unexpected problems: %v", problems)
	}
	if _, err := s.UpsertAccount("", accountFixture("u3", "pw", p1.ID, "")); err != nil {
		t.Fatal(err)
	}

	// BulkUpstream delete skips referenced upstreams.
	used, err := s.UpsertUpstream("", upstreamFixture("被引用"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertAccount("", accountFixture("u1", "pw", p1.ID, used.ID)); err != nil {
		t.Fatal(err)
	}
	deleted, skipped := s.BulkDeleteUpstreams([]string{used.ID, "missing"})
	if deleted != 0 || len(skipped) != 2 {
		t.Fatalf("bulk upstream delete: %d %v", deleted, skipped)
	}
	if !strings.Contains(skipped[0].Reason, "引用") {
		t.Fatalf("expected reference reason, got %q", skipped[0].Reason)
	}

	// Paged list + query.
	spare, err := s.UpsertUpstream("", upstreamFixture("备用-HK"))
	if err != nil {
		t.Fatal(err)
	}
	items, total, err := s.ListUpstreams(1, 0, "")
	if err != nil || total != 3 || len(items) != 1 {
		t.Fatalf("paged list: total=%d n=%d err=%v", total, len(items), err)
	}
	if items, total, err = s.ListUpstreams(50, 0, "HK"); total != 1 || items[0].ID != spare.ID {
		t.Fatalf("keyword search: %+v total=%d err=%v", items, total, err)
	}
	if items, total, err = s.ListUpstreams(50, 0, "8080"); total != 3 {
		t.Fatalf("host search: %d", total)
	}

	// Bulk ports enable/disable + delete-blocking.
	updated, problems = s.BulkPorts([]string{p2.ID, "nope"}, "disable")
	if updated != 1 || len(problems) != 1 {
		t.Fatalf("bulk ports disable: %v %v", updated, problems)
	}
	updated, problems = s.BulkPorts([]string{p1.ID, p2.ID}, "enable")
	if updated != 2 {
		t.Fatalf("enable upstreams via ports: issues: %v", problems)
	}
	// A port with a non-default account must stay un-deletable.
	p3, err := s.UpsertPort("", portFixture(10203))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertAccount("", accountFixture("tom", "pw", p3.ID, "")); err != nil {
		t.Fatal(err)
	}
	updated, problems = s.BulkPorts([]string{p3.ID}, "delete")
	if updated != 0 || !strings.Contains(problems[0].Reason, "请先删除绑定账户") {
		t.Fatalf("delete blocked: %v %v", updated, problems)
	}

	// Log pagination + clearing.
	for i := 0; i < 30; i++ {
		if err := s.InsertLog(testEntry(10201, "alice", "info", "")); err != nil {
			t.Fatal(err)
		}
	}
	total3, _ := s.CountLogs(logx.Filter{})
	page1, _ := s.QueryLogs(logx.Filter{Limit: 10, Offset: 0, Account: "alice"})
	page2, _ := s.QueryLogs(logx.Filter{Limit: 10, Offset: 20, Account: "alice"})
	if total3 != 30 || len(page1) != 10 || len(page2) != 10 {
		t.Fatalf("log paging: total=%d p1=%d p2=%d", total3, len(page1), len(page2))
	}
	rowsCleared, err := s.ClearLogs()
	if err != nil || rowsCleared != 30 {
		t.Fatalf("clear logs: %d %v", rowsCleared, err)
	}
	if total4, _ := s.CountLogs(logx.Filter{}); total4 != 0 {
		t.Fatalf("clear not applied: %d", total4)
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
