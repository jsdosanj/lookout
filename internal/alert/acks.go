package alert

import "time"

// AckRecord is one persisted acknowledgement loaded at startup. It mirrors the
// fields the engine needs without coupling the alert package to a storage type.
type AckRecord struct {
	RuleID string
	Server string
	Until  time.Time
}

// AckStore persists acknowledgements so they survive a control-plane restart. It
// is optional: an engine with no AckStore keeps acks in memory only. The store
// methods are best-effort durability — a failed write never blocks alerting.
type AckStore interface {
	SaveAck(ruleID, server string, until time.Time) error
	DeleteAck(ruleID, server string) error
	LoadAcks() ([]AckRecord, error)
}

// SetAckStore attaches a persistence backend and re-hydrates previously saved
// acknowledgements so a just-restarted engine re-applies them when each incident
// re-forms. Call once during wiring, before live observation.
func (e *Engine) SetAckStore(as AckStore) {
	if e == nil || as == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.acks = as
	recs, err := as.LoadAcks()
	if err != nil {
		return
	}
	for _, rec := range recs {
		e.savedAcks[key(rec.RuleID, rec.Server)] = rec.Until
	}
}

// saveAck records an ack in memory and (best-effort) in the backing store.
// Caller holds e.mu.
func (e *Engine) saveAck(ruleID, server string, until time.Time) {
	e.savedAcks[key(ruleID, server)] = until
	if e.acks != nil {
		_ = e.acks.SaveAck(ruleID, server, until)
	}
}

// clearAck forgets an ack (on resolve or a severity change, so a fresh problem
// re-alerts) in memory and in the backing store. Caller holds e.mu.
func (e *Engine) clearAck(ruleID, server string) {
	k := key(ruleID, server)
	if _, ok := e.savedAcks[k]; !ok && e.acks == nil {
		return
	}
	delete(e.savedAcks, k)
	if e.acks != nil {
		_ = e.acks.DeleteAck(ruleID, server)
	}
}
