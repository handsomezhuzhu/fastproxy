// Package admin serves the FastProxy management panel (React SPA) and its
// JSON API.
package admin

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/handsomezhuzhu/fastproxy/config"
	"github.com/handsomezhuzhu/fastproxy/importer"
	"github.com/handsomezhuzhu/fastproxy/logx"
	"github.com/handsomezhuzhu/fastproxy/probe"
	"github.com/handsomezhuzhu/fastproxy/proxy"
	"github.com/handsomezhuzhu/fastproxy/subscription"
)

const (
	cookieName = "fp_admin"
	sessionTTL = 12 * time.Hour
)

//go:embed all:dist
var distFS embed.FS

// Server is the admin panel HTTP server.
type Server struct {
	store *config.Store
	mgr   *proxy.Manager
	log   *logx.Logger
	probe *probe.Runner
	ver   string
	web   fs.FS

	singMu  sync.Mutex
	singbox SingBoxReloader

	subsMu sync.Mutex
	subs   *subscription.Manager

	// proxyMu guards the trusted reverse-proxy list installed by main.
	proxyMu sync.RWMutex
	proxies []*net.IPNet

	// login throttles admin password attempts globally (single admin).
	login *loginLimiter
}

// SingBoxReloader is the embedded sing-box manager as seen by the panel.
// Reload rebuilds the box from the store after upstream changes; Dial is
// passed to proxy.SingBoxDialer. *singbox.Manager satisfies it.
type SingBoxReloader interface {
	Dial(up config.Upstream, addr string, timeout time.Duration) (net.Conn, error)
	Reload(ctx context.Context) error
	LastError() error
	Status() map[string]any
}

// SetSingBox installs the embedded sing-box manager. Upstream create/update/
// delete/import/bulk-delete handlers reload it and use Dial as the dialer
// for VMess and advanced VLESS nodes.
func (s *Server) SetSingBox(r SingBoxReloader) {
	s.singMu.Lock()
	defer s.singMu.Unlock()
	s.singbox = r
	if r != nil {
		proxy.SingBoxDialer = r.Dial
	}
}

func (s *Server) singboxReloader() SingBoxReloader {
	s.singMu.Lock()
	defer s.singMu.Unlock()
	return s.singbox
}

// SetSubscriptions installs the subscription refresher used by the API.
func (s *Server) SetSubscriptions(m *subscription.Manager) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	s.subs = m
}

func (s *Server) subscriptionManager() *subscription.Manager {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	return s.subs
}

// singboxError returns the embedded box's last startup/reload error (nil when
// it is healthy or not installed).
func (s *Server) singboxError() error {
	r := s.singboxReloader()
	if r == nil {
		return nil
	}
	return r.LastError()
}

// reloadSingBox applies upstream changes to the embedded box and returns a
// warning string for the response ("" on success). The store is already
// updated when this runs.
func (s *Server) reloadSingBox() string {
	r := s.singboxReloader()
	if r == nil {
		return ""
	}
	if err := r.Reload(context.Background()); err != nil {
		s.log.Warnf("内置 sing-box 重载失败: %v", err)
		return "内置 sing-box 重载失败: " + err.Error()
	}
	return ""
}

func NewServer(store *config.Store, mgr *proxy.Manager, logger *logx.Logger, prober *probe.Runner, version string) *Server {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		log.Fatalf("嵌入的前端资源损坏: %v", err)
	}
	policy, _ := EnvLoginPolicy()
	return &Server{store: store, mgr: mgr, log: logger, probe: prober, ver: version,
		web:   sub,
		login: newLoginLimiter(policy.MaxAttempts, policy.Lock, time.Now)}
}

// SetLoginPolicy overrides the login throttle (main uses it to log parsed env
// values; tests use it to shrink windows). The existing clock (if any) is kept
// so a test may install a fake clock before or after this call.
func (s *Server) SetLoginPolicy(maxAttempts int, lock time.Duration) {
	now := time.Now
	if s.login != nil {
		s.login.mu.Lock()
		now = s.login.now
		s.login.mu.Unlock()
	}
	s.login = newLoginLimiter(maxAttempts, lock, now)
}

// SetLoginClock installs a fake clock for the login limiter (tests).
func (s *Server) SetLoginClock(now func() time.Time) {
	s.login.mu.Lock()
	defer s.login.mu.Unlock()
	s.login.now = now
}

