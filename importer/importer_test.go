package importer

import (
	"strings"
	"testing"
)

func check(t *testing.T, got Result, wantN int) {
	t.Helper()
	if len(got.Proxies) != wantN {
		t.Fatalf("expected %d proxies, got %d (problems: %v)", wantN, len(got.Proxies), got.Problems)
	}
}

func TestParseURILines(t *testing.T) {
	text := `# comment
http://u:p@1.2.3.4:8080#US-01
https://5.6.7.8:8443
socks5://x:y@9.9.9.9:1080
socks5h://9.9.9.8:1080
`
	r := Parse(text)
	check(t, r, 4)
	if r.Proxies[0].Type != "http" || r.Proxies[0].Name != "US-01" || r.Proxies[0].User != "u" || r.Proxies[0].Pass != "p" {
		t.Fatalf("uri parse mismatch: %+v", r.Proxies[0])
	}
	if r.Proxies[1].Type != "http" || r.Proxies[1].Host != "5.6.7.8:8443" {
		t.Fatalf("https mapping mismatch: %+v", r.Proxies[1])
	}
	if r.Proxies[2].Type != "socks5" || r.Proxies[2].User != "x" {
		t.Fatalf("socks5 mismatch: %+v", r.Proxies[2])
	}
	if r.Proxies[3].Type != "socks5" || r.Proxies[3].Host != "9.9.9.8:1080" {
		t.Fatalf("socks5h mismatch: %+v", r.Proxies[3])
	}
}

func TestParsePlainLines(t *testing.T) {
	text := "1.2.3.4:8080\n5.6.7.8:8081:user:pa:ss\n"
	r := Parse(text)
	check(t, r, 2)
	if r.Proxies[0].Type != "http" || r.Proxies[0].Host != "1.2.3.4:8080" {
		t.Fatalf("plain mismatch: %+v", r.Proxies[0])
	}
	if r.Proxies[1].User != "user" || r.Proxies[1].Pass != "pa:ss" {
		t.Fatalf("plain creds mismatch: %+v", r.Proxies[1])
	}
}

func TestParseBase64Subscription(t *testing.T) {
	inner := "http://a:b@1.1.1.1:80\nsocks5://2.2.2.2:1080\n"
	b64 := base64Of(inner)
	r := Parse(b64)
	check(t, r, 2)
}

func TestParseSingBoxJSON(t *testing.T) {
	text := `{"outbounds":[
      {"type":"http","server":"1.2.3.4","server_port":8080,"username":"u","password":"p","tag":"US"},
      {"type":"socks","server":"5.6.7.8","server_port":1080,"tag":"S"},
      {"type":"vmess","server":"x","server_port":443,"tag":"V"}
    ]}`
	r := Parse(text)
	// vmess without a uuid cannot work: skipped with an explicit problem.
	check(t, r, 2)
	if r.Proxies[0].Name != "US" || r.Proxies[0].Type != "http" || r.Proxies[0].Host != "1.2.3.4:8080" {
		t.Fatalf("sing-box mismatch: %+v", r.Proxies[0])
	}
	if r.Proxies[1].Type != "socks5" {
		t.Fatalf("socks mapping mismatch: %+v", r.Proxies[1])
	}
	if len(r.Problems) != 1 || !strings.Contains(r.Problems[0], "uuid") {
		t.Fatalf("expected skipped vmess (no uuid) problem, got %v", r.Problems)
	}
}

func TestParseClashFlow(t *testing.T) {
	text := `proxies:
  - {name: "HK-01", type: http, server: 1.1.1.1, port: 8080, username: u, password: p}
  - {name: "SG-01", type: socks5, server: 2.2.2.2, port: 1080}
  - {name: "SS-01", type: ss, server: 3.3.3.3, port: 8388, cipher: x}
proxy-groups:
  - name: PROXY
`
	r := Parse(text)
	// Bare ss with an unknown cipher is still skipped (with a problem).
	check(t, r, 2)
	if r.Proxies[0].Name != "HK-01" || r.Proxies[0].User != "u" {
		t.Fatalf("clash flow mismatch: %+v", r.Proxies[0])
	}
	if r.Proxies[1].Type != "socks5" || r.Proxies[1].Host != "2.2.2.2:1080" {
		t.Fatalf("clash socks mismatch: %+v", r.Proxies[1])
	}
	if len(r.Problems) == 0 {
		t.Fatal("expected skipped ss problem")
	}
}

func TestParseClashBlock(t *testing.T) {
	text := `proxies:
  - name: "HK-02"
    type: http
    server: 1.1.1.2
    port: 8080
    username: uu
    password: pp
  - name: "US-02"
    type: socks5
    server: 2.2.2.3
    port: 1080
`
	r := Parse(text)
	check(t, r, 2)
	if r.Proxies[0].User != "uu" || r.Proxies[0].Pass != "pp" || r.Proxies[0].Host != "1.1.1.2:8080" {
		t.Fatalf("clash block mismatch: %+v", r.Proxies[0])
	}
	if r.Proxies[1].Type != "socks5" {
		t.Fatalf("clash block socks mismatch: %+v", r.Proxies[1])
	}
}

func TestParseSkipsUnsupported(t *testing.T) {
	r := Parse("vmess://abc\ntrojan://x@y:443\n")
	if len(r.Proxies) != 0 {
		t.Fatalf("expected no proxies, got %+v", r.Proxies)
	}
	if len(r.Problems) == 0 {
		t.Fatal("expected problems")
	}
}

func TestParseEmpty(t *testing.T) {
	r := Parse("   ")
	if len(r.Problems) == 0 {
		t.Fatal("expected empty-content problem")
	}
}

func base64Of(s string) string {
	const tbl = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var b []byte
	data := []byte(s)
	for i := 0; i < len(data); i += 3 {
		var n uint32
		rem := len(data) - i
		n = uint32(data[i]) << 16
		if rem > 1 {
			n |= uint32(data[i+1]) << 8
		}
		if rem > 2 {
			n |= uint32(data[i+2])
		}
		b = append(b, tbl[(n>>18)&63], tbl[(n>>12)&63])
		if rem > 1 {
			b = append(b, tbl[(n>>6)&63])
		} else {
			b = append(b, '=')
		}
		if rem > 2 {
			b = append(b, tbl[n&63])
		} else {
			b = append(b, '=')
		}
	}
	return string(b)
}
