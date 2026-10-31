// Package config persists FastProxy entities (upstreams, ports, accounts)
// and admin settings in a single JSON file.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	DefaultUpstream string `json:"default_upstream"` // upstream id, "" = 直连(不走上游)
	Enabled         bool   `json:"enabled"`
	Remark          string `json:"remark,omitempty"`
}

// Account is one inbound credential. Username is globally unique; the account
// may be bound to one port or valid on all ports, and may override the port's
// default upstream to route to a different destination.
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
	AccessLog           bool   `json:"access_log"`            // 记录每次连接
	LogLevel            string `json:"log_level"`             // debug|info|warn|error
	LogBuffer           int    `json:"log_buffer"`            // 内存日志条数
	AllowDirect         bool   `json:"allow_direct"`          // 允许端口不绑定上游（直连）
	DialTimeoutSec      int    `json:"dial_timeout_sec"`      // 连接目标/上游超时
	HandshakeTimeoutSec int    `json:"handshake_timeout_sec"` // 客户端握手超时
}

// DefaultSettings returns the built-in defaults.
func DefaultSettings() Settings {
	return Settings{
		AccessLog:           true,
		LogLevel:            "info",
		LogBuffer:           1000,
		AllowDirect:         true,
		DialTimeoutSec:      15,
		HandshakeTimeoutSec: 60,
	}
}

// Config is the on-disk document.
type Config struct {
	AdminPasswordHash string      `json:"admin_password_hash,omitempty"`
	SessionSecret     string      `json:"session_secret,omitempty"`
	Settings          *Settings   `json:"settings,omitempty"`
	Upstreams         []*Upstream `json:"upstreams"`
	Ports             []*Port     `json:"ports"`
	Accounts          []*Account  `json:"accounts"`
}

// Store is a thread-safe JSON file store.
type Store struct {
	mu        sync.RWMutex
	path      string
	adminPort int
	cfg       Config
}

// Open loads (or creates) the config file. adminPort is rejected as a port id.
func Open(path string, adminPort int) (*Store, error) {
	s := &Store{path: path, adminPort: adminPort}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		d := DefaultSettings()
		s.cfg = Config{SessionSecret: randomHex(32), Settings: &d, Upstreams: []*Upstream{}, Ports: []*Port{}, Accounts: []*Account{}}
		if err := s.saveLocked(); err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(data, &s.cfg); err != nil {
			return nil, fmt.Errorf("无法解析配置文件 %s: %w", path, err)
		}
		changed := false
		if s.cfg.SessionSecret == "" {
			s.cfg.SessionSecret = randomHex(32)
			changed = true
		}
		if s.cfg.Upstreams == nil {
			s.cfg.Upstreams = []*Upstream{}
			changed = true
		}
		if s.cfg.Ports == nil {
			s.cfg.Ports = []*Port{}
			changed = true
		}
		if s.cfg.Accounts == nil {
			s.cfg.Accounts = []*Account{}
			changed = true
		}
		if s.cfg.Settings == nil {
			d := DefaultSettings()
			s.cfg.Settings = &d
			changed = true
		}
		if changed {
			_ = s.saveLocked()
		}
	}
	// Fail fast on unresolved secret references so operators notice a missing
	// env var immediately instead of facing mysterious auth failures later.
	if missing := s.checkSecretRefsLocked(); len(missing) > 0 {
		return nil, missingRefError("配置文件中的敏感字段", missing)
	}
	return s, nil
}

// Path returns the config file path.
func (s *Store) Path() string { return s.path }

// --- upstreams ---

