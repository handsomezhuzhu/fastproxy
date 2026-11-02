package config

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handsomezhuzhu/fastproxy/logx"
)

// --- helpers for raw SQLite inspection ---

func rawUpstreamPassword(t *testing.T, s *Store, id string) string {
	t.Helper()
	var v string
	if err := s.db.QueryRow(`SELECT password FROM upstreams WHERE id=?`, id).Scan(&v); err != nil {
		t.Fatalf("read upstream password: %v", err)
	}
	return v
}

func rawAccountPassword(t *testing.T, s *Store, id string) string {
	t.Helper()
	var v string
	if err := s.db.QueryRow(`SELECT password FROM accounts WHERE id=?`, id).Scan(&v); err != nil {
		t.Fatalf("read account password: %v", err)
	}
	return v
}

func countMigrations(t *testing.T, s *Store) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	return n
}

func userVersion(t *testing.T, s *Store) int {
	t.Helper()
	var v int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	return v
}

func upstreamColumnsOf(t *testing.T, s *Store) []string {
	t.Helper()
	rows, err := s.db.Query(`PRAGMA table_info(upstreams)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		out = append(out, name)
	}
	return out
}

func execAll(t *testing.T, db *sql.DB, stmts ...string) {
	t.Helper()
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
}

// --- tests ---

// TestFreshDatabaseIsAtLatestVersion checks the baseline schema, PRAGMA
// user_version and the migration bookkeeping of a brand-new database.
func TestFreshDatabaseIsAtLatestVersion(t *testing.T) {
	s := tempStore(t, 7788)
	if v := s.SchemaVersion(); v != latestVersion() {
		t.Fatalf("schema version %d, want %d", v, latestVersion())
	}
	if v := userVersion(t, s); v != latestVersion() {
		t.Fatalf("PRAGMA user_version %d, want %d", v, latestVersion())
	}
	if n := countMigrations(t, s); n != len(migrations) {
		t.Fatalf("%d migration rows, want %d", n, len(migrations))
	}
	var names []string
	rows, err := s.db.Query(`SELECT name FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		rows.Scan(&n)
		names = append(names, n)
	}
	if len(names) != len(migrations) || !strings.Contains(names[0], "baseline") {
		t.Fatalf("unexpected migration names: %v", names)
	}
}

