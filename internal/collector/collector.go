// Package collector defines the Universal Collector contract for the Lookout
// agent: the Collector interface, a registry collectors self-register into, and
// the versioned signed evidence envelope every collector emits.
//
// This is a STUB for the collector agent to fill in. It is a NEW package
// (internal/collector) deliberately kept separate from the existing
// internal/collect package (which holds the legacy single-host Report and is
// NOT modified) so the universal-collector work proceeds without breaking the
// current agent. See COLLECT.md for the plan.
//
// Constraints (binding — CLAUDE.md + plan 01-lookout-agent):
//   - Standard library only (the module's sole dep is golang.org/x/crypto).
//   - Outbound-only; collectors NEVER open a listening port.
//   - No shell: collectors that exec use exec.Command with explicit argv only
//     (no "sh -c"), allow-listed full-path binaries, scrubbed env.
//   - Capability-gated and resource-budgeted by the scheduler.
package collector

import (
	"context"
	"time"
)

// SchemaVersion is the envelope (wire) version. Bump MAJOR for a breaking
// change (requires dual-write migration), MINOR for additive changes. This is
// the envelope version; each collector ALSO versions its own payload schema via
// Metadata.SchemaID (e.g. "lookout.posture.v1").
const SchemaVersion = 1

// Capability is a least-privilege grant a collector requires (e.g.
// "read.software", "exec.trivy", "read.eventlog"). Operator policy grants/denies
// per tenant; the agent enforces it before running the collector.
type Capability string

// Metadata describes a collector to the registry and scheduler.
type Metadata struct {
	ID              string        // unique, e.g. "system_inventory" (maps to C1..C12)
	SchemaID        string        // payload schema id, e.g. "lookout.inventory.v1"
	RequiredCaps    []Capability  // least-privilege capabilities this collector needs
	DefaultInterval time.Duration // scheduler default cadence (jittered upstream)
}

// Record is the typed result a collector produces for one run. Payload is the
// collector-specific structured data (marshaled to JSON by the transport).
// Large payloads (raw vuln output, log batches) are referenced via BlobHint so
// the transport can route them to R2 instead of inlining.
type Record struct {
	SchemaID string         // echoes Metadata.SchemaID for the consumer to pin
	Payload  any            // collector-specific struct (JSON-serializable)
	BlobHint *BlobReference // non-nil when a large payload should go to R2
}

// BlobReference points at a large payload the agent should upload to R2 via a
// pre-signed URL rather than inline in the ingest body.
type BlobReference struct {
	Suggestion string // suggested key suffix, e.g. "vuln/<ts>.json.zst"
	Bytes      []byte // raw bytes to upload (compressed upstream)
}

// Collector is the contract every C1..C12 collector implements. Implementations
// are pure-ish data gatherers: budgeted, capability-gated, outbound-only, and
// shell-free. Collect must honor ctx cancellation (the scheduler enforces a
// wall-clock timeout and circuit-breaks overruns).
type Collector interface {
	// Meta returns static registration metadata.
	Meta() Metadata
	// Collect gathers one snapshot. It must respect ctx deadline/cancellation
	// and must not panic; return an error instead.
	Collect(ctx context.Context) (Record, error)
}

// Envelope is the versioned, signed wrapper around every emitted Record. The
// transport builds and signs it; the ingest Worker verifies the signature
// against the enrolled agent's pubkey and enforces replay protection via
// (Nonce, Timestamp).
type Envelope struct {
	SchemaVersion int       `json:"schemaVersion"` // == SchemaVersion (envelope/wire version)
	CollectorID   string    `json:"collectorId"`
	SchemaID      string    `json:"schemaId"` // payload schema, e.g. "lookout.posture.v1"
	AgentID       string    `json:"agentId"`
	TenantID      string    `json:"tenantId"`
	Timestamp     time.Time `json:"ts"`
	Nonce         string    `json:"nonce"`     // anti-replay (with Timestamp)
	Payload       any       `json:"payload"`   // the Record.Payload
	BlobR2Key     string    `json:"blobR2Key"` // set when payload went to R2; else ""
	Signature     string    `json:"sig"`       // detached signature over the canonical bytes
}
