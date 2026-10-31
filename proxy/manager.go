package proxy

import (
	"bufio"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/handsomezhuzhu/fastproxy/config"
	"github.com/handsomezhuzhu/fastproxy/logx"
)

// trace collects per-connection facts for the access log.
type trace struct {
	proto   string
	account string
	target  string
	status  string // "" (ok) | auth-failed | dial-error | bad-request | error
	errText string
}

func (t *trace) fail(status, msg string) {
	t.status = status
	t.errText = msg
}

// meter accumulates connection and traffic counters for a port or account.
type meter struct {
	connsTotal, connsCur atomic.Int64
	in, out              atomic.Int64
}

// PortStatus is one port's state as served to the admin panel.
type PortStatus struct {
	ID         string `json:"id"`
	Port       int    `json:"port"`
	Enabled    bool   `json:"enabled"`
	Listening  bool   `json:"listening"`
	Error      string `json:"error,omitempty"`
	ConnsTotal int64  `json:"conns_total"`
	ConnsCur   int64  `json:"conns_cur"`
	BytesIn    int64  `json:"bytes_in"`
	BytesOut   int64  `json:"bytes_out"`
}

// AccountStatus is one account's traffic counters.
type AccountStatus struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	PortID     string `json:"port_id"`
	UpstreamID string `json:"upstream_id"`
	Enabled    bool   `json:"enabled"`
	ConnsTotal int64  `json:"conns_total"`
	ConnsCur   int64  `json:"conns_cur"`
	BytesIn    int64  `json:"bytes_in"`
	BytesOut   int64  `json:"bytes_out"`
}

// Manager owns one listener per enabled port. Account and upstream changes
// apply instantly on the next connection; only port changes restart listeners.
type Manager struct {
	store *config.Store
	log   *logx.Logger

	mu        sync.Mutex
	entries   map[string]*entry
	listenErr map[string]string
	meters    map[string]*meter
}

type entry struct {
	m     *Manager
	port  config.Port
	ln    net.Listener
	stats *meter

	trMu sync.Mutex
	trs  map[string]*http.Transport
}

// NewManager creates a manager bound to the store. Call Sync to start listeners.
func NewManager(store *config.Store, logger *logx.Logger) *Manager {
	return &Manager{
		store:     store,
		log:       logger,
		entries:   map[string]*entry{},
		listenErr: map[string]string{},
		meters:    map[string]*meter{},
	}
}

// Sync aligns running listeners with the enabled ports in the store,
// restarting only the entries whose port configuration actually changed.
func (m *Manager) Sync() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	desired := map[string]*config.Port{}
	for _, p := range m.store.Ports() {
		if p.Enabled {
			desired[p.ID] = p
		}
	}
	for id, e := range m.entries {
		want := desired[id]
		if want != nil && samePort(e.port, *want) {
			continue
		}
		e.close()
		delete(m.entries, id)
		delete(m.listenErr, id)
		if want == nil {
			m.log.Infof("线路停止: 端口 %d", e.port.Port)
		}
	}
	for id, p := range desired {
		if _, ok := m.entries[id]; ok {
			continue
		}
		e, err := m.newEntry(*p)
		if err != nil {
			errs = append(errs, err)
			m.listenErr[id] = err.Error()
			m.log.Warnf("%v", err)
			continue
		}
		m.entries[id] = e
		delete(m.listenErr, id)
		go e.serve()
		m.log.Infof("线路启动: 端口 %d -> %s", p.Port, upstreamDesc(m.defaultUpstream(p)))
	}
	return errors.Join(errs...)
}

// Status returns per-port and per-account counters.
func (m *Manager) Status() ([]PortStatus, []AccountStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ports := make([]PortStatus, 0, len(m.store.Ports()))
	for _, p := range m.store.Ports() {
		row := PortStatus{ID: p.ID, Port: p.Port, Enabled: p.Enabled}
		if mt, ok := m.meters["port:"+p.ID]; ok {
			fillMeter(mt, &row.ConnsTotal, &row.ConnsCur, &row.BytesIn, &row.BytesOut)
		}
		_, row.Listening = m.entries[p.ID]
		row.Error = m.listenErr[p.ID]
		ports = append(ports, row)
	}
	accounts := make([]AccountStatus, 0, len(m.store.Accounts()))
	for _, a := range m.store.Accounts() {
		row := AccountStatus{ID: a.ID, Username: a.Username, PortID: a.PortID,
			UpstreamID: a.UpstreamID, Enabled: a.Enabled}
		if mt, ok := m.meters["acc:"+a.ID]; ok {
			fillMeter(mt, &row.ConnsTotal, &row.ConnsCur, &row.BytesIn, &row.BytesOut)
		}
		accounts = append(accounts, row)
	}
	return ports, accounts
}

// Close stops every listener.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, e := range m.entries {
		e.close()
		delete(m.entries, id)
	}
}

// --- entry construction / lifecycle ---

func (m *Manager) newEntry(p config.Port) (*entry, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p.Port))
	if err != nil {
		return nil, fmt.Errorf("端口 %d 监听失败: %w", p.Port, err)
	}
	return &entry{
		m:     m,
		port:  p,
		ln:    ln,
		stats: m.meterFor("port:" + p.ID),
		trs:   map[string]*http.Transport{},
	}, nil
}

func (e *entry) close() {
	e.ln.Close()
	e.trMu.Lock()
	defer e.trMu.Unlock()
	for _, tr := range e.trs {
		tr.CloseIdleConnections()
	}
	e.trs = map[string]*http.Transport{}
}

// serve accepts connections until the listener closes.
func (e *entry) serve() {
	for {
		conn, err := e.ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		go e.handle(conn)
	}
}

