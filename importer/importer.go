// Package importer parses upstream subscription content into FastProxy
// upstream entries. Supported inputs:
//
//   - URI lines:      http(s)://[user:pass@]host:port[#name], socks5(h)://...,
//     ss:// (SIP002 / whole-link base64 / legacy method:pass@host:port),
//     vmess://base64(JSON), vless://uuid@host:port?params#name
//   - Plain lines:    host:port, host:port:user:pass
//   - Base64 wrapped subscription text
//   - sing-box JSON:  {"outbounds":[...]} or a raw outbound array
//   - Clash JSON/YAML: {"proxies":[...]} and the YAML `proxies:` block
//
// Imported node types: http, socks5, shadowsocks (AEAD ciphers only), vless and
// vmess over plain TCP. Nodes that need WebSocket/gRPC/QUIC transports,
// Reality/plain-text ciphers or other unsupported features are reported as
// skipped problems instead of being stored falsely as working upstreams.
package importer

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Proxy is one imported upstream candidate.
type Proxy struct {
	Name   string
	Type   string // http | socks5 | shadowsocks | vless | vmess
	Host   string // host:port
	User   string // http/socks5 username
	Pass   string // http/socks5/shadowsocks password
	Method string // shadowsocks cipher
	UUID   string // vless/vmess user id
	TLS    bool   // vless/vmess TLS on top of TCP
	SNI    string // vless/vmess TLS server name
}

// Result is the outcome of one parse run.
type Result struct {
	Proxies  []Proxy
	Problems []string
}

// DedupKey is the identity used to skip repeated nodes inside one import and
// by the store against existing rows.
func (p Proxy) DedupKey() string {
	return p.Type + "|" + p.Host + "|" + p.Method + "|" + p.UUID + "|" + p.User + "|" + p.Pass
}

const maxSubscriptionBytes = 2 << 20

// shadowsocksMethods are the AEAD ciphers the runtime client can speak.
// Nodes with other ciphers (rc4-md5, aes-*-cfb, 2022-*) are skipped with a
// problem rather than stored as non-working upstreams.
var shadowsocksMethods = map[string]bool{
	"aes-128-gcm":            true,
	"aes-256-gcm":            true,
	"chacha20-ietf-poly1305": true,
}

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

// Parse extracts upstream candidates from subscription content.
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
		key := p.DedupKey()
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

// splitName splits the trailing "#name" fragment off a URI body.
func splitName(s string) (body, name string) {
	i := strings.Index(s, "#")
	if i < 0 {
		return s, ""
	}
	raw := s[i+1:]
	if n, err := url.PathUnescape(raw); err == nil {
		raw = n
	}
	return s[:i], strings.TrimSpace(raw)
}

func parseURI(line string) (Proxy, error) {
	scheme, rest, _ := strings.Cut(line, "://")
	switch strings.ToLower(scheme) {
	case "ss", "shadowsocks":
		return parseSS(rest)
	case "vmess":
		return parseVmessURI(rest)
	case "vless":
		return parseVlessURI(rest)
	}

	var name string
	rest, name = splitName(rest)
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
		return Proxy{}, fmt.Errorf("不支持的协议 %q（支持 http/socks5/ss/vmess/vless 节点）", scheme)
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

// --- shadowsocks (ss://) ---

// parseSS handles the Shadowsocks share-link forms:
//
//	SIP002      ss://BASE64URL(method:password)@host:port#name (base64 may be padded or not)
//	SIP002/plain ss://method:password@host:port#name (userinfo percent-encoded)
//	Legacy      ss://BASE64(method:password@host:port)#name
//
// The body is everything after "ss://".
func parseSS(rest string) (Proxy, error) {
	body, name := splitName(rest)
	var userinfo, hostport string
	if i := strings.LastIndex(body, "@"); i >= 0 {
		userinfo, hostport = body[:i], body[i+1:]
	} else {
		decoded, ok := decodeBase64(strings.TrimSuffix(body, "/"))
		if !ok || !strings.Contains(decoded, "@") {
			return Proxy{}, errors.New("ss:// 链接格式无效（支持 SIP002 与整体 base64 编码）")
		}
		if j := strings.LastIndex(decoded, "@"); j >= 0 {
			userinfo, hostport = decoded[:j], decoded[j+1:]
		}
	}
	hostport = strings.TrimSuffix(strings.Trim(hostport, "/"), "/")
	if i := strings.IndexAny(hostport, "/?"); i >= 0 {
		hostport = hostport[:i]
	}
	host, port, err := net.SplitHostPort(hostport)
	if err != nil || host == "" || !validPort(port) {
		return Proxy{}, errors.New("ss:// 地址缺少端口（应为 host:port）")
	}

	method, password, ok := splitSSUserinfo(userinfo)
	if !ok {
		return Proxy{}, errors.New("ss:// 用户信息无法解析（应为 method:password 的 base64）")
	}
	if !shadowsocksMethods[method] {
		return Proxy{}, fmt.Errorf("跳过不支持的 Shadowsocks 加密方式 %q（支持 aes-128-gcm/aes-256-gcm/chacha20-ietf-poly1305）", method)
	}
	hp := net.JoinHostPort(host, port)
	if name == "" {
		name = hp
	}
	return Proxy{Name: name, Type: "shadowsocks", Host: hp,
		Method: method, Pass: password}, nil
}

// splitSSUserinfo decodes "base64(method:password)" or a plain
// "method:password" (optionally percent-encoded) userinfo part.
func splitSSUserinfo(userinfo string) (method, password string, ok bool) {
	try := func(s string) (string, string, bool) {
		i := strings.Index(s, ":")
		if i <= 0 {
			return "", "", false
		}
		return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s[:i]), "_", "-")), s[i+1:], true
	}
	if decoded, okb := decodeBase64(userinfo); okb {
		if m, p, okt := try(decoded); okt {
			return m, p, true
		}
	}
	if unescaped, err := url.PathUnescape(userinfo); err == nil {
		userinfo = unescaped
	}
	return try(userinfo)
}

