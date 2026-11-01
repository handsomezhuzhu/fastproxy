package singbox

import (
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/certificate"
	"github.com/sagernet/sing-box/adapter/endpoint"
	"github.com/sagernet/sing-box/adapter/inbound"
	"github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/adapter/service"
	"github.com/sagernet/sing-box/dns"
	"github.com/sagernet/sing-box/dns/transport/local"
	"github.com/sagernet/sing-box/protocol/direct"
	"github.com/sagernet/sing-box/protocol/socks"
	"github.com/sagernet/sing-box/protocol/vless"
	"github.com/sagernet/sing-box/protocol/vmess"
)

// Registries is the sing-box type registry used for the embedded instance.
// Only the types FastProxy emits are registered (socks inbound; direct/socks/
// vless/vmess outbounds; local DNS), which keeps the binary free of the ~40
// other protocol packages pulled in by include.Context (tor, ssh, QUIC
// stacks, tun, wireguard, tailscale...).
type Registries struct {
	Inbounds     *inbound.Registry
	Outbounds    *outbound.Registry
	Endpoints    *endpoint.Registry
	DNS          *dns.TransportRegistry
	Services     *service.Registry
	Certificates *certificate.Registry
}

// ClientRegistries is the minimal set for the embedded client.
func ClientRegistries() Registries {
	inbounds := inbound.NewRegistry()
	socks.RegisterInbound(inbounds)

	outbounds := outbound.NewRegistry()
	direct.RegisterOutbound(outbounds)
	socks.RegisterOutbound(outbounds)
	vless.RegisterOutbound(outbounds)
	vmess.RegisterOutbound(outbounds)

	dnsRegistry := dns.NewTransportRegistry()
	local.RegisterTransport(dnsRegistry)

	return Registries{
		Inbounds:     inbounds,
		Outbounds:    outbounds,
		Endpoints:    endpoint.NewRegistry(),
		DNS:          dnsRegistry,
		Services:     service.NewRegistry(),
		Certificates: certificate.NewRegistry(),
	}
}

var (
	_ adapter.InboundRegistry             = (*inbound.Registry)(nil)
	_ adapter.OutboundRegistry            = (*outbound.Registry)(nil)
	_ adapter.DNSTransportRegistry        = (*dns.TransportRegistry)(nil)
	_ adapter.EndpointRegistry            = (*endpoint.Registry)(nil)
	_ adapter.ServiceRegistry             = (*service.Registry)(nil)
	_ adapter.CertificateProviderRegistry = (*certificate.Registry)(nil)
)
