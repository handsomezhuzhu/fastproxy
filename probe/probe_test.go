package probe

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/handsomezhuzhu/fastproxy/config"
)

// fakeHTTPProxy is a minimal HTTP CONNECT proxy.
func fakeHTTPProxy(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil {
					return
				}
				if req.Method != http.MethodConnect {
					return
				}
				up, err := net.Dial("tcp", req.Host)
				if err != nil {
					return
				}
				defer up.Close()
				fmt.Fprint(c, "HTTP/1.1 200 Connection Established\r\n\r\n")
				cc := &bufferedConn{br: br, Conn: c}
				done := make(chan struct{}, 2)
				go func() { io.Copy(up, cc); done <- struct{}{} }()
				go func() { io.Copy(cc, up); done <- struct{}{} }()
				<-done
			}(conn)
		}
	}()
	return ln.Addr().String()
}

type bufferedConn struct {
	br *bufio.Reader
	net.Conn
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.br.Read(p) }

// fakeIPServer replies with a fixed IP body.
func fakeIPServer(t *testing.T) string {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, "203.0.113.9\n")
	}))
	t.Cleanup(ts.Close)
	return strings.TrimPrefix(ts.URL, "http://")
}

func TestCheckAddrDirect(t *testing.T) {
	addr := fakeIPServer(t)
	res := CheckAddr(nil, addr, 5*time.Second, false)
	if !res.OK {
		t.Fatalf("direct probe failed: %s", res.Error)
	}
	if res.ExitIP != "203.0.113.9" {
		t.Fatalf("unexpected exit IP: %q", res.ExitIP)
	}
}

func TestCheckAddrViaHTTPUpstream(t *testing.T) {
	proxyAddr := fakeHTTPProxy(t)
	up := &config.Upstream{ID: "u1", Name: "fake", Type: config.UpstreamHTTP, Host: proxyAddr}
	res := CheckAddr(up, fakeIPServer(t), 5*time.Second, false)
	if !res.OK {
		t.Fatalf("upstream probe failed: %s", res.Error)
	}
	if res.ExitIP != "203.0.113.9" {
		t.Fatalf("unexpected exit IP: %q", res.ExitIP)
	}
}

func TestCheckAddrUpstreamDown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // nothing listens there now
	up := &config.Upstream{ID: "u1", Type: config.UpstreamHTTP, Host: addr}
	res := CheckAddr(up, "127.0.0.1:1", 2*time.Second, false)
	if res.OK || res.Error == "" {
		t.Fatalf("expected failure, got %+v", res)
	}
}

func TestExtractIP(t *testing.T) {
	cases := map[string]string{
		"1.2.3.4":              "1.2.3.4",
		"1.2.3.4\n":            "1.2.3.4",
		"  your ip: 5.6.7.8  ": "5.6.7.8",
		"2001:db8::1":          "2001:db8::1",
		"no ip here at all":    "",
		"{\"ip\":\"9.9.9.9\"}": "9.9.9.9",
	}
	for body, want := range cases {
		if got := extractIP(body); got != want {
			t.Errorf("extractIP(%q) = %q, want %q", body, got, want)
		}
	}
}
