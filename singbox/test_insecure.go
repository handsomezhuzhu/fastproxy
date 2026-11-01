package singbox

import (
	"github.com/sagernet/sing-box/option"

	"github.com/handsomezhuzhu/fastproxy/config"
)

// allowInsecure is false in production: the embedded client verifies
// certificates strictly. Test files flip it for the duration of a test, the
// same way proxy.testRoots injects a trusted pool for the native client.
var allowInsecure bool

// relaxTLS is applied by the manager right after options are built. It is a
// no-op in production builds and marks outbound TLS insecure when a test has
// set allowInsecure (self-signed sing-box server certificates).
func (m *Manager) relaxTLS(opts *option.Options) {
	if !allowInsecure {
		return
	}
	for i := range opts.Outbounds {
		if c, ok := opts.Outbounds[i].Options.(option.OutboundTLSOptionsWrapper); ok {
			if tls := c.TakeOutboundTLSOptions(); tls != nil {
				tls.Insecure = true
			}
		}
	}
}

var _ = config.Upstream{}
