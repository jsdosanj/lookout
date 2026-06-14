package auth

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// throttle is a small in-memory failed-attempt limiter keyed by
// account+IP. After maxAttempts failures inside attemptWindow the key is locked
// for lockoutFor; a success clears it. It is self-contained and safe for
// concurrent use — sufficient for brute-force resistance on a single control
// plane instance (it is not a distributed rate limiter).
type throttle struct {
	mu      sync.Mutex
	entries map[string]*attemptEntry
}

type attemptEntry struct {
	failures int
	first    time.Time // start of the current window
	until    time.Time // locked until this time (zero = not locked)
}

const (
	maxAttempts   = 5
	attemptWindow = 15 * time.Minute
	lockoutFor    = 15 * time.Minute
)

func newThrottle() *throttle {
	return &throttle{entries: map[string]*attemptEntry{}}
}

// clientIP extracts a best-effort client IP for keying attempts.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// locked reports whether key is currently in backoff. It also rolls the window
// over when it has expired so old failures don't accumulate forever.
func (t *throttle) locked(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.entries[key]
	if !ok {
		return false
	}
	now := time.Now()
	if !e.until.IsZero() {
		if now.Before(e.until) {
			return true
		}
		// Lockout elapsed; reset.
		delete(t.entries, key)
		return false
	}
	if now.Sub(e.first) > attemptWindow {
		delete(t.entries, key)
	}
	return false
}

// fail records a failed attempt and reports whether the key is now locked out.
func (t *throttle) fail(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	e, ok := t.entries[key]
	if !ok || now.Sub(e.first) > attemptWindow {
		e = &attemptEntry{first: now}
		t.entries[key] = e
	}
	e.failures++
	if e.failures >= maxAttempts {
		e.until = now.Add(lockoutFor)
		return true
	}
	return false
}

// reset clears any recorded failures for key (call on a successful auth).
func (t *throttle) reset(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, key)
}
