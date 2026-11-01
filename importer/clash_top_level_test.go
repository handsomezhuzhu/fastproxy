package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseClashTopLevelProxies documents the behaviour of the flat YAML
// reader for a `proxies:` list whose items are plain URI lines.
//
// A bare URI has no "k: v" pair, so it is neither collected as a node key nor
// recognised as a Clash node; such documents are handled by the URI-line
// parser instead (the Clash reader bails out and Parse falls through). Clash
// nodes in block style need their usual `- name: ...` skeleton.
func TestParseClashTopLevelProxies(t *testing.T) {
	proxies, err := os.ReadFile(filepath.Join("testdata", "clash_top_level.txt"))
	if err != nil {
		t.Fatal(err)
	}
	nodes := strings.Fields(strings.TrimSpace(string(proxies)))
	if len(nodes) != 20 {
		t.Fatalf("fixture should hold 20 lines, got %d", len(nodes))
	}

	// Same indent as the key and nested under it: both are not Clash nodes.
	for _, text := range []string{
		"proxies:\n- " + strings.Join(nodes, "\n- ") + "\n",
		"proxies:\n  - " + strings.Join(nodes, "\n  - ") + "\n",
	} {
		if r := Parse(text); len(r.Proxies) != 0 {
			t.Fatalf("bare URI items are not Clash nodes, got %d proxies", len(r.Proxies))
		}
	}

	// A block-style Clash node with the same REALITY fields is imported.
	text := `proxies:
  - name: node1
    type: vless
    server: d1.example.com
    port: 443
    uuid: b831381d-6324-4d53-ad4f-8cda48b30811
    network: tcp
    tls: true
    servername: s1.example.com
    security: reality
    client-fingerprint: chrome
    public-key: PKB94
    short-id: s94
`
	r := Parse(text)
	check(t, r, 1)
	p := r.Proxies[0]
	if p.Name != "node1" || p.Host != "d1.example.com:443" || p.Type != "vless" ||
		p.SNI != "s1.example.com" || p.Security != "reality" ||
		p.PBK != "PKB94" || p.SID != "s94" || p.Fingerprint != "chrome" {
		t.Fatalf("block-style reality node mismatch: %+v", p)
	}
	if len(r.Problems) != 0 {
		t.Fatalf("unexpected problems: %v", r.Problems)
	}
}

// TestParseClashDocumentWithPrefix covers a YAML document that starts with
// other top-level keys before `proxies:` (the common Clash export).
func TestParseClashDocumentWithPrefix(t *testing.T) {
	text := `port: 7890
socks-port: 7891
allow-lan: false
proxies:
  - name: "R1"
    type: vless
    server: 1.1.1.1
    port: 443
    uuid: b831381d-6324-4d53-ad4f-8cda48b30811
    network: tcp
    tls: true
    servername: a.example.com
    flow: xtls-rprx-vision
  - name: "VM"
    type: vmess
    server: 2.2.2.2
    port: 443
    uuid: b831381d-6324-4d53-ad4f-8cda48b30811
    network: ws
    tls: true
    path: /vm
    host: cdn.example.com
proxy-groups:
  - name: PROXY
    type: select
`
	r := Parse(text)
	check(t, r, 2)
	if r.Proxies[0].Name != "R1" || r.Proxies[0].Flow != "xtls-rprx-vision" ||
		r.Proxies[0].Security != "tls" || !r.Proxies[0].TLS {
		t.Fatalf("clash vision node mismatch: %+v", r.Proxies[0])
	}
	vm := r.Proxies[1]
	if vm.Name != "VM" || vm.Transport != "ws" || vm.Path != "/vm" ||
		vm.HostHeader != "cdn.example.com" || !vm.TLS {
		t.Fatalf("clash vmess node mismatch: %+v", vm)
	}
}
