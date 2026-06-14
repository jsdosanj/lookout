// Agent enrollment + TOFU hostname binding.
//
// This adds per-agent credentials on top of the legacy shared LOOKOUT_TOKEN:
// an agent enrolls once (presenting the shared token) and receives a unique
// per-agent token bound to a server-assigned agent identity. Reports
// authenticated with a per-agent token are keyed by the BOUND identity, not by
// the client-supplied hostname, which closes the cross-host forge/overwrite gap.
//
// Trust-on-first-use (TOFU): the first identity to report a given hostname pins
// it. A later report that sets the same hostname from a DIFFERENT identity is
// rejected, which stops cross-host overwrite even in legacy mode (where many
// hosts share one token but each is a distinct identity once enrolled, and the
// shared token itself reports as the single "shared" identity).
package store

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
)

// SharedIdentity is the agent identity used for reports authenticated with the
// legacy shared token (rather than a per-agent token).
const SharedIdentity = "shared"

// Agent is one enrolled agent's credentials and metadata.
type Agent struct {
	ID         string    `json:"id"`         // server-assigned identity (storage key for its reports)
	TokenHash  string    `json:"token_hash"` // sha256-free: we store the raw token hex (file is 0600); compared in constant time
	Hostname   string    `json:"hostname"`   // hostname at enrollment time (display only)
	EnrolledAt time.Time `json:"enrolled_at"`
	LastSeen   time.Time `json:"last_seen,omitempty"`
}

// AgentStore holds per-agent credentials and the TOFU hostname→identity pins,
// persisted to its own 0600 JSON file (mirrors Store's patterns).
type AgentStore struct {
	mu     sync.RWMutex
	path   string
	agents map[string]*Agent // by agent ID
	byTok  map[string]*Agent // by token (in-memory index)
	pins   map[string]string // hostname -> identity (agent ID or SharedIdentity)
}

type agentPersisted struct {
	Agents []*Agent          `json:"agents"`
	Pins   map[string]string `json:"hostname_pins"`
}

// OpenAgents loads the agent store from path, or starts empty if absent.
func OpenAgents(path string) (*AgentStore, error) {
	s := &AgentStore{
		path:   path,
		agents: map[string]*Agent{},
		byTok:  map[string]*Agent{},
		pins:   map[string]string{},
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	var p agentPersisted
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	for _, a := range p.Agents {
		s.agents[a.ID] = a
		s.byTok[a.TokenHash] = a
	}
	for h, id := range p.Pins {
		s.pins[h] = id
	}
	return s, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Enroll issues a new per-agent identity + token, recording the hostname for
// display. It returns the agent ID and the plaintext token (shown once).
func (s *AgentStore) Enroll(hostname string) (id, token string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = randomHex(8)
	for s.agents[id] != nil { // vanishingly unlikely; loop for safety
		id = randomHex(8)
	}
	token = randomHex(32)
	a := &Agent{ID: id, TokenHash: token, Hostname: hostname, EnrolledAt: time.Now().UTC()}
	s.agents[id] = a
	s.byTok[token] = a
	return id, token, s.persist()
}

// AgentByToken returns the agent whose per-agent token matches, in constant
// time. ok is false if no token matches.
func (s *AgentStore) AgentByToken(token string) (*Agent, bool) {
	if token == "" {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Constant-time over the candidate set: compare against every stored token so
	// a match doesn't leak via timing which/whether a token exists.
	var match *Agent
	tb := []byte(token)
	for _, a := range s.agents {
		if subtle.ConstantTimeCompare([]byte(a.TokenHash), tb) == 1 {
			match = a
		}
	}
	if match == nil {
		return nil, false
	}
	return match, true
}

// BindHostname applies TOFU: it pins hostname to identity on first sight, and
// thereafter rejects a different identity claiming the same hostname. On a
// successful (re)bind by the owning identity it also records LastSeen for
// per-agent identities. Returns an error if the binding conflicts.
func (s *AgentStore) BindHostname(hostname, identity string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if owner, ok := s.pins[hostname]; ok {
		if owner != identity {
			return fmt.Errorf("hostname %q is pinned to a different agent identity", hostname)
		}
	} else {
		s.pins[hostname] = identity
	}
	if a := s.agents[identity]; a != nil {
		a.LastSeen = time.Now().UTC()
	}
	return s.persist()
}

// List returns all enrolled agents sorted by ID (for admin/diagnostics).
func (s *AgentStore) List() []*Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Agent, 0, len(s.agents))
	for _, a := range s.agents {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// persist writes the agent store atomically (temp file + rename, 0600). Caller
// holds the lock.
func (s *AgentStore) persist() error {
	p := agentPersisted{Pins: s.pins}
	for _, a := range s.agents {
		p.Agents = append(p.Agents, a)
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
