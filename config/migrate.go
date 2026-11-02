package config

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// migration is one ordered schema/data step. Version numbers are positive and
// strictly increasing; applied migrations are recorded in schema_migrations
// and never run twice.
type migration struct {
	Version int
	Name    string
	Apply   func(*sql.Tx) error
}

// schemaMigrationsDDL creates the bookkeeping table itself (always outside the
// versioned list so an older database can be inspected first).
const schemaMigrationsDDL = `CREATE TABLE IF NOT EXISTS schema_migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at INTEGER NOT NULL)`

// migrations is the ordered migration list. v1 is the full baseline schema
// (plus in-place column backfill for pre-versioning databases); later entries
// evolve data only, so a fresh database and an upgraded one converge.
//
// Adding a migration: append a new entry with the next version; never edit or
// reorder released entries.
var migrations = []migration{
	{Version: 1, Name: "baseline schema", Apply: migrateBaseline},
	{Version: 2, Name: "normalise upstream TLS into security", Apply: migrateSecurity},
	{Version: 3, Name: "encrypt upstream/account passwords", Apply: migrateEncryptPasswords},
}

// latestVersion is the version a freshly created database reports.
func latestVersion() int { return migrations[len(migrations)-1].Version }

// migrate applies every pending migration in a single transaction and keeps
// PRAGMA user_version in sync with the highest applied version.
func (s *Store) migrate() error {
	if _, err := s.db.Exec(schemaMigrationsDDL); err != nil {
		return fmt.Errorf("初始化数据库失败: %w", err)
	}
	applied, err := s.appliedVersions()
	if err != nil {
		return fmt.Errorf("读取数据库版本失败: %w", err)
	}

	// Validate the list once (catches accidental duplicate/out-of-order edits).
	ordered := append([]migration(nil), migrations...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Version < ordered[j].Version })
	prev := 0
	for _, m := range ordered {
		if m.Version <= prev || m.Version <= 0 || m.Name == "" || m.Apply == nil {
			return fmt.Errorf("数据库迁移定义无效: version=%d name=%q", m.Version, m.Name)
		}
		prev = m.Version
	}

	pending := make([]migration, 0, len(ordered))
	for _, m := range ordered {
		if _, done := applied[m.Version]; !done {
			pending = append(pending, m)
		}
	}
	maxApplied := 0
	for v := range applied {
		if v > maxApplied {
			maxApplied = v
		}
	}
	if len(pending) == 0 {
		return s.setUserVersion(maxApplied)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开始数据库迁移事务失败: %w", err)
	}
	defer tx.Rollback()

	for _, m := range pending {
		if err := m.Apply(tx); err != nil {
			return fmt.Errorf("数据库迁移 v%d（%s）失败，已回滚: %w", m.Version, m.Name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version, name, applied_at)
			VALUES(?,?,strftime('%s','now'))`, m.Version, m.Name); err != nil {
			return fmt.Errorf("记录数据库迁移 v%d 失败，已回滚: %w", m.Version, err)
		}
	}
	// PRAGMA user_version does not accept bound parameters.
	if err := setUserVersionTx(tx, latestVersion()); err != nil {
		return fmt.Errorf("更新数据库版本号失败，已回滚: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交数据库迁移失败，已回滚: %w", err)
	}
	return nil
}

// appliedVersions returns the migration versions recorded in the database.
func (s *Store) appliedVersions() (map[int]bool, error) {
	rows, err := s.db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

// SchemaVersion reports the highest applied migration version.
func (s *Store) SchemaVersion() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var v int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&v); err != nil {
		return 0
	}
	return v
}

func (s *Store) setUserVersion(v int) error {
	_, err := s.db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, v))
	return err
}

func setUserVersionTx(tx *sql.Tx, v int) error {
	_, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, v))
	return err
}

// txColumnInfo lists the column names of a table inside a transaction.
func txColumnInfo(tx *sql.Tx, table string) (map[string]bool, error) {
	rows, err := tx.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols[name] = true
	}
	return cols, rows.Err()
}

// txTableExists reports whether a table is present.
func txTableExists(tx *sql.Tx, table string) (bool, error) {
	var name string
	err := tx.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// migrateBaseline creates the current schema from scratch and backfills every
// column that older databases may be missing (pre-v0.4 probe columns, the
// protocol columns and accounts.is_default), then tags those databases as v1.
func migrateBaseline(tx *sql.Tx) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS upstreams (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			host TEXT NOT NULL,
			username TEXT NOT NULL DEFAULT '',
			password TEXT NOT NULL DEFAULT '',
			method TEXT NOT NULL DEFAULT '',
			uuid TEXT NOT NULL DEFAULT '',
			tls INTEGER NOT NULL DEFAULT 0,
			sni TEXT NOT NULL DEFAULT '',
			security TEXT NOT NULL DEFAULT '',
			pbk TEXT NOT NULL DEFAULT '',
			sid TEXT NOT NULL DEFAULT '',
			flow TEXT NOT NULL DEFAULT '',
			fingerprint TEXT NOT NULL DEFAULT '',
			transport TEXT NOT NULL DEFAULT '',
			path TEXT NOT NULL DEFAULT '',
			host_header TEXT NOT NULL DEFAULT '',
			service_name TEXT NOT NULL DEFAULT '',
			vmess_security TEXT NOT NULL DEFAULT '',
			remark TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS ports (
			id TEXT PRIMARY KEY,
			port INTEGER NOT NULL UNIQUE,
			default_upstream TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			remark TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS accounts (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			password TEXT NOT NULL,
			port_id TEXT NOT NULL DEFAULT '',
			upstream_id TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			remark TEXT NOT NULL DEFAULT '',
			is_default INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ts INTEGER NOT NULL,
			level TEXT NOT NULL,
			port INTEGER NOT NULL DEFAULT 0,
			account TEXT NOT NULL DEFAULT '',
			client TEXT NOT NULL DEFAULT '',
			proto TEXT NOT NULL DEFAULT '',
			target TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			bytes_in INTEGER NOT NULL DEFAULT 0,
			bytes_out INTEGER NOT NULL DEFAULT 0,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			msg TEXT NOT NULL DEFAULT '')`,
		`CREATE INDEX IF NOT EXISTS idx_logs_ts ON logs(ts)`,
		`CREATE INDEX IF NOT EXISTS idx_logs_port_ts ON logs(port, ts)`,
		`CREATE INDEX IF NOT EXISTS idx_logs_account_ts ON logs(account, ts)`,
		`CREATE INDEX IF NOT EXISTS idx_logs_level_ts ON logs(level, ts)`,
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("创建基础表失败: %w", err)
		}
	}

	// Probe columns arrived in v0.4, protocol columns (shadowsocks/vless/vmess)
	// and the REALITY/transport columns later on: ALTER older databases so the
	// baseline schema matches the CREATE TABLE above.
	upstreamCols := map[string]string{
		"health":         "TEXT NOT NULL DEFAULT 'unknown'",
		"latency_ms":     "INTEGER NOT NULL DEFAULT 0",
		"exit_ip":        "TEXT NOT NULL DEFAULT ''",
		"last_check_at":  "INTEGER NOT NULL DEFAULT 0",
		"last_error":     "TEXT NOT NULL DEFAULT ''",
		"method":         "TEXT NOT NULL DEFAULT ''",
		"uuid":           "TEXT NOT NULL DEFAULT ''",
		"tls":            "INTEGER NOT NULL DEFAULT 0",
		"sni":            "TEXT NOT NULL DEFAULT ''",
		"security":       "TEXT NOT NULL DEFAULT ''",
		"pbk":            "TEXT NOT NULL DEFAULT ''",
		"sid":            "TEXT NOT NULL DEFAULT ''",
		"flow":           "TEXT NOT NULL DEFAULT ''",
		"fingerprint":    "TEXT NOT NULL DEFAULT ''",
		"transport":      "TEXT NOT NULL DEFAULT ''",
		"path":           "TEXT NOT NULL DEFAULT ''",
		"host_header":    "TEXT NOT NULL DEFAULT ''",
		"service_name":   "TEXT NOT NULL DEFAULT ''",
		"vmess_security": "TEXT NOT NULL DEFAULT ''",
	}
	if err := addMissingColumns(tx, "upstreams", upstreamCols); err != nil {
		return err
	}
	if err := addMissingColumns(tx, "accounts", map[string]string{
		"is_default": "INTEGER NOT NULL DEFAULT 0",
	}); err != nil {
		return err
	}
	return nil
}

