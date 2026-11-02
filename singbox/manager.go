package singbox

import (
	"context"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/sagernet/sing-box"

	"github.com/handsomezhuzhu/fastproxy/config"
)

// portStart is the first loopback port handed to an upstream's SOCKS inbound.
const portStart = 20000

// Dialer is the hook FastProxy's proxy package calls for upstreams that need
// sing-box. proxy sets proxy.SingBoxDialer = mgr.Dial at startup.
type Dialer interface {
	Dial(up config.Upstream, addr string, timeout time.Duration) (net.Conn, error)
}

// Logger is the minimal logging surface the manager needs.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// UpstreamsSource provides the current upstream pool (config.Store).
type UpstreamsSource interface {
	Upstreams() []*config.Upstream
}

// Manager owns the embedded sing-box instance and maps upstream IDs to their
// loopback SOCKS5 ports.
//
// A single Box serves every upstream that needs it: each upstream gets a
// SOCKS inbound on 127.0.0.1:<port> plus a vless/vmess outbound, and a route
// rule sends that inbound's traffic to its outbound (final = direct).
type Manager struct {
	src UpstreamsSource
	log Logger
	reg Registries

	mu      sync.Mutex
	box     *box.Box
	ports   map[string]uint16 // upstream id -> local SOCKS port
	lastErr error

	// Status surface for the panel (GET /api/singbox/status).
	startedAt    time.Time
	reloadCount  int
	lastReloadAt time.Time
	events       []string // ring buffer, newest last

	dialMu sync.Mutex // serialises Reload against in-flight Dial handshakes
}

const maxStatusEvents = 100

// addEventLocked appends one status line. Caller must hold m.mu.
func (m *Manager) addEventLocked(format string, args ...any) {
	line := time.Now().Format(time.RFC3339) + " " + fmt.Sprintf(format, args...)
	m.events = append(m.events, line)
	if len(m.events) > maxStatusEvents {
		m.events = append([]string(nil), m.events[len(m.events)-maxStatusEvents:]...)
	}
}

// Status returns a JSON-friendly snapshot for the panel. It never blocks on a
// reload for long: the same mutex protects the fields and Reload holds it only
// while rebuilding.
func (m *Manager) Status() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := map[string]string{}
	if m.src != nil {
		for _, u := range m.src.Upstreams() {
			if u != nil {
				names[u.ID] = u.Name
			}
		}
	}
	type row struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Port uint16 `json:"port"`
	}
	rows := make([]row, 0, len(m.ports))
	for id, port := range m.ports {
		rows = append(rows, row{ID: id, Name: names[id], Port: port})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Port < rows[j].Port })
	var started, reloaded int64
	if !m.startedAt.IsZero() {
		started = m.startedAt.Unix()
	}
	if !m.lastReloadAt.IsZero() {
		reloaded = m.lastReloadAt.Unix()
	}
	errText := ""
	if m.lastErr != nil {
		errText = m.lastErr.Error()
	}
	events := append([]string(nil), m.events...)
	return map[string]any{
		"running":        m.box != nil,
		"started_at":     started,
		"reload_count":   m.reloadCount,
		"last_reload_at": reloaded,
		"last_error":     errText,
		"upstreams":      rows,
		"logs":           events,
	}
}

// New creates a stopped manager. Call Start or Reload to spawn sing-box.
func New(src UpstreamsSource, logger Logger) *Manager {
	return &Manager{src: src, log: logger, reg: ClientRegistries(), ports: map[string]uint16{}}
}

// UseRegistry swaps the sing-box type registry used for the next (re)build.
// Tests install the server-side inbounds this way; production sticks with
// ClientRegistries.
func (m *Manager) UseRegistry(reg Registries) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reg = reg
}

// Start builds and launches sing-box from the current upstream pool.
func (m *Manager) Start(ctx context.Context) error {
	return m.Reload(ctx)
}

