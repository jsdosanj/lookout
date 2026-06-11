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

	"github.com/jsdosanj/servmonitor/internal/collect"
)

// Server is the latest known state of one monitored host.
type Server struct {
	ID         string         `json:"id"` // hostname for the MVP
	LastSeen   time.Time      `json:"last_seen"`
	LastReport collect.Report `json:"last_report"`
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
	s.servers[id] = &Server{ID: id, LastSeen: time.Now().UTC(), LastReport: *rep}
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