// Upstreams returns a deep copy of the upstream pool. Sensitive fields
// (username/password) have their ${VAR} references expanded from the
// environment; the on-disk file keeps the raw references.
func (s *Store) Upstreams() []*Upstream {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Upstream, 0, len(s.cfg.Upstreams))
	for _, u := range s.cfg.Upstreams {
		c := resolveUpstream(*u)
		out = append(out, &c)
	}
	return out
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

	old := s.cfg.Upstreams
	if id == "" {
		in.ID = randomHex(4)
		s.cfg.Upstreams = append(append([]*Upstream{}, old...), in)
	} else {
		found := false
		s.cfg.Upstreams = make([]*Upstream, 0, len(old))
		for _, u := range old {
			if u.ID == id {
				c := *in
				c.ID = id
				s.cfg.Upstreams = append(s.cfg.Upstreams, &c)
				found = true
				continue
			}
			s.cfg.Upstreams = append(s.cfg.Upstreams, u)
		}
		if !found {
			s.cfg.Upstreams = old
			return nil, ErrNotFound
		}
	}
	if err := s.saveLocked(); err != nil {
		s.cfg.Upstreams = old
		return nil, err
	}
	return s.getUpstreamLocked(in.ID), nil
}

// DeleteUpstream removes an upstream unless it is still referenced.
func (s *Store) DeleteUpstream(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.cfg.Ports {
		if p.DefaultUpstream == id {
			return fmt.Errorf("上游仍被端口 %d 引用，请先修改端口绑定", p.Port)
		}
	}
	for _, a := range s.cfg.Accounts {
		if a.UpstreamID == id {
			return fmt.Errorf("上游仍被账户 %s 引用，请先修改账户绑定", a.Username)
		}
	}
	old := s.cfg.Upstreams
	next := make([]*Upstream, 0, len(old))
	found := false
	for _, u := range old {
		if u.ID == id {
			found = true
			continue
		}
		next = append(next, u)
	}
	if !found {
		return ErrNotFound
	}
	s.cfg.Upstreams = next
	if err := s.saveLocked(); err != nil {
		s.cfg.Upstreams = old
		return err
	}
	return nil
}

func (s *Store) getUpstreamLocked(id string) *Upstream {
	for _, u := range s.cfg.Upstreams {
		if u.ID == id {
			c := resolveUpstream(*u)
			return &c
		}
	}
	return nil
}

// --- ports ---

// Ports returns a deep copy of all inbound ports.
func (s *Store) Ports() []*Port {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Port, 0, len(s.cfg.Ports))
	for _, p := range s.cfg.Ports {
		c := *p
		out = append(out, &c)
	}
	return out
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
	if in.DefaultUpstream == "" && !s.settingsLocked().AllowDirect {
		return nil, errors.New("当前设置不允许直连端口，请先绑定默认上游")
	}
	if in.DefaultUpstream != "" && s.getUpstreamLocked(in.DefaultUpstream) == nil {
		return nil, errors.New("默认上游不存在")
	}

	old := s.cfg.Ports
	if id == "" {
		for _, p := range s.cfg.Ports {
			if p.Port == in.Port {
				return nil, fmt.Errorf("端口 %d 已被其他线路使用", in.Port)
			}
		}
		in.ID = randomHex(4)
		s.cfg.Ports = append(append([]*Port{}, old...), in)
	} else {
		found := false
		s.cfg.Ports = make([]*Port, 0, len(old))
		for _, p := range old {
			if p.ID == id {
				c := *in
				c.ID = id
				s.cfg.Ports = append(s.cfg.Ports, &c)
				found = true
				continue
			}
			if p.Port == in.Port {
				return nil, fmt.Errorf("端口 %d 已被其他线路使用", in.Port)
			}
			s.cfg.Ports = append(s.cfg.Ports, p)
		}
		if !found {
			s.cfg.Ports = old
			return nil, ErrNotFound
		}
	}
	if err := s.saveLocked(); err != nil {
		s.cfg.Ports = old
		return nil, err
	}
	return s.getPortLocked(in.ID), nil
}

