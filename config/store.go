// Package config persists FastProxy entities in a SQLite database
// (pure-Go driver; a single self-contained file, WAL journaling, no CGO).
//
// Layout (created on first open):
//
//	settings(key, value)            -- admin hash, session secret, JSON settings
//	upstreams(id, name, type, host, username, password, method, uuid, tls, sni,
//	          security, pbk, sid, flow, fingerprint, transport, path, host_header,
//	          service_name, vmess_security, remark, created_at, updated_at)
//	ports(id, port UNIQUE, default_upstream, enabled, remark, created_at, updated_at)
//	accounts(id, username UNIQUE, password, port_id, upstream_id, enabled, remark, ...)
//	logs(ts, level, port, account, client, proto, target, status, bytes..., msg)
//	schema_migrations(version, name, applied_at)
//
// Schema changes go through the versioned migrations in migrate.go; the
// upstreams.tls column is legacy and no longer read or written (see security).
// Passwords are sealed with AES-256-GCM (secrets.go) before they reach SQLite.
package config

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/handsomezhuzhu/fastproxy/logx"
)

// Supported upstream proxy types.
const (
	UpstreamHTTP        = "http"
	UpstreamSocks5      = "socks5"
	UpstreamShadowsocks = "shadowsocks"
	UpstreamVless       = "vless"
	UpstreamVmess       = "vmess"
)

// UpstreamTypes lists every accepted Upstream.Type value (panel order).
var UpstreamTypes = []string{UpstreamHTTP, UpstreamSocks5, UpstreamShadowsocks, UpstreamVless, UpstreamVmess}

// ShadowsocksMethods lists the AEAD ciphers the Shadowsocks client accepts.
var ShadowsocksMethods = []string{"aes-128-gcm", "aes-256-gcm", "chacha20-ietf-poly1305"}

// ErrNotFound is returned when an entity id does not exist.
var ErrNotFound = errors.New("不存在")

// Upstream is one upstream proxy in the pool.
//
// Type selects the egress protocol:
//
//	http / socks5  -> Username + Password
//	shadowsocks    -> Method + Password
//	vless / vmess  -> UUID + Security (+ SNI)
type Upstream struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"` // http | socks5 | shadowsocks | vless | vmess
	Host     string `json:"host"` // host:port
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Method   string `json:"method,omitempty"` // shadowsocks cipher
	UUID     string `json:"uuid,omitempty"`   // vless / vmess user id
	SNI      string `json:"sni,omitempty"`    // vless / vmess: TLS server name ("" = host)
	Remark   string `json:"remark,omitempty"`

	// vless / vmess advanced options (tunnelled through embedded sing-box).
	// Security is the single source of truth for TLS: none | tls | reality.
	Security      string `json:"security,omitempty"`       // "" | "none" | "tls" | "reality"
	PBK           string `json:"pbk,omitempty"`            // REALITY public key
	SID           string `json:"sid,omitempty"`            // REALITY short id
	Flow          string `json:"flow,omitempty"`           // xtls-rprx-vision
	Fingerprint   string `json:"fingerprint,omitempty"`    // uTLS fingerprint (default chrome)
	Transport     string `json:"transport,omitempty"`      // ""/tcp/ws/grpc
	Path          string `json:"path,omitempty"`           // ws path
	HostHeader    string `json:"host_header,omitempty"`    // ws Host header
	ServiceName   string `json:"service_name,omitempty"`   // grpc service name
	VMessSecurity string `json:"vmess_security,omitempty"` // vmess cipher, default auto

	// Probe state (maintained by the prober and passive failure tracking).
	Health      string `json:"health"` // ok | fail | unknown
	LatencyMS   int64  `json:"latency_ms"`
	ExitIP      string `json:"exit_ip"`
	LastCheckAt int64  `json:"last_check_at"` // unix seconds, 0 = never
	LastError   string `json:"last_error,omitempty"`
}

// Port is one inbound listening endpoint. It carries no upstream: routing is
// decided entirely by the authenticated account.
type Port struct {
	ID      string `json:"id"`
	Port    int    `json:"port"`
	Enabled bool   `json:"enabled"`
	Remark  string `json:"remark,omitempty"`
}

// Account is one inbound credential. Username is globally unique. Accounts are
// always bound to exactly one port (there is no global account concept) and
// decide the destination via their optional upstream binding; an empty
// UpstreamID means direct egress.
//
// Every port owns one default account (is_default=true) which may not be
// deleted or moved to another port; deleting the port cascades it.
type Account struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	PortID     string `json:"port_id"`     // required, port scoping
	UpstreamID string `json:"upstream_id"` // "" = 直连；否则上游 id
	Enabled    bool   `json:"enabled"`
	Remark     string `json:"remark,omitempty"`
	IsDefault  bool   `json:"is_default"`
}

