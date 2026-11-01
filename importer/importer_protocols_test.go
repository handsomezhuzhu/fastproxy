package importer

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

// ssUserinfo builds base64(method:password) in the URL-safe form used by SIP002.
func ssUserinfo(t *testing.T, method, pass string) string {
	t.Helper()
	return base64.RawURLEncoding.EncodeToString([]byte(method + ":" + pass))
}

func TestParseSSSIP002(t *testing.T) {
	ui := ssUserinfo(t, "aes-256-gcm", "p@ss:word")
	// Padded base64 variant as well.
	padded := base64.StdEncoding.EncodeToString([]byte("chacha20-ietf-poly1305:pw"))

	text := strings.Join([]string{
		"ss://" + ui + "@1.2.3.4:8388#HK-%E8%8A%82%E7%82%B9",
		"ss://" + padded + "@[2001:db8::1]:8388#v6",
		"ss://aes-128-gcm:plain%40pw@5.6.7.8:9000#percent",
		"ss://aes-256-gcm:p@ss@9.9.9.9:8388", // last '@' wins, raw password
	}, "\n")
	r := Parse(text)
	check(t, r, 4)

	first := r.Proxies[0]
	if first.Type != "shadowsocks" || first.Host != "1.2.3.4:8388" ||
		first.Method != "aes-256-gcm" || first.Pass != "p@ss:word" || first.Name != "HK-节点" {
		t.Fatalf("SIP002 mismatch: %+v", first)
	}
	if r.Proxies[1].Host != "[2001:db8::1]:8388" || r.Proxies[1].Method != "chacha20-ietf-poly1305" {
		t.Fatalf("padded/IPv6 mismatch: %+v", r.Proxies[1])
	}
	if r.Proxies[2].Method != "aes-128-gcm" || r.Proxies[2].Pass != "plain@pw" {
		t.Fatalf("percent-encoded mismatch: %+v", r.Proxies[2])
	}
	if r.Proxies[3].Pass != "p@ss" || r.Proxies[3].Host != "9.9.9.9:8388" {
		t.Fatalf("raw password mismatch: %+v", r.Proxies[3])
	}
}

// TestParseSSWholeBase64 covers both legacy forms:
//
//	ss://base64(method:password@host:port)
//	ss://base64(method:password)@host:port   (SIP002 userinfo without padding)
func TestParseSSWholeBase64(t *testing.T) {
	legacy := base64.StdEncoding.EncodeToString([]byte("aes-256-gcm:legacyPass@3.3.3.3:8388"))
	r := Parse("ss://" + legacy + "#legacy")
	check(t, r, 1)
	if r.Proxies[0].Host != "3.3.3.3:8388" || r.Proxies[0].Method != "aes-256-gcm" ||
		r.Proxies[0].Pass != "legacyPass" || r.Proxies[0].Name != "legacy" {
		t.Fatalf("legacy whole-base64 mismatch: %+v", r.Proxies[0])
	}

	inner := "aes-128-gcm:sip002Pass"
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding} {
		r := Parse("ss://" + enc.EncodeToString([]byte(inner)) + "@4.4.4.4:8388")
		check(t, r, 1)
		if r.Proxies[0].Method != "aes-128-gcm" || r.Proxies[0].Pass != "sip002Pass" {
			t.Fatalf("userinfo base64 mismatch: %+v (input %q)", r.Proxies[0], inner)
		}
	}
}

func TestParseSSSkipsUnsupportedCipher(t *testing.T) {
	for _, method := range []string{"rc4-md5", "aes-256-cfb", "2022-blake3-aes-128-gcm"} {
		r := Parse("ss://" + ssUserinfo(t, method, "pw") + "@1.2.3.4:8388#x")
		if len(r.Proxies) != 0 || len(r.Problems) != 1 || !strings.Contains(r.Problems[0], method) {
			t.Fatalf("%s: expected skip problem, got %+v / %v", method, r.Proxies, r.Problems)
		}
	}
}

func TestParseSSMalformed(t *testing.T) {
	for _, line := range []string{
		"ss://",                                 // empty
		"ss://bm90IGEgdXNlcmluZm8@1.2.3.4:8388", // decodes without ':'
		"ss://" + ssUserinfo(t, "aes-128-gcm", "p") + "@1.2.3.4", // no port
	} {
		r := Parse(line)
		if len(r.Proxies) != 0 || len(r.Problems) == 0 {
			t.Fatalf("%q: expected problem, got %+v / %v", line, r.Proxies, r.Problems)
		}
	}
}

// vmessLink encodes a vmess:// share link from a JSON object.
func vmessLink(t *testing.T, obj map[string]any) string {
	t.Helper()
	b, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(b)
}

