// Package store keeps the latest report per server plus its resource history and
// the health configuration, persisted in an embedded SQLite database. SQLite is a
// single file (no external server), so the control plane stays a single static
// binary with zero operational infrastructure — the JSON-file MVP it replaces did
// not survive scale or give real, time-based retention.
//
// The pure-Go modernc.org/sqlite driver is used so the binary needs no cgo and
// cross-compiles like the rest of Lookout.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/jsdosanj/lookout/internal/collect"
)

// Server is the latest known state of one monitored host.
type Server struct {
	ID         string         `json:"id"` // hostname for the MVP
	LastSeen   time.Time      `json:"last_seen"`
	LastReport collect.Report `json:"last_report"`
	History    []Sample       `json:"history,omitempty"`
}

// Sample is one point in a server's resource-usage time series.
type Sample struct {
	At   time.Time `json:"at"`
	CPU  float64   `json:"cpu"`  // %
	Mem  float64   `json:"mem"`  // %
	Disk float64   `json:"disk"` // % (busiest disk)
}

// defaultRetention is how long resource samples are kept before the retention
// sweep on each report prunes them. Configurable via SetRetention.
const defaultRetention = 14 * 24 * time.Hour

// historyLimit caps how many of the most recent samples are loaded for display
// (charts). Retention governs deletion; this only bounds a single read.
const historyLimit = 720

// configKeyHealth is the config-table key under which HealthConfig is stored.
const configKeyHealth = "health"

// sampleFrom derives a point-in-time sample from a report.
func sampleFrom(rep *collect.Report, at time.Time) Sample {
	var memPct, diskPct float64
	if rep.Specs.MemTotalMB > 0 {
		memPct = float64(rep.Specs.MemUsedMB) / float64(rep.Specs.MemTotalMB) * 100
	}
	for _, d := range rep.Specs.Disks {
		if d.TotalMB == 0 {
			continue
		}
		if p := float64(d.UsedMB) / float64(d.TotalMB) * 100; p > diskPct {
			diskPct = p
		}
	}
	return Sample{At: at, CPU: rep.Specs.CPUPercent, Mem: memPct, Disk: diskPct}
}

// Store is a concurrency-safe, SQLite-backed set of servers, their resource
// history, and the health configuration.
type Store struct {
	db        *sql.DB
	retention time.Duration

	mu     sync.RWMutex
	health *HealthConfig // cached; persisted to the config table
}

const schema = `
CREATE TABLE IF NOT EXISTS servers (
  id        TEXT PRIMARY KEY,
  last_seen INTEGER NOT NULL,   -- unix nanoseconds
  report    TEXT NOT NULL       -- JSON-encoded collect.Report
);
CREATE TABLE IF NOT EXISTS samples (
  server_id TEXT NOT NULL,
  at        INTEGER NOT NULL,   -- unix nanoseconds
  cpu       REAL NOT NULL,
  mem       REAL NOT NULL,
  disk      REAL NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_samples_server_at ON samples(server_id, at);
CREATE TABLE IF NOT EXISTS config (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS acks (
  rule_id TEXT NOT NULL,
  server  TEXT NOT NULL,
  until   INTEGER NOT NULL,     -- unix nanoseconds
  PRIMARY KEY (rule_id, server)
);
`

