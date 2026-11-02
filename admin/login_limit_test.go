package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock is an injectable monotonic clock (no sleeping in tests).
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{t: time.Unix(1_700_000_000, 0)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// loginWith posts to /api/login from the given peer address.
func (s *Server) loginWith(t *testing.T, remoteAddr, password string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"password":"` + password + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// TestLoginLockoutGlobal covers the single-admin global throttle: N failures
// lock every login (regardless of IP) with 429 + Retry-After, the lock expires
// by the clock, and a success clears the counter.
func TestLoginLockoutGlobal(t *testing.T) {
	s := testServer(t)
	if err := s.store.SetAdminPasswordHash(mustHash(t, "correct-horse")); err != nil {
		t.Fatal(err)
	}
	clock := newFakeClock()
	s.SetLoginClock(clock.Now)
	s.SetLoginPolicy(3, 2*time.Minute)

	// Two failures from different addresses: global counting, 401 each.
	for _, ip := range []string{"10.0.0.1:1000", "10.0.0.2:2000"} {
		if rec := s.loginWith(t, ip, "wrong"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("failure %s: %d %s", ip, rec.Code, rec.Body.String())
		}
	}
	if left := s.login.remainingAttempts(); left != 1 {
		t.Fatalf("remaining attempts %d, want 1", left)
	}
	// Third failure trips the lock and is already answered with 429.
	rec := s.loginWith(t, "10.0.0.3:3000", "wrong")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "120" {
		t.Fatalf("third failure: %d Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}

	// Now even the correct password and a brand-new address are refused.
	rec = s.loginWith(t, "192.0.2.9:4000", "correct-horse")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("locked login: %d %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Retry-After"); got != "120" {
		t.Fatalf("Retry-After %q, want 120", got)
	}
	var resp struct {
		Error         string `json:"error"`
		RetryAfterSec int    `json:"retry_after_sec"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.RetryAfterSec != 120 || resp.Error == "" {
		t.Fatalf("429 body: %+v", resp)
	}
	// No session cookie is handed out while locked.
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("locked login must not set a session cookie")
	}

	// Half the window later it is still locked, with fewer seconds left.
	clock.Advance(90 * time.Second)
	rec = s.loginWith(t, "192.0.2.9:4000", "correct-horse")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "30" {
		t.Fatalf("mid-lock: %d Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}

	// After the window the login works again and resets the counter.
	clock.Advance(31 * time.Second)
	rec = s.loginWith(t, "192.0.2.9:4000", "correct-horse")
	if rec.Code != http.StatusOK {
		t.Fatalf("after lock: %d %s", rec.Code, rec.Body.String())
	}
	var cookies []*http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == cookieName {
			cookies = append(cookies, c)
		}
	}
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie flags: %+v", cookies)
	}
	if left := s.login.remainingAttempts(); left != 3 {
		t.Fatalf("success must reset the counter, remaining %d", left)
	}
	// A single failure after reset does not lock.
	if rec := s.loginWith(t, "10.0.0.1:1000", "wrong"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("post-reset failure: %d", rec.Code)
	}
}

// TestLoginLockoutConcurrent hammers the limiter from many goroutines; the
// lock must trip exactly once the threshold is reached and never panic.
func TestLoginLockoutConcurrent(t *testing.T) {
	l := newLoginLimiter(5, time.Minute, time.Now)
	var locked atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.attempt(false) > 0 {
				locked.Add(1)
			}
		}()
	}
	wg.Wait()
	// Failures after the trip are refused too (they observe the lock), so the
	// counter proves every later call was throttled, never counted further.
	// The first maxAttempts-1 failures return 0; the trip and all later calls
	// observe the lock: 64-(5-1) = 60.
	throttled := int(locked.Load())
	if throttled != 64-(5-1) {
		t.Fatalf("lock observed %d times, want %d", throttled, 64-(5-1))
	}
	if l.lockedFor() <= 0 {
		t.Fatal("limiter should be locked")
	}
	if left := l.remainingAttempts(); left != 5 {
		t.Fatalf("failure counter must reset when the lock trips, remaining %d", left)
	}
}

// TestEnvLoginPolicy covers LOGIN_MAX_ATTEMPTS / LOGIN_LOCK_DURATION parsing.
func TestEnvLoginPolicy(t *testing.T) {
	t.Setenv("LOGIN_MAX_ATTEMPTS", "")
	t.Setenv("LOGIN_LOCK_DURATION", "")
	p, problems := EnvLoginPolicy()
	if len(problems) != 0 || p.MaxAttempts != 5 || p.Lock != 5*time.Minute {
		t.Fatalf("defaults: %+v %v", p, problems)
	}
	t.Setenv("LOGIN_MAX_ATTEMPTS", "7")
	t.Setenv("LOGIN_LOCK_DURATION", "90s")
	p, problems = EnvLoginPolicy()
	if len(problems) != 0 || p.MaxAttempts != 7 || p.Lock != 90*time.Second {
		t.Fatalf("custom: %+v %v", p, problems)
	}
	t.Setenv("LOGIN_MAX_ATTEMPTS", "abc")
	t.Setenv("LOGIN_LOCK_DURATION", "-1s")
	p, problems = EnvLoginPolicy()
	if len(problems) != 2 || p.MaxAttempts != 5 || p.Lock != 5*time.Minute {
		t.Fatalf("invalid values must fall back: %+v %v", p, problems)
	}
}

func mustHash(t *testing.T, pw string) string {
	t.Helper()
	h, err := HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
