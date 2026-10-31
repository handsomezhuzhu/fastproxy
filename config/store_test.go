package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tempStore(t *testing.T, adminPort int) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "fastproxy.json"), adminPort)
	if err != nil {
		t.Fatal(err)
	}
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

func TestOpenCreatesDefaultFile(t *testing.T) {
	s := tempStore(t, 8080)
	if len(s.Upstreams())+len(s.Ports())+len(s.Accounts()) != 0 {
		t.Fatal("expected empty store")
	}
	if s.SessionSecret() == "" {
		t.Fatal("session secret should be generated")
	}
	if s.AdminPasswordHash() != "" {
		t.Fatal("admin password should be unset initially")
	}
}

func TestUpstreamCRUDAndReferences(t *testing.T) {
	s := tempStore(t, 8080)
	saved, err := s.UpsertUpstream("", upstreamFixture("美国-家宽"))
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID == "" || saved.Type != UpstreamHTTP {
		t.Fatalf("unexpected upstream: %+v", saved)
	}

	// Referenced upstream must not be deletable.
	if _, err := s.UpsertPort("", portFixture(10001, saved.ID)); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUpstream(saved.ID); err == nil {
		t.Fatal("expected delete to fail while referenced by a port")
	}

	// Bad inputs rejected.
	if _, err := s.UpsertUpstream("", &Upstream{Name: "x", Type: "ftp", Host: "1.2.3.4:21"}); err == nil {
		t.Fatal("expected type error")
	}
	if _, err := s.UpsertUpstream("", &Upstream{Name: "x", Host: "no-port"}); err == nil {
		t.Fatal("expected host error")
	}

	// Unbound upstream deletes fine.
	spare, err := s.UpsertUpstream("", upstreamFixture("备用"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUpstream(spare.ID); err != nil {
		t.Fatalf("delete spare: %v", err)
	}

	// Reload roundtrip.
	s2 := tempStore(t, 8080)
	_ = s2
	sReload, err := Open(s.Path(), 8080)
	if err != nil {
		t.Fatal(err)
	}
	ups := sReload.Upstreams()
	if len(ups) != 1 || ups[0].Name != "美国-家宽" {
		t.Fatalf("reload mismatch: %+v", ups)
	}
}

func TestPortValidation(t *testing.T) {
	s := tempStore(t, 8080)
	up, err := s.UpsertUpstream("", upstreamFixture("u"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertPort("", portFixture(10001, up.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertPort("", portFixture(10001, up.ID)); err == nil {
		t.Fatal("expected duplicate port error")
	}
	if _, err := s.UpsertPort("", portFixture(8080, up.ID)); err == nil {
		t.Fatal("expected admin-port conflict")
	}
	if _, err := s.UpsertPort("", portFixture(10002, "missing")); err == nil {
		t.Fatal("expected missing upstream error")
	}
	if _, err := s.UpsertPort("", portFixture(10002, "")); err != nil {
		t.Fatalf("direct port should be allowed: %v", err)
	}
	// Toggle + reload.
	if _, err := s.SetPortEnabled("nosuch", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	first := s.Ports()[0]
	if _, err := s.SetPortEnabled(first.ID, false); err != nil {
		t.Fatal(err)
	}
	if p := s.Ports()[0]; p.ID != first.ID || p.Enabled {
		t.Fatalf("disable not applied: %+v", p)
	}
}

func TestAccountValidationAndUniqueness(t *testing.T) {
	s := tempStore(t, 8080)
	up, err := s.UpsertUpstream("", upstreamFixture("u"))
	if err != nil {
		t.Fatal(err)
	}
	port, err := s.UpsertPort("", portFixture(10001, up.ID))
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
	// Global account (port_id "") is allowed.
	if _, err := s.UpsertAccount("", accountFixture("jerry", "pw", "", "")); err != nil {
		t.Fatalf("global account rejected: %v", err)
	}
	// Delete account fine; delete bound port unbinds it.
	acc := s.Accounts()[0]
	if err := s.DeleteAccount(acc.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePort(port.ID); err != nil {
		t.Fatal(err)
	}
	for _, a := range s.Accounts() {
		if a.PortID == port.ID {
			t.Fatalf("account still bound to deleted port: %+v", a)
		}
	}
}

func TestMissingEntity(t *testing.T) {
	s := tempStore(t, 8080)
	if _, err := s.UpsertPort("nosuch", portFixture(1, "")); !errors.Is(err, ErrNotFound) {
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
}

func TestSecretEnvInterpolation(t *testing.T) {
	t.Setenv("FP_TEST_UP_PASS", "s3cret")
	t.Setenv("FP_TEST_ACC_PASS", "accpass")
	s := tempStore(t, 8080)

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

	// The on-disk file keeps the raw reference, never the resolved secret.
	raw, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "${FP_TEST_UP_PASS}") || strings.Contains(string(raw), "s3cret") {
		t.Fatalf("file must keep the reference and hide the secret:\n%s", raw)
	}

	// Reload resolves again.
	s2, err := Open(s.Path(), 8080)
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.Upstreams()[0].Password; got != "s3cret" {
		t.Fatalf("reload expansion failed: %q", got)
	}

	// Missing env var is rejected at save time.
	if _, err := s.UpsertUpstream("", &Upstream{
		Name: "u2", Type: UpstreamHTTP, Host: "1.2.3.4:8080", Password: "${FP_TEST_MISSING}",
	}); err == nil {
		t.Fatal("expected missing-env error")
	}

	// ${VAR:-default} syntax works.
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
	path := filepath.Join(t.TempDir(), "fp.json")
	body := `{"upstreams":[{"id":"u1","name":"u","type":"http","host":"1.2.3.4:8080","password":"${FP_TEST_NONE_XYZ}"}],"ports":[],"accounts":[]}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, 8080); err == nil {
		t.Fatal("expected startup error for missing secret env")
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
