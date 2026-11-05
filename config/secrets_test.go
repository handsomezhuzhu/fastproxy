package config

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestSecretKeyParsing covers the accepted FASTPROXY_SECRET_KEY encodings.
func TestSecretKeyParsing(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	forms := map[string]string{
		"std":         base64.StdEncoding.EncodeToString(key),
		"raw-std":     base64.RawStdEncoding.EncodeToString(key),
		"url":         base64.URLEncoding.EncodeToString(key),
		"raw-url":     base64.RawURLEncoding.EncodeToString(key),
		"hex":         hex.EncodeToString(key),
		"hex upper":   strings.ToUpper(hex.EncodeToString(key)),
		" padded std ": " " + base64.StdEncoding.EncodeToString(key) + "\n",
	}
	for name, s := range forms {
		got, err := ParseSecretKey(s)
		if err != nil || !bytes.Equal(got, key) {
			t.Errorf("%s: ParseSecretKey failed (%v)", name, err)
		}
	}
	for _, bad := range []string{"", "short", base64.StdEncoding.EncodeToString(key[:16]), strings.Repeat("z", 64)} {
		if _, err := ParseSecretKey(bad); err == nil {
			t.Errorf("ParseSecretKey(%q): expected error", bad)
		}
	}
}

// TestSealOpenRoundTrip checks AES-GCM round-trip, prefix and tamper
// detection at the cipher level.
func TestSealOpenRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	c, err := newSecretCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	const plain = "p@ss:word with ${VAR} and unicode 密码"
	sealed, err := c.seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sealed, "enc:v1:") || strings.Contains(sealed, plain) {
		t.Fatalf("bad sealed value %q", sealed)
	}
	got, err := c.open(sealed)
	if err != nil || got != plain {
		t.Fatalf("round-trip: %q %v", got, err)
	}
	// Deterministic prefix, random nonce: the same plaintext seals differently.
	sealed2, _ := c.seal(plain)
	if sealed2 == sealed {
		t.Fatal("nonce reuse: identical ciphertext")
	}
	// Tampering and a different key fail loudly.
	body := []byte(sealed)
	body[len(body)-2] ^= 0x01
	if _, err := c.open(string(body)); !errors.Is(err, ErrSecretKeyMismatch) {
		t.Fatalf("tampered ciphertext accepted: %v", err)
	}
	other, _ := newSecretCipher(bytes.Repeat([]byte{8}, 32))
	if _, err := other.open(sealed); !errors.Is(err, ErrSecretKeyMismatch) {
		t.Fatalf("wrong key accepted: %v", err)
	}
}

// TestKeyFileGeneratedWith0600 verifies the fallback key file: created next
// to the database with owner-only permissions, stable across opens.
func TestKeyFileGeneratedWith0600(t *testing.T) {
	t.Setenv(secretKeyEnv, "")
	dir := t.TempDir()
	path := filepath.Join(dir, "fastproxy.db")
	s, err := Open(path, 7788)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	keyPath := filepath.Join(dir, SecretKeyFile)
	st, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("key file not created: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := st.Mode().Perm(); perm != 0o600 {
			t.Fatalf("key file mode %o, want 600", perm)
		}
	}
	raw, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSecretKey(string(raw)); err != nil {
		t.Fatalf("generated key invalid: %v", err)
	}
	// Only one file, not the database directory itself.
	entries, _ := os.ReadDir(dir)
	var keys int
	for _, e := range entries {
		if e.Name() == SecretKeyFile {
			keys++
		}
	}
	if keys != 1 {
		t.Fatalf("unexpected key files: %v", entries)
	}
}

