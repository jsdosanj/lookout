// Package spool is the Lookout agent's offline buffer: an append-only, size-
// capped, replay-safe on-disk queue of signed envelopes that could not be
// shipped (no network, ingest 5xx, etc.).
//
// Design:
//   - One file per spooled envelope, named "<unixnano>-<nonce>.json" in a spool
//     dir. One-file-per-record makes append atomic (write temp + rename) and
//     delete-on-ship trivial, with no whole-file rewrite or lock contention.
//   - Replay-safe: a record is removed only AFTER the shipper confirms ingest
//     accepted it. A crash between ship and delete causes at-most a duplicate
//     send, which the ingest Worker's nonce cache de-duplicates. We never drop
//     an un-acked record.
//   - Size-capped: when the spool exceeds maxBytes we drop the OLDEST records
//     first (the freshest posture/vuln data is the most valuable), emitting a
//     count the agent reports in its health telemetry.
//
// At-rest encryption is a wave0 TODO (machine-bound key); see Open below.
//
// Standard library only. Safe for concurrent Add/Drain via an internal mutex.
package spool

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/jsdosanj/lookout/internal/collector"
)

// Spool is an on-disk envelope buffer.
type Spool struct {
	dir      string
	maxBytes int64
	mu       sync.Mutex
}

// Entry is one spooled envelope plus the on-disk file backing it, so a
// successful ship can Remove exactly that file.
type Entry struct {
	File     string             // absolute path of the spool file
	Envelope collector.Envelope // decoded envelope to ship
}

// New opens (creating if needed) a spool rooted at dir, capped at maxBytes.
func New(dir string, maxBytes int64) (*Spool, error) {
	if maxBytes <= 0 {
		return nil, errors.New("spool: maxBytes must be > 0")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Spool{dir: dir, maxBytes: maxBytes}, nil
}

// Add appends an envelope to the spool. It enforces the size cap by evicting
// the oldest entries first, and returns the number evicted (for health
// telemetry). Write is atomic via temp-file + rename.
func (s *Spool) Add(env collector.Envelope) (evicted int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, err := json.Marshal(env)
	if err != nil {
		return 0, err
	}
	// Filename sorts by time then nonce for stable oldest-first ordering.
	name := fmtFilename(env)
	final := filepath.Join(s.dir, name)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return 0, err
	}
	if err := os.Rename(tmp, final); err != nil {
		return 0, err
	}
	return s.enforceCapLocked()
}

// Drain returns up to n spooled entries, oldest first, for the shipper to
// replay. The caller must Remove each entry it successfully ships; un-removed
// entries are returned again on the next Drain (at-least-once delivery).
func (s *Spool) Drain(n int) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	files, err := s.sortedFilesLocked()
	if err != nil {
		return nil, err
	}
	if n > 0 && len(files) > n {
		files = files[:n]
	}
	var out []Entry
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue // file vanished (concurrent remove) — skip
		}
		var env collector.Envelope
		if err := json.Unmarshal(b, &env); err != nil {
			// Corrupt entry: quarantine by removing so it can't wedge the queue.
			_ = os.Remove(f)
			continue
		}
		out = append(out, Entry{File: f, Envelope: env})
	}
	return out, nil
}

// Remove deletes a spooled entry after it was successfully shipped.
func (s *Spool) Remove(file string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(file)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Len returns the number of spooled entries.
func (s *Spool) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	files, _ := s.sortedFilesLocked()
	return len(files)
}

// --- internals -------------------------------------------------------------

func (s *Spool) sortedFilesLocked() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || strings.HasSuffix(e.Name(), ".tmp") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		files = append(files, e.Name())
	}
	// Lexicographic sort == chronological because filenames are zero-padded
	// unixnano-first. Map back to absolute paths.
	sort.Strings(files)
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = filepath.Join(s.dir, f)
	}
	return out, nil
}

// enforceCapLocked evicts oldest entries until total size <= maxBytes.
func (s *Spool) enforceCapLocked() (evicted int, err error) {
	files, err := s.sortedFilesLocked()
	if err != nil {
		return 0, err
	}
	var total int64
	sizes := make([]int64, len(files))
	for i, f := range files {
		if fi, err := os.Stat(f); err == nil {
			sizes[i] = fi.Size()
			total += fi.Size()
		}
	}
	i := 0
	for total > s.maxBytes && i < len(files) {
		if err := os.Remove(files[i]); err == nil {
			total -= sizes[i]
			evicted++
		}
		i++
	}
	return evicted, nil
}

// fmtFilename builds a sortable, collision-free filename: 20-digit zero-padded
// unixnano + sanitized nonce. The nonce is base64url (no path separators) so
// it's filesystem-safe.
func fmtFilename(env collector.Envelope) string {
	var sb strings.Builder
	// 20 digits covers max int64 nanos.
	ns := env.Timestamp.UnixNano()
	digits := []byte("00000000000000000000")
	for i := len(digits) - 1; i >= 0 && ns > 0; i-- {
		digits[i] = byte('0' + ns%10)
		ns /= 10
	}
	sb.Write(digits)
	sb.WriteByte('-')
	sb.WriteString(env.Nonce)
	sb.WriteString(".json")
	return sb.String()
}
