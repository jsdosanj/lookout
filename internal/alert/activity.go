package alert

import (
	"sync"
	"time"
)

// Activity is one recorded delivery attempt, for the dashboard's recent-alerts
// surface and for diagnostics.
type Activity struct {
	At       time.Time
	Server   string
	Severity string
	Channel  string
	Resolved bool
	Repeat   bool
	Reason   string
	Err      string // empty on success
}

// Recorder keeps the last N delivery attempts in memory (newest first). It is
// the LogFunc sink for an Engine and is safe for concurrent use.
type Recorder struct {
	mu  sync.Mutex
	max int
	buf []Activity
}

// NewRecorder returns a recorder that keeps up to max recent activities.
func NewRecorder(max int) *Recorder {
	if max < 1 {
		max = 1
	}
	return &Recorder{max: max}
}

// Log is the alert.LogFunc the engine calls on every delivery attempt.
func (r *Recorder) Log(n Notification, channel string, err error) {
	a := Activity{
		At: n.At, Server: n.Server, Severity: n.Severity, Channel: channel,
		Resolved: n.Resolved, Repeat: n.Repeat, Reason: n.Reason,
	}
	if err != nil {
		a.Err = err.Error()
	}
	r.mu.Lock()
	r.buf = append([]Activity{a}, r.buf...)
	if len(r.buf) > r.max {
		r.buf = r.buf[:r.max]
	}
	r.mu.Unlock()
}

// Recent returns up to n recent activities, newest first.
func (r *Recorder) Recent(n int) []Activity {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n > len(r.buf) {
		n = len(r.buf)
	}
	return append([]Activity(nil), r.buf[:n]...)
}
