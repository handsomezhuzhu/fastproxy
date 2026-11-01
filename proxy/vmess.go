package proxy

import (
	"errors"
	"net"
	"time"

	"github.com/handsomezhuzhu/fastproxy/config"
)

// ErrVmessUnsupported is kept for compatibility with older callers; VMess is
// now served by the embedded sing-box instance (see SingBoxDialer) and only
// reports this error when that instance was never initialised.
var ErrVmessUnsupported = errors.New(
	"vmess 需要内置 sing-box，但 sing-box 尚未初始化")

// dialVmess routes VMess through the embedded sing-box hook. It exists so the
// type is recognised even when callers bypass DialTunnel.
func dialVmess(up config.Upstream, addr string, timeout time.Duration) (net.Conn, error) {
	return dialViaSingBox(up, addr, timeout)
}

