package config

import "testing"

// TestDedupeMigrationKeepsOldestAndRepoints verifies the v5 cleanup: exact
// duplicate upstreams collapse to the oldest row and every reference follows.
func TestDedupeMigrationKeepsOldestAndRepoints(t *testing.T) {
	s := tempStore(t, 7788)

	sealed, err := sealSecret("p")
	if err != nil {
		t.Fatal(err)
	}
	insert := func(id, name string, created int64) {
		t.Helper()
		_, err := s.db.Exec(`INSERT INTO upstreams
			(id, name, type, host, username, password, method, uuid, sni, remark, health,
			 created_at, updated_at, security, pbk, sid, flow, fingerprint, transport,
			 path, host_header, service_name, vmess_security)
			VALUES(?,?,?,?,?,?,'','','','','unknown',?,?,'','','','','','','','','','')`,
			id, name, UpstreamHTTP, "1.1.1.1:8080", "u", sealed, created, created)
		if err != nil {
			t.Fatal(err)
		}
	}
	insert("keep0001", "oldest", 100)
	insert("dup00002", "newer", 200)

	if _, err := s.db.Exec(`INSERT INTO ports (id, port, enabled, created_at, updated_at)
		VALUES('port0001', 10021, 1, 100, 100)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO accounts
		(id, username, password, port_id, upstream_id, enabled, remark, is_default, created_at, updated_at)
		VALUES('acc00001', 'user1', 'x', 'port0001', 'dup00002', 1, '', 0, 100, 100)`); err != nil {
		t.Fatal(err)
	}

	sub, err := s.UpsertSubscription("", &Subscription{
		Name: "sub", URL: "https://example.com/sub", Enabled: true, IntervalSec: 600,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO subscription_nodes (subscription_id, upstream_id, node_key)
		VALUES(?, 'dup00002', 'node-key')`, sub.ID); err != nil {
		t.Fatal(err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateDedupeUpstreams(tx); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM upstreams`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 upstream after dedupe, got %d", count)
	}
	var id, name string
	if err := s.db.QueryRow(`SELECT id, name FROM upstreams`).Scan(&id, &name); err != nil {
		t.Fatal(err)
	}
	if id != "keep0001" || name != "oldest" {
		t.Fatalf("oldest row must survive, got %s/%s", id, name)
	}

	var accountUpstream string
	if err := s.db.QueryRow(`SELECT upstream_id FROM accounts WHERE id='acc00001'`).Scan(&accountUpstream); err != nil {
		t.Fatal(err)
	}
	if accountUpstream != "keep0001" {
		t.Fatalf("account should point at keeper, got %q", accountUpstream)
	}

	var mapped string
	if err := s.db.QueryRow(`SELECT upstream_id FROM subscription_nodes WHERE subscription_id=?`, sub.ID).Scan(&mapped); err != nil {
		t.Fatal(err)
	}
	if mapped != "keep0001" {
		t.Fatalf("subscription mapping should point at keeper, got %q", mapped)
	}
}
