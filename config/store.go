// Package config persists FastProxy entities in a SQLite database
// (pure-Go driver; a single self-contained file, WAL journaling, no CGO).
//
// Layout (created on first open):
//
//	settings(key, value)            -- admin hash, session secret, JSON settings
//	upstreams(id, name, type, host, username, password, remark, created_at, updated_at)
//	ports(id, port UNIQUE, default_upstream, enabled, remark, created_at, updated_at)
//	accounts(id, username UNIQUE, password, port_id, upstream_id, enabled, remark, ...)
//	logs(ts, level, port, account, client, proto, target, status, bytes..., msg)
//
// A legacy fastproxy JSON config next to the database is imported once.
package config

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/handsomezhuzhu/fastproxy/logx"
)

// Supported upstream proxy types.
const (
	UpstreamHTTP   = "http"
	UpstreamSocks5 = "socks5"
)

// ErrNotFound is returned when an entity id does not exist.
var ErrNotFound = errors.New("不存在")

// Upstream is one upstream proxy in the pool.
type Upstream struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"` // "http" or "socks5"
	Host     string `json:"host"` // host:port
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Remark   string `json:"remark,omitempty"`
}

// Port is one inbound listening port bound to a default upstream.
type Port struct {
	ID              string `json:"id"`
	Port            int    `json:"port"`
	DefaultUpstream string `json:"default_upstream"` // upstream id, "" = 直连
	Enabled         bool   `json:"enabled"`
	Remark          string `json:"remark,omitempty"`
}

// Account is one inbound credential. Username is globally unique; the account
// may be bound to one port or valid on all ports, and may override the port's
// default upstream.
type Account struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	PortID     string `json:"port_id"`     // "" = 所有端口可用
	UpstreamID string `json:"upstream_id"` // "" = 跟随端口默认上游
	Enabled    bool   `json:"enabled"`
	Remark     string `json:"remark,omitempty"`
}

// Settings is the global configuration entry exposed in the admin panel.
type Settings struct {
	AccessLog           bool   `json:"access_log"`
	LogLevel            string `json:"log_level"`
	LogBuffer           int    `json:"log_buffer"`
	LogRetentionDays    int    `json:"log_retention_days"`
	AllowDirect         bool   `json:"allow_direct"`
	DialTimeoutSec      int    `json:"dial_timeout_sec"`
	HandshakeTimeoutSec int    `json:"handshake_timeout_sec"`
}

// DefaultSettings returns the built-in defaults.
func DefaultSettings() Settings {
	return Settings{
		AccessLog:           true,
		LogLevel:            "info",
		LogBuffer:           1000,
		LogRetentionDays:    7,
		AllowDirect:         true,
		DialTimeoutSec:      15,
		HandshakeTimeoutSec: 60,
	}
}

// Store is a SQLite-backed store. All methods are safe for concurrent use.
type Store struct {
	mu        sync.RWMutex
	db        *sql.DB
	path      string
	adminPort int
	settings  Settings
}

// Open opens (or creates) the SQLite database, applies the schema and
// imports a legacy JSON config when the database is empty.
func Open(path string, adminPort int) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", filepath.ToSlash(path))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one writer, avoids SQLITE_BUSY loops
	s := &Store{db: db, path: path, adminPort: adminPort}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.importLegacyLocked(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.loadSettingsLocked(); err != nil {
		db.Close()
		return nil, err
	}
	if os.Getenv("SESSION_SECRET") == "" && s.SessionSecret() == "" {
		if err := s.RotateSessionSecret(); err != nil {
			db.Close()
			return nil, err
		}
	}
	if missing := s.checkSecretRefsLocked(); len(missing) > 0 {
		db.Close()
		return nil, missingRefError("配置文件中的敏感字段", missing)
	}
	return s, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// Path returns the database path.
func (s *Store) Path() string { return s.path }

func (s *Store) migrate() error {
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
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("初始化数据库失败: %w", err)
		}
	}
	return nil
}

