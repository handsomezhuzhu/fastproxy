package proxy

import (
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/handsomezhuzhu/fastproxy/config"
)

// VLESS protocol constants (TCP transport only, no flow control).
const (
	vlessVersion = 0x00
	vlessCmdTCP  = 0x01
)

// dialVless opens a VLESS tunnel (TCP transport, optionally over TLS) to addr.
//
// Request header: [0x00 version][16B uuid][0x00 addonsLen][0x01 cmd=TCP]
// [2B port BE][ATYP][addr]. The response header [version][addonsLen][addons]
// is drained before the connection is handed back for raw relaying.
func dialVless(up config.Upstream, addr string, timeout time.Duration) (net.Conn, error) {
	uuid, err := parseVlessUUID(up.UUID)
	if err != nil {
		return nil, err
	}
	target, err := encodeAddr(addr)
	if err != nil {
		return nil, err
	}
	// VLESS moves the 2-byte big-endian port in front of ATYP, so the address
	// block written after the header is ATYP + ADDR only (no trailing port).
	if len(target) < 3 {
		return nil, fmt.Errorf("目标地址 %q 编码无效", addr)
	}
	port, target := target[len(target)-2:], target[:len(target)-2]
	raw, err := net.DialTimeout("tcp", up.Host, timeout)
	if err != nil {
		return nil, fmt.Errorf("连接上游代理 %s 失败: %w", up.Host, err)
	}
	conn := raw
	raw.SetDeadline(time.Now().Add(timeout))
	if up.TLS {
		tlsConn := tls.Client(raw, vlessTLSConfig(tlsServerName(up)))
		if err := tlsConn.Handshake(); err != nil {
			raw.Close()
			return nil, fmt.Errorf("VLESS TLS 握手失败: %w", err)
		}
		conn = tlsConn
	}

	header := make([]byte, 0, 19+len(target)+len(port))
	header = append(header, vlessVersion)
	header = append(header, uuid...)
	header = append(header, 0x00)        // addons length
	header = append(header, vlessCmdTCP) // command
	header = append(header, port...)
	header = append(header, target...)
	if n, err := conn.Write(header); err != nil || n != len(header) {
		conn.Close()
		if err == nil {
			return nil, fmt.Errorf("VLESS 请求头只写出 %d/%d 字节", n, len(header))
		}
		return nil, fmt.Errorf("VLESS 发送请求头失败: %w", err)
	}

	// Response header: version, addons length, addons.
	head := make([]byte, 2)
	if _, err := io.ReadFull(conn, head); err != nil {
		conn.Close()
		return nil, fmt.Errorf("VLESS 读取响应头失败（节点可能是 REALITY/XTLS Vision、UUID 错误或已离线）: %w", err)
	}
	if head[0] != vlessVersion {
		conn.Close()
		return nil, fmt.Errorf("VLESS 响应版本异常: 0x%02x", head[0])
	}
	if head[1] > 0 {
		if _, err := io.CopyN(io.Discard, conn, int64(head[1])); err != nil {
			conn.Close()
			return nil, fmt.Errorf("VLESS 读取响应附加数据失败: %w", err)
		}
	}
	raw.SetDeadline(time.Time{})
	return conn, nil
}

// vlessTLSConfig builds the client TLS configuration. Certificates are
// always verified against the system roots (no InsecureSkipVerify); tests
// inject a throwaway CA through testRoots.
func vlessTLSConfig(serverName string) *tls.Config {
	return &tls.Config{
		ServerName: serverName,
		MinVersion: tls.VersionTLS12,
		RootCAs:    testRoots, // nil outside *_test.go
	}
}

// tlsServerName resolves the TLS SNI: explicit sni, else the host name part
// of the upstream address (IP literals yield "" which disables SNI).
func tlsServerName(up config.Upstream) string {
	if up.SNI != "" {
		return up.SNI
	}
	host, _, err := net.SplitHostPort(up.Host)
	if err != nil {
		return ""
	}
	host = trimBrackets(host)
	if net.ParseIP(host) != nil {
		return ""
	}
	return host
}

// parseVlessUUID accepts the canonical 8-4-4-4-12 form or 32 hex chars.
func parseVlessUUID(s string) ([]byte, error) {
	clean := make([]byte, 0, 32)
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '-':
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
			'a', 'b', 'c', 'd', 'e', 'f', 'A', 'B', 'C', 'D', 'E', 'F':
			clean = append(clean, c)
		default:
			return nil, fmt.Errorf("UUID %q 含非法字符", s)
		}
	}
	if len(clean) != 32 {
		return nil, fmt.Errorf("UUID %q 长度无效（应为 36 位标准格式）", s)
	}
	out := make([]byte, 16)
	if _, err := hex.Decode(out, clean); err != nil {
		return nil, fmt.Errorf("UUID %q 无效: %w", s, err)
	}
	return out, nil
}