// Settings is the global configuration entry exposed in the admin panel.
type Settings struct {
	AccessLog           bool   `json:"access_log"`
	LogLevel            string `json:"log_level"`
	LogBuffer           int    `json:"log_buffer"`
	LogRetentionDays    int    `json:"log_retention_days"`
	DialTimeoutSec      int    `json:"dial_timeout_sec"`
	HandshakeTimeoutSec int    `json:"handshake_timeout_sec"`

	ProbeEnabled     bool   `json:"probe_enabled"`
	ProbeIntervalSec int    `json:"probe_interval_sec"`
	ProbeTimeoutSec  int    `json:"probe_timeout_sec"`
	ProbeConcurrency int    `json:"probe_concurrency"`
	ProbeTarget      string `json:"probe_target"`
}

// DefaultSettings returns the built-in defaults.
func DefaultSettings() Settings {
	return Settings{
		AccessLog:           true,
		LogLevel:            "info",
		LogBuffer:           1000,
		LogRetentionDays:    7,
		DialTimeoutSec:      15,
		HandshakeTimeoutSec: 60,
		ProbeEnabled:        true,
		ProbeIntervalSec:    300,
		ProbeTimeoutSec:     8,
		ProbeConcurrency:    6,
		ProbeTarget:         "api.ipify.org",
	}
}

// PortRange is the starting-level window of inbound ports a deployment may
// use (PORT_RANGE, default 10001-10050). It is not a runtime setting: it is
// fixed at startup and only used to validate port create/update.
type PortRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// DefaultPortRange is used when PORT_RANGE is unset.
var DefaultPortRange = PortRange{Min: 10001, Max: 10050}

// String renders the range in PORT_RANGE syntax ("min-max" or "port").
func (r PortRange) String() string {
	if r.Min == r.Max {
		return fmt.Sprintf("%d", r.Min)
	}
	return fmt.Sprintf("%d-%d", r.Min, r.Max)
}

// Contains reports whether port falls inside the range.
func (r PortRange) Contains(port int) bool { return port >= r.Min && port <= r.Max }

// ParsePortRange parses "10001-10050" or a single port "10080". Bounds must
// satisfy 1 <= min <= max <= 65535.
func ParsePortRange(spec string) (PortRange, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return DefaultPortRange, nil
	}
	parse := func(v string) (int, error) {
		v = strings.TrimSpace(v)
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("PORT_RANGE 格式错误: %q 不是数字（示例 10001-10050 或 10080）", spec)
		}
		if n < 1 || n > 65535 {
			return 0, fmt.Errorf("PORT_RANGE 端口 %d 越界（允许 1-65535）", n)
		}
		return n, nil
	}
	if !strings.Contains(spec, "-") {
		n, err := parse(spec)
		if err != nil {
			return PortRange{}, err
		}
		return PortRange{Min: n, Max: n}, nil
	}
	parts := strings.Split(spec, "-")
	if len(parts) != 2 {
		return PortRange{}, fmt.Errorf("PORT_RANGE 格式错误: %q（示例 10001-10050）", spec)
	}
	min, err := parse(parts[0])
	if err != nil {
		return PortRange{}, err
	}
	max, err := parse(parts[1])
	if err != nil {
		return PortRange{}, err
	}
	if min > max {
		return PortRange{}, fmt.Errorf("PORT_RANGE 起点 %d 大于终点 %d", min, max)
	}
	return PortRange{Min: min, Max: max}, nil
}

// Store is a SQLite-backed store. All methods are safe for concurrent use.
type Store struct {
	mu        sync.RWMutex
	db        *sql.DB
	path      string
	adminPort int
	portRange PortRange
	settings  Settings
}

// Open opens (or creates) the SQLite database, applies pending migrations and
// ensures the admin session secret and password encryption key exist.
//
// OPEN_TIMEOUT_MS bounds how long SQLite waits for a lock at open time
// (default 15000 ms, which tolerates the backup job holding a read lock).
func Open(path string, adminPort int) (*Store, error) {
	return OpenWithPortRange(path, adminPort, DefaultPortRange)
}

