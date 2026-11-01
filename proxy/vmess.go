package proxy

import (
	"errors"
	"net"
	"time"

	"github.com/handsomezhuzhu/fastproxy/config"
)

// ErrVmessUnsupported is returned for vmess upstreams.
//
// About VMess: no small, dependable, pure-Go client library exists that
// implements VMess AEAD (the only variant modern servers accept — the legacy
// MD5/alt-header variant is deprecated and blocked by default). The realistic
// options are xray-core or sing-box, which would pull in a full proxy engine
// (30+ transitive modules, custom transports, config dialects) just for one
// egress protocol. Rather than ship untested crypto against an unimplemented
// library, vmess nodes are imported and stored as valid configuration but the
// tunnel reports this error. The health prober surfaces it as a failed check.
var ErrVmessUnsupported = errors.New(
	"vmess 暂未支持：已保存配置，但出站隧道尚未实现（当前支持 http/socks5/shadowsocks/vless）")

// dialVmess fails explicitly; it exists so DialTunnel recognises the type.
func dialVmess(config.Upstream, string, time.Duration) (net.Conn, error) {
	return nil, ErrVmessUnsupported
}
