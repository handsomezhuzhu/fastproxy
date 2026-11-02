package subscription

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/handsomezhuzhu/fastproxy/config"
)

func TestManagerRefreshReconcilesNodes(t *testing.T) {
	t.Setenv("SUB_ALLOW_PRIVATE", "1")
	var body atomic.Value
	body.Store("http://u:p@1.1.1.1:8080#a\nsocks5://2.2.2.2:1080#b\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body.Load().(string))
	}))
	defer srv.Close()

	store, err := config.Open(filepath.Join(t.TempDir(), "fp.db"), 7788)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sub, err := store.UpsertSubscription("", &config.Subscription{
		Name: "test", URL: srv.URL, Enabled: true, IntervalSec: 600,
	})
	if err != nil {
		t.Fatal(err)
	}
	var changes atomic.Int32
	m := NewManager(store, nil, func() { changes.Add(1) })

	res, err := m.Refresh(context.Background(), sub.ID)
	if err != nil || res.Added != 2 {
		t.Fatalf("first refresh: %+v err=%v", res, err)
	}
	if n := len(store.Upstreams()); n != 2 {
		t.Fatalf("upstreams = %d, want 2", n)
	}
	if changes.Load() != 1 {
		t.Fatalf("onChange calls = %d", changes.Load())
	}

	// Same content: update in place, no duplicates, no onChange needed.
	res, err = m.Refresh(context.Background(), sub.ID)
	if err != nil || res.Updated != 2 || res.Added != 0 {
		t.Fatalf("second refresh: %+v err=%v", res, err)
	}
	if n := len(store.Upstreams()); n != 2 {
		t.Fatalf("duplicate upstreams: %d", n)
	}

	// Drop one node: it disappears from the pool.
	body.Store("http://u:p@1.1.1.1:8080#a\n")
	res, err = m.Refresh(context.Background(), sub.ID)
	if err != nil || res.Removed != 1 {
		t.Fatalf("third refresh: %+v err=%v", res, err)
	}
	names := []string{}
	for _, u := range store.Upstreams() {
		names = append(names, u.Name)
	}
	if strings.Join(names, ",") != "a" {
		t.Fatalf("remaining nodes: %v", names)
	}
}

func TestManagerConcurrentRefreshIsCoalesced(t *testing.T) {
	t.Setenv("SUB_ALLOW_PRIVATE", "1")
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		fmt.Fprint(w, "socks5://2.2.2.2:1080#a")
	}))
	defer srv.Close()
	store, err := config.Open(filepath.Join(t.TempDir(), "fp.db"), 7788)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sub, err := store.UpsertSubscription("", &config.Subscription{Name: "x", URL: srv.URL, Enabled: true, IntervalSec: 600})
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(store, nil, nil)
	done := make(chan struct{})
	go func() {
		_, _ = m.Refresh(context.Background(), sub.ID)
		close(done)
	}()
	// Give the first call time to mark in-flight, then the second must fail.
	time.Sleep(20 * time.Millisecond)
	if _, err := m.Refresh(context.Background(), sub.ID); err == nil || !strings.Contains(err.Error(), "正在刷新") {
		t.Fatalf("expected in-flight error, got %v", err)
	}
	close(release)
	<-done
}
