// Package importer parses Resin-style upstream subscription content into
// FastProxy upstream entries. Supported inputs:
//
//   - URI lines:      http(s)://[user:pass@]host:port[#name], socks5(h)://...
//   - Plain lines:    host:port, host:port:user:pass
//   - Base64 wrapped subscription text
//   - sing-box JSON:  {"outbounds":[...]} or a raw outbound array
//   - Clash JSON/YAML: {"proxies":[...]} and the YAML `proxies:` block
//
// Only http/https and socks5/socks nodes are imported; other node types are
// reported as skipped problems.
package importer

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Proxy is one imported upstream candidate.
type Proxy struct {
	Name string
	Type string // http | socks5
	Host string // host:port
	User string
	Pass string
}

// Result is the outcome of one parse run.
type Result struct {
	Proxies  []Proxy
	Problems []string
}

const maxSubscriptionBytes = 2 << 20

// Fetch downloads a subscription URL (http/https only).
func Fetch(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return "", errors.New("订阅链接必须以 http:// 或 https:// 开头")
	}
	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "FastProxy/1.0 (subscription import)")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("拉取订阅失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("订阅返回 %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSubscriptionBytes+1))
	if err != nil {
		return "", err
	}
	if len(body) > maxSubscriptionBytes {
		return "", errors.New("订阅内容超过 2MB 限制")
	}
	return string(body), nil
}

// Parse extracts http/socks5 upstream candidates from subscription content.
func Parse(text string) Result {
	text = strings.TrimSpace(strings.TrimPrefix(text, "\ufeff"))
	if text == "" {
		return Result{Problems: []string{"内容为空"}}
	}

	// JSON subscriptions (sing-box outbounds / Clash JSON / raw arrays).
	if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
		if r, ok := parseJSON(text); ok {
			return r
		}
	}

	// Clash YAML `proxies:` block.
	if strings.Contains(text, "proxies:") {
		if r, ok := parseClashYAML(text); ok {
			return r
		}
	}

	// Whole-subscription base64 encoding (common for URI line lists).
	if decoded, ok := tryBase64(text); ok {
		if strings.Contains(decoded, "://") || strings.Contains(decoded, ":") {
			r := Parse(decoded)
			return r
		}
	}

	return parseLines(text)
}

// --- line formats ---

func parseLines(text string) Result {
	var res Result
	seen := map[string]bool{}
	for i, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		p, err := parseLine(line)
		if err != nil {
			res.Problems = append(res.Problems, fmt.Sprintf("第 %d 行: %v", i+1, err))
			continue
		}
		key := p.Type + "|" + p.Host + "|" + p.User + "|" + p.Pass
		if seen[key] {
			continue
		}
		seen[key] = true
		res.Proxies = append(res.Proxies, p)
	}
	if len(res.Proxies) == 0 && len(res.Problems) == 0 {
		res.Problems = append(res.Problems, "未识别到任何上游节点")
	}
	return res
}

func parseLine(line string) (Proxy, error) {
	if strings.Contains(line, "://") {
		return parseURI(line)
	}
	if p, err := parsePlain(line); err == nil {
		return p, nil
	}
	return Proxy{}, errors.New("无法识别的格式（支持 scheme://user:pass@host:port、IP:PORT、IP:PORT:USER:PASS）")
}

func parseURI(line string) (Proxy, error) {
	scheme, rest, _ := strings.Cut(line, "://")
	var name string
	if i := strings.LastIndex(rest, "#"); i >= 0 {
		name = strings.TrimSpace(rest[i+1:])
		rest = rest[:i]
	}
	if i := strings.IndexAny(rest, "/?"); i >= 0 {
		rest = rest[:i]
	}
	var user, pass string
	if i := strings.LastIndex(rest, "@"); i >= 0 {
		userinfo := rest[:i]
		rest = rest[i+1:]
		if c := strings.Index(userinfo, ":"); c >= 0 {
			user, pass = userinfo[:c], userinfo[c+1:]
		} else {
			user = userinfo
		}
	}
	host, port, err := net.SplitHostPort(rest)
	if err != nil || host == "" {
		return Proxy{}, errors.New("地址缺少端口（应为 host:port）")
	}
	var typ string
	switch strings.ToLower(scheme) {
	case "http", "https":
		typ = "http"
	case "socks5", "socks5h", "socks":
		typ = "socks5"
	default:
		return Proxy{}, fmt.Errorf("不支持的协议 %q（仅导入 http/socks5 节点）", scheme)
	}
	hp := net.JoinHostPort(host, port)
	if name == "" {
		name = hp
	}
	return Proxy{Name: name, Type: typ, Host: hp, User: user, Pass: pass}, nil
}

func parsePlain(line string) (Proxy, error) {
	parts := strings.Split(line, ":")
	if len(parts) == 2 {
		host, port, err := net.SplitHostPort(line)
		if err != nil || !validPort(port) {
			return Proxy{}, errors.New("不是有效的 host:port")
		}
		hp := net.JoinHostPort(host, port)
		return Proxy{Name: hp, Type: "http", Host: hp}, nil
	}
	if len(parts) >= 4 {
		host, port := parts[0], parts[1]
		if host == "" || !validPort(port) {
			return Proxy{}, errors.New("不是有效的 IP:PORT:USER:PASS")
		}
		hp := net.JoinHostPort(host, port)
		return Proxy{Name: hp, Type: "http", Host: hp,
			User: parts[2], Pass: strings.Join(parts[3:], ":")}, nil
	}
	return Proxy{}, errors.New("不是有效的 host:port / host:port:user:pass")
}

func validPort(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n > 0 && n <= 65535
}

// --- base64 ---

