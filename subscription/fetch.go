// Package subscription fetches and parses remote node lists (subscriptions) and
// reconciles them with the upstream pool.
package subscription

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	maxBodyBytes = 4 << 20 // 4 MiB of subscription text
	maxRedirects = 3
)

// FetchResult is one HTTP fetch of a subscription.
type FetchResult struct {
	Body         string
	ETag         string
	LastModified string
	NotModified  bool
}

// Fetch downloads a subscription. It only allows http(s), rejects private /
// loopback / link-local addresses (unless SUB_ALLOW_PRIVATE=1, useful for a
// local subconverter), follows at most three redirects and caps the body at
// 4 MiB.
func Fetch(ctx context.Context, rawURL, etag, lastModified string) (FetchResult, error) {
	var out FetchResult
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return out, fmt.Errorf("订阅地址必须是 http(s) URL")
	}
	if err := checkURL(ctx, u); err != nil {
		return out, err
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           safeDialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
	}
	client := &http.Client{
		Timeout:   25 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("订阅重定向超过 %d 次", maxRedirects)
			}
			return checkURL(req.Context(), req.URL)
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("User-Agent", "FastProxy/1.0 (+https://github.com/handsomezhuzhu/fastproxy)")
	req.Header.Set("Accept", "text/plain, application/json, application/yaml, */*")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}
	resp, err := client.Do(req)
	if err != nil {
		return out, fmt.Errorf("订阅拉取失败: %w", err)
	}
	defer resp.Body.Close()
	out.ETag = resp.Header.Get("ETag")
	out.LastModified = resp.Header.Get("Last-Modified")
	if resp.StatusCode == http.StatusNotModified {
		out.NotModified = true
		return out, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		hint := ""
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			hint = "（Token 失效、需要白名单，或服务商要求特定 User-Agent）"
		case http.StatusNotFound:
			hint = "（订阅地址不存在或已失效）"
		case http.StatusTooManyRequests:
			hint = "（请求过于频繁，请调大刷新间隔）"
		default:
			if resp.StatusCode >= 500 {
				hint = "（订阅服务商暂时不可用）"
			}
		}
		return out, fmt.Errorf("订阅拉取失败: HTTP %d %s%s", resp.StatusCode, http.StatusText(resp.StatusCode), hint)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return out, fmt.Errorf("读取订阅内容失败: %w", err)
	}
	if len(body) > maxBodyBytes {
		return out, fmt.Errorf("订阅内容超过 %d MiB，已拒绝", maxBodyBytes>>20)
	}
	out.Body = string(body)
	return out, nil
}

func allowPrivate() bool {
	return os.Getenv("SUB_ALLOW_PRIVATE") == "1"
}

func addressAllowed(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil && ip4.Equal(net.IPv4bcast) {
		return false
	}
	return true
}

// checkURL rejects non-http(s) and reserved destinations. DNS is resolved
// here for an early, readable error; safeDialContext repeats the check at dial
// time so DNS rebinding cannot slip through.
func checkURL(ctx context.Context, u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("订阅地址必须是 http(s) URL")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("订阅地址缺少主机名")
	}
	if allowPrivate() {
		return nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("订阅地址解析失败: %w", err)
	}
	for _, a := range addrs {
		if addressAllowed(a.IP) {
			return nil
		}
	}
	return fmt.Errorf("订阅地址指向内网/保留地址，已拒绝（如确需本地订阅请设置 SUB_ALLOW_PRIVATE=1）")
}

// safeDialContext resolves the host and dials only a public IP.
func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if allowPrivate() {
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, addr)
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("订阅地址解析失败: %w", err)
	}
	var lastErr error
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	for _, a := range addrs {
		if !addressAllowed(a.IP) {
			continue
		}
		conn, derr := dialer.DialContext(ctx, network, net.JoinHostPort(a.IP.String(), port))
		if derr == nil {
			return conn, nil
		}
		lastErr = derr
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("订阅地址指向内网/保留地址，已拒绝")
}