// TestLegacyDatabaseUpgrade is the main upgrade scenario: a v0.x database
// with data (plaintext passwords, boolean tls, mixed security) is opened by
// the new binary and must keep its data while gaining versions/columns.
func TestLegacyDatabaseUpgrade(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.db")

	old, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	execAll(t, old,
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`INSERT INTO settings(key, value) VALUES('session_secret', 'legacy-session-secret')`,
		// v0.3-era upstreams table: no probe/protocol columns at all.
		`CREATE TABLE upstreams (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, type TEXT NOT NULL,
			host TEXT NOT NULL, username TEXT NOT NULL DEFAULT '',
			password TEXT NOT NULL DEFAULT '', remark TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`,
		// v0.4-era columns exist here (so v1's ALTERs are partial no-ops).
		`ALTER TABLE upstreams ADD COLUMN health TEXT NOT NULL DEFAULT 'unknown'`,
		`ALTER TABLE upstreams ADD COLUMN tls INTEGER NOT NULL DEFAULT 0`,
		`CREATE TABLE ports (
			id TEXT PRIMARY KEY, port INTEGER NOT NULL UNIQUE,
			default_upstream TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1,
			remark TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`,
		`CREATE TABLE accounts (
			id TEXT PRIMARY KEY, username TEXT NOT NULL UNIQUE, password TEXT NOT NULL,
			port_id TEXT NOT NULL DEFAULT '', upstream_id TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1, remark TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`,
		`CREATE TABLE logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT, ts INTEGER NOT NULL, level TEXT NOT NULL,
			port INTEGER NOT NULL DEFAULT 0, account TEXT NOT NULL DEFAULT '',
			client TEXT NOT NULL DEFAULT '', proto TEXT NOT NULL DEFAULT '',
			target TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT '',
			bytes_in INTEGER NOT NULL DEFAULT 0, bytes_out INTEGER NOT NULL DEFAULT 0,
			duration_ms INTEGER NOT NULL DEFAULT 0, msg TEXT NOT NULL DEFAULT '')`,
		`INSERT INTO upstreams (id, name, type, host, username, password, remark, tls, created_at, updated_at)
			VALUES ('u-tls', 'old-tls', 'vless', '2.2.2.2:443', '', 'legacy-pw-1', '', 1, 1, 1)`,
		`INSERT INTO upstreams (id, name, type, host, username, password, remark, tls, created_at, updated_at)
			VALUES ('u-plain', 'old-plain', 'http', '1.2.3.4:8080', 'bob', 'legacy-pw-2', '', 0, 2, 2)`,
		`INSERT INTO ports (id, port, enabled, remark, created_at, updated_at)
			VALUES ('p1', 10001, 1, 'old port', 1, 1)`,
		`INSERT INTO accounts (id, username, password, port_id, upstream_id, enabled, remark, created_at, updated_at)
			VALUES ('a1', 'alice', 'acct-pw-1', 'p1', 'u-plain', 1, '', 1, 1)`,
		`INSERT INTO logs (ts, level, port, account, proto, target, status)
			VALUES (1700000000000, 'info', 10001, 'alice', 'http', 'example.com:80', '')`,
	)
	old.Close()
	if _, err := os.Stat(filepath.Join(dir, "legacy.db")); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path, 7788)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	defer s.Close()

	if v := s.SchemaVersion(); v != latestVersion() {
		t.Fatalf("schema version %d, want %d", v, latestVersion())
	}
	cols := upstreamColumnsOf(t, s)
	for _, want := range []string{"security", "pbk", "sid", "flow", "fingerprint", "transport", "path",
		"host_header", "service_name", "vmess_security", "method", "uuid", "sni", "last_error", "tls"} {
		found := false
		for _, c := range cols {
			if c == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("column %s missing after upgrade: %v", want, cols)
		}
	}

	// Data survived.
	ups := s.Upstreams()
	if len(ups) != 2 {
		t.Fatalf("upstreams after upgrade: %d", len(ups))
	}
	byID := map[string]*Upstream{}
	for _, u := range ups {
		byID[u.ID] = u
	}
	// v2: tls=1 -> security=tls, tls=0 -> security=none, passwords re-sealed.
	if u := byID["u-tls"]; u == nil || u.Security != SecurityTLS || u.Password != "legacy-pw-1" {
		t.Fatalf("tls upstream not migrated: %+v", u)
	}
	if u := byID["u-plain"]; u == nil || u.Security != SecurityNone || u.Password != "legacy-pw-2" || u.Username != "bob" {
		t.Fatalf("plain upstream not migrated: %+v", u)
	}
	accs := s.Accounts()
	if len(accs) != 1 || accs[0].Username != "alice" || accs[0].Password != "acct-pw-1" {
		t.Fatalf("accounts after upgrade: %+v", accs)
	}
	if n, _ := s.CountLogs(logx.Filter{Port: 10001}); n != 1 {
		t.Fatalf("logs lost: %d", n)
	}
	// v3: plaintext no longer on disk.
	for _, id := range []string{"u-tls", "u-plain"} {
		if raw := rawUpstreamPassword(t, s, id); !IsSealed(raw) || strings.Contains(raw, "legacy-pw") {
			t.Fatalf("%s not encrypted: %q", id, raw)
		}
	}
	if raw := rawAccountPassword(t, s, "a1"); !IsSealed(raw) || strings.Contains(raw, "acct-pw-1") {
		t.Fatalf("account not encrypted: %q", raw)
	}
	// Ports still work.
	if p := s.getPortLocked("p1"); p == nil || p.Port != 10001 {
		t.Fatalf("port lost: %+v", p)
	}
}