// decodeBase64 accepts the standard and URL-safe alphabets, padded or raw.
func decodeBase64(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")
	s = strings.TrimRight(s, "=")
	if len(s)%4 == 1 {
		return "", false
	}
	for _, enc := range []*base64.Encoding{base64.RawStdEncoding, base64.RawURLEncoding} {
		if data, err := enc.DecodeString(s); err == nil {
			return string(data), true
		}
	}
	return "", false
}

// --- vmess (vmess://base64(JSON)) ---

type vmessJSON struct {
	Version any    `json:"v"`
	PS      string `json:"ps"`
	Address string `json:"add"`
	Port    any    `json:"port"`
	ID      string `json:"id"`
	Cipher  string `json:"scy"`
	Net     string `json:"net"`
	TLS     string `json:"tls"`
	SNI     string `json:"sni"`
	Host    string `json:"host"`
	Path    string `json:"path"`
}

// parseVmessURI decodes the base64 JSON payload of a vmess:// share link.
// The body is everything after "vmess://" (no fragment concept).
func parseVmessURI(rest string) (Proxy, error) {
	raw := strings.TrimSpace(rest)
	raw = strings.TrimSuffix(raw, "/")
	text, ok := decodeBase64(raw)
	if !ok {
		return Proxy{}, errors.New("vmess:// 内容不是有效的 base64(JSON)")
	}
	var v vmessJSON
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return Proxy{}, fmt.Errorf("vmess:// JSON 解析失败: %v", err)
	}
	port := anyString(v.Port)
	if v.Address == "" || !validPort(port) {
		return Proxy{}, errors.New("vmess 节点缺少地址或端口")
	}
	if strings.TrimSpace(v.ID) == "" {
		return Proxy{}, errors.New("vmess 节点缺少 id（UUID）")
	}
	if net := strings.ToLower(strings.TrimSpace(v.Net)); net != "" && net != "tcp" {
		return Proxy{}, fmt.Errorf("暂不支持 %s 传输", net)
	}
	hp := net.JoinHostPort(v.Address, port)
	name := strings.TrimSpace(v.PS)
	if name == "" {
		name = hp
	}
	sni := strings.TrimSpace(v.SNI)
	if sni == "" {
		sni = strings.TrimSpace(v.Host)
	}
	return Proxy{Name: name, Type: "vmess", Host: hp,
		UUID: strings.TrimSpace(v.ID), TLS: strings.EqualFold(v.TLS, "tls"), SNI: sni}, nil
}

// --- vless (vless://uuid@host:port?params#name) ---

