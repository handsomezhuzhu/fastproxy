package proxy

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/handsomezhuzhu/fastproxy/config"
)

// serveHTTP handles an inbound connection identified as HTTP proxy traffic
// (absolute-form requests and CONNECT tunnels).
func (e *entry) serveHTTP(cc *clientConn, ctr *countedConn, tr *trace) error {
	tr.proto = "http"
	conn := cc.Conn
	req, err := http.ReadRequest(cc.br)
	if err != nil {
		tr.fail("bad-request", err.Error())
		return err
	}
	user, pass, ok := parseProxyAuth(req.Header.Get("Proxy-Authorization"))
	if !ok {
		tr.fail("auth-failed", "缺少代理认证头")
		writePlainResponse(conn, http.StatusProxyAuthRequired)
		return errClientAuth
	}
	acc, aok := e.auth(ctr, user, pass)
	if !aok {
		tr.account = user
		tr.fail("auth-failed", "账号或密码错误")
		writePlainResponse(conn, http.StatusProxyAuthRequired)
		return errClientAuth
	}
	tr.account = acc.Username
	defer e.releaseAcc(ctr)

	if req.Method == http.MethodConnect {
		return e.handleConnect(cc, req, acc, tr)
	}
	return e.handlePlainHTTP(cc, req, acc, tr)
}

// handleConnect tunnels HTTPS (or any TCP traffic) through the upstream proxy.
func (e *entry) handleConnect(cc *clientConn, req *http.Request, acc *config.Account, tr *trace) error {
	conn := cc.Conn
	addr := req.Host
	if addr == "" {
		addr = req.URL.Host
	}
	if addr == "" {
		tr.fail("bad-request", "CONNECT 缺少目标地址")
		writePlainResponse(conn, http.StatusBadRequest)
		return errors.New("CONNECT 缺少目标地址")
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, "443")
	}
	tr.target = addr
	realUp := e.upstreamFor(acc)
	up, err := dialTarget(realUp, addr, e.dialTimeout())
	if err != nil {
		if realUp != nil {
			e.m.markUpstreamFail(realUp.ID, err.Error())
		}
		tr.fail("dial-error", err.Error())
		writePlainResponse(conn, http.StatusBadGateway)
		return err
	}
	if _, err := conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		up.Close()
		tr.fail("error", err.Error())
		return err
	}
	conn.SetDeadline(time.Time{})
	relay(cc, up)
	return nil
}

// handlePlainHTTP forwards an absolute-form HTTP request through the upstream.
func (e *entry) handlePlainHTTP(cc *clientConn, req *http.Request, acc *config.Account, tr *trace) error {
	conn := cc.Conn
	if !req.URL.IsAbs() || req.URL.Host == "" {
		tr.fail("bad-request", "HTTP 代理请求必须使用绝对地址")
		writePlainResponse(conn, http.StatusBadRequest)
		return errors.New("HTTP 代理请求必须使用绝对地址（如 GET http://host/path）")
	}
	if req.URL.Scheme != "http" {
		tr.fail("bad-request", "明文通道仅支持 http://")
		writePlainResponse(conn, http.StatusBadRequest)
		return fmt.Errorf("仅支持 http:// 明文请求，%s 请使用 CONNECT", req.URL.Scheme)
	}
	tr.target = req.URL.String()
	out := req.Clone(req.Context())
	out.Header.Del("Proxy-Authorization")
	out.Header.Del("Proxy-Connection")
	out.Header.Del("Connection")
	out.Close = true

	up := e.upstreamFor(acc)
	if up != nil && up.Type != config.UpstreamHTTP {
		tunnel, err := openHTTPTunnel(up, out.Host, e.dialTimeout())
		if err != nil {
			e.m.markUpstreamFail(up.ID, err.Error())
			tr.fail("dial-error", err.Error())
			writePlainResponse(conn, http.StatusBadGateway)
			return err
		}
		return e.plainViaTunnel(cc, out, tunnel, tr)
	}
	tr2 := e.transportFor(up) // nil upstream = direct
	resp, err := tr2.RoundTrip(out)
	if err != nil {
		if up != nil {
			e.m.markUpstreamFail(up.ID, err.Error())
		}
		tr.fail("dial-error", err.Error())
		writePlainResponse(conn, http.StatusBadGateway)
		return fmt.Errorf("转发失败: %w", err)
	}
	defer resp.Body.Close()
	resp.Close = true
	return resp.Write(conn)
}