// addMissingColumns ALTERs a table to add every column in cols that is absent.
// Columns are added in a fixed order (order given by the CREATE TABLE
// definitions in migrateBaseline) so upgraded and fresh databases end up with
// the same physical layout.
func addMissingColumns(tx *sql.Tx, table string, cols map[string]string) error {
	existing, err := txColumnInfo(tx, table)
	if err != nil {
		return fmt.Errorf("读取 %s 表结构失败: %w", table, err)
	}
	for _, name := range columnOrder(table, cols) {
		if existing[name] {
			continue
		}
		if _, err := tx.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, name, cols[name])); err != nil {
			return fmt.Errorf("升级 %s.%s 失败: %w", table, name, err)
		}
	}
	return nil
}

// canonicalColumns lists column order for the tables migrations ALTER; names
// absent from a caller's map are skipped.
var canonicalColumns = map[string][]string{
	"upstreams": {"health", "latency_ms", "exit_ip", "last_check_at", "last_error", "method", "uuid",
		"tls", "sni", "security", "pbk", "sid", "flow", "fingerprint", "transport", "path",
		"host_header", "service_name", "vmess_security"},
	"accounts": {"is_default"},
}

func columnOrder(table string, cols map[string]string) []string {
	var out []string
	for _, name := range canonicalColumns[table] {
		if _, ok := cols[name]; ok {
			out = append(out, name)
		}
	}
	if len(out) == len(cols) {
		return out
	}
	// Unknown table/columns: fall back to alphabetical for determinism.
	for name := range cols {
		found := false
		for _, seen := range out {
			if seen == name {
				found = true
				break
			}
		}
		if !found {
			out = append(out, name)
		}
	}
	return out
}

