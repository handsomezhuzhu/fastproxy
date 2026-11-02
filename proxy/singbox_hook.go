package proxy

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/handsomezhuzhu/fastproxy/config"
)

// ErrVmessUnsupported is kept for compatibility with older callers; VLESS and
// VMess are now both served by the embedded sing-box instance (see
// SingBoxDialer) and only report this error when it was never initialised.
var ErrVmessUnsupported = errors.New(
	"vless/vmess 需要内置 sing-box，但 sing-box 尚未初始化")

// dialViaSingBox routes a VLESS/VMess connection through the embedded sing-box
// manager (each such upstream owns a loopback SOCKS inbound inside the box).
func dialViaSingBox(up config.Upstream, addr string, timeout time.Duration) (net.Conn, error) {
	dial := SingBoxDialer
	if dial == nil {
		return nil, fmt.Errorf("%w：上游 %s（%s）无法直连", ErrVmessUnsupported, up.Name, up.Type)
	}
	conn, err := dial(up, addr, timeout)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, errors.New("内置 sing-box 返回了空连接")
	}
	return conn, nil
}