// TestLegacySecurityNotOverwritten: an existing non-empty security value
// (REALITY) must survive v2 regardless of the tls flag.
func TestLegacySecurityNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy2.db")
	old, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	execAll(t, old,
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at INTEGER NOT NULL)`,
		`INSERT INTO schema_migrations VALUES(1, 'baseline schema', 1)`,
		`CREATE TABLE upstreams (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, type TEXT NOT NULL, host TEXT NOT NULL,
			username TEXT NOT NULL DEFAULT '', password TEXT NOT NULL DEFAULT '',
			method TEXT NOT NULL DEFAULT '', uuid TEXT NOT NULL DEFAULT '',
			tls INTEGER NOT NULL DEFAULT 0, sni TEXT NOT NULL DEFAULT '',
			security TEXT NOT NULL DEFAULT '', pbk TEXT NOT NULL DEFAULT '',
			sid TEXT NOT NULL DEFAULT '', flow TEXT NOT NULL DEFAULT '',
			fingerprint TEXT NOT NULL DEFAULT '', transport TEXT NOT NULL DEFAULT '',
			path TEXT NOT NULL DEFAULT '', host_header TEXT NOT NULL DEFAULT '',
			service_name TEXT NOT NULL DEFAULT '', vmess_security TEXT NOT NULL DEFAULT '',
			remark TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
			health TEXT NOT NULL DEFAULT 'unknown', latency_ms INTEGER NOT NULL DEFAULT 0,
			exit_ip TEXT NOT NULL DEFAULT '', last_check_at INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE ports (
			id TEXT PRIMARY KEY, port INTEGER NOT NULL UNIQUE,
			default_upstream TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1,
			remark TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`,
		`CREATE TABLE accounts (
			id TEXT PRIMARY KEY, username TEXT NOT NULL UNIQUE, password TEXT NOT NULL,
			port_id TEXT NOT NULL DEFAULT '', upstream_id TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1, remark TEXT NOT NULL DEFAULT '',
			is_default INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`,
		`CREATE TABLE logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT, ts INTEGER NOT NULL, level TEXT NOT NULL,
			port INTEGER NOT NULL DEFAULT 0, account TEXT NOT NULL DEFAULT '',
			client TEXT NOT NULL DEFAULT '', proto TEXT NOT NULL DEFAULT '',
			target TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT '',
			bytes_in INTEGER NOT NULL DEFAULT 0, bytes_out INTEGER NOT NULL DEFAULT 0,
			duration_ms INTEGER NOT NULL DEFAULT 0, msg TEXT NOT NULL DEFAULT '')`,
		`INSERT INTO upstreams (id, name, type, host, uuid, tls, security, pbk, created_at, updated_at)
			VALUES ('r1', 'reality', 'vless', '3.3.3.3:443', 'b831381d-6324-4d53-ad4f-8cda48b30811', 1, 'reality', 'PK', 1, 1)`,
		`INSERT INTO upstreams (id, name, type, host, tls, security, created_at, updated_at)
			VALUES ('e1', 'empty-sec', 'vless', '4.4.4.4:443', 1, '', 2, 2)`,
		`INSERT INTO upstreams (id, name, type, host, tls, security, created_at, updated_at)
			VALUES ('e2', 'empty-sec-plain', 'vless', '5.5.5.5:443', 0, '', 3, 3)`,
	)
	old.Close()

	s, err := Open(path, 7788)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got := map[string]string{}
	for _, u := range s.Upstreams() {
		got[u.ID] = u.Security
	}
	if got["r1"] != SecurityReality {
		t.Fatalf("reality security overwritten: %q", got["r1"])
	}
	if got["e1"] != SecurityTLS || got["e2"] != SecurityNone {
		t.Fatalf("tls normalisation wrong: %v", got)
	}
	if v := s.SchemaVersion(); v != latestVersion() {
		t.Fatalf("version %d", v)
	}
}

// TestMigrationIdempotentAcrossOpens opens the same database repeatedly and
// checks no migration is ever applied twice and user_version stays stable.
func TestMigrationIdempotentAcrossOpens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "idem.db")
	for i := 0; i < 3; i++ {
		s, err := Open(path, 7788)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		if n := countMigrations(t, s); n != len(migrations) {
			t.Fatalf("open %d: %d migration rows, want %d", i, n, len(migrations))
		}
		// A row written between opens stays sealed and readable.
		if i == 0 {
			if _, err := s.UpsertUpstream("", &Upstream{Name: "x", Type: UpstreamHTTP, Host: "h:80", Password: "pw"}); err != nil {
				t.Fatal(err)
			}
		}
		if i > 0 {
			ups := s.Upstreams()
			if len(ups) != 1 || ups[0].Password != "pw" {
				t.Fatalf("open %d: data mismatch %+v", i, ups)
			}
		}
		if v := userVersion(t, s); v != latestVersion() {
			t.Fatalf("open %d: user_version %d", i, v)
		}
		s.Close()
	}
}

// TestMigrationFailureRollsBack verifies that a failing step aborts the whole
// transaction: no partial schema, no migration record.
func TestMigrationFailureRollsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fail.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	orig := migrations
	t.Cleanup(func() { migrations = orig })
	key := make([]byte, 32)
	err = MigrateForTest(db, key)
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}
	migrations = append(append([]migration(nil), orig...), migration{
		Version: latestVersion() + 1, Name: "boom",
		Apply: func(tx *sql.Tx) error {
			if _, err := tx.Exec(`CREATE TABLE half_done (id INTEGER)`); err != nil {
				return err
			}
			return sql.ErrNoRows
		},
	})
	if err := MigrateForTest(db, key); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected the failing migration to be reported, got %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='half_done'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("partial schema of the failed migration was committed")
	}
	// The baseline recorded by the first run is still there.
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != len(orig) {
		t.Fatalf("failed migration was recorded: %d rows, want %d", n, len(orig))
	}
}

// TestPortRangeValidation covers PORT_RANGE parsing and the boundary checks
// enforced when ports are created or updated.
func TestPortRangeValidation(t *testing.T) {
	cases := []struct {
		in      string
		want    PortRange
		wantErr bool
	}{
		{"", DefaultPortRange, false},
		{"10001-10050", PortRange{10001, 10050}, false},
		{"10080", PortRange{10080, 10080}, false},
		{" 1-65535 ", PortRange{1, 65535}, false},
		{"10050-10001", PortRange{}, true},
		{"0-100", PortRange{}, true},
		{"100-70000", PortRange{}, true},
		{"abc", PortRange{}, true},
		{"1-2-3", PortRange{}, true},
		{"-5", PortRange{}, true},
	}
	for _, c := range cases {
		got, err := ParsePortRange(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParsePortRange(%q): expected error", c.in)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("ParsePortRange(%q) = %v, %v; want %v", c.in, got, err, c.want)
		}
	}
	if DefaultPortRange.String() != "10001-10050" || (PortRange{10080, 10080}).String() != "10080" {
		t.Fatal("PortRange.String mismatch")
	}

	s, err := OpenWithPortRange(filepath.Join(t.TempDir(), "pr.db"), 7788, PortRange{Min: 10001, Max: 10050})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Boundaries pass, outside fails with a PORT_RANGE hint.
	for _, p := range []int{10001, 10050} {
		if _, err := s.UpsertPort("", portFixture(p)); err != nil {
			t.Fatalf("boundary port %d rejected: %v", p, err)
		}
	}
	for _, p := range []int{10000, 10051, 9999, 20000} {
		_, err := s.UpsertPort("", portFixture(p))
		if err == nil || !strings.Contains(err.Error(), "PORT_RANGE") || !strings.Contains(err.Error(), "10001-10050") {
			t.Fatalf("port %d: expected range error, got %v", p, err)
		}
	}
	// Updating an existing port out of range is rejected too.
	ports := s.Ports()
	if _, err := s.UpsertPort(ports[0].ID, portFixture(10099)); err == nil || !strings.Contains(err.Error(), "PORT_RANGE") {
		t.Fatalf("update out of range: %v", err)
	}
}
