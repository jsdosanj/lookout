package alert

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
)

// persistRule is the on-disk form of a Rule. RepeatEvery is stored as seconds so
// the JSON is human-editable and stable (time.Duration marshals as nanoseconds).
type persistRule struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Server         string   `json:"server"`
	MinSeverity    string   `json:"min_severity"` // "warning" | "critical" | "stale"
	Channels       []string `json:"channels"`
	FlapWindow     int      `json:"flap_window"`
	RepeatEverySec int      `json:"repeat_every_sec"`
}

func toPersist(r Rule) persistRule {
	return persistRule{
		ID: r.ID, Name: r.Name, Server: r.Server, MinSeverity: r.MinSeverity.String(),
		Channels: r.Channels, FlapWindow: r.FlapWindow,
		RepeatEverySec: int(r.RepeatEvery / time.Second),
	}
}

func (p persistRule) toRule() Rule {
	return Rule{
		ID: p.ID, Name: p.Name, Server: p.Server, MinSeverity: SeverityOf(p.MinSeverity),
		Channels: p.Channels, FlapWindow: p.FlapWindow,
		RepeatEvery: time.Duration(p.RepeatEverySec) * time.Second,
	}
}

// RuleStore is a concurrency-safe, file-backed set of alert rules editable from
// the dashboard. It mirrors store.Store's atomic-write pattern. The set of
// available channel IDs is fixed by deployment config (env), so rules reference
// channels by ID but the store does not own channels.
type RuleStore struct {
	mu    sync.Mutex
	path  string
	rules []Rule
}

// OpenRuleStore loads rules from path. If the file does not exist, it seeds the
// store with defaults (a fleet-wide warning+ rule across all configured channels)
// and persists them, so a fresh install has working alerting out of the box.
func OpenRuleStore(path string, channelIDs []string) (*RuleStore, error) {
	rs := &RuleStore{path: path}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			rs.rules = defaultRules(channelIDs)
			if err := rs.persist(); err != nil {
				return nil, err
			}
			return rs, nil
		}
		return nil, err
	}
	var pr []persistRule
	if err := json.Unmarshal(b, &pr); err != nil {
		return nil, err
	}
	for _, p := range pr {
		rs.rules = append(rs.rules, p.toRule())
	}
	return rs, nil
}

// defaultRules is the seed rule set: fire fleet-wide at warning and above, with
// 2-observation flap-damping and a 30-minute escalation reminder.
func defaultRules(channelIDs []string) []Rule {
	return []Rule{{
		ID: "fleet-default", Name: "Fleet: warning and above", Server: "*",
		MinSeverity: SevWarning, Channels: channelIDs, FlapWindow: 2,
		RepeatEvery: 30 * time.Minute,
	}}
}

// Rules returns a copy of the current rules, sorted by name.
func (rs *RuleStore) Rules() []Rule {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	out := append([]Rule(nil), rs.rules...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Upsert adds a new rule (empty ID) or replaces an existing one (matching ID),
// persists, and returns the saved rule. The caller is responsible for pushing the
// new rule set into the live Engine via Engine.SetRules.
func (rs *RuleStore) Upsert(r Rule) (Rule, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if r.ID == "" {
		r.ID = newRuleID()
	}
	replaced := false
	for i := range rs.rules {
		if rs.rules[i].ID == r.ID {
			rs.rules[i] = r
			replaced = true
			break
		}
	}
	if !replaced {
		rs.rules = append(rs.rules, r)
	}
	return r, rs.persist()
}

// Delete removes a rule by ID and persists. It is not an error if the ID is absent.
func (rs *RuleStore) Delete(id string) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	out := rs.rules[:0]
	for _, r := range rs.rules {
		if r.ID != id {
			out = append(out, r)
		}
	}
	rs.rules = out
	return rs.persist()
}

// persist writes the rules atomically (temp file + rename). Caller holds the lock.
func (rs *RuleStore) persist() error {
	pr := make([]persistRule, 0, len(rs.rules))
	for _, r := range rs.rules {
		pr = append(pr, toPersist(r))
	}
	b, err := json.MarshalIndent(pr, "", "  ")
	if err != nil {
		return err
	}
	tmp := rs.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, rs.path)
}

func newRuleID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("rule-%d", time.Now().UnixNano())
	}
	return "rule-" + hex.EncodeToString(b[:])
}
