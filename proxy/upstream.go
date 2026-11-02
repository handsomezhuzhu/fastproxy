package proxy

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/handsomezhuzhu/fastproxy/config"
)

// dialTarget opens a connection to addr ("host:port"). When up is nil the
// connection is made directly (no upstream proxy).
func dialTarget(up *config.Upstream, addr string, timeout time.Duration) (net.Conn, error) {
	if up == nil {
		conn, err := net.DialTimeout("tcp", addr, timeout)
		if err != nil {
			return nil, fmt.Errorf("连接目标 %s 失败: %w", addr, err)
		}
		return conn, nil
	}
	return DialTunnel(*up, addr, timeout)
}

// SingBoxDialer is set at startup by main to the embedded sing-box manager.
// Every VLESS and VMess upstream is tunnelled through it; nil means those
// upstreams report a clear "not initialised" error.
var SingBoxDialer func(up config.Upstream, addr string, timeout time.Duration) (net.Conn, error)

// DialTunnel opens a tunnel to addr ("host:port", usually the origin server)
// through the upstream proxy, authenticating against it when needed.
//
// The tunnel protocol is chosen by up.Type: http (CONNECT), socks5,
// shadowsocks (AEAD TCP), or vless/vmess (embedded sing-box outbound).
func DialTunnel(up config.Upstream, addr string, timeout time.Duration) (net.Conn, error) {
	switch up.Type {
	case config.UpstreamShadowsocks:
		return dialShadowsocks(up, addr, timeout)
	case config.UpstreamVless, config.UpstreamVmess:
		return dialViaSingBox(up, addr, timeout)
	}
	conn, err := net.DialTimeout("tcp", up.Host, timeout)
	if err != nil {
		return nil, fmt.Errorf("连接上游代理 %s 失败: %w", up.Host, err)
	}
	if up.Type == config.UpstreamSocks5 {
		if err := Socks5Handshake(conn, up.Username, up.Password, addr, timeout); err != nil {
			conn.Close()
			return nil, fmt.Errorf("上游 SOCKS5 握手失败: %w", err)
		}
		return conn, nil
	}
	bc, err := httpConnect(conn, up.Username, up.Password, addr, timeout)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return bc, nil
}

// httpConnect sends CONNECT to an upstream HTTP proxy and validates the 2xx reply.
// RFC 7231: a CONNECT reply has no body, so the header block is parsed
// manually — http.ReadResponse (with a nil request) would wrongly treat a
// bare 200 as body-until-EOF and block forever while the tunnel stays idle.
func httpConnect(conn net.Conn, user, pass, addr string, timeout time.Duration) (net.Conn, error) {
	conn.SetDeadline(time.Now().Add(timeout))

	fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n", addr, addr)
	if user != "" || pass != "" {
		fmt.Fprintf(conn, "Proxy-Authorization: Basic %s\r\n", basicAuth(user, pass))
	}
	fmt.Fprint(conn, "\r\n")

	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("读取上游 CONNECT 响应失败: %w", err)
	}
	parts := strings.Fields(status)
	if len(parts) < 2 {
		return nil, fmt.Errorf("上游响应格式错误: %q", strings.TrimSpace(status))
	}
	code, cerr := strconv.Atoi(parts[1])
	if cerr != nil {
		return nil, fmt.Errorf("上游响应状态码错误: %q", status)
	}
	if code < 200 || code > 299 {
		return nil, fmt.Errorf("上游代理拒绝 CONNECT: %s", strings.TrimSpace(status))
	}
	for {
		line, rerr := br.ReadString('\n')
		if rerr != nil {
			return nil, fmt.Errorf("读取上游响应头失败: %w", rerr)
		}
		if strings.TrimRight(line, "\r\n") == "" {
			break
		}
	}
	conn.SetDeadline(time.Time{})
	return &bufferedConn{br: br, Conn: conn}, nil
}

// Socks5Handshake performs a client-side SOCKS5 handshake through conn,
// tunnelling to addr. It is also used by the test suite.
func Socks5Handshake(conn net.Conn, user, pass, addr string, timeout time.Duration) error {
	conn.SetDeadline(time.Now().Add(timeout))
	defer conn.SetDeadline(time.Time{})

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("目标地址 %q 无效", addr)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return fmt.Errorf("目标端口 %q 无效", portStr)
	}

	greeting := []byte{5, 1, 0}
	if user != "" || pass != "" {
		greeting = []byte{5, 1, 2}
	}
	if _, err := conn.Write(greeting); err != nil {
		return err
	}
	head := make([]byte, 2)
	if _, err := io.ReadFull(conn, head); err != nil {
		return err
	}
	if head[0] != 5 {
		return errors.New("对方不是 SOCKS5 代理")
	}
	if head[1] == 0x02 {
		if user == "" && pass == "" {
			return errors.New("对方要求账号密码认证")
		}
		auth := []byte{1, byte(len(user))}
		auth = append(auth, user...)
		auth = append(auth, byte(len(pass)))
		auth = append(auth, pass...)
		if _, err := conn.Write(auth); err != nil {
			return err
		}
		ar := make([]byte, 2)
		if _, err := io.ReadFull(conn, ar); err != nil {
			return err
		}
		if ar[0] != 1 {
			return errors.New("SOCKS5 认证版本异常")
		}
		if ar[1] != 0 {
			return errors.New("SOCKS5 账号或密码错误")
		}
	} else if head[1] != 0 {
		return fmt.Errorf("SOCKS5 不支持的方法: 0x%02x", head[1])
	}

	req := []byte{5, 1, 0} // VER, CMD=CONNECT, RSV
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
			return errors.New("目标域名过长")
		}
		req = append(req, 3, byte(len(host)))
		req = append(req, host...)
	}
	var portBytes [2]byte
	binary.BigEndian.PutUint16(portBytes[:], uint16(port))
	req = append(req, portBytes[:]...)
	if _, err := conn.Write(req); err != nil {
		return err
	}

	rep := make([]byte, 4)
	if _, err := io.ReadFull(conn, rep); err != nil {
		return err
	}
	if rep[1] != 0 {
		return fmt.Errorf("SOCKS5 连接失败: %s", socks5ErrText(rep[1]))
	}
	switch rep[3] {
	case 1:
		rep = make([]byte, 4+2)
	case 3:
		if _, err := io.ReadFull(conn, rep[:1]); err != nil {
			return err
		}
		rep = make([]byte, int(rep[0])+2)
	case 4:
		rep = make([]byte, 16+2)
	default:
		return fmt.Errorf("SOCKS5 未知地址类型: %d", rep[3])
	}
	if _, err := io.ReadFull(conn, rep); err != nil {
		return err
	}
	return nil
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

func basicAuth(user, pass string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
}