// OpenWithPortRange opens the database with an explicit PORT_RANGE window.
func OpenWithPortRange(path string, adminPort int, pr PortRange) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	// The key is resolved before any migration so v3 can seal plaintext rows.
	if err := loadSecretKey(path); err != nil {
		return nil, err
	}
	if pr.Min == 0 && pr.Max == 0 {
		pr = DefaultPortRange
	}
	lockMS := 15000
	if v := strings.TrimSpace(os.Getenv("OPEN_TIMEOUT_MS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			lockMS = n
		}
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)",
		filepath.ToSlash(path), lockMS)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one writer, avoids SQLITE_BUSY loops
	s := &Store{db: db, path: path, adminPort: adminPort, portRange: pr}
	if err := s.migrate(); err != nil {
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

// PortRange returns the startup-level inbound port window.
func (s *Store) PortRange() PortRange { return s.portRange }

// boolInt renders a bool as SQLite's 0/1.
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
	// Probe settings arrived in v0.4; backfill defaults for older rows.
	if st.ProbeIntervalSec <= 0 {
		st.ProbeEnabled = true
		st.ProbeIntervalSec = DefaultSettings().ProbeIntervalSec
		st.ProbeTimeoutSec = DefaultSettings().ProbeTimeoutSec
		st.ProbeTarget = DefaultSettings().ProbeTarget
	}
	if st.ProbeTimeoutSec <= 0 {
		st.ProbeTimeoutSec = DefaultSettings().ProbeTimeoutSec
	}
	if st.ProbeConcurrency <= 0 {
		st.ProbeConcurrency = DefaultSettings().ProbeConcurrency
	}
	if st.ProbeConcurrency > 32 {
		st.ProbeConcurrency = 32
	}
	if strings.TrimSpace(st.ProbeTarget) == "" {
		st.ProbeTarget = DefaultSettings().ProbeTarget
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
	in.ProbeTarget = strings.TrimSpace(in.ProbeTarget)
	if in.ProbeConcurrency == 0 {
		in.ProbeConcurrency = DefaultSettings().ProbeConcurrency
	}
	if in.ProbeIntervalSec < 30 || in.ProbeIntervalSec > 3600 {
		return Settings{}, errors.New("探测间隔需在 30-3600 秒之间")
	}
	if in.ProbeTimeoutSec < 3 || in.ProbeTimeoutSec > 30 {
		return Settings{}, errors.New("探测超时需在 3-30 秒之间")
	}
	if in.ProbeConcurrency < 1 || in.ProbeConcurrency > 32 {
		return Settings{}, errors.New("检测并发数需在 1-32 之间")
	}
	if in.ProbeTarget == "" || strings.ContainsAny(in.ProbeTarget, "/: ") {
		return Settings{}, errors.New("探测目标应为纯域名，如 api.ipify.org")
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

// upstreamColumns is the column list shared by every upstream read; the
// matching scanUpstream keeps SELECT and Scan in lockstep. The legacy `tls`
// column is deliberately absent: security is the only TLS source of truth.
const upstreamColumns = `id, name, type, host, username, password, method, uuid, sni, remark,
	health, latency_ms, exit_ip, last_check_at, last_error,
	security, pbk, sid, flow, fingerprint, transport, path, host_header, service_name, vmess_security`

// upstreamScanner is satisfied by both *sql.Row and *sql.Rows.
type upstreamScanner interface {
	Scan(dest ...any) error
}

func scanUpstream(sc upstreamScanner) (Upstream, error) {
	var u Upstream
	if err := sc.Scan(&u.ID, &u.Name, &u.Type, &u.Host, &u.Username, &u.Password,
		&u.Method, &u.UUID, &u.SNI, &u.Remark,
		&u.Health, &u.LatencyMS, &u.ExitIP, &u.LastCheckAt, &u.LastError,
		&u.Security, &u.PBK, &u.SID, &u.Flow, &u.Fingerprint,
		&u.Transport, &u.Path, &u.HostHeader, &u.ServiceName, &u.VMessSecurity); err != nil {
		return Upstream{}, err
	}
	return resolveUpstream(u)
}

func (s *Store) Upstreams() []*Upstream {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Upstream, 0, 4)
	rows, err := s.db.Query(`SELECT ` + upstreamColumns + ` FROM upstreams ORDER BY created_at`)
	if err != nil {
		log.Printf("config: 查询上游列表失败: %v", err)
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		u, err := scanUpstream(rows)
		if err != nil {
			log.Printf("config: 读取上游记录失败: %v", err)
			continue
		}
		out = append(out, &u)
	}
	if err := rows.Err(); err != nil {
		log.Printf("config: 遍历上游列表失败: %v", err)
	}
	return out
}

func (s *Store) getUpstreamLocked(id string) *Upstream {
	u, err := scanUpstream(s.db.QueryRow(`SELECT `+upstreamColumns+` FROM upstreams WHERE id=?`, id))
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("config: 读取上游 %s 失败: %v", id, err)
		}
		return nil
	}
	return &u
}

// resolveUpstream decrypts the stored password and expands ${VAR} references
// in the credentials of a copy of the upstream. Decryption runs first so a
// sealed placeholder is expanded and a plaintext legacy row keeps working.
func resolveUpstream(u Upstream) (Upstream, error) {
	pass, err := openSecret(u.Password)
	if err != nil {
		return Upstream{}, fmt.Errorf("上游[%s]密码解密失败: %w", u.Name, err)
	}
	u.Password = pass
	u.Username, _ = expandEnv(u.Username)
	u.Password, _ = expandEnv(u.Password)
	return u, nil
}

// normalizeUpstreamType maps the accepted spellings of upstream types onto
// the canonical constants. An empty type means http (legacy clients).
func normalizeUpstreamType(t string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "":
		return UpstreamHTTP, true
	case UpstreamHTTP:
		return UpstreamHTTP, true
	case UpstreamSocks5:
		return UpstreamSocks5, true
	case UpstreamShadowsocks:
		return UpstreamShadowsocks, true
	case UpstreamVless:
		return UpstreamVless, true
	case UpstreamVmess:
		return UpstreamVmess, true
	}
	return "", false
}

