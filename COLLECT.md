# Lookout — Universal Collector plan (`internal/collector`)

> Scaffolding note for the collector agent. This describes the new
> `internal/collector` package added in DosanjhLabs Wave 0. It does **not**
> modify any existing Lookout code.

## What this is

`internal/collector` is the **Universal Collector** contract that generalizes
the existing single-host monitoring agent into a plugin-based collector feeding
the whole DosanjhLabs suite (Cairn, Sightline, Bastion, Perimeter, Ward,
Passage, Ledger). It defines three things, as stubs to fill in:

- `Collector` — the interface every collector (C1..C12) implements
  (`Meta()` + `Collect(ctx)`).
- `Registry` — a concurrency-safe registry collectors self-register into from
  `init()`; the agent's collector set = the packages it imports.
- `Envelope` — the **versioned, signed** wire wrapper every record is emitted
  in (`{schemaVersion, collectorId, schemaId, agentId, tenantId, ts, nonce,
  payload, blobR2Key, sig}`).

## Why a NEW package (decision)

The repo already has `internal/collect` (package `collect`) holding the legacy
single-host `Report` + `Collect()` and the OS-specific gatherers. To avoid
breaking that working code and to avoid same-package symbol collisions
(`Collect`, `Report`), the universal collector is a **separate package**,
`internal/collector`. The legacy `internal/collect` is untouched; collectors can
reuse its parsing helpers later by importing it. Nothing existing was modified.

## Versioning

- The **envelope** has `SchemaVersion` (currently `1`): MAJOR bump = breaking
  change (dual-write migration required), MINOR = additive.
- Each collector ALSO versions its **payload** schema via `Metadata.SchemaID`
  (e.g. `lookout.posture.v1`). Consumers pin a payload schema version. Breaking
  changes ripple to ~7 products, so follow the suite schema-governance RFC.

## Binding constraints (from `01-lookout-agent.md` + `CLAUDE.md`)

- **Go standard library only** (module's sole dependency is
  `golang.org/x/crypto`).
- **Outbound-only**; no collector ever opens a listening port.
- **No shell**: any exec uses `exec.Command` with explicit argv (no `sh -c`),
  allow-listed full-path binaries, scrubbed env — zero shell-injection surface.
- **Capability-gated + resource-budgeted**: each collector declares
  `RequiredCaps`; the scheduler enforces per-collector CPU/RSS/wall-clock
  budgets and circuit-breaks overruns (directly answers the osquery
  runaway-query / battery-drain complaints).
- **Heavy scanners (Trivy/Nuclei) run on the agent, never in Workers**; OpenVAS
  is relayed, not run.

## Collector set to implement (C1..C12)

`system_inventory` (C1), `software` (C2), `patch` (C3), `posture` (C4), `vuln`
(C5), `accounts` (C6), `events` (C7), `netservices` (C8), `certs` (C9),
`backup` (C10), `security_tools`/EDR-MDM presence (C11), `agent` meta/health
(C12). Each is its own package under `internal/collector/<name>` that
`Register()`s itself in `init()` and emits a `lookout.<name>.v<N>` payload.

## Next steps for the collector agent

1. Add one package per collector under `internal/collector/`, each calling
   `collector.Register(...)` in `init()`.
2. Implement the transport that builds + signs the `Envelope` and routes large
   payloads to R2 via `BlobReference`/pre-signed URL (outbound HTTPS only).
3. Implement the budgeted, jittered scheduler that reads `Registry.All()` and
   enforces capability grants + resource budgets + circuit breakers.