// Open opens (creating if absent) the SQLite database at path and ensures the
// schema. A single open connection serializes writes, which is exactly right for
// this low-write, single-binary workload and avoids SQLite "database is locked".
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, err
	}
	s := &Store{db: db, retention: defaultRetention}
	if err := s.loadHealth(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// SetRetention overrides how long samples are kept. Values <= 0 are ignored.
func (s *Store) SetRetention(d time.Duration) {
	if d > 0 {
		s.mu.Lock()
		s.retention = d
		s.mu.Unlock()
	}
}

// Save upserts a report keyed by hostname, appends a resource sample, and prunes
// samples older than the retention window for that server.
func (s *Store) Save(rep *collect.Report) error {
	id := rep.Host.Hostname
	if id == "" {
		id = "unknown"
	}
	now := time.Now().UTC()
	repJSON, err := json.Marshal(rep)
	if err != nil {
		return err
	}
	smp := sampleFrom(rep, now)

	s.mu.RLock()
	retention := s.retention
	s.mu.RUnlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(
		`INSERT INTO servers (id, last_seen, report) VALUES (?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET last_seen=excluded.last_seen, report=excluded.report`,
		id, now.UnixNano(), string(repJSON),
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO samples (server_id, at, cpu, mem, disk) VALUES (?, ?, ?, ?, ?)`,
		id, smp.At.UnixNano(), smp.CPU, smp.Mem, smp.Disk,
	); err != nil {
		return err
	}
	cutoff := now.Add(-retention).UnixNano()
	if _, err := tx.Exec(`DELETE FROM samples WHERE server_id = ? AND at < ?`, id, cutoff); err != nil {
		return err
	}
	return tx.Commit()
}

// List returns all servers (each with its recent history) sorted by ID.
func (s *Store) List() []*Server {
	rows, err := s.db.Query(`SELECT id, last_seen, report FROM servers ORDER BY id`)
	if err != nil {
		return nil
	}
	// Drain and close the cursor BEFORE loading per-server history: with a single
	// open connection, querying history while this cursor is still open would
	// deadlock (the connection is checked out until rows is closed).
	var out []*Server
	for rows.Next() {
		srv, err := scanServer(rows)
		if err != nil {
			continue
		}
		out = append(out, srv)
	}
	rows.Close()
	for _, srv := range out {
		srv.History = s.history(srv.ID)
	}
	return out
}

// Get returns one server by ID, with its recent history.
func (s *Store) Get(id string) (*Server, bool) {
	row := s.db.QueryRow(`SELECT id, last_seen, report FROM servers WHERE id = ?`, id)
	srv, err := scanServer(row)
	if err != nil {
		return nil, false
	}
	srv.History = s.history(srv.ID)
	return srv, true
}

// scanRow is satisfied by both *sql.Row and *sql.Rows.
type scanRow interface {
	Scan(dest ...any) error
}

func scanServer(r scanRow) (*Server, error) {
	var (
		id       string
		lastSeen int64
		repJSON  string
	)
	if err := r.Scan(&id, &lastSeen, &repJSON); err != nil {
		return nil, err
	}
	var rep collect.Report
	if err := json.Unmarshal([]byte(repJSON), &rep); err != nil {
		return nil, err
	}
	return &Server{ID: id, LastSeen: time.Unix(0, lastSeen).UTC(), LastReport: rep}, nil
}

// history loads the most recent samples for a server, oldest-first (for charts).
func (s *Store) history(id string) []Sample {
	rows, err := s.db.Query(
		`SELECT at, cpu, mem, disk FROM samples WHERE server_id = ? ORDER BY at DESC LIMIT ?`,
		id, historyLimit,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var rev []Sample
	for rows.Next() {
		var (
			at             int64
			cpu, mem, disk float64
		)
		if err := rows.Scan(&at, &cpu, &mem, &disk); err != nil {
			continue
		}
		rev = append(rev, Sample{At: time.Unix(0, at).UTC(), CPU: cpu, Mem: mem, Disk: disk})
	}
	// Reverse into chronological order.
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

// HealthConfig returns a snapshot of the current health configuration (defaults
// plus per-host/group overrides). It is never nil.
func (s *Store) HealthConfig() *HealthConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.health
}

// SetHealthConfig persists and caches a new health configuration.
func (s *Store) SetHealthConfig(cfg *HealthConfig) error {
	if cfg == nil {
		cfg = DefaultHealthConfig()
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(
		`INSERT INTO config (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		configKeyHealth, string(b),
	); err != nil {
		return err
	}
	s.mu.Lock()
	s.health = cfg
	s.mu.Unlock()
	return nil
}

// loadHealth populates the cached health config from the config table, falling
// back to defaults when none is stored yet.
func (s *Store) loadHealth() error {
	var value string
	err := s.db.QueryRow(`SELECT value FROM config WHERE key = ?`, configKeyHealth).Scan(&value)
	if err == sql.ErrNoRows {
		s.health = DefaultHealthConfig()
		return nil
	}
	if err != nil {
		return err
	}
	var cfg HealthConfig
	if err := json.Unmarshal([]byte(value), &cfg); err != nil {
		return fmt.Errorf("decode health config: %w", err)
	}
	s.health = &cfg
	return nil
}

// ImportLegacyJSON migrates a pre-SQLite data file (a JSON array of *Server) into
// the database. It is a no-op-safe one-shot used on first boot; existing rows are
// upserted, so re-running is harmless.
func (s *Store) ImportLegacyJSON(b []byte) error {
	var list []*Server
	if err := json.Unmarshal(b, &list); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, srv := range list {
		repJSON, err := json.Marshal(srv.LastReport)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO servers (id, last_seen, report) VALUES (?, ?, ?)
			 ON CONFLICT(id) DO UPDATE SET last_seen=excluded.last_seen, report=excluded.report`,
			srv.ID, srv.LastSeen.UnixNano(), string(repJSON),
		); err != nil {
			return err
		}
		for _, smp := range srv.History {
			if _, err := tx.Exec(
				`INSERT INTO samples (server_id, at, cpu, mem, disk) VALUES (?, ?, ?, ?, ?)`,
				srv.ID, smp.At.UnixNano(), smp.CPU, smp.Mem, smp.Disk,
			); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
