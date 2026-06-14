package scheduler

import (
	"fmt"
	"sync"
	"time"
)

// breaker is a per-collector circuit breaker. After `threshold` consecutive
// failures it opens for `cooldown`; while open the collector is skipped. A
// single success closes it and resets the failure count.
//
// Closed (failures < threshold)  --threshold fails-->  Open (until openUntil)
//
//	^                                                   |
//	+-----------------(success)-------------------------+
//	                  (cooldown elapsed ⇒ a probe run is allowed)
type breaker struct {
	mu        sync.Mutex
	failures  int
	openUntil time.Time
}

// open reports whether the breaker is currently open (and not yet eligible for
// a probe). When the cooldown has elapsed it returns false so exactly one probe
// run is allowed; if that probe fails, fail() re-opens it.
func (b *breaker) open() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.openUntil.IsZero() {
		return false
	}
	return time.Now().Before(b.openUntil)
}

// fail records a failure and opens the breaker if the threshold is reached.
// Returns true if this failure (re)opened the breaker.
func (b *breaker) fail(threshold int, cooldown time.Duration) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	if b.failures >= threshold {
		b.openUntil = time.Now().Add(cooldown)
		return true
	}
	return false
}

// success closes the breaker and resets the failure count.
func (b *breaker) success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.openUntil = time.Time{}
}

// recoveredPanic wraps a recovered panic value as an error.
func recoveredPanic(r any) error {
	return fmt.Errorf("collector panicked: %v", r)
}
