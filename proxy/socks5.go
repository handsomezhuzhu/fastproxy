package proxy

import (
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

var errClientAuth = errors.New("客户端认证失败")

// serveSocks5 handles an inbound connection whose first byte identified it as
// SOCKS5. Username/password auth (RFC 1929) is mandatory: only method 0x02
// is ever offered, so unauthenticated clients cannot proceed.
func (e *entry) serveSocks5(cc *clientConn, ctr *countedConn) error {
	conn := cc.Conn
	head := make([]byte, 2) // VER (peeked but still buffered), NMETHODS
	if _, err := io.ReadFull(cc.br, head); err != nil {
		return err
	}
	if head[0] != 5 {
		return errors.New("socks5: bad version")
	}
	if head[1] == 0 {
		return errors.New("socks5: no methods offered")
	}
	methods := make([]byte, head[1])
	if _, err := io.ReadFull(cc.br, methods); err != nil {
		return err
	}
	supportsAuth := false
	for _, m := range methods {
		if m == 2 {
			supportsAuth = true
		}
	}
	if !supportsAuth {
		conn.Write([]byte{5, 0xFF}) // no acceptable methods
		return errors.New("socks5: 客户端不支持用户名/密码认证")
	}
	if _, err := conn.Write([]byte{5, 2}); err != nil {
		return err
	}

	// Username/password subnegotiation (RFC 1929).
	ver := make([]byte, 2) // VER, ULEN
	if _, err := io.ReadFull(cc.br, ver); err != nil {
		return err
	}
	if ver[0] != 1 {
		return errors.New("socks5: bad auth version")
	}
	user := make([]byte, ver[1])
	if _, err := io.ReadFull(cc.br, user); err != nil {
		return err
	}
	plen := make([]byte, 1)
	if _, err := io.ReadFull(cc.br, plen); err != nil {
		return err
	}
	pass := make([]byte, plen[0])
	if _, err := io.ReadFull(cc.br, pass); err != nil {
		return err
	}
	acc, ok := e.auth(ctr, string(user), string(pass))
	if !ok {
		conn.Write([]byte{1, 1})
		return errClientAuth
	}
	defer e.releaseAcc(ctr)
	if _, err := conn.Write([]byte{1, 0}); err != nil {
		return err
	}

	// Request: VER CMD RSV ATYP ...
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(cc.br, hdr); err != nil {
		return err
	}
	if hdr[0] != 5 {
		return errors.New("socks5: bad request version")
	}
	if hdr[1] != 1 { // only CONNECT
		conn.Write([]byte{5, 7, 0, 1, 0, 0, 0, 0, 0, 0})
		return fmt.Errorf("socks5: 不支持的命令 %d", hdr[1])
	}
	var host string
	switch hdr[3] {
	case 1: // IPv4
		b := make([]byte, 4)
		if _, err := io.ReadFull(cc.br, b); err != nil {
			return err
		}
		host = net.IP(b).String()
	case 4: // IPv6
		b := make([]byte, 16)
		if _, err := io.ReadFull(cc.br, b); err != nil {
			return err
		}
		host = net.IP(b).String()
	case 3: // domain
		l := make([]byte, 1)
		if _, err := io.ReadFull(cc.br, l); err != nil {
			return err
		}
		b := make([]byte, l[0])
		if _, err := io.ReadFull(cc.br, b); err != nil {
			return err
		}
		host = string(b)
	default:
		conn.Write([]byte{5, 8, 0, 1, 0, 0, 0, 0, 0, 0})
		return errors.New("socks5: 未知地址类型")
	}
	pb := make([]byte, 2)
	if _, err := io.ReadFull(cc.br, pb); err != nil {
		return err
	}
	addr := net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(pb))))

	up, err := dialTarget(e.upstreamFor(acc), addr)
	if err != nil {
		conn.Write([]byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0}) // general failure
		return err
	}
	if _, err := conn.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		up.Close()
		return err
	}
	conn.SetDeadline(time.Time{})
	relay(cc, up)
	return nil
}

// constEq is a constant-time string comparison helper.
func constEq(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
