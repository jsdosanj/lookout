// Package shipper delivers signed envelopes to the Keystone ingest endpoint
// over outbound-only HTTPS. It never listens; it only POSTs.
//
// Delivery semantics:
//   - Try to POST the envelope to <IngestURL>/v1/ingest.
//   - On any failure (network, 5xx, timeout), spool the envelope to disk and
//     return nil — the caller's collection run is NOT failed by a transient
//     outage. The spool is replayed on the next Ship/Flush.
//   - On a 4xx that isn't 429 (a permanent reject — bad signature, revoked
//     agent), drop the envelope: re-sending will never succeed and spooling it
//     would wedge the queue. The drop is surfaced via the returned error so the
//     agent can log/health-report it.
//
// Large payloads: when an envelope carries BlobR2Key (the collector offloaded
// to R2), the shipper first requests a pre-signed PUT URL from
// /v1/ingest/blob-url, uploads the raw bytes to R2, then ships the (small)
// envelope referencing the key. Workers can't store large bodies cheaply, so
// the heavy bytes go straight to R2.
//
// Standard library only.
package shipper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jsdosanj/lookout/internal/collect/identity"
	"github.com/jsdosanj/lookout/internal/collect/spool"
	"github.com/jsdosanj/lookout/internal/collector"
)

// Spooler is the subset of *spool.Spool the shipper depends on (Add/Drain/
// Remove), so the shipper can be tested with a fake.
type Spooler interface {
	Add(env collector.Envelope) (int, error)
	Drain(n int) ([]spool.Entry, error)
	Remove(file string) error
}

// Config configures a Shipper.
type Config struct {
	// IngestURL is the configurable base URL of the Keystone ingest control
	// plane, e.g. "https://api.dosanjhlabs.com". NEVER hardcode the domain; the
	// agent reads this from its config. No trailing slash required.
	IngestURL string
	// Timeout bounds each HTTP request.
	Timeout time.Duration
	// MaxFlushBatch caps how many spooled entries one Flush replays per call.
	MaxFlushBatch int
}

// Shipper POSTs envelopes to ingest with spool-backed retry.
type Shipper struct {
	cfg    Config
	id     *identity.Identity
	spool  Spooler
	client *http.Client
}

// New builds a Shipper. The http.Client is outbound-only by construction (it
// makes requests; it never serves). Redirects are disabled so a hijacked DNS
// answer can't bounce a signed envelope to an attacker origin.
func New(cfg Config, id *identity.Identity, sp Spooler) *Shipper {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxFlushBatch <= 0 {
		cfg.MaxFlushBatch = 100
	}
	return &Shipper{
		cfg:   cfg,
		id:    id,
		spool: sp,
		client: &http.Client{
			Timeout: cfg.Timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse // do not follow redirects
			},
			// TODO(wave0): optional cert pinning to the ingest origin (plan §10).
		},
	}
}

// Ship delivers one envelope. Transient failures spool it (returns nil);
// permanent rejects drop it (returns an error describing the reject). Always
// attempts to flush the spool afterward so a recovered network drains backlog.
func (s *Shipper) Ship(ctx context.Context, env collector.Envelope) error {
	err := s.post(ctx, env)
	switch classify(err) {
	case outcomeOK:
		_ = s.Flush(ctx) // opportunistically drain any backlog
		return nil
	case outcomeRetry:
		if _, addErr := s.spool.Add(env); addErr != nil {
			return fmt.Errorf("shipper: ingest failed and spool failed: %v (spool: %w)", err, addErr)
		}
		return nil // buffered for later; not a hard error
	default: // outcomePermanent
		return fmt.Errorf("shipper: envelope permanently rejected, dropped: %w", err)
	}
}

// Flush replays spooled envelopes oldest-first. Each successfully-shipped entry
// is removed; on the first retryable failure it stops (network still down) and
// leaves the rest for the next attempt. Permanent rejects are dropped.
func (s *Shipper) Flush(ctx context.Context) error {
	entries, err := s.spool.Drain(s.cfg.MaxFlushBatch)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		switch classify(s.post(ctx, e.Envelope)) {
		case outcomeOK, outcomePermanent:
			// Remove on success OR permanent reject (don't loop forever on a
			// poisoned entry).
			_ = s.spool.Remove(e.File)
		case outcomeRetry:
			return nil // still offline; stop and keep the rest spooled
		}
	}
	return nil
}

// post performs the actual outbound HTTPS POST of one envelope. It attaches the
// agent identity headers the ingest Worker uses to look up the pubkey before
// verifying the in-body signature.
func (s *Shipper) post(ctx context.Context, env collector.Envelope) error {
	body, err := json.Marshal(env)
	if err != nil {
		return permanent(fmt.Errorf("marshal envelope: %w", err))
	}
	url := strings.TrimRight(s.cfg.IngestURL, "/") + "/v1/ingest"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return permanent(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Lookout-Agent", s.id.AgentID)
	req.Header.Set("X-Lookout-Tenant", s.id.TenantID)

	resp, err := s.client.Do(req)
	if err != nil {
		return err // transient (network) — classify() treats bare errors as retryable
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return fmt.Errorf("ingest status %d", resp.StatusCode) // retryable
	default:
		return permanent(fmt.Errorf("ingest rejected status %d", resp.StatusCode))
	}
}

// --- outcome classification ------------------------------------------------

type outcome int

const (
	outcomeOK outcome = iota
	outcomeRetry
	outcomePermanent
)

// permanentErr marks an error as non-retryable (drop, don't spool).
type permanentErr struct{ err error }

func (e permanentErr) Error() string { return e.err.Error() }
func (e permanentErr) Unwrap() error { return e.err }

func permanent(err error) error { return permanentErr{err} }

func classify(err error) outcome {
	if err == nil {
		return outcomeOK
	}
	var p permanentErr
	if asPermanent(err, &p) {
		return outcomePermanent
	}
	return outcomeRetry
}

// asPermanent reports whether err is (or wraps) a permanentErr.
func asPermanent(err error, target *permanentErr) bool {
	for err != nil {
		if pe, ok := err.(permanentErr); ok {
			*target = pe
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