// DeletePort removes an inbound port; accounts bound to it are unbound to "".
func (s *Store) DeletePort(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldPorts := s.cfg.Ports
	next := make([]*Port, 0, len(oldPorts))
	found := false
	for _, p := range oldPorts {
		if p.ID == id {
			found = true
			continue
		}
		next = append(next, p)
	}
	if !found {
		return ErrNotFound
	}

	oldAccounts := s.cfg.Accounts
	s.cfg.Ports = next
	for _, a := range s.cfg.Accounts {
		if a.PortID == id {
			a.PortID = ""
		}
	}
	if err := s.saveLocked(); err != nil {
		s.cfg.Ports = oldPorts
		s.cfg.Accounts = oldAccounts
		return err
	}
	return nil
}

// ImportUpstreams appends candidates, skipping exact duplicates
// (same type+host+user+password). Returns the number added and their names.
func (s *Store) ImportUpstreams(candidates []*Upstream) (int, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := func(u *Upstream) string {
		return u.Type + "|" + u.Host + "|" + u.Username + "|" + u.Password
	}
	seen := map[string]bool{}
	for _, u := range s.cfg.Upstreams {
		seen[key(u)] = true
	}
	old := s.cfg.Upstreams
	next := append([]*Upstream{}, old...)
	added := 0
	var duplicates []string
	for _, u := range candidates {
		k := key(u)
		if seen[k] {
			duplicates = append(duplicates, u.Name+" ("+u.Host+")")
			continue
		}
		seen[k] = true
		u.ID = randomHex(4)
		next = append(next, u)
		added++
	}
	if added == 0 {
		return 0, duplicates
	}
	s.cfg.Upstreams = next
	if err := s.saveLocked(); err != nil {
		s.cfg.Upstreams = old
		return 0, duplicates
	}
	return added, duplicates
}

// SetPortEnabled toggles an inbound port.
func (s *Store) SetPortEnabled(id string, enabled bool) (*Port, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.cfg.Ports {
		if p.ID == id {
			old := *p
			p.Enabled = enabled
			if err := s.saveLocked(); err != nil {
				*p = old
				return nil, err
			}
			c := *p
			return &c, nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) getPortLocked(id string) *Port {
	for _, p := range s.cfg.Ports {
		if p.ID == id {
			c := *p
			return &c
		}
	}
	return nil
}

// --- accounts ---

// Accounts returns a deep copy of all accounts. Password ${VAR} references
// are expanded from the environment; the on-disk file keeps raw references.
func (s *Store) Accounts() []*Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Account, 0, len(s.cfg.Accounts))
	for _, a := range s.cfg.Accounts {
		c := resolveAccount(*a)
		out = append(out, &c)
	}
	return out
}

// resolveAccount expands secret references in a copy of the account.
func resolveAccount(a Account) Account {
	a.Password, _ = expandEnv(a.Password)
	return a
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

	old := s.cfg.Accounts
	if id == "" {
		for _, a := range s.cfg.Accounts {
			if a.Username == in.Username {
				return nil, fmt.Errorf("用户名 %s 已存在", in.Username)
			}
		}
		in.ID = randomHex(4)
		s.cfg.Accounts = append(append([]*Account{}, old...), in)
	} else {
		found := false
		s.cfg.Accounts = make([]*Account, 0, len(old))
		for _, a := range old {
			if a.ID == id {
				c := *in
				c.ID = id
				s.cfg.Accounts = append(s.cfg.Accounts, &c)
				found = true
				continue
			}
			if a.Username == in.Username {
				return nil, fmt.Errorf("用户名 %s 已存在", in.Username)
			}
			s.cfg.Accounts = append(s.cfg.Accounts, a)
		}
		if !found {
			s.cfg.Accounts = old
			return nil, ErrNotFound
		}
	}
	if err := s.saveLocked(); err != nil {
		s.cfg.Accounts = old
		return nil, err
	}
	return s.getAccountLocked(in.ID), nil
}