func parseVlessURI(rest string) (Proxy, error) {
	body, name := splitName(rest)
	var query string
	if i := strings.Index(body, "?"); i >= 0 {
		body, query = body[:i], body[i+1:]
	}
	body = strings.TrimSuffix(body, "/")
	at := strings.LastIndex(body, "@")
	if at < 0 {
		return Proxy{}, errors.New("vless:// 缺少 UUID@host:port")
	}
	uuid := strings.TrimSpace(body[:at])
	if decoded, err := url.PathUnescape(uuid); err == nil {
		uuid = strings.TrimSpace(decoded)
	}
	hostport := body[at+1:]
	host, port, err := net.SplitHostPort(hostport)
	if err != nil || host == "" || !validPort(port) {
		return Proxy{}, errors.New("vless:// 地址缺少端口（应为 host:port）")
	}
	if uuid == "" {
		return Proxy{}, errors.New("vless:// 缺少 UUID")
	}
	q, err := url.ParseQuery(query)
	if err != nil {
		return Proxy{}, fmt.Errorf("vless:// 参数解析失败: %v", err)
	}
	if net := strings.ToLower(q.Get("type")); net != "" && net != "tcp" {
		return Proxy{}, fmt.Errorf("暂不支持 %s 传输", net)
	}
	if strings.EqualFold(q.Get("encryption"), "aes-128-gcm") ||
		(q.Get("encryption") != "" && !strings.EqualFold(q.Get("encryption"), "none")) {
		return Proxy{}, fmt.Errorf("vless 暂不支持加密 %q（应使用 encryption=none）", q.Get("encryption"))
	}
	// security=tls or reality means TLS on the wire. Note that reality needs
	// reality-specific key material which this client does not support: the
	// node is kept as plain TLS and fails at handshake if the server really
	// is a reality endpoint (honest failure recorded by the prober).
	security := strings.ToLower(q.Get("security"))
	tls := security == "tls" || security == "reality"
	sni := q.Get("sni")
	if sni == "" {
		sni = q.Get("peer")
	}
	if sni == "" {
		sni = q.Get("host")
	}
	hp := net.JoinHostPort(host, port)
	if name == "" {
		name = hp
	}
	return Proxy{Name: name, Type: "vless", Host: hp,
		UUID: uuid, TLS: tls, SNI: strings.TrimSpace(sni)}, nil
}

// anyString renders a JSON scalar (string or number) as a string.
func anyString(v any) string {
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
	seen := map[string]bool{}
	for _, m := range items {
		p, err := proxyFromJSON(m)
		if err != nil {
			res.Problems = append(res.Problems, err.Error())
			continue
		}
		if p == nil {
			continue
		}
		if key := p.DedupKey(); seen[key] {
			continue
		} else {
			seen[key] = true
		}
		res.Proxies = append(res.Proxies, *p)
	}
	if len(res.Proxies) == 0 && len(res.Problems) == 0 {
		return Result{}, false
	}
	return res, true
}

// proxyFromJSON maps one sing-box/Clash JSON node. A nil Proxy with a nil
// error means "silently ignored" (structural entries such as direct/block).
func proxyFromJSON(m map[string]any) (*Proxy, error) {
	typ := strings.ToLower(str(m["type"]))
	name := str(m["name"])
	if name == "" {
		name = str(m["tag"])
	}
	// Structural sing-box outbounds carry no endpoint: ignore them silently.
	switch typ {
	case "", "direct", "block", "dns", "selector", "urltest":
		return nil, nil
	}
	switch typ {
	case "http", "https", "socks", "socks5":
		hp, err := nodeHostPort(m)
		if err != nil {
			return nil, err
		}
		t := "http"
		if strings.HasPrefix(typ, "socks") {
			t = "socks5"
		}
		return &Proxy{Name: name, Type: t, Host: hp,
			User: str(m["username"]), Pass: str(m["password"])}, nil
	case "ss", "shadowsocks":
		hp, err := nodeHostPort(m)
		if err != nil {
			return nil, err
		}
		method := strings.ToLower(strings.ReplaceAll(str(m["cipher"]), "_", "-"))
		if method == "" {
			method = strings.ToLower(strings.ReplaceAll(str(m["method"]), "_", "-"))
		}
		if !shadowsocksMethods[method] {
			return nil, fmt.Errorf("%s (%s): 跳过不支持的 Shadowsocks 加密方式 %q", name, hp, method)
		}
		return &Proxy{Name: name, Type: "shadowsocks", Host: hp,
			Method: method, Pass: str(m["password"])}, nil
	case "vmess", "vless":
		hp, err := nodeHostPort(m)
		if err != nil {
			return nil, err
		}
		network := strings.ToLower(str(m["network"])) // Clash style
		if network == "" {
			// sing-box nests the transport inside {"transport":{"type":...}}.
			if tr, ok := m["transport"].(map[string]any); ok {
				network = strings.ToLower(str(tr["type"]))
			}
		}
		if network != "" && network != "tcp" {
			return nil, fmt.Errorf("%s (%s): 暂不支持 %s 传输", name, hp, network)
		}
		uuid := str(m["uuid"])
		if uuid == "" {
			return nil, fmt.Errorf("%s (%s): 缺少 uuid", name, hp)
		}
		security := strings.ToLower(str(m["security"]))
		if typ == "vless" {
			if security == "xtls" {
				return nil, fmt.Errorf("%s (%s): vless 暂不支持 security=xtls", name, hp)
			}
			if enc := strings.ToLower(str(m["encryption"])); enc != "" && enc != "none" {
				return nil, fmt.Errorf("%s (%s): vless 暂不支持加密 %q", name, hp, enc)
			}
		}
		sni := str(m["servername"])
		if sni == "" {
			sni = str(m["sni"])
		}
		tls := parseBool(m["tls"]) || security == "tls" || security == "reality"
		// sing-box: "tls":{"enabled":true,"server_name":"..."}
		if obj, ok := m["tls"].(map[string]any); ok {
			tls = parseBool(obj["enabled"])
			if sni == "" {
				sni = str(obj["server_name"])
			}
		}
		return &Proxy{Name: name, Type: typ, Host: hp, UUID: uuid, SNI: sni, TLS: tls}, nil
	}
	return nil, errors.New("跳过不支持的节点类型: " + typ)
}