// importLegacyLocked imports a fastproxy JSON config stored next to the
// database (from v0.2.x) when the database has no entities yet.
func (s *Store) importLegacyLocked() error {
	legacy := filepath.Join(filepath.Dir(s.path), "config.json")
	if _, err := os.Stat(legacy); err != nil {
		return nil
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM upstreams`).Scan(&n); err != nil {
		return err
	}
	var m int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&m); err != nil {
		return err
	}
	if n+m > 0 {
		return nil // database already holds data; keep the legacy file untouched
	}

	raw, err := os.ReadFile(legacy)
	if err != nil {
		return nil
	}
	var doc struct {
		AdminPasswordHash string            `json:"admin_password_hash"`
		SessionSecret     string            `json:"session_secret"`
		Settings          *Settings         `json:"settings"`
		Upstreams         []*Upstream       `json:"upstreams"`
		Ports             []*Port           `json:"ports"`
		Accounts          []*Account        `json:"accounts"`
		Rules             []json.RawMessage `json:"rules"` // v0.1 flat layout
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("旧配置文件 %s 解析失败: %w", legacy, err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().Unix()

	if doc.AdminPasswordHash != "" {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO settings(key, value) VALUES('admin_password_hash', ?)`, doc.AdminPasswordHash); err != nil {
			return err
		}
	}
	if doc.SessionSecret == "" {
		doc.SessionSecret = randomHex(32)
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO settings(key, value) VALUES('session_secret', ?)`, doc.SessionSecret); err != nil {
		return err
	}

	for _, u := range doc.Upstreams {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO upstreams
			(id, name, type, host, username, password, remark, created_at, updated_at)
			VALUES(?,?,?,?,?,?,?,?,?)`,
			u.ID, u.Name, u.Type, u.Host, u.Username, u.Password, u.Remark, now, now); err != nil {
			return err
		}
	}
	for _, p := range doc.Ports {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO ports
			(id, port, default_upstream, enabled, remark, created_at, updated_at)
			VALUES(?,?,?,?,?,?,?)`,
			p.ID, p.Port, p.DefaultUpstream, boolInt(p.Enabled), p.Remark, now, now); err != nil {
			return err
		}
	}
	for _, a := range doc.Accounts {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO accounts
			(id, username, password, port_id, upstream_id, enabled, remark, created_at, updated_at)
			VALUES(?,?,?,?,?,?,?,?,?)`,
			a.ID, a.Username, a.Password, a.PortID, a.UpstreamID, boolInt(a.Enabled), a.Remark, now, now); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// Keep the legacy file for reference but stop reading it.
	_ = os.Rename(legacy, legacy+".migrated")
	return nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// --- settings ---

