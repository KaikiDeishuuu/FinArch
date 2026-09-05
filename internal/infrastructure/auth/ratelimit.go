package auth

import (
	"strings"
	"sync"
	"time"
)

// fixedWindow tracks hit counts within a rolling fixed time window for a single key.
type fixedWindow struct {
	count     int
	windowEnd time.Time
}

// IPRateLimiter limits the number of requests per IP within a fixed time window.
//
// It is safe for concurrent use.
type IPRateLimiter struct {
	mu       sync.Mutex
	windows  map[string]*fixedWindow
	max      int
	duration time.Duration
}

// NewIPRateLimiter creates a rate limiter that allows at most max requests per
// duration per unique key (typically an IP address).
func NewIPRateLimiter(max int, duration time.Duration) *IPRateLimiter {
	l := &IPRateLimiter{
		windows:  make(map[string]*fixedWindow),
		max:      max,
		duration: duration,
	}
	go l.cleanupLoop()
	return l
}

// Allow returns true if the key is within the rate limit, false otherwise.
func (l *IPRateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	w, ok := l.windows[key]
	if !ok || now.After(w.windowEnd) {
		l.windows[key] = &fixedWindow{count: 1, windowEnd: now.Add(l.duration)}
		return true
	}
	if w.count >= l.max {
		return false
	}
	w.count++
	return true
}

// cleanupLoop periodically removes expired entries to prevent unbounded memory growth.
func (l *IPRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		l.mu.Lock()
		now := time.Now()
		for key, w := range l.windows {
			if now.After(w.windowEnd) {
				delete(l.windows, key)
			}
		}
		l.mu.Unlock()
	}
}

// LoginAttemptTracker tracks failed login attempts per email and implements
// account lockout after repeated failures.
type LoginAttemptTracker struct {
	mu      sync.Mutex
	entries map[string]*attemptEntry

	maxFailures int
	lockout     time.Duration
}

type attemptEntry struct {
	failures    int
	lockedUntil time.Time
	lastFailure time.Time
}

func canonicalLoginKey(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// NewLoginAttemptTracker creates a tracker that locks an account for lockout
// duration after maxFailures consecutive failures.
func NewLoginAttemptTracker(maxFailures int, lockout time.Duration) *LoginAttemptTracker {
	t := &LoginAttemptTracker{
		entries:     make(map[string]*attemptEntry),
		maxFailures: maxFailures,
		lockout:     lockout,
	}
	go t.cleanupLoop()
	return t
}

// IsLocked returns true if the email is currently locked out.
func (t *LoginAttemptTracker) IsLocked(email string) bool {
	email = canonicalLoginKey(email)
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.entries[email]
	if !ok {
		return false
	}
	return time.Now().Before(e.lockedUntil)
}

// RecordFailure increments the failure counter for the email. If maxFailures is
// reached the account is locked for the configured lockout duration.
func (t *LoginAttemptTracker) RecordFailure(email string) {
	email = canonicalLoginKey(email)
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.entries[email]
	if !ok {
		e = &attemptEntry{}
		t.entries[email] = e
	}
	e.failures++
	e.lastFailure = time.Now()
	if e.failures >= t.maxFailures {
		e.lockedUntil = e.lastFailure.Add(t.lockout)
	}
}

// RecordSuccess clears the failure counter for the email after a successful login.
func (t *LoginAttemptTracker) RecordSuccess(email string) {
	email = canonicalLoginKey(email)
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, email)
}

func (t *LoginAttemptTracker) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		t.cleanupExpired(time.Now())
	}
}

func (t *LoginAttemptTracker) cleanupExpired(now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for email, e := range t.entries {
		// A below-threshold entry has a zero lockedUntil. Basing cleanup on
		// that zero value used to erase every such counter on the next tick
		// and weakened brute-force protection.
		retentionBase := e.lastFailure
		if e.lockedUntil.After(retentionBase) {
			retentionBase = e.lockedUntil
		}
		if e.failures == 0 || retentionBase.IsZero() || now.After(retentionBase.Add(time.Hour)) {
			delete(t.entries, email)
		}
	}
}
