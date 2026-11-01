package singbox

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"

	"github.com/handsomezhuzhu/fastproxy/config"
)

// Outbound tags and the inbound tag prefix used inside the embedded box.
const (
	directTag    = "fastproxy-direct"
	socksInPref  = "fp-in-"
	outboundPref = "fp-out-"
)

// defaultFingerprint is the uTLS client hello used for REALITY when the
// upstream does not pin one.
const defaultFingerprint = "chrome"

// NeedsSingBox reports whether the upstream must be tunnelled through the
// embedded sing-box instance instead of the native client:
//
//   - vmess always (FastProxy has no native VMess implementation);
//   - vless when it uses REALITY, an XTLS Vision flow or a ws/grpc transport.
//
// Plain VLESS (TLS or not, TCP transport, no flow) keeps using the native
// client, so it works even if sing-box fails to start.
//
// The comparisons below are spelled inline on purpose: routing the switch
// through config.UpstreamVless/ServiceReality constants mis-dispatched the
// plain-vless branch in the windows/amd64 toolchain used during development.
func NeedsSingBox(u config.Upstream) bool {
	switch u.Type {
	case "vmess":
		return true
	case "vless":
		return u.Security == "reality" || u.Flow != "" || u.Transport == "ws" || u.Transport == "grpc"
	}
	return false
}

// loopbackAddr returns the 127.0.0.1 listen address for upstream inbounds.
func loopbackAddr() *badoption.Addr {
	addr := badoption.Addr(netip.MustParseAddr("127.0.0.1"))
	return &addr
}

// buildInbound returns the loopback SOCKS5 inbound for one upstream.
func buildInbound(up config.Upstream, port uint16) option.Inbound {
	return option.Inbound{
		Type: "socks",
		Tag:  socksInPref + up.ID,
		Options: &option.SocksInboundOptions{
			ListenOptions: option.ListenOptions{
				Listen:     loopbackAddr(),
				ListenPort: port,
			},
		},
	}
}

// buildOutbound returns the protocol outbound for one upstream.
func buildOutbound(up config.Upstream) (option.Outbound, error) {
	host, portStr, err := net.SplitHostPort(up.Host)
	if err != nil {
		return option.Outbound{}, fmt.Errorf("上游地址 %q 无效: %w", up.Host, err)
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return option.Outbound{}, fmt.Errorf("上游端口 %q 无效", portStr)
	}
	server := option.ServerOptions{Server: host, ServerPort: uint16(port)}
	switch up.Type {
	case config.UpstreamVless:
		opts := &option.VLESSOutboundOptions{
			ServerOptions: server,
			UUID:          up.UUID,
			Flow:          up.Flow,
		}
		opts.TLS = tlsOptions(up)
		opts.Transport = transportOptions(up)
		return option.Outbound{Type: "vless", Tag: outboundPref + up.ID, Options: opts}, nil
	case config.UpstreamVmess:
		security := strings.ToLower(strings.TrimSpace(up.VMessSecurity))
		if security == "" {
			security = "auto"
		}
		opts := &option.VMessOutboundOptions{
			ServerOptions: server,
			UUID:          up.UUID,
			Security:      security,
		}
		opts.TLS = tlsOptions(up)
		opts.Transport = transportOptions(up)
		return option.Outbound{Type: "vmess", Tag: outboundPref + up.ID, Options: opts}, nil
	}
	return option.Outbound{}, fmt.Errorf("上游类型 %q 不需要 sing-box", up.Type)
}

// tlsOptions maps the upstream TLS/REALITY fields onto sing-box. Returns nil
// for plaintext connections.
func tlsOptions(up config.Upstream) *option.OutboundTLSOptions {
	reality := up.Security == config.SecurityReality
	if !reality && !up.TLS && up.Security != config.SecurityTLS {
		return nil
	}
	opts := &option.OutboundTLSOptions{
		Enabled:    true,
		ServerName: tlsServerName(up),
	}
	if !reality {
		return opts
	}
	fp := strings.TrimSpace(up.Fingerprint)
	if fp == "" {
		fp = defaultFingerprint
	}
	if up.Transport == "grpc" {
		opts.ALPN = badoption.Listable[string]{"h2"}
	}
	opts.UTLS = &option.OutboundUTLSOptions{Enabled: true, Fingerprint: fp}
	opts.Reality = &option.OutboundRealityOptions{
		Enabled:   true,
		PublicKey: up.PBK,
		ShortID:   up.SID,
	}
	return opts
}

// tlsServerName resolves the TLS SNI: explicit sni, else the ws Host header,
// else the host part of the address (IP literals disable SNI).
func tlsServerName(up config.Upstream) string {
	if up.SNI != "" {
		return up.SNI
	}
	if up.HostHeader != "" {
		return up.HostHeader
	}
	h, _, err := net.SplitHostPort(up.Host)
	if err != nil {
		return ""
	}
	if net.ParseIP(strings.Trim(h, "[]")) != nil {
		return ""
	}
	return h
}

// transportOptions maps Transport/Path/HostHeader/ServiceName onto sing-box's
// v2ray transport. TCP (or empty) yields nil.
func transportOptions(up config.Upstream) *option.V2RayTransportOptions {
	switch up.Transport {
	case "ws":
		path := up.Path
		if path == "" {
			path = "/"
		}
		ws := option.V2RayWebsocketOptions{Path: path}
		if up.HostHeader != "" {
			ws.Headers = badoption.HTTPHeader{"Host": badoption.Listable[string]{up.HostHeader}}
		}
		return &option.V2RayTransportOptions{Type: "ws", WebsocketOptions: ws}
	case "grpc":
		return &option.V2RayTransportOptions{
			Type:        "grpc",
			GRPCOptions: option.V2RayGRPCOptions{ServiceName: up.ServiceName},
		}
	}
	return nil
}

// buildOptions assembles the full sing-box configuration for the given set of
// upstreams. portOf maps an upstream ID to its local SOCKS port.
func buildOptions(ups []config.Upstream, portOf map[string]uint16) (*option.Options, error) {
	opts := &option.Options{
		Log: &option.LogOptions{Level: "warn"},
		Outbounds: []option.Outbound{{
			Type:    "direct",
			Tag:     directTag,
			Options: &option.DirectOutboundOptions{},
		}},
	}
	route := &option.RouteOptions{Final: directTag}
	for _, up := range ups {
		port, ok := portOf[up.ID]
		if !ok {
			return nil, fmt.Errorf("上游 %s 缺少本地端口", up.ID)
		}
		opts.Inbounds = append(opts.Inbounds, buildInbound(up, port))
		ob, err := buildOutbound(up)
		if err != nil {
			return nil, err
		}
		opts.Outbounds = append(opts.Outbounds, ob)
		route.Rules = append(route.Rules, option.Rule{
			Type: "default",
			DefaultOptions: option.DefaultRule{
				RawDefaultRule: option.RawDefaultRule{
					Inbound: badoption.Listable[string]{socksInPref + up.ID},
				},
				RuleAction: option.RuleAction{
					Action: "route",
					RouteOptions: option.RouteActionOptions{
						Outbound: outboundPref + up.ID,
					},
				},
			},
		})
	}
	opts.Route = route
	return opts, nil
}