func TestParseVmessJSON(t *testing.T) {
	link := vmessLink(t, map[string]any{
		"v": "2", "ps": "VM-TLS", "add": "vm.example.com", "port": "443",
		"id": "b831381d-6324-4d53-ad4f-8cda48b30811", "scy": "auto",
		"net": "tcp", "tls": "tls", "sni": "sni.example.com", "host": "", "path": "",
	})
	r := Parse(link)
	check(t, r, 1)
	p := r.Proxies[0]
	if p.Type != "vmess" || p.Host != "vm.example.com:443" || p.Name != "VM-TLS" ||
		p.UUID != "b831381d-6324-4d53-ad4f-8cda48b30811" || !p.TLS || p.SNI != "sni.example.com" {
		t.Fatalf("vmess mismatch: %+v", p)
	}

	// Numeric port + no TLS + no sni (falls back to host field).
	link2 := vmessLink(t, map[string]any{
		"ps": "plain", "add": "1.1.1.1", "port": 8080, "id": "abc", "net": "tcp", "tls": "",
	})
	r2 := Parse(link2)
	check(t, r2, 1)
	if r2.Proxies[0].Host != "1.1.1.1:8080" || r2.Proxies[0].TLS {
		t.Fatalf("numeric port mismatch: %+v", r2.Proxies[0])
	}
}

func TestParseVmessRejectsNonTCP(t *testing.T) {
	for _, net := range []string{"ws", "grpc", "h2", "quic"} {
		link := vmessLink(t, map[string]any{
			"add": "h", "port": "443", "id": "u", "net": net, "tls": "tls",
		})
		r := Parse(link)
		if len(r.Proxies) != 0 || len(r.Problems) != 1 || !strings.Contains(r.Problems[0], net) {
			t.Fatalf("%s: expected skip problem, got %+v / %v", net, r.Proxies, r.Problems)
		}
	}
	r := Parse("vmess://not-base64!!")
	if len(r.Proxies) != 0 || len(r.Problems) == 0 {
		t.Fatalf("garbage vmess should be a problem: %+v / %v", r.Proxies, r.Problems)
	}
}

func TestParseVlessURI(t *testing.T) {
	text := strings.Join([]string{
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@vl.example.com:443" +
			"?encryption=none&security=tls&type=tcp&sni=cdn.example.com#VL-TLS",
		"vless://11111111-2222-3333-4444-555555555555@1.2.3.4:80?encryption=none&type=tcp#VL-plain",
		"vless://11111111-2222-3333-4444-555555555555@1.2.3.5:443?encryption=none&security=none#VL-notls",
		"vless://22222222-2222-3333-4444-555555555555@1.2.3.6:443?encryption=none&security=tls&type=ws&path=%2Fws#VL-ws",
		"vless://33333333-2222-3333-4444-555555555555@1.2.3.7:443?encryption=none&type=grpc#VL-grpc",
		"vless://44444444-2222-3333-4444-555555555555@1.2.3.8:443?encryption=aes-128-gcm&type=tcp#VL-enc",
	}, "\n")
	r := Parse(text)
	if len(r.Proxies) != 3 {
		t.Fatalf("expected 3 proxies, got %d: %+v (problems %v)", len(r.Proxies), r.Proxies, r.Problems)
	}
	first := r.Proxies[0]
	if first.Type != "vless" || first.Host != "vl.example.com:443" || !first.TLS ||
		first.SNI != "cdn.example.com" || first.UUID != "b831381d-6324-4d53-ad4f-8cda48b30811" ||
		first.Name != "VL-TLS" {
		t.Fatalf("vless mismatch: %+v", first)
	}
	if r.Proxies[1].TLS || r.Proxies[1].Host != "1.2.3.4:80" {
		t.Fatalf("plain vless mismatch: %+v", r.Proxies[1])
	}
	if r.Proxies[2].TLS {
		t.Fatalf("security=none must not enable TLS: %+v", r.Proxies[2])
	}
	if len(r.Problems) != 3 {
		t.Fatalf("expected 3 skipped problems (ws/grpc/encryption), got %v", r.Problems)
	}
	joined := strings.Join(r.Problems, "\n")
	for _, want := range []string{"ws", "grpc", "aes-128-gcm"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing problem mentioning %q: %v", want, r.Problems)
		}
	}
}

func TestParseVlessRealityMapsToTLS(t *testing.T) {
	r := Parse("vless://b831381d-6324-4d53-ad4f-8cda48b30811@r.example.com:443" +
		"?security=reality&type=tcp&sni=www.microsoft.com&pbk=x&sid=y#reality")
	check(t, r, 1)
	if !r.Proxies[0].TLS || r.Proxies[0].SNI != "www.microsoft.com" {
		t.Fatalf("reality should map to TLS: %+v", r.Proxies[0])
	}
}

