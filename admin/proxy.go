package admin

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
)

// TrustedProxiesEnv names the comma-separated CIDR list of reverse proxies
// whose forwarding headers are honoured (TRUSTED_PROXIES).
const TrustedProxiesEnv = "TRUSTED_PROXIES"

// ParseTrustedProxies parses "127.0.0.1/32, 172.16.0.0/12". A bare IP is
// treated as a single-host CIDR. An empty string yields no trusted proxies.
func ParseTrustedProxies(raw string) ([]*net.IPNet, error) {
	var out []*net.IPNet
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.Contains(part, "/") {
			if ip := net.ParseIP(part); ip == nil {
				return nil, fmt.Errorf("TRUSTED_PROXIES 中的 %q 不是合法 IP 或 CIDR", part)
			}
			bits := 32
			if strings.Contains(part, ":") {
				bits = 128
			}
			part = fmt.Sprintf("%s/%d", part, bits)
		}
		_, n, err := net.ParseCIDR(part)
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXIES 中的 %q 不是合法 CIDR: %w", part, err)
		}
		out = append(out, n)
	}
	return out, nil
}

// TrustedProxiesFromEnv reads and parses TRUSTED_PROXIES.
func TrustedProxiesFromEnv() ([]*net.IPNet, error) {
	return ParseTrustedProxies(os.Getenv(TrustedProxiesEnv))
}

// SetTrustedProxies installs the reverse proxies whose X-Forwarded-* headers
// are trusted. An empty list means "trust nobody" (headers ignored).
func (s *Server) SetTrustedProxies(nets []*net.IPNet) {
	s.proxyMu.Lock()
	defer s.proxyMu.Unlock()
	s.proxies = append([]*net.IPNet(nil), nets...)
}

// isTrustedProxy reports whether ip belongs to a configured trusted proxy.
func (s *Server) isTrustedProxy(ip net.IP) bool {
	if ip == nil {
		return false
	}
	s.proxyMu.RLock()
	defer s.proxyMu.RUnlock()
	for _, n := range s.proxies {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// clientInfo is the resolved view of the request's origin.
type clientInfo struct {
	addr    string // client address (no port)
	trusted bool   // request came from a trusted proxy
	proto   string // "http"/"https"
	xfp     string // X-Forwarded-Proto as received (only when trusted)
}

// remoteIP parses the TCP peer address of the request.
func remoteIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	host = strings.Trim(host, "[]")
	return net.ParseIP(host)
}

// clientInfo resolves the client IP and scheme.
//
// When the TCP peer is a trusted proxy, the client is the right-most
// X-Forwarded-For hop that is not itself a trusted proxy (a client may append
// bogus hops on the left; only the right side is written by our proxies).
// X-Forwarded-Proto is honoured only in that case as well.
func (s *Server) clientInfo(r *http.Request) clientInfo {
	peerIP := remoteIP(r)
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		peer = strings.Trim(host, "[]")
	}
	info := clientInfo{addr: peer, proto: schemeOf(r)}
	if !s.isTrustedProxy(peerIP) {
		return info
	}
	info.trusted = true
	if xfp := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); xfp != "" {
		x := strings.ToLower(strings.TrimSpace(strings.Split(xfp, ",")[0]))
		if x == "https" || x == "http" {
			info.xfp = x
			info.proto = x
		}
	}
	if client := forwardedClient(r.Header.Values("X-Forwarded-For"), peerIP, s.isTrustedProxy); client != "" {
		info.addr = client
	}
	return info
}

// forwardedClient walks the X-Forwarded-For hops from right to left and returns
// the first address that is not a trusted proxy (governance: "right-most
// untrusted"). The direct peer is the starting point, so an empty or fully
// trusted chain falls back to it.
func forwardedClient(headerValues []string, peer net.IP, trusted func(net.IP) bool) string {
	var hops []string
	for _, v := range headerValues {
		for _, h := range strings.Split(v, ",") {
			if h = strings.TrimSpace(h); h != "" {
				hops = append(hops, h)
			}
		}
	}
	for i := len(hops) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.Trim(hops[i], "[]"))
		if ip == nil {
			// Unparsable hop: stop; everything further left is untrustworthy.
			break
		}
		if !trusted(ip) {
			return ip.String()
		}
	}
	if peer != nil {
		return peer.String()
	}
	return ""
}

// schemeOf reports the scheme of the direct connection.
func schemeOf(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// requestIsHTTPS reports whether the client reached the panel over HTTPS,
// either directly (TLS) or through a trusted proxy with X-Forwarded-Proto:
// https. Cookies are marked Secure in that case.
func (s *Server) requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	// Only a trusted proxy may claim HTTPS; an untrusted peer's XFP is ignored.
	return s.clientInfo(r).xfp == "https"
}