// openHTTPTunnel opens a raw TCP tunnel to the origin of a plain HTTP request
// as required by tunnel-only upstreams (socks5/shadowsocks/vless). A request
// deadline is preset so reading the response cannot hang forever.
func openHTTPTunnel(up *config.Upstream, hostHeader string, timeout time.Duration) (net.Conn, error) {
	addr := hostHeader
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, "80")
	}
	tunnel, err := dialTarget(up, addr, timeout)
	if err != nil {
		return nil, err
	}
	tunnel.SetDeadline(time.Now().Add(timeout))
	return tunnel, nil
}

// plainViaTunnel sends an origin-form request through a tunnel to the origin
// server (tunnel-only upstreams).
func (e *entry) plainViaTunnel(cc *clientConn, out *http.Request, tunnel net.Conn, tr *trace) error {
	conn := cc.Conn
	defer tunnel.Close()

	// origin-form: the origin server must not receive the absolute URL.
	fwd := *out
	fwd.RequestURI = ""
	fwd.URL.Scheme = ""
	fwd.URL.Host = ""
	bw := bufio.NewWriter(tunnel)
	if err := fwd.Write(bw); err != nil {
		tr.fail("upstream-error", err.Error())
		writePlainResponse(conn, http.StatusBadGateway)
		return err
	}
	if err := bw.Flush(); err != nil {
		tr.fail("upstream-error", err.Error())
		writePlainResponse(conn, http.StatusBadGateway)
		return err
	}
	resp, err := http.ReadResponse(bufio.NewReader(tunnel), out)
	if err != nil {
		tr.fail("upstream-error", err.Error())
		writePlainResponse(conn, http.StatusBadGateway)
		return fmt.Errorf("读取上游响应失败: %w", err)
	}
	defer resp.Body.Close()
	resp.Close = true
	return resp.Write(conn)
}

// newTransport builds an outbound transport. up == nil dials targets directly.
func newTransport(up *config.Upstream, timeout time.Duration) *http.Transport {
	tr := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: timeout}).DialContext,
		MaxIdleConns:        64,
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     90 * time.Second,
	}
	if up != nil && up.Type == config.UpstreamHTTP {
		proxyURL := &url.URL{Scheme: "http", Host: up.Host}
		if up.Username != "" || up.Password != "" {
			proxyURL.User = url.UserPassword(up.Username, up.Password)
		}
		tr.Proxy = func(*http.Request) (*url.URL, error) { return proxyURL, nil }
	}
	return tr
}

func parseProxyAuth(header string) (user, pass string, ok bool) {
	const prefix = "Basic "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", "", false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(header[len(prefix):]))
	if err != nil {
		return "", "", false
	}
	i := strings.IndexByte(string(raw), ':')
	if i < 0 {
		return "", "", false
	}
	return string(raw[:i]), string(raw[i+1:]), true
}

func writePlainResponse(w io.Writer, code int) {
	msg := http.StatusText(code)
	body := msg + "\n"
	var b strings.Builder
	fmt.Fprintf(&b, "HTTP/1.1 %d %s\r\nConnection: close\r\nContent-Type: text/plain; charset=utf-8\r\n", code, msg)
	if code == http.StatusProxyAuthRequired {
		b.WriteString("Proxy-Authenticate: Basic realm=\"fastproxy\"\r\n")
	}
	fmt.Fprintf(&b, "Content-Length: %d\r\n\r\n%s", len(body), body)
	io.WriteString(w, b.String())
}