func (e *entry) handshakeTimeout() time.Duration {
	return time.Duration(e.m.store.Settings().HandshakeTimeoutSec) * time.Second
}

func (e *entry) dialTimeout() time.Duration {
	return time.Duration(e.m.store.Settings().DialTimeoutSec) * time.Second
}

// handle sniffs the first byte: 0x05 means SOCKS5, anything else means HTTP.
// One access-log entry is emitted per connection when it finishes.
func (e *entry) handle(raw net.Conn) {
	st := e.stats
	st.connsTotal.Add(1)
	st.connsCur.Add(1)
	start := time.Now()
	lin, lout := &atomic.Int64{}, &atomic.Int64{}
	tr := &trace{}
	defer func() {
		st.connsCur.Add(-1)
		raw.Close()
		e.m.logConn(e.port.Port, remoteIP(raw.RemoteAddr()), tr, lin.Load(), lout.Load(), start)
	}()

	raw.SetDeadline(time.Now().Add(e.handshakeTimeout()))
	ctr := &countedConn{Conn: raw, port: st, lin: lin, lout: lout}
	br := bufio.NewReaderSize(ctr, 8192)
	head, err := br.Peek(1)
	if err != nil {
		return
	}
	c := &clientConn{br: br, Conn: ctr}
	if head[0] == 5 {
		err = e.serveSocks5(c, ctr, tr)
	} else {
		err = e.serveHTTP(c, ctr, tr)
	}
	if err != nil && tr.errText == "" {
		tr.fail("error", err.Error())
	}
}

// logConn writes the per-connection access entry.
func (m *Manager) logConn(port int, client string, tr *trace, in, out int64, start time.Time) {
	cfg := m.store.Settings()
	status := tr.status
	if status == "" {
		status = "ok"
	}
	if !cfg.AccessLog && tr.status == "" {
		return // access logging disabled: only failures are recorded
	}
	level := "info"
	if tr.status != "" {
		level = "warn"
	}
	m.log.Emit(logx.Entry{
		Level:      level,
		Port:       port,
		Account:    tr.account,
		Client:     client,
		Proto:      tr.proto,
		Target:     tr.target,
		Status:     status,
		BytesIn:    in,
		BytesOut:   out,
		DurationMS: time.Since(start).Milliseconds(),
		Msg:        tr.errText,
	})
}

func remoteIP(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}

// --- auth and upstream resolution (live from store, hot-reload friendly) ---

// auth authenticates (user, pass) for this port and returns the account.
// It also attaches the account meter to the connection counters.
func (e *entry) auth(ctr *countedConn, user, pass string) (*config.Account, bool) {
	var acc *config.Account
	for _, a := range e.m.store.Accounts() {
		if a.Enabled && a.Username == user && (a.PortID == "" || a.PortID == e.port.ID) {
			acc = a
			break
		}
	}
	if acc == nil ||
		subtle.ConstantTimeCompare([]byte(pass), []byte(acc.Password)) != 1 {
		return nil, false
	}
	ctr.acc = e.m.meterFor("acc:" + acc.ID)
	ctr.acc.connsTotal.Add(1)
	ctr.acc.connsCur.Add(1)
	return acc, true
}

// releaseAcc marks an authenticated account connection as finished.
func (e *entry) releaseAcc(ctr *countedConn) {
	if ctr.acc != nil {
		ctr.acc.connsCur.Add(-1)
	}
}

// upstreamFor resolves the effective upstream for an account:
// account override first, then the port default; nil means dial direct.
func (e *entry) upstreamFor(acc *config.Account) *config.Upstream {
	if acc.UpstreamID != "" {
		for _, u := range e.m.store.Upstreams() {
			if u.ID == acc.UpstreamID {
				c := *u
				return &c
			}
		}
		return nil
	}
	if e.port.DefaultUpstream != "" {
		for _, u := range e.m.store.Upstreams() {
			if u.ID == e.port.DefaultUpstream {
				c := *u
				return &c
			}
		}
	}
	return nil
}

func (m *Manager) defaultUpstream(p *config.Port) *config.Upstream {
	if p.DefaultUpstream == "" {
		return nil
	}
	for _, u := range m.store.Upstreams() {
		if u.ID == p.DefaultUpstream {
			c := *u
			return &c
		}
	}
	return nil
}

// transportFor returns a cached outbound transport for the upstream identity
// (or direct dialing when up is nil). Cache is keyed by identity so upstream
// edits naturally produce a fresh transport.
func (e *entry) transportFor(up *config.Upstream) *http.Transport {
	key := "direct"
	if up != nil {
		key = up.Type + "|" + up.Host + "|" + up.Username + "|" + up.Password
	}
	e.trMu.Lock()
	defer e.trMu.Unlock()
	if tr, ok := e.trs[key]; ok {
		return tr
	}
	tr := newTransport(up, e.dialTimeout())
	e.trs[key] = tr
	return tr
}

// --- helpers ---

func (m *Manager) meterFor(key string) *meter {
	if mt, ok := m.meters[key]; ok {
		return mt
	}
	mt := &meter{}
	m.meters[key] = mt
	return mt
}

func fillMeter(mt *meter, total, cur, in, out *int64) {
	*total = mt.connsTotal.Load()
	*cur = mt.connsCur.Load()
	*in = mt.in.Load()
	*out = mt.out.Load()
}

func samePort(a, b config.Port) bool { return a == b }

func upstreamDesc(u *config.Upstream) string {
	if u == nil {
		return "直连"
	}
	return fmt.Sprintf("%s 上游 %s", u.Type, u.Host)
}