func TestParseClashNewProtocols(t *testing.T) {
	text := `proxies:
  - {name: "SS-ok", type: ss, server: 3.3.3.3, port: 8388, cipher: aes-256-gcm, password: sspw}
  - {name: "SS-bad", type: ss, server: 3.3.3.4, port: 8388, cipher: rc4-md5, password: sspw}
  - name: "VL-01"
    type: vless
    server: 4.4.4.4
    port: 443
    uuid: b831381d-6324-4d53-ad4f-8cda48b30811
    network: tcp
    tls: true
    servername: cdn.example.com
  - name: "VM-ws"
    type: vmess
    server: 5.5.5.5
    port: 443
    uuid: b831381d-6324-4d53-ad4f-8cda48b30811
    network: ws
  - name: "VM-01"
    type: vmess
    server: 5.5.5.6
    port: 80
    uuid: b831381d-6324-4d53-ad4f-8cda48b30811
    network: tcp
    tls: false
  - {name: "HK", type: http, server: 1.1.1.1, port: 8080, username: u, password: p}
proxy-groups:
  - name: PROXY
`
	r := Parse(text)
	check(t, r, 4)
	if r.Proxies[0].Type != "shadowsocks" || r.Proxies[0].Method != "aes-256-gcm" || r.Proxies[0].Pass != "sspw" {
		t.Fatalf("clash ss mismatch: %+v", r.Proxies[0])
	}
	vl := r.Proxies[1]
	if vl.Type != "vless" || !vl.TLS || vl.SNI != "cdn.example.com" || vl.Host != "4.4.4.4:443" {
		t.Fatalf("clash vless mismatch: %+v", vl)
	}
	vm := r.Proxies[2]
	if vm.Type != "vmess" || vm.TLS || vm.Host != "5.5.5.6:80" {
		t.Fatalf("clash vmess mismatch: %+v", vm)
	}
	if r.Proxies[3].Type != "http" || r.Proxies[3].User != "u" {
		t.Fatalf("clash http mismatch: %+v", r.Proxies[3])
	}
	joined := strings.Join(r.Problems, "\n")
	if !strings.Contains(joined, "rc4-md5") || !strings.Contains(joined, "ws") {
		t.Fatalf("expected cipher + transport problems, got %v", r.Problems)
	}
}

func TestParseSingBoxNewProtocols(t *testing.T) {
	text := `{"outbounds":[
      {"type":"shadowsocks","tag":"SS","server":"1.1.1.1","server_port":8388,"method":"chacha20-ietf-poly1305","password":"x"},
      {"type":"vless","tag":"VL","server":"2.2.2.2","server_port":443,"uuid":"u-1","tls":{"enabled":true,"server_name":"s.example.com"},"transport":{"type":"tcp"}},
      {"type":"vmess","tag":"VM","server":"3.3.3.3","server_port":80,"uuid":"u-2","transport":{"type":"ws"}},
      {"type":"direct","tag":"direct"},
      {"type":"selector","tag":"sel","outbounds":["direct"]}
    ]}`
	r := Parse(text)
	check(t, r, 2)
	if r.Proxies[0].Type != "shadowsocks" || r.Proxies[0].Method != "chacha20-ietf-poly1305" {
		t.Fatalf("sing-box ss mismatch: %+v", r.Proxies[0])
	}
	vl := r.Proxies[1]
	if vl.Type != "vless" || !vl.TLS || vl.Host != "2.2.2.2:443" {
		t.Fatalf("sing-box vless mismatch: %+v", vl)
	}
	if len(r.Problems) != 1 || !strings.Contains(r.Problems[0], "ws") {
		t.Fatalf("expected ws skip for vmess, got %v", r.Problems)
	}
}

// TestParseBase64SubscriptionWithNewProtocols covers a whole base64-wrapped
// subscription containing every supported scheme.
func TestParseBase64SubscriptionWithNewProtocols(t *testing.T) {
	vm := vmessLink(t, map[string]any{"ps": "vm", "add": "6.6.6.6", "port": "443", "id": "u", "net": "tcp", "tls": "tls"})
	inner := strings.Join([]string{
		"http://a:b@1.1.1.1:80",
		"socks5://2.2.2.2:1080",
		"ss://" + ssUserinfo(t, "aes-256-gcm", "pw") + "@3.3.3.3:8388#ss",
		vm,
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@4.4.4.4:443?encryption=none&security=tls#vl",
	}, "\n")
	r := Parse(base64.StdEncoding.EncodeToString([]byte(inner)))
	check(t, r, 5)
	types := map[string]int{}
	for _, p := range r.Proxies {
		types[p.Type]++
	}
	for _, want := range []string{"http", "socks5", "shadowsocks", "vmess", "vless"} {
		if types[want] != 1 {
			t.Fatalf("type %s count = %d (all: %v)", want, types[want], types)
		}
	}
}

func TestParseInLineDedup(t *testing.T) {
	line := "ss://" + ssUserinfo(t, "aes-256-gcm", "pw") + "@3.3.3.3:8388"
	r := Parse(line + "\n" + line + "?plugin=x#dup")
	check(t, r, 1)
}

func TestSSUserinfoPercentDecodeMatches(t *testing.T) {
	// url.QueryEscape/PathUnescape round-trip on the plain form.
	pass := url.PathEscape("pa:ss/w@rd")
	r := Parse("ss://aes-128-gcm:" + pass + "@1.1.1.1:8388")
	check(t, r, 1)
	if r.Proxies[0].Pass != "pa:ss/w@rd" {
		t.Fatalf("percent decoding mismatch: %q", r.Proxies[0].Pass)
	}
}
