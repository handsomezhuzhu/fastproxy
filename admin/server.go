// Package admin serves the FastProxy management panel and its JSON API.
package admin

import (
	_ "embed"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/handsomezhuzhu/fastproxy/config"
	"github.com/handsomezhuzhu/fastproxy/proxy"
)

const (
	cookieName   = "fp_admin"
	sessionTTL   = 12 * time.Hour
	maxFails     = 10
	lockDuration = 10 * time.Minute
)

//go:embed index.html
var indexHTML []byte

// Server is the admin panel HTTP server.
type Server struct {
	store *config.Store
	mgr   *proxy.Manager
	ver   string

	mu    sync.Mutex
	fails map[string]*failState
}

type failState struct {
	count       int
	lockedUntil time.Time
}

// NewServer wires the panel to the store and proxy manager.
func NewServer(store *config.Store, mgr *proxy.Manager, version string) *Server {
	return &Server{store: store, mgr: mgr, ver: version, fails: map[string]*failState{}}
}

// Handler returns the panel's routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/session", s.handleSession)
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)
	mux.HandleFunc("GET /api/data", s.auth(s.handleData))
	mux.HandleFunc("GET /api/stats", s.auth(s.handleStats))

	mux.HandleFunc("POST /api/upstreams", s.auth(s.handleCreateUpstream))
	mux.HandleFunc("PUT /api/upstreams/{id}", s.auth(s.handleUpdateUpstream))
	mux.HandleFunc("DELETE /api/upstreams/{id}", s.auth(s.handleDeleteUpstream))

	mux.HandleFunc("POST /api/ports", s.auth(s.handleCreatePort))
	mux.HandleFunc("PUT /api/ports/{id}", s.auth(s.handleUpdatePort))
	mux.HandleFunc("PATCH /api/ports/{id}", s.auth(s.handlePatchPort))
	mux.HandleFunc("DELETE /api/ports/{id}", s.auth(s.handleDeletePort))

	mux.HandleFunc("POST /api/accounts", s.auth(s.handleCreateAccount))
	mux.HandleFunc("PUT /api/accounts/{id}", s.auth(s.handleUpdateAccount))
	mux.HandleFunc("PATCH /api/accounts/{id}", s.auth(s.handlePatchAccount))
	mux.HandleFunc("DELETE /api/accounts/{id}", s.auth(s.handleDeleteAccount))

	mux.HandleFunc("POST /api/password", s.auth(s.handlePassword))
	mux.HandleFunc("GET /", s.handleIndex)
	return mux
}

// --- static page ---

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(indexHTML)
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
		log.Printf("管理面板登录失败 ip=%s", ip)
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
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- read endpoints ---

func (s *Server) handleData(w http.ResponseWriter, r *http.Request) {
	ports, accounts := s.mgr.Status()
	writeJSON(w, http.StatusOK, map[string]any{
		"version":   s.ver,
		"upstreams": s.store.Upstreams(),
		"ports":     s.store.Ports(),
		"accounts":  s.store.Accounts(),
		"status":    map[string]any{"ports": ports, "accounts": accounts},
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	ports, accounts := s.mgr.Status()
	writeJSON(w, http.StatusOK, map[string]any{
		"status": map[string]any{"ports": ports, "accounts": accounts},
	})
}

// --- upstreams ---

type upstreamInput struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Upstream string `json:"upstream"` // raw address, parsed server-side
	Remark   string `json:"remark"`
}

func (in *upstreamInput) toEntity() (*config.Upstream, error) {
	typ, host, user, pass, err := config.ParseUpstream(in.Upstream)
	if err != nil {
		return nil, err
	}
	return &config.Upstream{
		Name: in.Name, Type: typ, Host: host,
		Username: user, Password: pass, Remark: in.Remark,
	}, nil
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
	s.syncRespond(w, nil, saved)
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
	s.syncRespond(w, nil, saved)
}

func (s *Server) handleDeleteUpstream(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteUpstream(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.syncRespond(w, nil, nil)
}

// --- ports ---

type portInput struct {
	Port            int    `json:"port"`
	DefaultUpstream string `json:"default_upstream"`
	Remark          string `json:"remark"`
}

func (in *portInput) toEntity() *config.Port {
	return &config.Port{
		Port: in.Port, DefaultUpstream: in.DefaultUpstream, Remark: in.Remark, Enabled: true,
	}
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
	s.syncRespond(w, nil, saved)
}

func (s *Server) handleUpdatePort(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in portInput
	if !decodeJSON(w, r, &in) {
		return
	}
	saved, err := s.store.UpsertPort(id, in.toEntity())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.syncRespond(w, nil, saved)
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
	s.syncRespond(w, nil, saved)
}

func (s *Server) handleDeletePort(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeletePort(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	s.syncRespond(w, nil, nil)
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
	s.syncRespond(w, nil, saved)
}

func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in accountInput
	if !decodeJSON(w, r, &in) {
		return
	}
	saved, err := s.store.UpsertAccount(id, in.toEntity())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.syncRespond(w, nil, saved)
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
	s.syncRespond(w, nil, saved)
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteAccount(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	s.syncRespond(w, nil, nil)
}

// --- plumbing ---

// syncRespond applies changes to running listeners and answers. Deleted or
// changed ports restart their listener; accounts/upstreams apply hot.
func (s *Server) syncRespond(w http.ResponseWriter, err error, saved any) {
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var warning string
	if serr := s.mgr.Sync(); serr != nil {
		warning = serr.Error()
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
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
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