// migrateSecurity folds the legacy boolean tls column into security, the
// single source of truth from v2 on. Rows that already carry a security value
// (including REALITY) are left untouched, but the columns are still added to
// very old databases that predate them.
func migrateSecurity(tx *sql.Tx) error {
	if err := addMissingColumns(tx, "upstreams", map[string]string{
		"tls":      "INTEGER NOT NULL DEFAULT 0",
		"security": "TEXT NOT NULL DEFAULT ''",
	}); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE upstreams SET security = CASE
			WHEN tls = 1 THEN 'tls' ELSE 'none' END
		WHERE security IS NULL OR TRIM(security) = ''`); err != nil {
		return fmt.Errorf("迁移 upstreams.security 失败: %w", err)
	}
	// Normalise historical spellings so downstream switch statements match.
	if _, err := tx.Exec(`UPDATE upstreams SET security = LOWER(TRIM(security))
		WHERE security <> LOWER(TRIM(security))`); err != nil {
		return fmt.Errorf("规范化 upstreams.security 失败: %w", err)
	}
	return nil
}

// migrateEncryptPasswords seals existing plaintext upstream/account passwords
// with the store's key. Values already carrying the enc:v1: prefix (a re-run
// after a partial manual restore) are skipped.
func migrateEncryptPasswords(tx *sql.Tx) error {
	for _, table := range []string{"upstreams", "accounts"} {
		exists, err := txTableExists(tx, table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		rows, err := tx.Query(`SELECT id, password FROM ` + table)
		if err != nil {
			return fmt.Errorf("读取 %s.password 失败: %w", table, err)
		}
		type row struct{ id, password string }
		var pending []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.id, &r.password); err != nil {
				rows.Close()
				return fmt.Errorf("读取 %s.password 失败: %w", table, err)
			}
			if r.password == "" || IsSealed(r.password) {
				continue
			}
			pending = append(pending, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, r := range pending {
			sealed, err := sealSecret(r.password)
			if err != nil {
				return fmt.Errorf("加密 %s(%s) 密码失败: %w", table, r.id, err)
			}
			if _, err := tx.Exec(`UPDATE `+table+` SET password=? WHERE id=?`, sealed, r.id); err != nil {
				return fmt.Errorf("写入 %s(%s) 加密密码失败: %w", table, r.id, err)
			}
		}
	}
	return nil
}

// MigrateForTest lets the test suite drive the migration engine against a
// temporary database handle (production always goes through Open).
func MigrateForTest(db *sql.DB, key []byte) error {
	if err := SetSecretKey(key, true); err != nil {
		return err
	}
	return (&Store{db: db}).migrate()
}

var _ = strings.TrimSpace