// NormalizeUpstream applies per-type normalization and validation to in
// (mutating it in place). The returned error message is user-facing.
func NormalizeUpstream(in *Upstream) error {
	typ, ok := normalizeUpstreamType(in.Type)
	if !ok {
		return fmt.Errorf("上游类型 %q 无效（仅支持 %s）", in.Type, strings.Join(UpstreamTypes, "/"))
	}
	in.Type = typ
	in.Host = strings.TrimSpace(in.Host)
	in.Method = strings.ToLower(strings.TrimSpace(in.Method))
	in.UUID = strings.TrimSpace(in.UUID)
	in.SNI = strings.TrimSpace(in.SNI)
	switch typ {
	case UpstreamShadowsocks:
		if in.Method == "" {
			return errors.New("Shadowsocks 上游必须填写加密方式（method）")
		}
		supported := false
		for _, m := range ShadowsocksMethods {
			if m == in.Method {
				supported = true
			}
		}
		if !supported {
			return fmt.Errorf("暂不支持的 Shadowsocks 加密方式 %q（支持 %s）",
				in.Method, strings.Join(ShadowsocksMethods, "/"))
		}
		if in.Password == "" {
			return errors.New("Shadowsocks 上游必须填写密码")
		}
	case UpstreamVless, UpstreamVmess:
		if in.UUID == "" {
			return errors.New("VLESS/VMess 上游必须填写 UUID")
		}
		if len(in.UUID) != 36 && len(in.UUID) != 32 {
			return fmt.Errorf("UUID %q 格式无效", in.UUID)
		}
		if validateProtocolUpstream != nil {
			if err := validateProtocolUpstream(in); err != nil {
				return err
			}
		}
	}
	return nil
}

func upstreamDedupKey(u *Upstream) string {
	return u.Type + "|" + u.Host + "|" + u.Method + "|" + u.UUID + "|" + u.Username + "|" + u.Password +
		"|" + u.PBK + "|" + u.SID + "|" + u.Flow + "|" + u.Security + "|" + u.Transport +
		"|" + u.Path + "|" + u.HostHeader + "|" + u.ServiceName
}