// Handler returns the panel's routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/session", s.handleSession)
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)
	mux.HandleFunc("GET /api/data", s.auth(s.handleData))
	mux.HandleFunc("GET /api/singbox/status", s.auth(s.handleSingBoxStatus))
	mux.HandleFunc("GET /api/stats", s.auth(s.handleStats))
	mux.HandleFunc("GET /api/logs", s.auth(s.handleLogs))
	mux.HandleFunc("DELETE /api/logs", s.auth(s.handleClearLogs))
	mux.HandleFunc("GET /api/metrics/hours", s.auth(s.handleMetrics))
	mux.HandleFunc("GET /api/settings", s.auth(s.handleGetSettings))
	mux.HandleFunc("PUT /api/settings", s.auth(s.handleUpdateSettings))

	mux.HandleFunc("POST /api/upstreams", s.auth(s.handleCreateUpstream))
	mux.HandleFunc("GET /api/upstreams", s.auth(s.handleListUpstreams))
	mux.HandleFunc("POST /api/upstreams/bulk", s.auth(s.handleBulkUpstreams))
	mux.HandleFunc("POST /api/upstreams/import", s.auth(s.handleImportUpstreams))
	mux.HandleFunc("POST /api/upstreams/parse", s.auth(s.handleParseUpstream))
	mux.HandleFunc("POST /api/upstreams/check", s.auth(s.handleCheckUpstreams))
	mux.HandleFunc("PUT /api/upstreams/{id}", s.auth(s.handleUpdateUpstream))
	mux.HandleFunc("DELETE /api/upstreams/{id}", s.auth(s.handleDeleteUpstream))

	mux.HandleFunc("GET /api/subscriptions", s.auth(s.handleListSubscriptions))
	mux.HandleFunc("POST /api/subscriptions", s.auth(s.handleCreateSubscription))
	mux.HandleFunc("PUT /api/subscriptions/{id}", s.auth(s.handleUpdateSubscription))
	mux.HandleFunc("DELETE /api/subscriptions/{id}", s.auth(s.handleDeleteSubscription))
	mux.HandleFunc("POST /api/subscriptions/{id}/refresh", s.auth(s.handleRefreshSubscription))

	mux.HandleFunc("POST /api/ports", s.auth(s.handleCreatePort))
	mux.HandleFunc("POST /api/ports/bulk", s.auth(s.handleBulkPorts))
	mux.HandleFunc("PUT /api/ports/{id}", s.auth(s.handleUpdatePort))
	mux.HandleFunc("PATCH /api/ports/{id}", s.auth(s.handlePatchPort))
	mux.HandleFunc("DELETE /api/ports/{id}", s.auth(s.handleDeletePort))

	mux.HandleFunc("POST /api/accounts", s.auth(s.handleCreateAccount))
	mux.HandleFunc("POST /api/accounts/bulk", s.auth(s.handleBulkAccounts))
	mux.HandleFunc("PUT /api/accounts/{id}", s.auth(s.handleUpdateAccount))
	mux.HandleFunc("PATCH /api/accounts/{id}", s.auth(s.handlePatchAccount))
	mux.HandleFunc("DELETE /api/accounts/{id}", s.auth(s.handleDeleteAccount))

	mux.HandleFunc("POST /api/password", s.auth(s.handlePassword))
	mux.HandleFunc("GET /", s.handleStatic)
	return mux
}

// --- static SPA ---

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || name == "." {
		name = "index.html"
	}
	data, err := fs.ReadFile(s.web, name)
	if err != nil {
		// SPA fallback for client-side routes.
		data, err = fs.ReadFile(s.web, "index.html")
		name = "index.html"
	}
	if err != nil {
		http.Error(w, "前端资源缺失，请先构建 web/", http.StatusInternalServerError)
		return
	}
	if name == "index.html" {
		w.Header().Set("Cache-Control", "no-store")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Write(data)
}

// --- sessions ---

func (s *Server) sign(exp int64) string {
	mac := hmac.New(sha256.New, []byte(s.store.SessionSecret()))
	mac.Write([]byte(strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Server) newCookie() *http.Cookie {
	exp := time.Now().Add(sessionTTL).Unix()
	return &http.Cookie{
		Name:     cookieName,
		Value:    fmt.Sprintf("%d.%s", exp, s.sign(exp)),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	}
}

// setSessionCookie attaches the session cookie, adding Secure when the client
// reached the panel over HTTPS (direct TLS or a trusted proxy's XFP=https).
func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, c *http.Cookie) {
	c.Secure = s.requestIsHTTPS(r)
	http.SetCookie(w, c)
}

func (s *Server) validSession(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	parts := strings.SplitN(c.Value, ".", 2)
	if len(parts) != 2 {
		return false
	}
	exp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	return hmac.Equal([]byte(s.sign(exp)), []byte(parts[1]))
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.validSession(r) {
			writeErr(w, http.StatusUnauthorized, "未登录或会话已过期")
			return
		}
		next(w, r)
	}
}

