package singbox

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter/inbound"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/protocol/socks"
	"github.com/sagernet/sing-box/protocol/vless"
	"github.com/sagernet/sing-box/protocol/vmess"
	"github.com/sagernet/sing/common/json"

	"github.com/handsomezhuzhu/fastproxy/config"
)

// serverRegistries is the client set plus the vless/vmess/socks inbounds the
// test sing-box servers ("remote nodes") need.
func serverRegistries() Registries {
	reg := ClientRegistries()
	registry := inbound.NewRegistry()
	socks.RegisterInbound(registry)
	vless.RegisterInbound(registry)
	vmess.RegisterInbound(registry)
	reg.Inbounds = registry
	return reg
}

func bufioReader(r interface{ Read([]byte) (int, error) }) *bufio.Reader {
	return bufio.NewReader(r)
}

// boxOptionsFromJSON validates and decodes raw JSON with sing-box's strict
// option parser (same path the shipped binary uses for config.json).
func boxOptionsFromJSON(t *testing.T, raw any) (*option.Options, error) {
	t.Helper()
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var opts option.Options
	if err := opts.UnmarshalJSONContext(serverRegistries().context(context.Background()), payload); err != nil {
		return nil, err
	}
	return &opts, nil
}

// testOptions builds client options with the test-only TLS relaxation.
func testOptions(t *testing.T, ups []config.Upstream, portOf map[string]uint16) *option.Options {
	t.Helper()
	allowInsecure = true
	t.Cleanup(func() { allowInsecure = false })
	opts, err := buildOptions(ups, portOf)
	if err != nil {
		t.Fatalf("buildOptions: %v", err)
	}
	return opts
}

// newHTTPTarget starts an httptest server answering "hello-through-singbox"
// and returns its host:port.
func newHTTPTarget(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello-through-singbox"))
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

// waitPortReady polls until the server inbound accepts TCP connections.
func waitPortReady(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("server %s never became ready", addr)
}