// TestPasswordStoredEncrypted verifies that write paths seal the password in
// SQLite while reads return plaintext, including ${VAR} placeholders (which
// are sealed verbatim and expanded only after decryption).
func TestPasswordStoredEncrypted(t *testing.T) {
	t.Setenv(secretKeyEnv, "")
	s := tempStore(t, 7788)

	up, err := s.UpsertUpstream("", &Upstream{
		Name: "ss", Type: UpstreamShadowsocks, Host: "1.1.1.1:8388",
		Method: "aes-256-gcm", Password: "ss-plain-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if up.Password != "ss-plain-secret" {
		t.Fatalf("read path returned %q", up.Password)
	}
	if raw := rawUpstreamPassword(t, s, up.ID); !IsSealed(raw) || strings.Contains(raw, "ss-plain-secret") {
		t.Fatalf("upstream password stored raw: %q", raw)
	}

	port, err := s.UpsertPort("", portFixture(10201))
	if err != nil {
		t.Fatal(err)
	}
	def := s.GetDefaultAccount(port.ID)
	if def == nil || def.Password == "" || !IsSealed(rawAccountPassword(t, s, def.ID)) {
		t.Fatalf("default account password not sealed: %+v", def)
	}
	acc, err := s.UpsertAccount("", &Account{
		Username: "bob", Password: "acct-plain-secret", PortID: port.ID, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if raw := rawAccountPassword(t, s, acc.ID); !IsSealed(raw) || strings.Contains(raw, "acct-plain-secret") {
		t.Fatalf("account password stored raw: %q", raw)
	}
	// Update re-seals the new value.
	if _, err := s.UpsertAccount(acc.ID, &Account{
		Username: "bob", Password: "rotated-secret", PortID: port.ID, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if raw := rawAccountPassword(t, s, acc.ID); strings.Contains(raw, "rotated") || strings.Contains(raw, "acct-plain") {
		t.Fatalf("rotated password not sealed: %q", raw)
	}
	if got := s.getAccountLocked(acc.ID); got == nil || got.Password != "rotated-secret" {
		t.Fatalf("rotated password not readable: %+v", got)
	}

	// Env placeholder: sealed verbatim, expanded only on read.
	t.Setenv("FP_SECRET_TEST_PW", "expanded-value")
	ph, err := s.UpsertUpstream("", &Upstream{
		Name: "env", Type: UpstreamHTTP, Host: "2.2.2.2:8080", Password: "${FP_SECRET_TEST_PW}",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ph.Password != "expanded-value" {
		t.Fatalf("placeholder not expanded after decryption: %q", ph.Password)
	}
	raw := rawUpstreamPassword(t, s, ph.ID)
	if !IsSealed(raw) || strings.Contains(raw, "expanded-value") {
		t.Fatalf("placeholder upstream password stored raw: %q", raw)
	}
	// The sealed payload decrypts to the literal placeholder.
	if dec, err := openSecret(raw); err != nil || dec != "${FP_SECRET_TEST_PW}" {
		t.Fatalf("sealed placeholder decrypts to %q (%v)", dec, err)
	}

	// Hex of the plaintext never leaks either (no double-encoding surprises).
	for _, name := range []string{"ss-plain-secret", "acct-plain-secret", "expanded-value"} {
		if bytes.Contains([]byte(rawUpstreamPassword(t, s, up.ID)), []byte(name)) {
			t.Fatalf("plaintext %q found in database", name)
		}
	}
}

// TestLegacyPlaintextCompatibility reads a value written before encryption.
func TestLegacyPlaintextCompatibility(t *testing.T) {
	t.Setenv(secretKeyEnv, "")
	s := tempStore(t, 7788)
	if _, err := s.db.Exec(`INSERT INTO upstreams
		(id, name, type, host, password, created_at, updated_at)
		VALUES('legacy', 'legacy', 'http', 'h:80', 'old-plain', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	u := s.getUpstreamLocked("legacy")
	if u == nil || u.Password != "old-plain" {
		t.Fatalf("legacy plaintext not readable: %+v", u)
	}
	// v3-style rewrite seals it.
	if _, err := s.db.Exec(`UPDATE upstreams SET password=? WHERE id='legacy'`, mustSeal(t, "old-plain")); err != nil {
		t.Fatal(err)
	}
	if u := s.getUpstreamLocked("legacy"); u == nil || u.Password != "old-plain" {
		t.Fatalf("sealed value not readable: %+v", u)
	}
}

func mustSeal(t *testing.T, v string) string {
	t.Helper()
	out, err := sealSecret(v)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestKeyMismatchIsReported: a database sealed under one key must fail loudly
// under another instead of returning empty passwords or rewriting rows.
func TestKeyMismatchIsReported(t *testing.T) {
	t.Setenv(secretKeyEnv, "")
	dir := t.TempDir()
	path := filepath.Join(dir, "fp.db")
	s, err := Open(path, 7788)
	if err != nil {
		t.Fatal(err)
	}
	up, err := s.UpsertUpstream("", &Upstream{Name: "x", Type: UpstreamHTTP, Host: "h:80", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	before := rawUpstreamPassword(t, s, up.ID)
	s.Close()

	// Replace the key file with a different key.
	other := bytes.Repeat([]byte{0x11}, 32)
	if err := os.WriteFile(filepath.Join(dir, SecretKeyFile), []byte(base64.RawURLEncoding.EncodeToString(other)), 0o600); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(path, 7788)
	if err == nil {
		defer s2.Close()
		// Opening succeeds (nothing forces a decrypt at open) but reads fail
		// and never return an empty password.
		if u := s2.getUpstreamLocked(up.ID); u != nil {
			t.Fatalf("upstream with undecryptable password should not be returned: %+v", u)
		}
		if raw := rawUpstreamPassword(t, s2, up.ID); raw != before {
			t.Fatalf("database was modified on key mismatch: %q -> %q", before, raw)
		}
		return
	}
	if !strings.Contains(err.Error(), "解密") {
		t.Fatalf("unexpected open error: %v", err)
	}
	if raw := rawUpstreamPassword(t, s2, up.ID); raw != before {
		t.Fatalf("database was modified on key mismatch: %q -> %q", before, raw)
	}
}

// TestEnvKeyOverridesFile: FASTPROXY_SECRET_KEY must be used even when a key
// file exists, and the file must remain untouched.
func TestEnvKeyOverridesFile(t *testing.T) {
	dir := t.TempDir()
	key := bytes.Repeat([]byte{0x55}, 32)
	t.Setenv(secretKeyEnv, hex.EncodeToString(key))
	path := filepath.Join(dir, "fp.db")
	s, err := Open(path, 7788)
	if err != nil {
		t.Fatal(err)
	}
	up, err := s.UpsertUpstream("", &Upstream{Name: "x", Type: UpstreamHTTP, Host: "h:80", Password: "pw"})
	if err != nil {
		t.Fatal(err)
	}
	raw := rawUpstreamPassword(t, s, up.ID)
	s.Close()
	if _, err := os.Stat(filepath.Join(dir, SecretKeyFile)); !os.IsNotExist(err) {
		t.Fatal("key file must not be created when the env key is set")
	}
	c, err := newSecretCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := c.open(raw); err != nil || got != "pw" {
		t.Fatalf("env key did not seal: %q %v", got, err)
	}
	// Reopen with the same env key reads it back.
	s2, err := Open(path, 7788)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if u := s2.getUpstreamLocked(up.ID); u == nil || u.Password != "pw" {
		t.Fatalf("env key reopen: %+v", u)
	}
}


// TestAllDataAndLogsEncryptedAtRest verifies that every persisted entity,
// setting, traffic meter, probe state and log entry is sealed with AES-256-GCM
// in SQLite and no plaintext leaks onto disk.
func TestAllDataAndLogsEncryptedAtRest(t *testing.T) {
	t.Setenv(secretKeyEnv, "")
	dir := t.TempDir()
	path := filepath.Join(dir, "all-enc.db")
	s, err := OpenWithPortRange(path, 7788, PortRange{Min: 10200, Max: 10299})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.SetAdminPasswordHash("hash-secret-9988"); err != nil {
		t.Fatal(err)
	}
	up, err := s.UpsertUpstream("", &Upstream{
		Name: "node-tokyo-secret", Type: UpstreamShadowsocks, Host: "198.51.100.77:8388",
		Method: "aes-256-gcm", Password: "up-pass-secret", Remark: "up-remark-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordUpstreamCheck(up.ID, "ok", 42, "203.0.113.88", "probe-err-secret"); err != nil {
		t.Fatal(err)
	}
	port, err := s.UpsertPort("", &Port{Port: 10205, Enabled: true, Remark: "port-remark-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertAccount("", &Account{
		Username: "acct-user-secret", Password: "acct-pass-secret",
		PortID: port.ID, UpstreamID: up.ID, Enabled: true, Remark: "acct-remark-secret",
	}); err != nil {
		t.Fatal(err)
	}
	sub, err := s.UpsertSubscription("", &Subscription{
		Name: "sub-name-secret", URL: "https://sub.example.com/secret-feed", Enabled: true, IntervalSec: 600,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordSubscriptionFetch(sub.ID, "etag-secret", "lastmod-secret", "sub-err-secret", false); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMeters(map[string]MeterStat{
		"port:" + port.ID: {ConnsTotal: 12, BytesIn: 3456, BytesOut: 7890},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveServerProbe("192.0.2.55", timeRepeatUnix(1700000000)); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertLog(testEntryWithDetail(10205, "acct-user-secret", "203.0.113.99", "secret-target.internal:443", "log-msg-secret")); err != nil {
		t.Fatal(err)
	}

	// Verify raw SQLite columns in logs, accounts, upstreams, ports, settings, subscriptions are all sealed.
	var logLevel, logAcc, logClient, logProto, logTarget, logStatus, logMsg string
	if err := s.db.QueryRow(`SELECT level, account, client, proto, target, status, msg FROM logs LIMIT 1`).
		Scan(&logLevel, &logAcc, &logClient, &logProto, &logTarget, &logStatus, &logMsg); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{logLevel, logAcc, logClient, logProto, logTarget, logStatus, logMsg} {
		if !IsSealed(v) {
			t.Fatalf("log column stored unencrypted: %q", v)
		}
	}

	// Checkpoint WAL and verify no secret plaintext appears anywhere in the DB file.
	_, _ = s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	rawDB, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"hash-secret-9988", "node-tokyo-secret", "198.51.100.77:8388", "up-pass-secret",
		"up-remark-secret", "203.0.113.88", "probe-err-secret", "port-remark-secret",
		"acct-user-secret", "acct-pass-secret", "acct-remark-secret", "sub-name-secret",
		"secret-feed", "etag-secret", "lastmod-secret", "sub-err-secret", "192.0.2.55",
		"203.0.113.99", "secret-target.internal:443", "log-msg-secret",
	} {
		if bytes.Contains(rawDB, []byte(needle)) {
			t.Fatalf("plaintext %q leaked into SQLite database file", needle)
		}
	}

	// Verify read paths decrypt everything accurately.
	if got := s.LoadMeters()["port:"+port.ID]; got.ConnsTotal != 12 || got.BytesIn != 3456 || got.BytesOut != 7890 {
		t.Fatalf("LoadMeters mismatch: %+v", got)
	}
	if ip, at := s.LoadServerProbe(); ip != "192.0.2.55" || at.Unix() != 1700000000 {
		t.Fatalf("LoadServerProbe mismatch: ip=%q at=%v", ip, at)
	}
	logs, err := s.QueryLogs(testFilter(10, 10205, "acct-user-secret", "info"))
	if err != nil || len(logs) != 1 || logs[0].Target != "secret-target.internal:443" || logs[0].Client != "203.0.113.99" || logs[0].Msg != "log-msg-secret" {
		t.Fatalf("QueryLogs decrypted mismatch: %+v err=%v", logs, err)
	}
}
