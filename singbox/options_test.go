package singbox

import (
	"context"
	"testing"

	"github.com/handsomezhuzhu/fastproxy/config"
)

// TestBuildOptionsShape pins the generated sing-box configuration: one SOCKS
// inbound per upstream (loopback, fixed port), one vless/vmess outbound per
// upstream and an inbound-tagged route rule with final=direct.
func TestBuildOptionsShape(t *testing.T) {
	ups := []config.Upstream{
		{ID: "a", Name: "a", Type: config.UpstreamVless, Host: "1.2.3.4:443",
			UUID: testUUID, Security: config.SecurityReality, Flow: config.UpstreamFlowVision,
			PBK: "pk", SID: "01", SNI: "www.example.com"},
		{ID: "b", Name: "b", Type: config.UpstreamVmess, Host: "[2001:db8::1]:8443",
			UUID: testUUID, Security: config.SecurityTLS, Transport: "ws",
			Path: "/vm", HostHeader: "cdn.example.com"},
	}
	opts, err := buildOptions(ups, map[string]uint16{"a": 20000, "b": 20001})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := opts.MarshalJSONContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{
		`"type":"socks"`, `"listen":"127.0.0.1"`, `"listen_port":20000`, `"listen_port":20001`,
		`"type":"vless"`, `"type":"vmess"`, `"type":"direct"`,
		`"uuid":"` + testUUID + `"`, `"flow":"xtls-rprx-vision"`,
		`"utls":{"enabled":true,"fingerprint":"chrome"}`,
		`"reality":{"enabled":true,"public_key":"pk","short_id":"01"}`,
		`"transport":{"type":"ws","path":"/vm","headers":{"Host":"cdn.example.com"}}`,
		`"server":"2001:db8::1"`, `"server_port":8443`,
		`"inbound":"fp-in-a"`, `"outbound":"fp-out-a"`,
		`"inbound":"fp-in-b"`, `"outbound":"fp-out-b"`, `"final":"fastproxy-direct"`,
	} {
		if !contains(text, want) {
			t.Fatalf("generated options missing %s\n%s", want, text)
		}
	}

	// The full option set must decode through sing-box's strict parser.
	if _, err := boxOptionsFromJSON(t, opts); err != nil {
		t.Fatalf("sing-box rejected generated options: %v", err)
	}
	_ = context.Background()
}

// TestBuildOptionsRequiresPort: every upstream needs a local port.
func TestBuildOptionsRequiresPort(t *testing.T) {
	_, err := buildOptions([]config.Upstream{{ID: "x", Type: config.UpstreamVmess, Host: "h:1", UUID: testUUID}}, nil)
	if err == nil {
		t.Fatal("expected an error for a missing port")
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
