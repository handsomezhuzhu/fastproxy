package admin

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

// Login throttling defaults; both are overridable through the environment
// (LOGIN_MAX_ATTEMPTS, LOGIN_LOCK_DURATION).
const (
	defaultLoginMaxAttempts = 5
	defaultLoginLock        = 5 * time.Minute
)

// loginLimiter is a process-global (not per-IP) failure counter for the single
// admin password. State lives in memory only: a restart clears the lock.
//
// After maxAttempts consecutive failures every login is refused with the
// remaining lock time until lockUntil passes; a successful login resets both
// the counter and the lock.
type loginLimiter struct {
	mu          sync.Mutex
	maxAttempts int
	lockFor     time.Duration
	failures    int
	lockUntil   time.Time
	now         func() time.Time // injectable clock (tests never sleep)
}

// LoginPolicy is the resolved throttling policy (exposed for startup logging).
type LoginPolicy struct {
	MaxAttempts int
	Lock        time.Duration
}

// EnvLoginPolicy parses LOGIN_MAX_ATTEMPTS / LOGIN_LOCK_DURATION. Invalid
// values fall back to the defaults and are reported in the returned errors so
// the caller can log a warning.
func EnvLoginPolicy() (LoginPolicy, []error) {
	p := LoginPolicy{MaxAttempts: defaultLoginMaxAttempts, Lock: defaultLoginLock}
	var problems []error
	if v := os.Getenv("LOGIN_MAX_ATTEMPTS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1000 {
			problems = append(problems, fmt.Errorf("LOGIN_MAX_ATTEMPTS=%q 无效（需为 1-1000 的整数），使用默认值 %d", v, defaultLoginMaxAttempts))
		} else {
			p.MaxAttempts = n
		}
	}
	if v := os.Getenv("LOGIN_LOCK_DURATION"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			problems = append(problems, fmt.Errorf("LOGIN_LOCK_DURATION=%q 无效（需为 Go duration，如 5m、30s），使用默认值 %s", v, defaultLoginLock))
		} else {
			p.Lock = d
		}
	}
	return p, problems
}

// newLoginLimiter builds a limiter with an explicit policy and clock.
func newLoginLimiter(maxAttempts int, lockFor time.Duration, now func() time.Time) *loginLimiter {
	if maxAttempts <= 0 {
		maxAttempts = defaultLoginMaxAttempts
	}
	if lockFor <= 0 {
		lockFor = defaultLoginLock
	}
	if now == nil {
		now = time.Now
	}
	return &loginLimiter{maxAttempts: maxAttempts, lockFor: lockFor, now: now}
}

// lockedFor returns the remaining lock time (0 = not locked).
func (l *loginLimiter) lockedFor() time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lockedForLocked()
}

func (l *loginLimiter) lockedForLocked() time.Duration {
	if l.lockUntil.IsZero() {
		return 0
	}
	remain := l.lockUntil.Sub(l.now())
	if remain <= 0 {
		return 0
	}
	return remain
}

// attempt records one login attempt with the given outcome and returns the
// remaining lock time when the caller must be refused.
//
//   - when locked, nothing is recorded and the remaining lock is returned;
//   - on success, the counter and lock are cleared;
//   - on failure, the counter grows; reaching the maximum starts the lock and
//     resets the counter.
func (l *loginLimiter) attempt(success bool) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if remain := l.lockedForLocked(); remain > 0 {
		return remain
	}
	// Expired lock: start counting afresh.
	if !l.lockUntil.IsZero() {
		l.lockUntil = time.Time{}
		l.failures = 0
	}
	if success {
		l.failures = 0
		return 0
	}
	l.failures++
	if l.failures >= l.maxAttempts {
		l.lockUntil = l.now().Add(l.lockFor)
		l.failures = 0
		return l.lockFor
	}
	return 0
}

// remainingAttempts reports how many failures are left before the lock trips
// (diagnostics/tests only).
func (l *loginLimiter) remainingAttempts() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	left := l.maxAttempts - l.failures
	if left < 0 {
		left = 0
	}
	return left
}