func (s *Store) loadSettingsLocked() error {
	st := DefaultSettings()
	var raw string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key='settings'`).Scan(&raw)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		b, _ := json.Marshal(st)
		if _, err := s.db.Exec(`INSERT OR REPLACE INTO settings(key, value) VALUES('settings', ?)`, string(b)); err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		if err := json.Unmarshal([]byte(raw), &st); err != nil {
			return fmt.Errorf("settings 解析失败: %w", err)
		}
	}
	if st.LogRetentionDays <= 0 {
		st.LogRetentionDays = DefaultSettings().LogRetentionDays
	}
	s.settings = st
	return nil
}

// Settings returns the current global settings (cached in memory).
func (s *Store) Settings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

// UpdateSettings validates and persists the global settings.
func (s *Store) UpdateSettings(in Settings) (Settings, error) {
	switch strings.ToLower(strings.TrimSpace(in.LogLevel)) {
	case "debug", "info", "warn", "error":
		in.LogLevel = strings.ToLower(strings.TrimSpace(in.LogLevel))
	case "":
		in.LogLevel = "info"
	default:
		return Settings{}, errors.New("日志级别仅支持 debug/info/warn/error")
	}
	if in.LogBuffer < 100 || in.LogBuffer > 10000 {
		return Settings{}, errors.New("日志缓存条数需在 100-10000 之间")
	}
	if in.LogRetentionDays < 1 || in.LogRetentionDays > 90 {
		return Settings{}, errors.New("日志保留天数需在 1-90 之间")
	}
	if in.DialTimeoutSec < 3 || in.DialTimeoutSec > 120 {
		return Settings{}, errors.New("连接超时需在 3-120 秒之间")
	}
	if in.HandshakeTimeoutSec < 10 || in.HandshakeTimeoutSec > 300 {
		return Settings{}, errors.New("握手超时需在 10-300 秒之间")
	}
	b, err := json.Marshal(in)
	if err != nil {
		return Settings{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`INSERT OR REPLACE INTO settings(key, value) VALUES('settings', ?)`, string(b)); err != nil {
		return Settings{}, err
	}
	s.settings = in
	return in, nil
}

// --- upstreams ---

func (s *Store) Upstreams() []*Upstream {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`SELECT id, name, type, host, username, password, remark FROM upstreams ORDER BY created_at`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Upstream
	for rows.Next() {
		var u Upstream
		if err := rows.Scan(&u.ID, &u.Name, &u.Type, &u.Host, &u.Username, &u.Password, &u.Remark); err != nil {
			continue
		}
		c := resolveUpstream(u)
		out = append(out, &c)
	}
	return out
}

func (s *Store) getUpstreamLocked(id string) *Upstream {
	var u Upstream
	err := s.db.QueryRow(`SELECT id, name, type, host, username, password, remark FROM upstreams WHERE id=?`, id).
		Scan(&u.ID, &u.Name, &u.Type, &u.Host, &u.Username, &u.Password, &u.Remark)
	if err != nil {
		return nil
	}
	c := resolveUpstream(u)
	return &c
}

// resolveUpstream expands secret references in a copy of the upstream.
func resolveUpstream(u Upstream) Upstream {
	u.Username, _ = expandEnv(u.Username)
	u.Password, _ = expandEnv(u.Password)
	return u
}

// UpsertUpstream validates and inserts (id == "") or updates an upstream.
func (s *Store) UpsertUpstream(id string, in *Upstream) (*Upstream, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	in.Name = strings.TrimSpace(in.Name)
	in.Remark = strings.TrimSpace(in.Remark)
	if in.Name == "" {
		return nil, errors.New("上游名称不能为空")
	}
	switch in.Type {
	case "", UpstreamHTTP:
		in.Type = UpstreamHTTP
	case UpstreamSocks5:
	default:
		return nil, errors.New("上游类型仅支持 http 或 socks5")
	}
	if _, _, err := net.SplitHostPort(in.Host); err != nil {
		return nil, errors.New("上游地址格式应为 host:port，如 1.2.3.4:8080")
	}
	if missing := dedupeMissing(append(mustRef("上游账号", in.Username), mustRef("上游密码", in.Password)...)); len(missing) > 0 {
		return nil, missingRefError("上游凭据", missing)
	}

	now := time.Now().Unix()
	if id == "" {
		in.ID = randomHex(4)
		_, err := s.db.Exec(`INSERT INTO upstreams
			(id, name, type, host, username, password, remark, created_at, updated_at)
			VALUES(?,?,?,?,?,?,?,?,?)`,
			in.ID, in.Name, in.Type, in.Host, in.Username, in.Password, in.Remark, now, now)
		if err != nil {
			return nil, err
		}
	} else {
		res, err := s.db.Exec(`UPDATE upstreams SET name=?, type=?, host=?, username=?, password=?, remark=?, updated_at=? WHERE id=?`,
			in.Name, in.Type, in.Host, in.Username, in.Password, in.Remark, now, id)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, ErrNotFound
		}
		in.ID = id
	}
	return s.getUpstreamLocked(in.ID), nil
}

// DeleteUpstream removes an upstream unless it is still referenced.
func (s *Store) DeleteUpstream(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var port int
	err := s.db.QueryRow(`SELECT port FROM ports WHERE default_upstream=? LIMIT 1`, id).Scan(&port)
	if err == nil {
		return fmt.Errorf("上游仍被端口 %d 引用，请先修改端口绑定", port)
	}
	var user string
	err = s.db.QueryRow(`SELECT username FROM accounts WHERE upstream_id=? LIMIT 1`, id).Scan(&user)
	if err == nil {
		return fmt.Errorf("上游仍被账户 %s 引用，请先修改账户绑定", user)
	}
	res, err := s.db.Exec(`DELETE FROM upstreams WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ImportUpstreams appends candidates in one transaction, skipping exact
// duplicates (type+host+user+password). Returns added count and dup names.
func (s *Store) ImportUpstreams(candidates []*Upstream) (int, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	seen := map[string]bool{}
	rows, err := s.db.Query(`SELECT type, host, username, password FROM upstreams`)
	if err == nil {
		for rows.Next() {
			var typ, host, user, pass string
			if rows.Scan(&typ, &host, &user, &pass) == nil {
				seen[typ+"|"+host+"|"+user+"|"+pass] = true
			}
		}
		rows.Close()
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, nil
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	added := 0
	var duplicates []string
	for _, u := range candidates {
		key := u.Type + "|" + u.Host + "|" + u.Username + "|" + u.Password
		if seen[key] {
			duplicates = append(duplicates, u.Name+" ("+u.Host+")")
			continue
		}
		seen[key] = true
		u.ID = randomHex(4)
		if _, err := tx.Exec(`INSERT OR IGNORE INTO upstreams
			(id, name, type, host, username, password, remark, created_at, updated_at)
			VALUES(?,?,?,?,?,?,?,?,?)`,
			u.ID, u.Name, u.Type, u.Host, u.Username, u.Password, u.Remark, now, now); err != nil {
			return added, duplicates
		}
		added++
	}
	if err := tx.Commit(); err != nil {
		return 0, duplicates
	}
	return added, duplicates
}

// --- ports ---

func (s *Store) Ports() []*Port {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`SELECT id, port, default_upstream, enabled, remark FROM ports ORDER BY port`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Port
	for rows.Next() {
		var p Port
		var enabled int
		if err := rows.Scan(&p.ID, &p.Port, &p.DefaultUpstream, &enabled, &p.Remark); err != nil {
			continue
		}
		p.Enabled = enabled == 1
		out = append(out, &p)
	}
	return out
}

func (s *Store) getPortLocked(id string) *Port {
	var p Port
	var enabled int
	err := s.db.QueryRow(`SELECT id, port, default_upstream, enabled, remark FROM ports WHERE id=?`, id).
		Scan(&p.ID, &p.Port, &p.DefaultUpstream, &enabled, &p.Remark)
	if err != nil {
		return nil
	}
	p.Enabled = enabled == 1
	return &p
}

// UpsertPort validates and inserts (id == "") or updates an inbound port.
func (s *Store) UpsertPort(id string, in *Port) (*Port, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	in.Remark = strings.TrimSpace(in.Remark)
	if in.Port <= 0 || in.Port > 65535 {
		return nil, errors.New("端口无效（1-65535）")
	}
	if in.Port == s.adminPort {
		return nil, fmt.Errorf("端口 %d 与管理面板端口冲突", s.adminPort)
	}
	if in.DefaultUpstream == "" && !s.settings.AllowDirect {
		return nil, errors.New("当前设置不允许直连端口，请先绑定默认上游")
	}
	if in.DefaultUpstream != "" && s.getUpstreamLocked(in.DefaultUpstream) == nil {
		return nil, errors.New("默认上游不存在")
	}

	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM ports WHERE port=? AND id<>?`, in.Port, id).Scan(&n)
	if n > 0 {
		return nil, fmt.Errorf("端口 %d 已被其他线路使用", in.Port)
	}

	now := time.Now().Unix()
	if id == "" {
		in.ID = randomHex(4)
		if _, err := s.db.Exec(`INSERT INTO ports
			(id, port, default_upstream, enabled, remark, created_at, updated_at)
			VALUES(?,?,?,?,?,?,?)`,
			in.ID, in.Port, in.DefaultUpstream, boolInt(in.Enabled), in.Remark, now, now); err != nil {
			return nil, err
		}
	} else {
		res, err := s.db.Exec(`UPDATE ports SET port=?, default_upstream=?, enabled=?, remark=?, updated_at=? WHERE id=?`,
			in.Port, in.DefaultUpstream, boolInt(in.Enabled), in.Remark, now, id)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, ErrNotFound
		}
		in.ID = id
	}
	return s.getPortLocked(in.ID), nil
}

// DeletePort removes an inbound port; accounts bound to it become global.
func (s *Store) DeletePort(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE accounts SET port_id='' WHERE port_id=?`, id); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM ports WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// SetPortEnabled toggles an inbound port.
func (s *Store) SetPortEnabled(id string, enabled bool) (*Port, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE ports SET enabled=?, updated_at=? WHERE id=?`, boolInt(enabled), time.Now().Unix(), id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.getPortLocked(id), nil
}

// --- accounts ---

func (s *Store) Accounts() []*Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`SELECT id, username, password, port_id, upstream_id, enabled, remark FROM accounts ORDER BY created_at`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Account
	for rows.Next() {
		var a Account
		var enabled int
		if err := rows.Scan(&a.ID, &a.Username, &a.Password, &a.PortID, &a.UpstreamID, &enabled, &a.Remark); err != nil {
			continue
		}
		a.Enabled = enabled == 1
		c := resolveAccount(a)
		out = append(out, &c)
	}
	return out
}

// resolveAccount expands secret references in a copy of the account.
func resolveAccount(a Account) Account {
	a.Password, _ = expandEnv(a.Password)
	return a
}

func (s *Store) getAccountLocked(id string) *Account {
	var a Account
	var enabled int
	err := s.db.QueryRow(`SELECT id, username, password, port_id, upstream_id, enabled, remark FROM accounts WHERE id=?`, id).
		Scan(&a.ID, &a.Username, &a.Password, &a.PortID, &a.UpstreamID, &enabled, &a.Remark)
	if err != nil {
		return nil
	}
	a.Enabled = enabled == 1
	c := resolveAccount(a)
	return &c
}

// UpsertAccount validates and inserts (id == "") or updates an account.
// Usernames are globally unique to keep auth lookup unambiguous.
func (s *Store) UpsertAccount(id string, in *Account) (*Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	in.Username = strings.TrimSpace(in.Username)
	in.Password = strings.TrimSpace(in.Password)
	in.Remark = strings.TrimSpace(in.Remark)
	if in.Username == "" || in.Password == "" {
		return nil, errors.New("用户名和密码不能为空")
	}
	if strings.ContainsAny(in.Username, " \t:") {
		return nil, errors.New("用户名不能包含空格或冒号")
	}
	if in.PortID != "" && s.getPortLocked(in.PortID) == nil {
		return nil, errors.New("绑定的端口不存在")
	}
	if in.UpstreamID != "" && s.getUpstreamLocked(in.UpstreamID) == nil {
		return nil, errors.New("绑定的上游不存在")
	}
	if missing := mustRef("账户密码", in.Password); len(missing) > 0 {
		return nil, missingRefError("账户密码", missing)
	}

	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM accounts WHERE username=? AND id<>?`, in.Username, id).Scan(&n)
	if n > 0 {
		return nil, fmt.Errorf("用户名 %s 已存在", in.Username)
	}

	now := time.Now().Unix()
	if id == "" {
		in.ID = randomHex(4)
		if _, err := s.db.Exec(`INSERT INTO accounts
			(id, username, password, port_id, upstream_id, enabled, remark, created_at, updated_at)
			VALUES(?,?,?,?,?,?,?,?,?)`,
			in.ID, in.Username, in.Password, in.PortID, in.UpstreamID, boolInt(in.Enabled), in.Remark, now, now); err != nil {
			return nil, err
		}
	} else {
		res, err := s.db.Exec(`UPDATE accounts SET username=?, password=?, port_id=?, upstream_id=?, enabled=?, remark=?, updated_at=? WHERE id=?`,
			in.Username, in.Password, in.PortID, in.UpstreamID, boolInt(in.Enabled), in.Remark, now, id)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, ErrNotFound
		}
		in.ID = id
	}
	return s.getAccountLocked(in.ID), nil
}