// UpsertUpstream validates and inserts (id == "") or updates an upstream.
// The password is sealed before it is written; the returned entity carries
// the plaintext (like every other read path).
func (s *Store) UpsertUpstream(id string, in *Upstream) (*Upstream, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	in.Name = strings.TrimSpace(in.Name)
	in.Remark = strings.TrimSpace(in.Remark)
	if in.Name == "" {
		return nil, errors.New("上游名称不能为空")
	}
	if err := NormalizeUpstream(in); err != nil {
		return nil, err
	}
	if _, _, err := net.SplitHostPort(in.Host); err != nil {
		return nil, errors.New("上游地址格式应为 host:port，如 1.2.3.4:8080")
	}
	if missing := dedupeMissing(append(mustRef("上游账号", in.Username), mustRef("上游密码", in.Password)...)); len(missing) > 0 {
		return nil, missingRefError("上游凭据", missing)
	}
	stored, err := sealSecret(in.Password)
	if err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	if id == "" {
		in.ID = randomHex(4)
		_, err := s.db.Exec(`INSERT INTO upstreams
			(id, name, type, host, username, password, method, uuid, sni, remark, health, created_at, updated_at,
			 security, pbk, sid, flow, fingerprint, transport, path, host_header, service_name, vmess_security)
			VALUES(?,?,?,?,?,?,?,?,?,?,'unknown',?,?,
			 ?,?,?,?,?,?,?,?,?,?)`,
			in.ID, in.Name, in.Type, in.Host, in.Username, stored,
			in.Method, in.UUID, in.SNI, in.Remark, now, now,
			in.Security, in.PBK, in.SID, in.Flow, in.Fingerprint,
			in.Transport, in.Path, in.HostHeader, in.ServiceName, in.VMessSecurity)
		if err != nil {
			return nil, err
		}
	} else {
		// Reset probe state when the endpoint identity changes (compared in
		// plaintext on both sides).
		reset := false
		var oldType, oldHost, oldUser, oldPass, oldMethod, oldUUID string
		err := s.db.QueryRow(`SELECT type, host, username, password, method, uuid FROM upstreams WHERE id=?`, id).
			Scan(&oldType, &oldHost, &oldUser, &oldPass, &oldMethod, &oldUUID)
		if err != nil {
			return nil, ErrNotFound
		}
		oldPlain, perr := openSecret(oldPass)
		if perr != nil {
			return nil, fmt.Errorf("上游旧密码解密失败: %w", perr)
		}
		if oldType != in.Type || oldHost != in.Host || oldUser != in.Username || oldPlain != in.Password ||
			oldMethod != in.Method || oldUUID != in.UUID {
			reset = true
		}
		const setCols = `name=?, type=?, host=?, username=?, password=?, method=?, uuid=?, sni=?, remark=?,
			security=?, pbk=?, sid=?, flow=?, fingerprint=?, transport=?, path=?, host_header=?, service_name=?, vmess_security=?`
		args := []any{in.Name, in.Type, in.Host, in.Username, stored,
			in.Method, in.UUID, in.SNI, in.Remark,
			in.Security, in.PBK, in.SID, in.Flow, in.Fingerprint,
			in.Transport, in.Path, in.HostHeader, in.ServiceName, in.VMessSecurity}
		var res sql.Result
		if reset {
			res, err = s.db.Exec(`UPDATE upstreams SET `+setCols+`,
				health='unknown', latency_ms=0, exit_ip='', last_check_at=0, last_error='', updated_at=? WHERE id=?`,
				append(args, now, id)...)
		} else {
			res, err = s.db.Exec(`UPDATE upstreams SET `+setCols+`, updated_at=? WHERE id=?`,
				append(args, now, id)...)
		}
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

// RecordUpstreamCheck stores the latest probe result for an upstream.
// exitIP is only overwritten when non-empty (a failed probe keeps the last
// known exit IP).
func (s *Store) RecordUpstreamCheck(id, health string, latencyMS int64, exitIP, errText string) error {
	if id == "" {
		return nil
	}
	if health == "" {
		health = "unknown"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE upstreams SET health=?, latency_ms=?,
		exit_ip=CASE WHEN ? <> '' THEN ? ELSE exit_ip END,
		last_check_at=?, last_error=? WHERE id=?`,
		health, latencyMS, exitIP, exitIP, time.Now().Unix(), errText, id)
	return err
}

// DeleteUpstream removes an upstream unless it is still referenced by an account.
func (s *Store) DeleteUpstream(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var user string
	err := s.db.QueryRow(`SELECT username FROM accounts WHERE upstream_id=? LIMIT 1`, id).Scan(&user)
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
// duplicates (see upstreamDedupKey) and candidates that fail validation.
// It returns the added count, the duplicate names and one problem per
// rejected candidate.
func (s *Store) ImportUpstreams(candidates []*Upstream) (int, []string, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	seen := map[string]bool{}
	rows, err := s.db.Query(`SELECT type, host, username, password, method, uuid, security, pbk, sid, flow, transport, path, host_header, service_name FROM upstreams`)
	if err == nil {
		for rows.Next() {
			var typ, host, user, pass, method, uuid string
			var sec, pbk, sid, flow, transport, path, hostHeader, service string
			if rows.Scan(&typ, &host, &user, &pass, &method, &uuid,
				&sec, &pbk, &sid, &flow, &transport, &path, &hostHeader, &service) == nil {
				// Compare in plaintext so freshly imported duplicates match
				// sealed rows too.
				if plain, derr := openSecret(pass); derr == nil {
					pass = plain
				}
				seen[typ+"|"+host+"|"+method+"|"+uuid+"|"+user+"|"+pass+
					"|"+pbk+"|"+sid+"|"+flow+"|"+sec+"|"+transport+"|"+path+"|"+hostHeader+"|"+service] = true
			}
		}
		rows.Close()
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, nil, []string{"打开数据库事务失败: " + err.Error()}
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	added := 0
	var duplicates, problems []string
	for _, u := range candidates {
		name := u.Name
		if name == "" {
			name = u.Host
		}
		if err := NormalizeUpstream(u); err != nil {
			problems = append(problems, name+" ("+u.Host+"): "+err.Error())
			continue
		}
		key := upstreamDedupKey(u)
		if seen[key] {
			duplicates = append(duplicates, name+" ("+u.Host+")")
			continue
		}
		seen[key] = true
		u.ID = randomHex(4)
		stored, serr := sealSecret(u.Password)
		if serr != nil {
			problems = append(problems, name+" ("+u.Host+"): 密码加密失败: "+serr.Error())
			continue
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO upstreams
			(id, name, type, host, username, password, method, uuid, sni, remark, created_at, updated_at,
			 security, pbk, sid, flow, fingerprint, transport, path, host_header, service_name, vmess_security)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,
			 ?,?,?,?,?,?,?,?,?,?)`,
			u.ID, name, u.Type, u.Host, u.Username, stored,
			u.Method, u.UUID, u.SNI, u.Remark, now, now,
			u.Security, u.PBK, u.SID, u.Flow, u.Fingerprint,
			u.Transport, u.Path, u.HostHeader, u.ServiceName, u.VMessSecurity); err != nil {
			return added, duplicates, problems
		}
		added++
	}
	if err := tx.Commit(); err != nil {
		return 0, duplicates, append(problems, "提交导入失败: "+err.Error())
	}
	return added, duplicates, problems
}

// --- ports ---

func (s *Store) Ports() []*Port {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Port, 0, 4)
	rows, err := s.db.Query(`SELECT id, port, enabled, remark FROM ports ORDER BY port`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var p Port
		var enabled int
		if err := rows.Scan(&p.ID, &p.Port, &enabled, &p.Remark); err != nil {
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
	err := s.db.QueryRow(`SELECT id, port, enabled, remark FROM ports WHERE id=?`, id).
		Scan(&p.ID, &p.Port, &enabled, &p.Remark)
	if err != nil {
		return nil
	}
	p.Enabled = enabled == 1
	return &p
}

// UpsertPort validates and inserts (id == "") or updates an inbound port.
// Inserting a new port auto-creates its default account (direct egress).
// Call GetDefaultAccount afterwards to read its credentials.
func (s *Store) UpsertPort(id string, in *Port) (*Port, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	in.Remark = strings.TrimSpace(in.Remark)
	if in.Port <= 0 || in.Port > 65535 {
		return nil, errors.New("端口无效（1-65535）")
	}
	if !s.portRange.Contains(in.Port) {
		return nil, fmt.Errorf("端口 %d 超出允许范围（当前 PORT_RANGE=%s，允许 %d-%d），请修改 PORT_RANGE 环境变量或选择范围内的端口",
			in.Port, s.portRange, s.portRange.Min, s.portRange.Max)
	}
	if in.Port == s.adminPort {
		return nil, fmt.Errorf("端口 %d 与管理面板端口冲突", s.adminPort)
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
			(id, port, enabled, remark, created_at, updated_at)
			VALUES(?,?,?,?,?,?)`,
			in.ID, in.Port, boolInt(in.Enabled), in.Remark, now, now); err != nil {
			return nil, err
		}
		if err := s.ensureDefaultAccountLocked(*in, ""); err != nil {
			if _, derr := s.db.Exec(`DELETE FROM ports WHERE id=?`, in.ID); derr != nil {
				return nil, derr
			}
			return nil, err
		}
	} else {
		res, err := s.db.Exec(`UPDATE ports SET port=?, enabled=?, remark=?, updated_at=? WHERE id=?`,
			in.Port, boolInt(in.Enabled), in.Remark, now, id)
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

// ensureDefaultAccountLocked inserts the port's default account when absent.
// upstreamID may be "" (direct) or a migrated upstream id.
func (s *Store) ensureDefaultAccountLocked(p Port, upstreamID string) error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM accounts WHERE port_id=? AND is_default=1`, p.ID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	username := fmt.Sprintf("default-%d", p.Port)
	for i := 2; ; i++ {
		var n int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM accounts WHERE username=?`, username).Scan(&n)
		if n == 0 {
			break
		}
		username = fmt.Sprintf("default-%d-%d", p.Port, i)
	}
	password, err := sealSecret(randomHex(6))
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	_, err = s.db.Exec(`INSERT INTO accounts
		(id, username, password, port_id, upstream_id, enabled, remark, is_default, created_at, updated_at)
		VALUES(?,?,?,?,?,1,'',1,?,?)`,
		randomHex(4), username, password, p.ID, upstreamID, now, now)
	return err
}

// GetDefaultAccount returns the port's default account (is_default=1), if any.
func (s *Store) GetDefaultAccount(portID string) *Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var a Account
	var enabled, def int
	err := s.db.QueryRow(`SELECT id, username, password, port_id, upstream_id, enabled, remark, is_default
		FROM accounts WHERE port_id=? AND is_default=1`, portID).
		Scan(&a.ID, &a.Username, &a.Password, &a.PortID, &a.UpstreamID, &enabled, &a.Remark, &def)
	if err != nil {
		return nil
	}
	a.Enabled = enabled == 1
	a.IsDefault = def == 1
	c, err := resolveAccount(a)
	if err != nil {
		return nil
	}
	return &c
}

// DeletePort removes an inbound port together with its default account.
// It fails with a clear error while other accounts are still bound.
func (s *Store) DeletePort(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var others int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM accounts WHERE port_id=? AND is_default=0`, id).Scan(&others)
	if others > 0 {
		return fmt.Errorf("请先删除绑定账户 %d 个", others)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM accounts WHERE port_id=?`, id); err != nil {
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
	out := make([]*Account, 0, 4)
	rows, err := s.db.Query(`SELECT id, username, password, port_id, upstream_id, enabled, remark, is_default FROM accounts ORDER BY created_at`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var a Account
		var enabled, def int
		if err := rows.Scan(&a.ID, &a.Username, &a.Password, &a.PortID, &a.UpstreamID, &enabled, &a.Remark, &def); err != nil {
			continue
		}
		a.Enabled = enabled == 1
		a.IsDefault = def == 1
		c, err := resolveAccount(a)
		if err != nil {
			continue
		}
		out = append(out, &c)
	}
	return out
}

// resolveAccount decrypts the stored password and expands ${VAR} references
// in a copy of the account (decryption first, env expansion second).
func resolveAccount(a Account) (Account, error) {
	pass, err := openSecret(a.Password)
	if err != nil {
		return Account{}, fmt.Errorf("账户[%s]密码解密失败: %w", a.Username, err)
	}
	a.Password = pass
	a.Password, _ = expandEnv(a.Password)
	return a, nil
}

func (s *Store) getAccountLocked(id string) *Account {
	var a Account
	var enabled, def int
	err := s.db.QueryRow(`SELECT id, username, password, port_id, upstream_id, enabled, remark, is_default FROM accounts WHERE id=?`, id).
		Scan(&a.ID, &a.Username, &a.Password, &a.PortID, &a.UpstreamID, &enabled, &a.Remark, &def)
	if err != nil {
		return nil
	}
	a.Enabled = enabled == 1
	a.IsDefault = def == 1
	c, err := resolveAccount(a)
	if err != nil {
		return nil
	}
	return &c
}

// UpsertAccount validates and inserts (id == "") or updates an account.
// Usernames are globally unique. Every account must be bound to a port; the
// upstream binding is optional ("" = direct). Default accounts keep their port
// binding and is_default flag (they cannot be created through this path).
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
	if in.PortID == "" {
		return nil, errors.New("账户必须绑定端口（端口为必填项）")
	}
	if s.getPortLocked(in.PortID) == nil {
		return nil, errors.New("绑定的端口不存在")
	}
	if in.UpstreamID == "" {
		// "" = direct egress; this is the only direct mode (no global switch).
	} else if s.getUpstreamLocked(in.UpstreamID) == nil {
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
	stored, err := sealSecret(in.Password)
	if err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	if id == "" {
		in.ID = randomHex(4)
		if _, err := s.db.Exec(`INSERT INTO accounts
			(id, username, password, port_id, upstream_id, enabled, remark, is_default, created_at, updated_at)
			VALUES(?,?,?,?,?,?,?,0,?,?)`,
			in.ID, in.Username, stored, in.PortID, in.UpstreamID, boolInt(in.Enabled), in.Remark, now, now); err != nil {
			return nil, err
		}
	} else {
		existing := s.getAccountLocked(id)
		if existing == nil {
			return nil, ErrNotFound
		}
		if existing.IsDefault && in.PortID != existing.PortID {
			return nil, errors.New("默认账户不可更换绑定端口，请为其所在端口设置新账号")
		}
		in.IsDefault = existing.IsDefault
		res, err := s.db.Exec(`UPDATE accounts SET username=?, password=?, port_id=?, upstream_id=?, enabled=?, remark=?, updated_at=? WHERE id=?`,
			in.Username, stored, in.PortID, in.UpstreamID, boolInt(in.Enabled), in.Remark, now, id)
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

// DeleteAccount removes an account by id. Default accounts cannot be deleted;
// they disappear together with their port.
func (s *Store) DeleteAccount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var def int
	err := s.db.QueryRow(`SELECT is_default FROM accounts WHERE id=?`, id).Scan(&def)
	if err != nil {
		return ErrNotFound
	}
	if def == 1 {
		return errors.New("默认账户不可删除，删除端口连带删除")
	}
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

// MetricPoint is one hour bucket for the dashboard chart.
type MetricPoint struct {
	TS       int64 `json:"ts"`
	Conns    int64 `json:"conns"`
	BytesIn  int64 `json:"bytes_in"`
	BytesOut int64 `json:"bytes_out"`
}

// QueryHourlyMetrics aggregates connection counts and bytes per hour for the
// last N hours, returned oldest-first (zero-filled).
func (s *Store) QueryHourlyMetrics(hours int) []MetricPoint {
	if hours <= 0 || hours > 24*30 {
		hours = 24
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	const bucketSec = 3600
	nowBucket := time.Now().Unix() / bucketSec * bucketSec
	sinceMS := (nowBucket - int64(hours-1)*bucketSec) * 1000

	rows, err := s.db.Query(`SELECT (ts/3600000) AS b, COUNT(*), COALESCE(SUM(bytes_in),0), COALESCE(SUM(bytes_out),0)
		FROM logs WHERE ts >= ? GROUP BY b ORDER BY b`, sinceMS)
	if err != nil {
		return []MetricPoint{}
	}
	defer rows.Close()

	byBucket := map[int64]MetricPoint{}
	for rows.Next() {
		var b int64
		var p MetricPoint
		if err := rows.Scan(&b, &p.Conns, &p.BytesIn, &p.BytesOut); err != nil {
			continue
		}
		byBucket[b*bucketSec] = p
	}

	out := make([]MetricPoint, 0, hours)
	for b := nowBucket - int64(hours-1)*bucketSec; b <= nowBucket; b += bucketSec {
		p := MetricPoint{TS: b}
		if found, ok := byBucket[b]; ok {
			p.Conns = found.Conns
			p.BytesIn = found.BytesIn
			p.BytesOut = found.BytesOut
		}
		out = append(out, p)
	}
	return out
}

func logsWhere(f logx.Filter) (string, []any) {
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
	if len(where) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(where, " AND "), args
}

// CountLogs returns the total number of access logs matching the filter.
func (s *Store) CountLogs(f logx.Filter) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	where, args := logsWhere(f)
	var n int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM logs`+where, args...).Scan(&n)
	return n, err
}

// ClearLogs deletes every access log and returns how many were removed.
func (s *Store) ClearLogs() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`DELETE FROM logs`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// QueryLogs returns access logs newest-first.
func (s *Store) QueryLogs(f logx.Filter) ([]logx.Entry, error) {
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	where, args := logsWhere(f)
	var offset string
	if f.Offset > 0 {
		offset = fmt.Sprintf(" OFFSET %d", f.Offset)
	}
	q := `SELECT ts, level, port, account, client, proto, target, status, bytes_in, bytes_out, duration_ms, msg FROM logs` +
		where + ` ORDER BY id DESC LIMIT ?` + offset
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

// ListUpstreams queries the pool with optional keyword + limit/offset for
// the paged upstream list UI ("GET /api/upstreams").
func (s *Store) ListUpstreams(limit, offset int, q string) ([]*Upstream, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var where string
	var args []any
	if q = strings.TrimSpace(q); q != "" {
		esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
		pat := "%" + esc + "%"
		where = ` WHERE (name LIKE ? ESCAPE '\' OR host LIKE ? ESCAPE '\' OR username LIKE ? ESCAPE '\'
			OR type LIKE ? ESCAPE '\')`
		args = append(args, pat, pat, pat, pat)
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM upstreams`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	qAll := `SELECT ` + upstreamColumns + ` FROM upstreams` + where +
		` ORDER BY created_at LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := s.db.Query(qAll, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]*Upstream, 0, limit)
	for rows.Next() {
		if u, err := scanUpstream(rows); err == nil {
			items = append(items, &u)
		}
	}
	return items, total, nil
}

// BulkOpError annotates a per-entity failure inside a bulk action.
type BulkOpError struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// BulkAccounts applies one action ("delete"|"enable"|"disable") to accounts.
func (s *Store) BulkAccounts(ids []string, action string) (int64, []BulkOpError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var updated int64
	var problems []BulkOpError
	now := time.Now().Unix()
	for _, id := range ids {
		if action == "delete" {
			var def int
			err := s.db.QueryRow(`SELECT is_default FROM accounts WHERE id=?`, id).Scan(&def)
			if err != nil {
				problems = append(problems, BulkOpError{ID: id, Reason: "不存在"})
				continue
			}
			if def == 1 {
				problems = append(problems, BulkOpError{ID: id, Reason: "默认账户不可删除"})
				continue
			}
			res, err := s.db.Exec(`DELETE FROM accounts WHERE id=?`, id)
			if err != nil {
				problems = append(problems, BulkOpError{ID: id, Reason: err.Error()})
				continue
			}
			if n, _ := res.RowsAffected(); n == 0 {
				problems = append(problems, BulkOpError{ID: id, Reason: "不存在"})
				continue
			}
			updated++
			continue
		}
		enabled := action == "enable"
		res, err := s.db.Exec(`UPDATE accounts SET enabled=?, updated_at=? WHERE id=?`, boolInt(enabled), now, id)
		if err != nil {
			problems = append(problems, BulkOpError{ID: id, Reason: err.Error()})
			continue
		}
		if n, _ := res.RowsAffected(); n == 0 {
			problems = append(problems, BulkOpError{ID: id, Reason: "不存在"})
			continue
		}
		updated++
	}
	return updated, problems
}

// BulkPorts applies one action ("enable"|"disable"|"delete") to ports.
// Deletion follows the same rules as a single remove (blocked while other
// accounts are bound, conditional default-account cascade on success).
func (s *Store) BulkPorts(ids []string, action string) (int64, []BulkOpError) {
	var updated int64
	var problems []BulkOpError
	for _, id := range ids {
		if action == "delete" {
			if err := s.DeletePort(id); err != nil {
				problems = append(problems, BulkOpError{ID: id, Reason: err.Error()})
				continue
			}
			updated++
			continue
		}
		s.mu.Lock()
		res, err := s.db.Exec(`UPDATE ports SET enabled=?, updated_at=? WHERE id=?`, boolInt(action == "enable"), time.Now().Unix(), id)
		s.mu.Unlock()
		if err != nil {
			problems = append(problems, BulkOpError{ID: id, Reason: err.Error()})
			continue
		}
		if n, _ := res.RowsAffected(); n == 0 {
			problems = append(problems, BulkOpError{ID: id, Reason: "不存在"})
			continue
		}
		updated++
	}
	return updated, problems
}

// BulkDeleteUpstreams deletes upstreams, skipping ones referenced by accounts.
func (s *Store) BulkDeleteUpstreams(ids []string) (int64, []BulkOpError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var deleted int64
	var skipped []BulkOpError
	for _, id := range ids {
		var user string
		err := s.db.QueryRow(`SELECT username FROM accounts WHERE upstream_id=? LIMIT 1`, id).Scan(&user)
		if err == nil {
			skipped = append(skipped, BulkOpError{ID: id, Reason: "仍被账户 " + user + " 引用"})
			continue
		}
		res, err := s.db.Exec(`DELETE FROM upstreams WHERE id=?`, id)
		if err != nil {
			skipped = append(skipped, BulkOpError{ID: id, Reason: err.Error()})
			continue
		}
		if n, _ := res.RowsAffected(); n == 0 {
			skipped = append(skipped, BulkOpError{ID: id, Reason: "不存在"})
			continue
		}
		deleted++
	}
	return deleted, skipped
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
	case "ss", "shadowsocks", "vless", "vmess":
		// Full share-link forms are parsed by the importer and submitted as
		// fields; ParseUpstream only handles http/socks5 address strings.
		return "", "", "", "", fmt.Errorf("%s:// 节点请在面板选择对应类型并按字段填写（或使用订阅导入）", scheme)
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