func tryBase64(text string) (string, bool) {
	compact := strings.Join(strings.Fields(text), "")
	if compact == "" || len(compact) < 16 {
		return "", false
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if data, err := enc.DecodeString(compact); err == nil {
			s := string(data)
			if printable(s) {
				return s, true
			}
		}
	}
	return "", false
}

func printable(s string) bool {
	if len(s) == 0 {
		return false
	}
	bad := 0
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if r < 0x20 || r == 0x7f {
			bad++
		}
	}
	return bad*10 < len(s)
}

// --- JSON (sing-box / Clash) ---

func parseJSON(text string) (Result, bool) {
	var root any
	if err := json.Unmarshal([]byte(text), &root); err != nil {
		return Result{}, false
	}
	var items []map[string]any
	switch v := root.(type) {
	case []any:
		for _, it := range v {
			if m, ok := it.(map[string]any); ok {
				items = append(items, m)
			}
		}
	case map[string]any:
		for _, key := range []string{"outbounds", "proxies"} {
			if arr, ok := v[key].([]any); ok {
				items = items[:0]
				for _, it := range arr {
					if m, ok := it.(map[string]any); ok {
						items = append(items, m)
					}
				}
				break
			}
		}
	}
	if len(items) == 0 {
		return Result{}, false
	}
	var res Result
	for _, m := range items {
		typ := strings.ToLower(str(m["type"]))
		if typ != "http" && typ != "https" && typ != "socks" && typ != "socks5" {
			if typ != "" {
				res.Problems = append(res.Problems, "跳过非 http/socks5 节点: "+typ)
			}
			continue
		}
		server := str(m["server"])
		port := ""
		for _, k := range []string{"server_port", "port"} {
			if p := str(m[k]); p != "" {
				port = p
				break
			}
		}
		if server == "" || !validPort(port) {
			res.Problems = append(res.Problems, "节点缺少 server/port: "+str(m["name"])+str(m["tag"]))
			continue
		}
		name := str(m["name"])
		if name == "" {
			name = str(m["tag"])
		}
		hp := net.JoinHostPort(server, port)
		if name == "" {
			name = hp
		}
		t := "http"
		if strings.HasPrefix(typ, "socks") {
			t = "socks5"
		}
		res.Proxies = append(res.Proxies, Proxy{Name: name, Type: t, Host: hp,
			User: str(m["username"]), Pass: str(m["password"])})
	}
	if len(res.Proxies) == 0 && len(res.Problems) == 0 {
		return Result{}, false
	}
	return res, true
}

func str(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	}
	return ""
}

// --- Clash YAML subset ---

// parseClashYAML handles the `proxies:` list in both flow style
// (`- {name: x, type: http, ...}`) and block style (key: value lines).
func parseClashYAML(text string) (Result, bool) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	inProxies := false
	var res Result
	var cur map[string]string
	var curIndent int
	flush := func() {
		if cur != nil {
			appendClashItem(&res, cur)
			cur = nil
		}
	}
	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if !inProxies {
			if strings.HasPrefix(trimmed, "proxies:") {
				inProxies = true
			}
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		if strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
			flush()
			cur = map[string]string{}
			curIndent = indent
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
			if rest == "" {
				continue
			}
			if strings.HasPrefix(rest, "{") {
				for k, v := range parseFlowMap(rest) {
					cur[k] = v
				}
				flush()
				continue
			}
			if k, v, ok := cutKV(rest); ok {
				cur[k] = v
			}
			continue
		}
		if cur == nil || indent <= curIndent {
			flush()
			inProxies = false
			continue
		}
		if k, v, ok := cutKV(trimmed); ok {
			cur[k] = v
		}
	}
	flush()
	if len(res.Proxies) == 0 && len(res.Problems) == 0 {
		return Result{}, false
	}
	return res, true
}

func appendClashItem(res *Result, m map[string]string) {
	typ := strings.ToLower(m["type"])
	if typ != "http" && typ != "https" && typ != "socks" && typ != "socks5" {
		if typ != "" {
			res.Problems = append(res.Problems, "跳过非 http/socks5 节点: "+typ)
		}
		return
	}
	server, port := m["server"], m["port"]
	if server == "" || !validPort(port) {
		res.Problems = append(res.Problems, "节点缺少 server/port: "+m["name"])
		return
	}
	hp := net.JoinHostPort(server, port)
	name := m["name"]
	if name == "" {
		name = hp
	}
	t := "http"
	if strings.HasPrefix(typ, "socks") {
		t = "socks5"
	}
	res.Proxies = append(res.Proxies, Proxy{Name: name, Type: t, Host: hp,
		User: m["username"], Pass: m["password"]})
}

// parseFlowMap parses `{name: a, type: http, ...}` (comma split respects quotes).
func parseFlowMap(s string) map[string]string {
	s = strings.TrimSpace(strings.TrimPrefix(s, "{"))
	s = strings.TrimSuffix(s, "}")
	out := map[string]string{}
	var part strings.Builder
	inQuote := byte(0)
	flush := func() {
		if k, v, ok := cutKV(part.String()); ok {
			out[k] = v
		}
		part.Reset()
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote != 0 {
			part.WriteByte(c)
			if c == inQuote {
				inQuote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			inQuote = c
			part.WriteByte(c)
		case ',':
			flush()
		default:
			part.WriteByte(c)
		}
	}
	flush()
	return out
}

func cutKV(s string) (string, string, bool) {
	i := strings.Index(s, ":")
	if i < 0 {
		return "", "", false
	}
	k := strings.ToLower(strings.TrimSpace(s[:i]))
	v := strings.TrimSpace(s[i+1:])
	v = strings.Trim(v, `"'`)
	if k == "" {
		return "", "", false
	}
	return k, v, true
}
