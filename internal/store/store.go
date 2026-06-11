// Package store keeps the latest report per server and persists it to a JSON
// file. This is the MVP store; the plan upgrades it to SQLite when we add
// history, querying, and RBAC.
package store

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/jsdosanj/lookout/internal/collect"
)

// Server is the latest known state of one monitored host.
type Server struct {
	ID         string         `json:"id"` // hostname for the MVP
	LastSeen   time.Time      `json:"last_seen"`
	LastReport collect.Report `json:"last_report"`
	History    []Sample       `json:"history,omitempty"`
}

// history returns a server's samples, safe to call on a nil *Server (first report).
func (s *Server) history() []Sample {
	if s == nil {
		return nil
	}
	return s.History
}

// Sample is one point in a server's resource-usage time series.
type Sample struct {
	At   time.Time `json:"at"`
	CPU  float64   `json:"cpu"`  // %
	Mem  float64   `json:"mem"`  // %
	Disk float64   `json:"disk"` // % (busiest disk)
}

// maxHistory caps how many samples we keep per server (MVP, in memory + file).
const maxHistory = 180

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

// Store is a concurrency-safe, file-backed set of servers.
type Store struct {
	mu      sync.RWMutex
	path    string
	servers map[string]*Server
}

// Open loads the store from path, or starts empty if the file doesn't exist.
func Open(path string) (*Store, error) {
	s := &Store{path: path, servers: map[string]*Server{}}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	var list []*Server
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}
	for _, srv := range list {
		s.servers[srv.ID] = srv
	}
	return s, nil
}

// Save upserts a report keyed by hostname and persists the store atomically.
func (s *Store) Save(rep *collect.Report) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := rep.Host.Hostname
	if id == "" {
		id = "unknown"
	}
	now := time.Now().UTC()
	hist := s.servers[id].history() // safe on nil
	hist = append(hist, sampleFrom(rep, now))
	if len(hist) > maxHistory {
		hist = hist[len(hist)-maxHistory:]
	}
	s.servers[id] = &Server{ID: id, LastSeen: now, LastReport: *rep, History: hist}
	return s.persist()
}

// List returns all servers sorted by ID.
func (s *Store) List() []*Server {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Server, 0, len(s.servers))
	for _, srv := range s.servers {
		out = append(out, srv)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Get returns one server by ID.
func (s *Store) Get(id string) (*Server, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	srv, ok := s.servers[id]
	return srv, ok
}

// persist writes the store atomically (temp file + rename). Caller holds the lock.
func (s *Store) persist() error {
	out := make([]*Server, 0, len(s.servers))
	for _, srv := range s.servers {
		out = append(out, srv)
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
