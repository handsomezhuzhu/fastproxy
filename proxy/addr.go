package proxy

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
)

// SOCKS5 address type bytes (RFC 1928 §5). Shadowsocks and VLESS reuse the
// same encoding for their target address.
const (
	atypIPv4   = 0x01
	atypDomain = 0x03
	atypIPv6   = 0x04
)

// encodeAddr renders "host:port" as an SOCKS5-style address:
// ATYP + ADDR + 2-byte big-endian port. Domain names are sent as-is (so the
// proxy side resolves them), IP literals are sent as raw bytes.
func encodeAddr(addr string) ([]byte, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("目标地址 %q 无效: %w", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("目标端口 %q 无效", portStr)
	}
	buf := make([]byte, 0, 1+1+len(host)+2)
	switch ip := net.ParseIP(host); {
	case ip == nil:
		host = trimBrackets(host)
		if len(host) == 0 || len(host) > 255 {
			return nil, fmt.Errorf("目标域名 %q 无效", host)
		}
		buf = append(buf, atypDomain, byte(len(host)))
		buf = append(buf, host...)
	case ip.To4() != nil:
		buf = append(buf, atypIPv4)
		buf = append(buf, ip.To4()...)
	default:
		buf = append(buf, atypIPv6)
		buf = append(buf, ip.To16()...)
	}
	var pb [2]byte
	binary.BigEndian.PutUint16(pb[:], uint16(port))
	return append(buf, pb[:]...), nil
}

func trimBrackets(host string) string {
	if len(host) >= 2 && host[0] == '[' && host[len(host)-1] == ']' {
		return host[1 : len(host)-1]
	}
	return host
}

// readAddr parses one SOCKS5-style address from r (server side of the test
// doubles and protocol readers). It returns "host:port".
func readAddr(r io.Reader) (string, error) {
	var atyp [1]byte
	if _, err := io.ReadFull(r, atyp[:]); err != nil {
		return "", err
	}
	var host string
	switch atyp[0] {
	case atypIPv4:
		b := make([]byte, 4)
		if _, err := io.ReadFull(r, b); err != nil {
			return "", err
		}
		host = net.IP(b).String()
	case atypIPv6:
		b := make([]byte, 16)
		if _, err := io.ReadFull(r, b); err != nil {
			return "", err
		}
		host = net.IP(b).String()
	case atypDomain:
		var l [1]byte
		if _, err := io.ReadFull(r, l[:]); err != nil {
			return "", err
		}
		b := make([]byte, l[0])
		if _, err := io.ReadFull(r, b); err != nil {
			return "", err
		}
		host = string(b)
	default:
		return "", fmt.Errorf("未知地址类型 0x%02x", atyp[0])
	}
	var pb [2]byte
	if _, err := io.ReadFull(r, pb[:]); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(pb[:])))), nil
}