// DeleteAccount removes an account by id.
func (s *Store) DeleteAccount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`DELETE FROM accounts WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetAccountEnabled toggles an account.
func (s *Store) SetAccountEnabled(id string, enabled bool) (*Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE accounts SET enabled=?, updated_at=? WHERE id=?`, boolInt(enabled), time.Now().Unix(), id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.getAccountLocked(id), nil
}

// --- admin settings ---

// AdminPasswordHash returns the stored password hash ("" when unset).
func (s *Store) AdminPasswordHash() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var v string
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE key='admin_password_hash'`).Scan(&v)
	return v
}

// SetAdminPasswordHash stores a new admin password hash.
func (s *Store) SetAdminPasswordHash(hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT OR REPLACE INTO settings(key, value) VALUES('admin_password_hash', ?)`, hash)
	return err
}

// SessionSecret returns the HMAC secret for admin session cookies.
// The SESSION_SECRET environment variable, when set, overrides the stored one.
func (s *Store) SessionSecret() string {
	if v := os.Getenv("SESSION_SECRET"); v != "" {
		return v
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var v string
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE key='session_secret'`).Scan(&v)
	return v
}

// RotateSessionSecret replaces the session secret, invalidating all sessions.
func (s *Store) RotateSessionSecret() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT OR REPLACE INTO settings(key, value) VALUES('session_secret', ?)`, randomHex(32))
	return err
}