// clientIP resolves the requesting client for logging: the right-most
// untrusted X-Forwarded-For hop when the peer is a configured trusted proxy,
// otherwise the TCP peer address.
func (s *Server) clientIP(r *http.Request) string {
	return s.clientInfo(r).addr
}

// --- session / password endpoints ---

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"auth":    s.validSession(r),
		"version": s.ver,
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	// Refuse early while locked so even a malformed body cannot be used to
	// probe the password.
	if remain := s.login.lockedFor(); remain > 0 {
		s.writeLocked(w, remain)
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	hash := s.store.AdminPasswordHash()
	ok := hash == "" || VerifyPassword(hash, in.Password)
	if remain := s.login.attempt(ok); remain > 0 {
		if !ok {
			s.log.Warnf("管理面板登录失败（已达上限，锁定 %s）: %s", remain.Round(time.Second), s.clientIP(r))
		}
		s.writeLocked(w, remain)
		return
	}
	if !ok {
		s.log.Warnf("管理面板登录失败: %s", s.clientIP(r))
		writeErr(w, http.StatusUnauthorized, "密码错误")
		return
	}
	s.log.Infof("管理面板登录成功: %s", s.clientIP(r))

	s.setSessionCookie(w, r, s.newCookie())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// writeLocked answers a throttled login with 429, Retry-After (seconds, always
// at least 1) and a JSON body the panel can show verbatim.
func (s *Server) writeLocked(w http.ResponseWriter, remain time.Duration) {
	sec := int((remain + time.Second - 1) / time.Second)
	if sec < 1 {
		sec = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(sec))
	writeJSON(w, http.StatusTooManyRequests, map[string]any{
		"error":           fmt.Sprintf("登录失败次数过多，请 %d 秒后再试", sec),
		"retry_after_sec": sec,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if hash := s.store.AdminPasswordHash(); hash != "" && !VerifyPassword(hash, in.Old) {
		writeErr(w, http.StatusBadRequest, "原密码错误")
		return
	}
	if len(in.New) < 6 {
		writeErr(w, http.StatusBadRequest, "新密码至少 6 位")
		return
	}
	hash, err := HashPassword(in.New)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "密码处理失败")
		return
	}
	if err := s.store.SetAdminPasswordHash(hash); err != nil {
		writeErr(w, http.StatusInternalServerError, "密码保存失败")
		return
	}
	// Rotate the session secret so old sessions die, then re-login the caller.
	if err := s.store.RotateSessionSecret(); err != nil {
		writeErr(w, http.StatusInternalServerError, "会话重置失败")
		return
	}
	s.setSessionCookie(w, r, s.newCookie())
	s.log.Infof("管理密码已修改")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- read endpoints ---

// handleSingBoxStatus reports the embedded sing-box state, registered upstreams
// and the recent manager events. The panel uses it for the dashboard card.
func (s *Server) handleSingBoxStatus(w http.ResponseWriter, r *http.Request) {
	if box := s.singboxReloader(); box != nil {
		writeJSON(w, http.StatusOK, box.Status())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"running": false, "started_at": 0, "reload_count": 0,
		"last_reload_at": 0, "last_error": "",
		"upstreams": []any{}, "logs": []string{},
	})
}

func (s *Server) handleData(w http.ResponseWriter, r *http.Request) {
	ports, accounts := s.mgr.Status()
	serverIP, checkedAt := s.probe.ServerIP()
	var checkedUnix int64
	if !checkedAt.IsZero() {
		checkedUnix = checkedAt.Unix()
	}
	probeInfo := map[string]any{"server_ip": serverIP, "last_check_at": checkedUnix}
	if err := s.singboxError(); err != nil {
		probeInfo["singbox_error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":    s.ver,
		"settings":   s.store.Settings(),
		"upstreams":  s.store.Upstreams(),
		"ports":      s.store.Ports(),
		"accounts":   s.store.Accounts(),
		"status":     map[string]any{"ports": ports, "accounts": accounts},
		"probe":      probeInfo,
		"port_range": s.store.PortRange(),
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	ports, accounts := s.mgr.Status()
	writeJSON(w, http.StatusOK, map[string]any{
		"status": map[string]any{"ports": ports, "accounts": accounts},
	})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := logx.Filter{
		Account: q.Get("account"),
		Level:   q.Get("level"),
	}
	if v := q.Get("limit"); v != "" {
		f.Limit, _ = strconv.Atoi(v)
	}
	if v := q.Get("offset"); v != "" {
		f.Offset, _ = strconv.Atoi(v)
	}
	if v := q.Get("port"); v != "" {
		f.Port, _ = strconv.Atoi(v)
	}
	total, _ := s.store.CountLogs(logx.Filter{Port: f.Port, Account: f.Account, Level: f.Level})
	writeJSON(w, http.StatusOK, map[string]any{"entries": s.log.Recent(f), "total": total})
}

// handleClearLogs wipes all access logs after confirmation.
func (s *Server) handleClearLogs(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.ClearLogs()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.log.Infof("清空日志: 删除 %d 条", n)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": n})
}

// handleListUpstreams is the paged upstream listing for the upstreams page.
func (s *Server) handleListUpstreams(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit <= 0 {
		limit = 50
	}
	items, total, err := s.store.ListUpstreams(limit, offset, q.Get("q"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "limit": limit, "offset": offset,
	})
}

// handleMetrics returns 24 hourly buckets of connections and traffic for the
// dashboard chart.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	type point struct {
		T        string `json:"t"`
		Conns    int64  `json:"conns"`
		BytesIn  int64  `json:"bytes_in"`
		BytesOut int64  `json:"bytes_out"`
	}
	raw := s.store.QueryHourlyMetrics(24)
	points := make([]point, 0, 24)
	for _, p := range raw {
		points = append(points, point{
			T:        time.Unix(p.TS, 0).Format("15:04"),
			Conns:    p.Conns,
			BytesIn:  p.BytesIn,
			BytesOut: p.BytesOut,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"points": points})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"settings": s.store.Settings()})
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var in config.Settings
	if !decodeJSON(w, r, &in) {
		return
	}
	saved, err := s.store.UpdateSettings(in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.log.SetLevel(logx.ParseLevel(saved.LogLevel))
	s.log.SetBuffer(saved.LogBuffer)
	s.log.SetAccess(saved.AccessLog)
	s.log.Infof("系统设置已更新")
	writeJSON(w, http.StatusOK, map[string]any{"settings": saved})
}

// --- upstreams ---

// upstreamInput is the panel payload for create/update. Two shapes are
// accepted:
//
//   - Address form (http/socks5): {name, type?, upstream:"http://user:pass@h:p"}
//   - Field form (all types):    {name, type, host:"h:p", username, password,
//     method, uuid, sni} plus the vless/vmess fields
//     (security, pbk, sid, flow, fingerprint, transport, path, host_header,
//     service_name, vmess_security). TLS is expressed only through security
//     ("none" | "tls" | "reality"); a legacy "tls" bool in the body is ignored.
//
// When "upstream" carries an ss:// vmess:// or vless:// share link it is parsed
// by the importer and the resulting fields win over the explicit ones.
type upstreamInput struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	Upstream      string `json:"upstream"`
	Host          string `json:"host"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	Method        string `json:"method"`
	UUID          string `json:"uuid"`
	SNI           string `json:"sni"`
	Security      string `json:"security"`
	PBK           string `json:"pbk"`
	SID           string `json:"sid"`
	Flow          string `json:"flow"`
	Fingerprint   string `json:"fingerprint"`
	Transport     string `json:"transport"`
	Path          string `json:"path"`
	HostHeader    string `json:"host_header"`
	ServiceName   string `json:"service_name"`
	VMessSecurity string `json:"vmess_security"`
	Remark        string `json:"remark"`
}

func (in *upstreamInput) toEntity() (*config.Upstream, error) {
	u := &config.Upstream{
		Name: in.Name, Type: in.Type, Host: in.Host,
		Username: in.Username, Password: in.Password,
		Method: in.Method, UUID: in.UUID, SNI: in.SNI,
		Remark:   in.Remark,
		Security: in.Security, PBK: in.PBK, SID: in.SID, Flow: in.Flow,
		Fingerprint: in.Fingerprint, Transport: in.Transport, Path: in.Path,
		HostHeader: in.HostHeader, ServiceName: in.ServiceName,
		VMessSecurity: in.VMessSecurity,
	}
	raw := strings.TrimSpace(in.Upstream)
	if raw == "" {
		if strings.TrimSpace(u.Host) == "" {
			return nil, errors.New("上游地址不能为空")
		}
		return u, nil
	}
	switch {
	case strings.HasPrefix(raw, "ss://"), strings.HasPrefix(raw, "shadowsocks://"),
		strings.HasPrefix(raw, "vmess://"), strings.HasPrefix(raw, "vless://"):
		// Share link: parse it into fields with the subscription importer.
		res := importer.Parse(raw)
		if len(res.Proxies) != 1 {
			msg := "无法解析节点链接"
			if len(res.Problems) > 0 {
				msg = res.Problems[0]
			}
			return nil, errors.New(msg)
		}
		p := res.Proxies[0]
		u.Type, u.Host = p.Type, p.Host
		u.Username, u.Password = p.User, p.Pass
		u.Method, u.UUID, u.SNI = p.Method, p.UUID, p.SNI
		u.Security, u.PBK, u.SID, u.Flow = p.Security, p.PBK, p.SID, p.Flow
		u.Fingerprint, u.Transport, u.Path = p.Fingerprint, p.Transport, p.Path
		u.HostHeader, u.ServiceName = p.HostHeader, p.ServiceName
		u.VMessSecurity = p.VMessSecurity
		if strings.TrimSpace(u.Name) == "" {
			u.Name = p.Name
		}
		return u, nil
	default:
		// Address form: http://user:pass@host:port or socks5://host:port.
		typ, host, user, pass, err := config.ParseUpstream(raw)
		if err != nil {
			return nil, err
		}
		u.Type, u.Host, u.Username, u.Password = typ, host, user, pass
		return u, nil
	}
}

func (s *Server) handleCreateUpstream(w http.ResponseWriter, r *http.Request) {
	var in upstreamInput
	if !decodeJSON(w, r, &in) {
		return
	}
	u, err := in.toEntity()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	saved, err := s.store.UpsertUpstream("", u)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.reloadRespond(w, saved)
}

func (s *Server) handleUpdateUpstream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in upstreamInput
	if !decodeJSON(w, r, &in) {
		return
	}
	u, err := in.toEntity()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	saved, err := s.store.UpsertUpstream(id, u)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// Upstream edits apply per-connection; restart listeners only when port
	// definitions changed, but run Sync for safety (no-op otherwise). The
	// embedded sing-box is rebuilt so the new outbound takes effect.
	s.reloadRespond(w, saved)
}

func (s *Server) handleDeleteUpstream(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteUpstream(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.reloadRespond(w, nil)
}

// handleImportUpstreams accepts {url} or {text} subscription content.
func (s *Server) handleImportUpstreams(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL  string `json:"url"`
		Text string `json:"text"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	content := in.Text
	if strings.TrimSpace(in.URL) != "" {
		var err error
		content, err = importer.Fetch(in.URL)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if strings.TrimSpace(content) == "" {
		writeErr(w, http.StatusBadRequest, "请填写订阅链接或节点文本")
		return
	}
	res := importer.Parse(content)
	candidates := make([]*config.Upstream, 0, len(res.Proxies))
	for _, p := range res.Proxies {
		candidates = append(candidates, &config.Upstream{
			Name: p.Name, Type: p.Type, Host: p.Host,
			Username: p.User, Password: p.Pass,
			Method: p.Method, UUID: p.UUID, SNI: p.SNI,
			Security: p.Security, PBK: p.PBK, SID: p.SID, Flow: p.Flow,
			Fingerprint: p.Fingerprint, Transport: p.Transport, Path: p.Path,
			HostHeader: p.HostHeader, ServiceName: p.ServiceName,
			VMessSecurity: p.VMessSecurity,
		})
	}
	added, duplicates, rejected := s.store.ImportUpstreams(candidates)
	if duplicates == nil {
		duplicates = []string{}
	}
	problems := append(append([]string{}, res.Problems...), rejected...)
	if problems == nil {
		problems = []string{}
	}
	s.log.Infof("上游导入: 识别 %d, 新增 %d, 重复 %d, 跳过 %d",
		len(res.Proxies), added, len(duplicates), len(problems))
	resp := map[string]any{
		"added":      added,
		"total":      len(res.Proxies),
		"duplicates": duplicates,
		"problems":   problems,
	}
	if s.reloadSingBox() != "" {
		resp["warning"] = "内置 sing-box 重载失败，新的 VMess/REALITY 节点暂不可用"
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleParseUpstream pre-parses a single share link for the panel's
// "paste and auto-fill the form" flow. It never touches the store.
//
//	POST /api/upstreams/parse {"text":"vless://..."}
//	200 {"proxy":{name,type,host,uuid,password,method,sni,security,pbk,
//	              sid,flow,fingerprint,transport,path,host_header,service_name,
//	              vmess_security}}
//	400 {"error":"中文原因"}
//
// TLS is conveyed by security only (none|tls|reality); no legacy tls flag is
// returned.
func (s *Server) handleParseUpstream(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string `json:"text"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		writeErr(w, http.StatusBadRequest, "请粘贴节点分享链接")
		return
	}
	if !strings.Contains(text, "://") && !strings.HasPrefix(text, "vmess://") {
		writeErr(w, http.StatusBadRequest, "不是有效的分享链接（应以 vless://、vmess:// 或 ss:// 开头）")
		return
	}
	res := importer.Parse(text)
	if len(res.Proxies) != 1 {
		msg := "无法解析节点链接"
		if len(res.Problems) > 0 {
			msg = res.Problems[0]
		} else if len(res.Proxies) > 1 {
			msg = "一次只能粘贴一条节点链接"
		}
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	p := res.Proxies[0]
	writeJSON(w, http.StatusOK, map[string]any{"proxy": map[string]any{
		"name":           p.Name,
		"type":           p.Type,
		"host":           p.Host,
		"username":       p.User,
		"uuid":           p.UUID,
		"password":       p.Pass,
		"method":         p.Method,
		"sni":            p.SNI,
		"security":       p.Security,
		"pbk":            p.PBK,
		"sid":            p.SID,
		"flow":           p.Flow,
		"fingerprint":    p.Fingerprint,
		"transport":      p.Transport,
		"path":           p.Path,
		"host_header":    p.HostHeader,
		"service_name":   p.ServiceName,
		"vmess_security": p.VMessSecurity,
	}})
}

// checkResult is one upstream probe outcome returned by
// POST /api/upstreams/check.
type checkResult struct {
	ID        string `json:"id"`
	Found     bool   `json:"-"`
	OK        bool   `json:"ok"`
	LatencyMS int64  `json:"latency_ms"`
	ExitIP    string `json:"exit_ip"`
	Error     string `json:"error"`
}

// checkConcurrency bounds parallel probes triggered by one API call (the
// same ceiling as the settings' probe_concurrency).
const checkConcurrency = 8

// handleCheckUpstreams probes the requested upstreams (empty/omitted ids =
// every upstream) and stores each result. This is the only probe entry point:
// the panel calls it per row ({"ids":["<id>"]}) for live refresh and once
// without ids (or with all ids) for "check all".
//
//	POST /api/upstreams/check {"ids":["a","b"]}
//	200 {"results":[{id,ok,latency_ms,exit_ip,error}],"ok":N,"failed":N}
func (s *Server) handleCheckUpstreams(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []string `json:"ids"`
	}
	// An empty body and "{}" both mean "all upstreams"; a malformed JSON body
	// is still rejected.
	if ct := r.Header.Get("Content-Type"); strings.HasPrefix(ct, "application/json") && bodyPresent(r) {
		if !decodeJSON(w, r, &in) {
			return
		}
	}
	ids := normalizeIDs(in.IDs)
	if len(ids) == 0 {
		for _, u := range s.store.Upstreams() {
			ids = append(ids, u.ID)
		}
	}

	out := make([]checkResult, len(ids))
	var ok, failed atomic.Int64
	var wg sync.WaitGroup
	sem := make(chan struct{}, checkConcurrency)
	for i, id := range ids {
		i, id := i, id
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				// A panicking prober must not take the whole panel down; the
				// slot reports a generic failure instead.
				if rec := recover(); rec != nil {
					out[i] = checkResult{ID: id, Found: true, Error: fmt.Sprintf("探测异常: %v", rec)}
					failed.Add(1)
				}
			}()
			res, found := s.probe.CheckOne(id)
			out[i] = checkResult{ID: id, Found: found, OK: res.OK, LatencyMS: res.LatencyMS,
				ExitIP: res.ExitIP, Error: res.Error}
			if !found {
				out[i].Error = "不存在"
				failed.Add(1)
			} else if res.OK {
				ok.Add(1)
			} else {
				failed.Add(1)
			}
		}()
	}
	wg.Wait()

	for _, res := range out {
		if res.Found && res.OK {
			s.log.Infof("上游探测 %s: 正常, 延迟 %dms, 出口 IP %s", res.ID, res.LatencyMS, res.ExitIP)
		} else {
			s.log.Warnf("上游探测 %s: 失败 %s", res.ID, res.Error)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"results": out, "ok": int(ok.Load()), "failed": int(failed.Load()),
	})
}

// bodyPresent reports whether the request carries a body at all.
func bodyPresent(r *http.Request) bool {
	if r.ContentLength != 0 {
		return true
	}
	return r.Body != nil && r.Body != http.NoBody && r.ContentLength < 0
}

// normalizeIDs trims, de-duplicates and drops empty ids while keeping input
// order.
func normalizeIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// handleBulkUpstreams applies a bulk action to upstreams (delete only; probes
// go through POST /api/upstreams/check).
func (s *Server) handleBulkUpstreams(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs    []string `json:"ids"`
		Action string   `json:"action"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	switch in.Action {
	case "delete":
		deleted, skipped := s.store.BulkDeleteUpstreams(in.IDs)
		var boxWarning string
		if deleted > 0 {
			boxWarning = s.reloadSingBox()
		}
		s.syncRespondWith(w, s.mgr.Sync(), boxWarning, map[string]any{"deleted": deleted, "skipped": skipped})
	default:
		writeErr(w, http.StatusBadRequest, "action 仅支持 delete（检测请使用 POST /api/upstreams/check）")
	}
}

// handleBulkAccounts applies bulk enable/disable/delete to accounts.
func (s *Server) handleBulkAccounts(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs    []string `json:"ids"`
		Action string   `json:"action"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	switch in.Action {
	case "delete", "enable", "disable":
		updated, problems := s.store.BulkAccounts(in.IDs, in.Action)
		s.syncRespond(w, map[string]any{"updated": updated, "errors": problems})
	default:
		writeErr(w, http.StatusBadRequest, "action 仅支持 delete/enable/disable")
	}
}

// handleBulkPorts applies bulk enable/disable/delete to ports.
func (s *Server) handleBulkPorts(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs    []string `json:"ids"`
		Action string   `json:"action"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	switch in.Action {
	case "delete", "enable", "disable":
		updated, problems := s.store.BulkPorts(in.IDs, in.Action)
		s.syncRespond(w, map[string]any{"updated": updated, "errors": problems})
	default:
		writeErr(w, http.StatusBadRequest, "action 仅支持 delete/enable/disable")
	}
}

// --- ports ---

type portInput struct {
	Port   int    `json:"port"`
	Remark string `json:"remark"`
}

func (in *portInput) toEntity() *config.Port {
	return &config.Port{Port: in.Port, Remark: in.Remark, Enabled: true}
}

func (s *Server) handleCreatePort(w http.ResponseWriter, r *http.Request) {
	var in portInput
	if !decodeJSON(w, r, &in) {
		return
	}
	saved, err := s.store.UpsertPort("", in.toEntity())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var warning string
	if serr := s.mgr.Sync(); serr != nil {
		warning = serr.Error()
	}
	resp := map[string]any{"ok": true, "saved": map[string]any{
		"port":            saved,
		"default_account": s.store.GetDefaultAccount(saved.ID),
	}}
	if warning != "" {
		resp["warning"] = warning
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleUpdatePort(w http.ResponseWriter, r *http.Request) {
	var in portInput
	if !decodeJSON(w, r, &in) {
		return
	}
	saved, err := s.store.UpsertPort(r.PathValue("id"), in.toEntity())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.syncRespond(w, saved)
}

func (s *Server) handlePatchPort(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	saved, err := s.store.SetPortEnabled(r.PathValue("id"), in.Enabled)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	s.syncRespond(w, saved)
}

func (s *Server) handleDeletePort(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeletePort(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	s.syncRespond(w, nil)
}

// --- accounts ---

type accountInput struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	PortID     string `json:"port_id"`
	UpstreamID string `json:"upstream_id"`
	Remark     string `json:"remark"`
}

func (in *accountInput) toEntity() *config.Account {
	return &config.Account{
		Username: in.Username, Password: in.Password, PortID: in.PortID,
		UpstreamID: in.UpstreamID, Remark: in.Remark, Enabled: true,
	}
}

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var in accountInput
	if !decodeJSON(w, r, &in) {
		return
	}
	saved, err := s.store.UpsertAccount("", in.toEntity())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.syncRespond(w, saved)
}

func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	var in accountInput
	if !decodeJSON(w, r, &in) {
		return
	}
	saved, err := s.store.UpsertAccount(r.PathValue("id"), in.toEntity())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.syncRespond(w, saved)
}

func (s *Server) handlePatchAccount(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	saved, err := s.store.SetAccountEnabled(r.PathValue("id"), in.Enabled)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	s.syncRespond(w, saved)
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	err := s.store.DeleteAccount(r.PathValue("id"))
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			writeErr(w, http.StatusNotFound, err.Error())
		} else {
			writeErr(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	s.syncRespond(w, nil)
}

// --- plumbing ---

// syncRespond applies changes to running listeners and answers. Only port
// definition changes restart listeners; accounts/upstreams apply hot.
// reloadBox rebuilds the embedded sing-box when upstreams changed.
func (s *Server) syncRespond(w http.ResponseWriter, saved any) {
	s.syncRespondWith(w, s.mgr.Sync(), "", saved)
}

// reloadRespond answers after an upstream mutation, reloading the embedded
// sing-box so the new outbound/inbound pair takes effect.
func (s *Server) reloadRespond(w http.ResponseWriter, saved any) {
	s.syncRespondWith(w, s.mgr.Sync(), s.reloadSingBox(), saved)
}

func (s *Server) syncRespondWith(w http.ResponseWriter, syncErr error, boxWarning string, saved any) {
	var warning string
	if syncErr != nil {
		warning = syncErr.Error()
	}
	if boxWarning != "" {
		if warning != "" {
			warning += "；"
		}
		warning += boxWarning
	}
	resp := map[string]any{"ok": true}
	if saved != nil {
		resp["saved"] = saved
	}
	if warning != "" {
		resp["warning"] = warning
	}
	writeJSON(w, http.StatusOK, resp)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		writeErr(w, http.StatusUnsupportedMediaType, "Content-Type 必须为 application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体格式错误")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

// --- subscriptions ---

type subscriptionInput struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Enabled     *bool  `json:"enabled"`
	IntervalSec int    `json:"interval_sec"`
}

func (s *Server) handleListSubscriptions(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListSubscriptions()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []config.SubscriptionInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) handleCreateSubscription(w http.ResponseWriter, r *http.Request) {
	var in subscriptionInput
	if !decodeJSON(w, r, &in) {
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	saved, err := s.store.UpsertSubscription("", &config.Subscription{
		Name: in.Name, URL: in.URL, Enabled: enabled, IntervalSec: in.IntervalSec,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if mgr := s.subscriptionManager(); mgr != nil && saved.Enabled {
		go mgr.Refresh(context.Background(), saved.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": saved})
}

func (s *Server) handleUpdateSubscription(w http.ResponseWriter, r *http.Request) {
	var in subscriptionInput
	if !decodeJSON(w, r, &in) {
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	saved, err := s.store.UpsertSubscription(r.PathValue("id"), &config.Subscription{
		Name: in.Name, URL: in.URL, Enabled: enabled, IntervalSec: in.IntervalSec,
	})
	if err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, config.ErrNotFound) {
			code = http.StatusNotFound
		}
		writeErr(w, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": saved})
}

func (s *Server) handleDeleteSubscription(w http.ResponseWriter, r *http.Request) {
	deleteNodes := true
	if v := strings.TrimSpace(r.URL.Query().Get("delete_nodes")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			deleteNodes = b
		}
	}
	if err := s.store.DeleteSubscription(r.PathValue("id"), deleteNodes); err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, config.ErrNotFound) {
			code = http.StatusNotFound
		}
		writeErr(w, code, err.Error())
		return
	}
	_ = s.reloadSingBox()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleRefreshSubscription(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	mgr := s.subscriptionManager()
	if mgr == nil {
		writeErr(w, http.StatusServiceUnavailable, "订阅刷新器未启动")
		return
	}
	result, err := mgr.Refresh(r.Context(), id)
	if err != nil && errors.Is(err, config.ErrNotFound) {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	saved, _ := s.store.GetSubscription(id)
	writeJSON(w, http.StatusOK, map[string]any{"result": result, "saved": saved})
}