// DeleteAccount removes an account by id.
func (s *Store) DeleteAccount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.cfg.Accounts
	next := make([]*Account, 0, len(old))
	found := false
	for _, a := range old {
		if a.ID == id {
			found = true
			continue
		}
		next = append(next, a)
	}
	if !found {
		return ErrNotFound
	}
	s.cfg.Accounts = next
	if err := s.saveLocked(); err != nil {
		s.cfg.Accounts = old
		return err
	}
	return nil
}

// SetAccountEnabled toggles an account.
func (s *Store) SetAccountEnabled(id string, enabled bool) (*Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.cfg.Accounts {
		if a.ID == id {
			old := *a
			a.Enabled = enabled
			if err := s.saveLocked(); err != nil {
				*a = old
				return nil, err
			}
			c := *a
			return &c, nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) getAccountLocked(id string) *Account {
	for _, a := range s.cfg.Accounts {
		if a.ID == id {
			c := resolveAccount(*a)
			return &c
		}
	}
	return nil
}

// --- admin settings ---

// AdminPasswordHash returns the stored password hash ("" when unset).
func (s *Store) AdminPasswordHash() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.AdminPasswordHash
}

// SetAdminPasswordHash stores a new admin password hash.
func (s *Store) SetAdminPasswordHash(hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.cfg.AdminPasswordHash
	s.cfg.AdminPasswordHash = hash
	if err := s.saveLocked(); err != nil {
		s.cfg.AdminPasswordHash = old
		return err
	}
	return nil
}

// SessionSecret returns the HMAC secret for admin session cookies.
// The SESSION_SECRET environment variable, when set, overrides the stored one
// (the file never contains it in that case).
func (s *Store) SessionSecret() string {
	if v := os.Getenv("SESSION_SECRET"); v != "" {
		return v
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.SessionSecret
}

// RotateSessionSecret replaces the session secret, invalidating all sessions.
func (s *Store) RotateSessionSecret() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.cfg.SessionSecret
	s.cfg.SessionSecret = randomHex(32)
	if err := s.saveLocked(); err != nil {
		s.cfg.SessionSecret = old
		return err
	}
	return nil
}

// Settings returns the current global settings.
func (s *Store) Settings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settingsLocked()
}

func (s *Store) settingsLocked() Settings {
	if s.cfg.Settings == nil {
		return DefaultSettings()
	}
	return *s.cfg.Settings
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
	if in.DialTimeoutSec < 3 || in.DialTimeoutSec > 120 {
		return Settings{}, errors.New("连接超时需在 3-120 秒之间")
	}
	if in.HandshakeTimeoutSec < 10 || in.HandshakeTimeoutSec > 300 {
		return Settings{}, errors.New("握手超时需在 10-300 秒之间")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.cfg.Settings
	s.cfg.Settings = &in
	if err := s.saveLocked(); err != nil {
		s.cfg.Settings = old
		return Settings{}, err
	}
	return in, nil
}

// checkSecretRefsLocked verifies that all ${VAR} references in sensitive
// fields resolve in the current environment. Returns the missing var names.
func (s *Store) checkSecretRefsLocked() []string {
	var missing []string
	for _, u := range s.cfg.Upstreams {
		missing = append(missing, mustRef("上游["+u.Name+"].账号", u.Username)...)
		missing = append(missing, mustRef("上游["+u.Name+"].密码", u.Password)...)
	}
	for _, a := range s.cfg.Accounts {
		missing = append(missing, mustRef("账户["+a.Username+"].密码", a.Password)...)
	}
	return dedupeMissing(missing)
}

func (s *Store) saveLocked() error {
	data, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(s.path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
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
	// drop any path/query/fragment after the authority
	if k := strings.IndexAny(rest, "/?#"); k >= 0 {
		rest = rest[:k]
	}
	if j := strings.LastIndex(rest, "@"); j >= 0 { // password may contain '@'
		userinfo := rest[:j]
		rest = rest[j+1:]
		if c := strings.Index(userinfo, ":"); c >= 0 { // password may contain ':'
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
