package subscription

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchETagAndBody(t *testing.T) {
	t.Setenv("SUB_ALLOW_PRIVATE", "1")
	var gotINM string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotINM = r.Header.Get("If-None-Match")
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		fmt.Fprint(w, "socks5://1.2.3.4:1080#a")
	}))
	defer srv.Close()

	first, err := Fetch(context.Background(), srv.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first.Body, "socks5://") || first.ETag != `"v1"` {
		t.Fatalf("first fetch: %+v", first)
	}
	second, err := Fetch(context.Background(), srv.URL, first.ETag, "")
	if err != nil || !second.NotModified || gotINM != `"v1"` {
		t.Fatalf("conditional fetch: %+v err=%v inM=%q", second, err, gotINM)
	}
}

func TestFetchRejectsPrivateByDefault(t *testing.T) {
	t.Setenv("SUB_ALLOW_PRIVATE", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()
	if _, err := Fetch(context.Background(), srv.URL, "", ""); err == nil ||
		!strings.Contains(err.Error(), "内网") {
		t.Fatalf("private URL must be rejected, got %v", err)
	}
}

func TestFetchHTTPErrorHint(t *testing.T) {
	t.Setenv("SUB_ALLOW_PRIVATE", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()
	_, err := Fetch(context.Background(), srv.URL, "", "")
	if err == nil || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "Token") {
		t.Fatalf("expected 403 hint, got %v", err)
	}
}

func TestFetchRejectsOversize(t *testing.T) {
	t.Setenv("SUB_ALLOW_PRIVATE", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, maxBodyBytes+1))
	}))
	defer srv.Close()
	if _, err := Fetch(context.Background(), srv.URL, "", ""); err == nil ||
		!strings.Contains(err.Error(), "超过") {
		t.Fatalf("oversize body must be rejected, got %v", err)
	}
}

func TestFetchRejectsNonHTTP(t *testing.T) {
	if _, err := Fetch(context.Background(), "file:///etc/passwd", "", ""); err == nil {
		t.Fatal("file:// must be rejected")
	}
}
