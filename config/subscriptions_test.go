package config

import (
	"strings"
	"testing"
)

func TestSubscriptionCRUDAndEncryptedURL(t *testing.T) {
	s := tempStore(t, 7788)
	sub, err := s.UpsertSubscription("", &Subscription{
		Name: "demo", URL: "https://example.com/sub?token=secret-token",
		Enabled: true, IntervalSec: 600,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sub.ID == "" || sub.URL != "https://example.com/sub?token=secret-token" {
		t.Fatalf("unexpected saved subscription: %+v", sub)
	}
	var raw string
	if err := s.db.QueryRow(`SELECT url FROM subscriptions WHERE id=?`, sub.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw == sub.URL || !strings.HasPrefix(raw, SecretPrefix) || strings.Contains(raw, "secret-token") {
		t.Fatalf("subscription URL must be sealed in SQLite, got %q", raw)
	}

	list, err := s.ListSubscriptions()
	if err != nil || len(list) != 1 || list[0].NodeCount != 0 {
		t.Fatalf("list: %+v err=%v", list, err)
	}
	sub.Name = "renamed"
	sub.IntervalSec = 900
	if _, err := s.UpsertSubscription(sub.ID, sub); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSubscription(sub.ID)
	if err != nil || got.Name != "renamed" || got.IntervalSec != 900 {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	if err := s.RecordSubscriptionFetch(sub.ID, `"etag-1"`, "Wed, 01 Jan 2025 00:00:00 GMT", "", true); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetSubscription(sub.ID)
	if got.LastSuccessAt == 0 || got.ETag != `"etag-1"` {
		t.Fatalf("fetch status not stored: %+v", got)
	}
	if err := s.DeleteSubscription(sub.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSubscription(sub.ID); err == nil {
		t.Fatal("subscription should be gone")
	}
}

func TestReplaceSubscriptionNodesDiff(t *testing.T) {
	s := tempStore(t, 7788)
	sub, err := s.UpsertSubscription("", &Subscription{Name: "s", URL: "https://example.com/s", Enabled: true, IntervalSec: 600})
	if err != nil {
		t.Fatal(err)
	}
	manual, err := s.UpsertUpstream("", &Upstream{Name: "manual", Type: UpstreamHTTP, Host: "9.9.9.9:8080"})
	if err != nil {
		t.Fatal(err)
	}

	nodes := []*Upstream{
		{Name: "a", Type: UpstreamHTTP, Host: "1.1.1.1:8080", Username: "u", Password: "p"},
		{Name: "b", Type: UpstreamSocks5, Host: "2.2.2.2:1080"},
	}
	res, err := s.ReplaceSubscriptionNodes(sub.ID, nodes)
	if err != nil || res.Added != 2 || res.Removed != 0 {
		t.Fatalf("first refresh: %+v err=%v", res, err)
	}
	if n := len(s.Upstreams()); n != 3 {
		t.Fatalf("manual + 2 subscription nodes = 3, got %d", n)
	}
	list, _ := s.ListSubscriptions()
	if len(list) != 1 || list[0].NodeCount != 2 {
		t.Fatalf("node count: %+v", list)
	}

	// Same list again: update in place, no duplicates.
	res, err = s.ReplaceSubscriptionNodes(sub.ID, nodes)
	if err != nil || res.Updated != 2 || res.Added != 0 || res.Removed != 0 {
		t.Fatalf("second refresh: %+v err=%v", res, err)
	}
	if n := len(s.Upstreams()); n != 3 {
		t.Fatalf("second refresh must not duplicate, got %d", n)
	}

	// Drop b: its upstream is removed, the manual one survives.
	res, err = s.ReplaceSubscriptionNodes(sub.ID, nodes[:1])
	if err != nil || res.Removed != 1 {
		t.Fatalf("third refresh: %+v err=%v", res, err)
	}
	names := map[string]bool{}
	for _, u := range s.Upstreams() {
		names[u.Name] = true
	}
	if !names["manual"] || !names["a"] || names["b"] {
		t.Fatalf("unexpected upstream set: %v", names)
	}
	if id := manual.ID; id == "" {
		t.Fatal("manual upstream lost its id")
	}
}