// Reload rebuilds the sing-box instance from the store: it closes the old box
// first (freeing the fixed loopback ports) and starts a new one. Ports already
// assigned to an upstream are reused so in-flight Dial lookups keep working.
// On failure the manager keeps no box and Dial reports LastError.
func (m *Manager) Reload(ctx context.Context) error {
	m.dialMu.Lock()
	defer m.dialMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reloadCount++
	m.addEventLocked("开始重载 sing-box")
	if m.box != nil {
		_ = m.box.Close()
		m.box = nil
	}
	ups := m.needed()
	if len(ups) == 0 {
		m.ports = map[string]uint16{}
		m.lastErr = nil
		m.lastReloadAt = time.Now()
		m.addEventLocked("没有需要 sing-box 的上游，实例已停止")
		return nil
	}
	ports, err := m.assignPorts(ups)
	if err != nil {
		m.lastErr = err
		m.lastReloadAt = time.Now()
		m.addEventLocked("分配本地端口失败: %v", err)
		return err
	}
	opts, err := buildOptions(ups, ports)
	if err != nil {
		m.lastErr = fmt.Errorf("生成 sing-box 配置失败: %w", err)
		m.lastReloadAt = time.Now()
		m.addEventLocked("生成配置失败: %v", err)
		return m.lastErr
	}
	m.relaxTLS(opts)
	instance, err := box.New(box.Options{Options: *opts, Context: m.reg.context(ctx)})
	if err == nil {
		err = instance.Start()
	}
	if err != nil {
		if instance != nil {
			_ = instance.Close()
		}
		m.lastErr = fmt.Errorf("启动内置 sing-box 失败: %w", err)
		m.lastReloadAt = time.Now()
		m.addEventLocked("启动失败: %v", err)
		return m.lastErr
	}
	m.box = instance
	m.ports = ports
	m.lastErr = nil
	m.startedAt = time.Now()
	m.lastReloadAt = m.startedAt
	m.addEventLocked("已启动，%d 个上游经内嵌出站", len(ups))
	if m.log != nil {
		m.log.Infof("内置 sing-box 已启动，%d 个上游经内嵌出站", len(ups))
	}
	return nil
}

// needed returns the upstreams that must go through sing-box and are
// dialable as configured.
func (m *Manager) needed() []config.Upstream {
	var out []config.Upstream
	if m.src == nil {
		return nil
	}
	for _, u := range m.src.Upstreams() {
		if u == nil {
			continue
		}
		if !NeedsSingBox(*u) || u.Host == "" || u.UUID == "" {
			continue
		}
		if u.Type == config.UpstreamVless && u.Security == config.SecurityReality && u.PBK == "" {
			m.addEventLocked("跳过上游 %s: REALITY 缺少 pbk", u.Name)
			if m.log != nil {
				m.log.Warnf("上游 %s 的 REALITY 配置缺少 pbk，已跳过", u.Name)
			}
			continue
		}
		if m.log != nil && u.Type == config.UpstreamVless && u.Security == "" {
			m.log.Warnf("上游 %s 未设置 security，将按明文 VLESS 连接", u.Name)
		}
		out = append(out, *u)
	}
	return out
}

// assignPorts keeps existing assignments and allocates free loopback ports for
// new upstreams. Upstreams that are gone release their port.
func (m *Manager) assignPorts(ups []config.Upstream) (map[string]uint16, error) {
	next := make(map[string]uint16, len(ups))
	used := map[uint16]bool{}
	for _, up := range ups {
		if p, ok := m.ports[up.ID]; ok {
			next[up.ID] = p
			used[p] = true
		}
	}
	candidate := portStart
	for _, up := range ups {
		if _, ok := next[up.ID]; ok {
			continue
		}
		for {
			if candidate > 65535 {
				return nil, fmt.Errorf("没有可用的本地端口分配给内置 sing-box（已分配 %d 个）", len(next))
			}
			port := uint16(candidate)
			candidate++
			if used[port] || !portFree(port) {
				continue
			}
			next[up.ID] = port
			used[port] = true
			break
		}
	}
	return next, nil
}