// --- log persistence (implements logx.Persist) ---

// InsertLog appends one access log entry.
func (s *Store) InsertLog(e logx.Entry) error {
	ts := time.Now().UnixMilli()
	if t, err := time.Parse(time.RFC3339, e.Time); err == nil {
		ts = t.UnixMilli()
	}
	_, err := s.db.Exec(`INSERT INTO logs
		(ts, level, port, account, client, proto, target, status, bytes_in, bytes_out, duration_ms, msg)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		ts, e.Level, e.Port, e.Account, e.Client, e.Proto, e.Target, e.Status, e.BytesIn, e.BytesOut, e.DurationMS, e.Msg)
	return err
}

// QueryLogs returns access logs newest-first.
func (s *Store) QueryLogs(f logx.Filter) ([]logx.Entry, error) {
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	var where []string
	var args []any
	if f.Port != 0 {
		where = append(where, "port = ?")
		args = append(args, f.Port)
	}
	if f.Account != "" {
		where = append(where, "account = ?")
		args = append(args, f.Account)
	}
	if lvl := logx.ParseLevel(f.Level); lvl > logx.LevelDebug {
		var allowed []string
		for l := lvl; l <= logx.LevelError; l++ {
			allowed = append(allowed, l.String())
		}
		where = append(where, "level IN ("+placeholders(len(allowed))+")")
		for _, l := range allowed {
			args = append(args, l)
		}
	}
	q := `SELECT ts, level, port, account, client, proto, target, status, bytes_in, bytes_out, duration_ms, msg FROM logs`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]logx.Entry, 0, limit)
	for rows.Next() {
		var e logx.Entry
		var ts int64
		if err := rows.Scan(&ts, &e.Level, &e.Port, &e.Account, &e.Client, &e.Proto, &e.Target,
			&e.Status, &e.BytesIn, &e.BytesOut, &e.DurationMS, &e.Msg); err != nil {
			continue
		}
		e.Time = time.UnixMilli(ts).Format(time.RFC3339)
		out = append(out, e)
	}
	return out, nil
}

// PruneLogs deletes access logs older than the given number of days.
func (s *Store) PruneLogs(days int) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -days).UnixMilli()
	res, err := s.db.Exec(`DELETE FROM logs WHERE ts < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func placeholders(n int) string {
	if n == 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// checkSecretRefsLocked verifies that all ${VAR} references in sensitive
// fields resolve in the current environment. Returns the missing var names.
func (s *Store) checkSecretRefsLocked() []string {
	var missing []string
	for _, u := range s.Upstreams() {
		missing = append(missing, mustRef("上游["+u.Name+"].账号", u.Username)...)
		missing = append(missing, mustRef("上游["+u.Name+"].密码", u.Password)...)
	}
	for _, a := range s.Accounts() {
		missing = append(missing, mustRef("账户["+a.Username+"].密码", a.Password)...)
	}
	return dedupeMissing(missing)
}

// ParseUpstream accepts a raw upstream address such as
// "1.2.3.4:8080", "http://user:pass@1.2.3.4:8080" or "socks5://host:1080"
// and returns (type, host:port, user, pass).
//
// Parsing is manual (not url.Parse) so that passwords containing '@', ':'
// or "${VAR}" environment references survive verbatim; values are NOT
// percent-decoded.
func ParseUpstream(raw string) (typ, host, user, pass string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", "", "", errors.New("上游地址不能为空")
	}
	scheme := "http"
	rest := raw
	if i := strings.Index(raw, "://"); i >= 0 {
		scheme = strings.ToLower(raw[:i])
		rest = raw[i+3:]
	}
	switch scheme {
	case "http", "https":
		typ = UpstreamHTTP
	case "socks5", "socks5h", "socks":
		typ = UpstreamSocks5
	default:
		return "", "", "", "", fmt.Errorf("不支持的协议 %q（支持 http:// 或 socks5://）", scheme)
	}
	if k := strings.IndexAny(rest, "/?#"); k >= 0 {
		rest = rest[:k]
	}
	if j := strings.LastIndex(rest, "@"); j >= 0 {
		userinfo := rest[:j]
		rest = rest[j+1:]
		if c := strings.Index(userinfo, ":"); c >= 0 {
			user, pass = userinfo[:c], userinfo[c+1:]
		} else {
			user = userinfo
		}
	}
	h, port, serr := net.SplitHostPort(rest)
	if serr != nil || h == "" {
		return "", "", "", "", errors.New("上游地址需包含端口，如 http://1.2.3.4:8080")
	}
	return typ, net.JoinHostPort(h, port), user, pass, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	return hex.EncodeToString(b)
}