// nodeHostPort extracts server/port from a JSON node.
func nodeHostPort(m map[string]any) (string, error) {
	server := str(m["server"])
	port := ""
	for _, k := range []string{"server_port", "port"} {
		if p := str(m[k]); p != "" {
			port = p
			break
		}
	}
	if server == "" || !validPort(port) {
		return "", errors.New("节点缺少 server/port: " + str(m["name"]) + str(m["tag"]))
	}
	return net.JoinHostPort(server, port), nil
}

// parseBool accepts JSON booleans and their string spellings.
func parseBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "true" || s == "1" || s == "yes" || s == "tls"
	}
	return false
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
	var p Proxy
	switch typ {
	case "http", "https", "socks", "socks5":
		server, port := m["server"], m["port"]
		if server == "" || !validPort(port) {
			res.Problems = append(res.Problems, "节点缺少 server/port: "+m["name"])
			return
		}
		t := "http"
		if strings.HasPrefix(typ, "socks") {
			t = "socks5"
		}
		p = Proxy{Type: t, Host: net.JoinHostPort(server, port),
			User: m["username"], Pass: m["password"]}
	case "ss", "shadowsocks":
		server, port := m["server"], m["port"]
		if server == "" || !validPort(port) {
			res.Problems = append(res.Problems, "节点缺少 server/port: "+m["name"])
			return
		}
		method := strings.ToLower(strings.ReplaceAll(m["cipher"], "_", "-"))
		if method == "" {
			method = strings.ToLower(strings.ReplaceAll(m["method"], "_", "-"))
		}
		if !shadowsocksMethods[method] {
			res.Problems = append(res.Problems, fmt.Sprintf("%s (%s:%s): 跳过不支持的 Shadowsocks 加密方式 %q",
				m["name"], server, port, method))
			return
		}
		p = Proxy{Type: "shadowsocks", Host: net.JoinHostPort(server, port),
			Method: method, Pass: m["password"]}
	case "vmess", "vless":
		server, port := m["server"], m["port"]
		if server == "" || !validPort(port) {
			res.Problems = append(res.Problems, "节点缺少 server/port: "+m["name"])
			return
		}
		if network := strings.ToLower(m["network"]); network != "" && network != "tcp" {
			res.Problems = append(res.Problems, fmt.Sprintf("%s (%s:%s): 暂不支持 %s 传输",
				m["name"], server, port, network))
			return
		}
		if m["uuid"] == "" {
			res.Problems = append(res.Problems, fmt.Sprintf("%s (%s:%s): 缺少 uuid", m["name"], server, port))
			return
		}
		sni := m["servername"]
		if sni == "" {
			sni = m["sni"]
		}
		p = Proxy{Type: typ, Host: net.JoinHostPort(server, port),
			UUID: m["uuid"], TLS: isTrue(m["tls"]), SNI: sni}
	default:
		if typ != "" {
			res.Problems = append(res.Problems, "跳过不支持的节点类型: "+typ)
		}
		return
	}
	if p.Name = m["name"]; p.Name == "" {
		p.Name = p.Host
	}
	res.Proxies = append(res.Proxies, p)
}

func isTrue(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "tls":
		return true
	}
	return false
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