// portFree probes whether 127.0.0.1:port can be bound right now.
func portFree(port uint16) bool {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port))))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// Dial opens addr through the upstream's local SOCKS5 listener.
func (m *Manager) Dial(up config.Upstream, addr string, timeout time.Duration) (net.Conn, error) {
	m.mu.Lock()
	port, ok := m.ports[up.ID]
	upstreamErr := m.lastErr
	running := m.box != nil
	m.mu.Unlock()
	if !ok {
		if upstreamErr != nil {
			return nil, fmt.Errorf("上游 %s 的内置 sing-box 出站不可用: %w", up.Name, upstreamErr)
		}
		if !running {
			return nil, fmt.Errorf("上游 %s 未在 sing-box 中注册（请刷新上游列表）", up.Name)
		}
		return nil, fmt.Errorf("上游 %s 未在 sing-box 中注册", up.Name)
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port))), timeout)
	if err != nil {
		return nil, fmt.Errorf("连接内置 sing-box（端口 %d）失败: %w", port, err)
	}
	if err := socks5Connect(conn, addr, timeout); err != nil {
		conn.Close()
		return nil, fmt.Errorf("经内置 sing-box 连接 %s 失败: %w", addr, err)
	}
	return conn, nil
}

// Close stops the embedded sing-box instance.
func (m *Manager) Close() error {
	m.dialMu.Lock()
	defer m.dialMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	var err error
	if m.box != nil {
		err = m.box.Close()
		m.box = nil
		m.addEventLocked("sing-box 已停止")
	}
	m.ports = map[string]uint16{}
	return err
}

// Running reports whether sing-box is currently up.
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.box != nil
}

// Ports returns a copy of the upstream id -> local SOCKS port map.
func (m *Manager) Ports() map[string]uint16 {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]uint16, len(m.ports))
	for k, v := range m.ports {
		out[k] = v
	}
	return out
}

// LastError returns the error of the most recent failed Reload, if any.
func (m *Manager) LastError() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastErr
}

// --- SOCKS5 client (no auth) ---

func socks5Connect(conn net.Conn, addr string, timeout time.Duration) error {
	_ = conn.SetDeadline(time.Now().Add(timeout))
	defer conn.SetDeadline(time.Time{})
	if _, err := conn.Write([]byte{5, 1, 0}); err != nil {
		return err
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return err
	}
	if resp[0] != 5 || resp[1] != 0 {
		return fmt.Errorf("SOCKS5 握手被拒绝（0x%02x）", resp[1])
	}
	req, err := socks5Request(addr)
	if err != nil {
		return err
	}
	if _, err := conn.Write(req); err != nil {
		return err
	}
	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil {
		return err
	}
	if head[0] != 5 {
		return fmt.Errorf("SOCKS5 响应版本异常: %d", head[0])
	}
	if head[1] != 0 {
		return fmt.Errorf("SOCKS5 连接失败: %s", socks5ErrText(head[1]))
	}
	// Drain BND.ADDR + BND.PORT before handing the socket over for relaying;
	// a leftover reply byte would corrupt the tunneled stream.
	var rest []byte
	switch head[3] {
	case 1:
		rest = make([]byte, 4+2)
	case 4:
		rest = make([]byte, 16+2)
	case 3:
		l := make([]byte, 1)
		if _, err := io.ReadFull(conn, l); err != nil {
			return err
		}
		rest = make([]byte, int(l[0])+2)
	default:
		return fmt.Errorf("SOCKS5 响应地址类型未知: %d", head[3])
	}
	_, err = io.ReadFull(conn, rest)
	return err
}

func socks5Request(addr string) ([]byte, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("目标地址 %q 无效", addr)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("目标端口 %q 无效", portStr)
	}
	req := []byte{5, 1, 0}
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			req = append(req, 1)
			req = append(req, ip4...)
		} else {
			req = append(req, 4)
			req = append(req, ip...)
		}
	} else {
		if len(host) > 255 {
			return nil, fmt.Errorf("目标域名过长")
		}
		req = append(req, 3, byte(len(host)))
		req = append(req, host...)
	}
	return append(req, byte(port>>8), byte(port)), nil
}

func socks5ErrText(code byte) string {
	switch code {
	case 1:
		return "general failure"
	case 2:
		return "connection not allowed"
	case 3:
		return "network unreachable"
	case 4:
		return "host unreachable"
	case 5:
		return "connection refused"
	case 6:
		return "TTL expired"
	case 7:
		return "command not supported"
	case 8:
		return "address type not supported"
	default:
		return fmt.Sprintf("code %d", code)
	}
}
