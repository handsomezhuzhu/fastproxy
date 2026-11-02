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
	"time"

	"github.com/handsomezhuzhu/fastproxy/config"
	"github.com/handsomezhuzhu/fastproxy/importer"
	"github.com/handsomezhuzhu/fastproxy/logx"
	"github.com/handsomezhuzhu/fastproxy/probe"
	"github.com/handsomezhuzhu/fastproxy/proxy"
)

const (
	cookieName   = "fp_admin"
	sessionTTL   = 12 * time.Hour
	maxFails     = 10
	lockDuration = 10 * time.Minute
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

	mu    sync.Mutex
	fails map[string]*failState
}

// SingBoxReloader is the embedded sing-box manager as seen by the panel.
// Reload rebuilds the box from the store after upstream changes; Dial is
// passed to proxy.SingBoxDialer. *singbox.Manager satisfies it.
type SingBoxReloader interface {
	Dial(up config.Upstream, addr string, timeout time.Duration) (net.Conn, error)
	Reload(ctx context.Context) error
	LastError() error
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

type failState struct {
	count       int
	lockedUntil time.Time
}

// NewServer wires the panel to the store, proxy manager, logger and prober.
func NewServer(store *config.Store, mgr *proxy.Manager, logger *logx.Logger, prober *probe.Runner, version string) *Server {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		log.Fatalf("嵌入的前端资源损坏: %v", err)
	}
	return &Server{store: store, mgr: mgr, log: logger, probe: prober, ver: version,
		web: sub, fails: map[string]*failState{}}
}

// Handler returns the panel's routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/session", s.handleSession)
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)
	mux.HandleFunc("GET /api/data", s.auth(s.handleData))
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
	mux.HandleFunc("POST /api/upstreams/check-all", s.auth(s.handleCheckAllUpstreams))
	mux.HandleFunc("PUT /api/upstreams/{id}", s.auth(s.handleUpdateUpstream))
	mux.HandleFunc("POST /api/upstreams/{id}/check", s.auth(s.handleCheckUpstream))
	mux.HandleFunc("POST /api/upstreams/{id}/test", s.auth(s.handleCheckUpstream)) // 兼容旧前端
	mux.HandleFunc("DELETE /api/upstreams/{id}", s.auth(s.handleDeleteUpstream))

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

func clientIP(r *http.Request) string {
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}

// --- session / password endpoints ---

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"auth":    s.validSession(r),
		"version": s.ver,
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	ip := clientIP(r)
	now := time.Now()

	s.mu.Lock()
	fs, ok := s.fails[ip]
	if !ok {
		fs = &failState{}
		s.fails[ip] = fs
	}
	if fs.lockedUntil.After(now) {
		s.mu.Unlock()
		writeErr(w, http.StatusTooManyRequests, "尝试次数过多，请 10 分钟后再试")
		return
	}
	if len(s.fails) > 1024 {
		for k, v := range s.fails {
			if v.lockedUntil.Before(now) && v.count == 0 {
				delete(s.fails, k)
			}
		}
	}
	hash := s.store.AdminPasswordHash()
	if hash != "" && !VerifyPassword(hash, in.Password) {
		fs.count++
		if fs.count >= maxFails {
			fs.lockedUntil = now.Add(lockDuration)
			fs.count = 0
		}
		s.mu.Unlock()
		s.log.Warnf("管理面板登录失败 ip=%s", ip)
		writeErr(w, http.StatusUnauthorized, "密码错误")
		return
	}
	delete(s.fails, ip)
	s.mu.Unlock()

	http.SetCookie(w, s.newCookie())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
	http.SetCookie(w, s.newCookie())
	s.log.Infof("管理密码已修改")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- read endpoints ---

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

// handleCheckUpstream probes one upstream now (health + latency + exit IP).
func (s *Server) handleCheckUpstream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	res, ok := s.probe.CheckOne(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "上游不存在")
		return
	}
	if res.OK {
		s.log.Infof("上游探测 %s: 正常, 延迟 %dms, 出口 IP %s", id, res.LatencyMS, res.ExitIP)
	} else {
		s.log.Warnf("上游探测 %s: 失败 %s", id, res.Error)
	}
	writeJSON(w, http.StatusOK, res)
}

// handleCheckAllUpstreams probes every upstream and refreshes the direct IP.
func (s *Server) handleCheckAllUpstreams(w http.ResponseWriter, r *http.Request) {
	ok, failed := s.probe.CheckAll()
	serverIP, _ := s.probe.ServerIP()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        ok,
		"failed":    failed,
		"checked":   ok + failed,
		"server_ip": serverIP,
	})
}

// handleBulkUpstreams applies bulk delete/check to upstreams.
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
	case "check":
		type checkOut struct {
			ID        string `json:"id"`
			OK        bool   `json:"ok"`
			LatencyMS int64  `json:"latency_ms"`
			ExitIP    string `json:"exit_ip,omitempty"`
			Error     string `json:"error,omitempty"`
		}
		out := make([]checkOut, 0, len(in.IDs))
		okCount := 0
		for _, id := range in.IDs {
			res, found := s.probe.CheckOne(id)
			if !found {
				out = append(out, checkOut{ID: id, OK: false, Error: "不存在"})
				continue
			}
			if res.OK {
				okCount++
			}
			out = append(out, checkOut{
				ID: id, OK: res.OK, LatencyMS: res.LatencyMS, ExitIP: res.ExitIP, Error: res.Error,
			})
		}
		serverIP, _ := s.probe.ServerIP()
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": okCount, "failed": len(out) - okCount, "checks": out, "server_ip": serverIP,
		})
	default:
		writeErr(w, http.StatusBadRequest, "action 仅支持 delete 或 check")
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
