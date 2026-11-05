// Package probe performs active health checks and exit-IP detection for
// upstream proxies, plus the server's own direct egress IP.
package probe

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/handsomezhuzhu/fastproxy/config"
	"github.com/handsomezhuzhu/fastproxy/logx"
	"github.com/handsomezhuzhu/fastproxy/proxy"
)

// Result is one probe outcome.
type Result struct {
	OK        bool   `json:"ok"`
	LatencyMS int64  `json:"latency_ms"`
	ExitIP    string `json:"exit_ip,omitempty"`
	Error     string `json:"error,omitempty"`
}

// dial opens a tunnel to addr through up (nil = direct connection).
func dial(up *config.Upstream, addr string, timeout time.Duration) (net.Conn, error) {
	if up == nil {
		return net.DialTimeout("tcp", addr, timeout)
	}
	return proxy.DialTunnel(*up, addr, timeout)
}

// CheckAddr fetches the exit IP by issuing an HTTP GET through the tunnel to
// addr. When useTLS is true the tunnel is upgraded to TLS first.
func CheckAddr(up *config.Upstream, addr string, timeout time.Duration, useTLS bool) Result {
	start := time.Now()
	conn, err := dial(up, addr, timeout)
	if err != nil {
		return Result{Error: err.Error()}
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))
	latency := time.Since(start).Milliseconds()

	host := addr
	var c net.Conn = conn
	if useTLS {
		h, _, _ := net.SplitHostPort(addr)
		host = h
		tlsConn := tls.Client(conn, &tls.Config{ServerName: h})
		if err := tlsConn.Handshake(); err != nil {
			return Result{Error: "TLS 握手失败: " + err.Error(), LatencyMS: latency}
		}
		c = tlsConn
	}
	fmt.Fprintf(c, "GET / HTTP/1.1\r\nHost: %s\r\nUser-Agent: FastProxy-Probe\r\nConnection: close\r\n\r\n", host)

	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		return Result{Error: "读取探测响应失败: " + err.Error(), LatencyMS: latency}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128<<10))
	if err != nil {
		return Result{Error: "读取探测响应体失败: " + err.Error(), LatencyMS: latency}
	}
	ip := extractIP(string(body))
	if ip == "" {
		return Result{Error: "响应中未找到出口 IP: " + truncate(string(body), 80), LatencyMS: latency}
	}
	return Result{OK: true, LatencyMS: latency, ExitIP: ip}
}

// Check probes an upstream: plain HTTP on :80 first, then TLS on :443.
func Check(up *config.Upstream, targetHost string, timeout time.Duration) Result {
	r := CheckAddr(up, targetHost+":80", timeout, false)
	if r.OK {
		return r
	}
	r2 := CheckAddr(up, targetHost+":443", timeout, true)
	if r2.OK {
		return r2
	}
	if r.Error == "" {
		r.Error = r2.Error
	}
	if r.LatencyMS == 0 {
		r.LatencyMS = r2.LatencyMS
	}
	return r
}

// extractIP finds the first IPv4/IPv6 literal in the body (tolerates JSON,
// key-value and plain-text responses).
func extractIP(body string) string {
	isIPChar := func(r rune) bool {
		return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') ||
			(r >= 'A' && r <= 'F') || r == ':' || r == '.'
	}
	for _, token := range strings.Fields(body) {
		for _, part := range strings.FieldsFunc(token, func(r rune) bool { return !isIPChar(r) }) {
			if ip := net.ParseIP(part); ip != nil {
				return ip.String()
			}
		}
	}
	return ""
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func healthFor(r Result) string {
	if r.OK {
		return "ok"
	}
	return "fail"
}

// Runner periodically probes upstreams and keeps the direct egress IP.
type Runner struct {
	store *config.Store
	log   *logx.Logger

	mu         sync.Mutex
	serverIP   string
	lastDirect time.Time

	checkMu sync.Mutex // serializes full scans
}

// NewRunner creates a prober bound to the store.
func NewRunner(store *config.Store, logger *logx.Logger) *Runner {
	r := &Runner{store: store, log: logger}
	if store != nil {
		r.serverIP, r.lastDirect = store.LoadServerProbe()
	}
	return r
}

// Start runs the periodic probe loop until ctx is done.
func (r *Runner) Start(ctx context.Context) {
	if r.store.Settings().ProbeEnabled {
		r.CheckAll()
	}
	for {
		interval := time.Duration(r.store.Settings().ProbeIntervalSec) * time.Second
		if interval < 30*time.Second {
			interval = 30 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
			if r.store.Settings().ProbeEnabled {
				r.CheckAll()
			}
		}
	}
}

// CheckAll probes the direct egress and every upstream concurrently.
func (r *Runner) CheckAll() (okCount, failCount int) {
	r.checkMu.Lock()
	defer r.checkMu.Unlock()

	st := r.store.Settings()
	timeout := time.Duration(st.ProbeTimeoutSec) * time.Second
	target := st.ProbeTarget

	if res := Check(nil, target, timeout); res.OK {
		now := time.Now()
		r.mu.Lock()
		r.serverIP = res.ExitIP
		r.lastDirect = now
		r.mu.Unlock()
		if r.store != nil {
			_ = r.store.SaveServerProbe(res.ExitIP, now)
		}
	}

	ups := r.store.Upstreams()
	var ok, failed atomic.Int64
	concurrency := st.ProbeConcurrency
	if concurrency <= 0 {
		concurrency = 6
	}
	if concurrency > 32 {
		concurrency = 32
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, u := range ups {
		u := u
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			res := Check(u, target, timeout)
			if err := r.store.RecordUpstreamCheck(u.ID, healthFor(res), res.LatencyMS, res.ExitIP, res.Error); err != nil && r.log != nil {
				r.log.Warnf("记录探测结果失败 %s: %v", u.Name, err)
			}
			if res.OK {
				ok.Add(1)
			} else {
				failed.Add(1)
			}
		}()
	}
	wg.Wait()

	if r.log != nil && len(ups) > 0 {
		r.log.Infof("探测完成: 上游 %d 个, 正常 %d, 失败 %d", len(ups), ok.Load(), failed.Load())
	}
	return int(ok.Load()), int(failed.Load())
}

// CheckOne probes a single upstream immediately.
func (r *Runner) CheckOne(id string) (Result, bool) {
	st := r.store.Settings()
	timeout := time.Duration(st.ProbeTimeoutSec) * time.Second
	var up *config.Upstream
	for _, u := range r.store.Upstreams() {
		if u.ID == id {
			up = u
			break
		}
	}
	if up == nil {
		return Result{}, false
	}
	res := Check(up, st.ProbeTarget, timeout)
	_ = r.store.RecordUpstreamCheck(id, healthFor(res), res.LatencyMS, res.ExitIP, res.Error)
	return res, true
}

// ServerIP returns the last detected direct egress IP and when it was probed.
func (r *Runner) ServerIP() (string, time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.serverIP, r.lastDirect
}

// RecordFailure marks an upstream as failed from real traffic (passive check).
func (r *Runner) RecordFailure(id, errText string) {
	if id == "" {
		return
	}
	_ = r.store.RecordUpstreamCheck(id, "fail", 0, "", errText)
}
