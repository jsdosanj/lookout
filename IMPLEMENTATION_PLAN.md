# servmonitor — Implementation Plan (draft v0.1)

> Status: **proposal for review.** This plan surfaces the architecture decisions and a
> phased roadmap. Several forks need your sign-off before we write the corresponding code
> (see **§9 Open decisions**). Nothing here is locked until you confirm.

---

## 1. What we're building (in plain English)

A tool that watches your servers and tells you, in plain English, whether they're healthy —
and warns you *before* something breaks. You install a small **agent** on each server
(Ubuntu, Debian, RHEL, Rocky, CentOS, AlmaLinux, Windows, macOS). The agent reports:

- **System specs** — CPU, memory, disk, network, OS version, uptime.
- **Installed apps/packages** and pending updates.
- **Running services** and processes (and whether the ones you care about are up).
- **Custom checks** — anything you can script (Nagios-compatible plugins + simple custom plugins).

A central **dashboard** shows all your servers, their health, history, and alerts. It's built
for **both** non-technical owners ("is everything OK? — yes/no, and what to do") and technical
admins (graphs, thresholds, raw metrics). Self-host it for free, or pay us to host it.

This is the Nagios idea, modernized: agent-based and outbound-only (no open ports on your
servers), plain-English by default, and easy enough for a non-expert to set up.

---

## 2. Honest caveats (read these first)

Three things in the request can't be promised as written. We will get as close as is
genuinely achievable and document exactly what we did:

1. **"No vulnerabilities."** No one can guarantee zero vulnerabilities. What we *can* do:
   secure-by-design architecture, threat modeling, dependency pinning + automated scanning
   (SCA), SAST/secret scanning in CI, least-privilege defaults, and a coordinated disclosure
   process. We'll state our security posture honestly.
2. **"NIST / HIPAA / SOC 2 / GDPR compliant."** Compliance is an organizational + audit
   process, not a property of code. We will *align* the architecture and controls with these
   frameworks and produce the evidence (data-flow docs, access controls, audit logs, encryption,
   retention, DPA) so an auditor can certify you. We won't claim certification we don't hold.
   (Nice loop: **Sightline** can assess servmonitor's own posture — dogfooding.)
3. **"Top of every search engine for every search."** We'll do strong technical + content SEO
   (fast static site, structured data, sitemaps, great docs, target keywords). Actual rankings
   depend on competition, backlinks, domain authority, and time — no one can guarantee #1.

---

## 3. Architecture (recommended)

```
  Monitored servers                      Control plane                    Users
  ┌──────────────┐   outbound mTLS/HTTPS  ┌───────────────────┐
  │  agent (Go)  │ ────────────────────► │  ingest + API     │ ◄──► Dashboard (web UI)
  │  - collectors│      (no inbound)      │  scheduler/alerts │ ◄──► Alerts (email, Slack,
  │  - plugins   │                        │  RBAC + MFA       │       webhook, PagerDuty)
  └──────────────┘                        │  config DB (PG)   │
                                          │  metrics (TSDB)   │
                                          └───────────────────┘
```

**Agent — single static Go binary.** Go gives us one small dependency-free binary per OS/arch,
which is the cleanest way to support 8 platforms securely. It runs as a service (systemd /
Windows service / launchd), talks **outbound only** to the control plane over mTLS with a
per-host enrollment token, and never opens an inbound port. (This is more secure than Nagios
NRPE, which listens on a port.) Collectors use OS-native sources (`/proc`, `dpkg`/`rpm`, `WMI`,
`system_profiler`).

**Plugin system — Nagios-compatible + simple custom.** Agents can run **Nagios plugins**
(exit codes 0/1/2/3 + perfdata), so the large existing plugin ecosystem works on day one.
For custom plugins, a "drop a script + a small YAML manifest in a folder" model — no
recompiling. This satisfies "users can build their own plugins."

**Control plane.** A Go service exposing the API, a check **scheduler**, an **alerting**
engine (states, dedup, escalation, notification channels), **RBAC + MFA**, and the config DB.
- **Config/users/state → PostgreSQL.**
- **Metrics (time-series) → a TSDB** (VictoriaMetrics or Prometheus-compatible). *Time-series
  is the crux of the stack decision — see §4.*

**Dashboard.** Web UI (overview health, per-server drill-down, graphs, alerts, admin/users).
Plain-English summaries on top, technical detail underneath.

**Packaging.**
- **Self-host:** one `docker compose up` (control plane + PG + TSDB + UI) so a non-expert can
  stand it up; agents install via a one-line script / `.deb` / `.rpm` / `.msi` / `.pkg`.
- **Hosted (paid):** we run the control plane; customers only install agents.

---

## 4. The key architectural fork: where does the dashboard run?

You asked for a **Cloudflare** setup doc for the dashboard. Monitoring is **time-series heavy**
(lots of metric points from many hosts), and Cloudflare's primitives (Workers + D1/SQLite + KV)
are excellent for a UI/control-plane/auth but **not** built to store high-volume time-series or
run a long-lived check scheduler/alerting engine. Two viable paths:

- **Option A — Containerized core + Cloudflare in front (recommended).** The control plane runs
  as a container (self-host via Docker; hosted = we run it on a container host). **Cloudflare
  provides:** the marketing site (Pages), DNS + CDN + TLS, **Zero-Trust Access** (SSO/MFA in
  front of the dashboard), and **Tunnel** (secure ingress with *no* open inbound ports). The
  `configuration_setup.md` then covers exactly this Cloudflare setup. Best fit for a robust,
  Nagios-class monitor; reuses the secure outbound-only theme.
- **Option B — Cloudflare-native.** Workers + D1 + Durable Objects + Analytics Engine
  (for time-series) + Queues. Matches your Sightline stack and the "dashboard in Cloudflare"
  ask most literally, but D1/Workers are an awkward fit for a monitoring engine and impose
  execution limits; more custom work, more risk for "robust like Nagios."

**Recommendation: Option A.** It keeps the dashboard genuinely Cloudflare-fronted (so the
Obsidian setup doc is real and useful) without forcing a time-series workload onto tools that
weren't designed for it.

---

## 5. Phased roadmap (MVP-first — don't boil the ocean)

Each phase ships something usable and verifiable.

**Phase 0 — Foundations & decisions (this step).** Lock §9 decisions, name, license, stack.
→ verify: you sign off on this plan.

**Phase 1 — Agent MVP (Linux first).** Go agent: collect specs/packages/services; outbound
enrollment + report over mTLS; `.deb`/`.rpm` + install script.
→ verify: agent registers and reports from Ubuntu + Rocky; data visible via API.

**Phase 2 — Control plane + minimal dashboard.** Ingest API, PostgreSQL, TSDB, a server list +
per-server detail page, basic up/down + threshold checks.
→ verify: two servers show live health and history in the UI.

**Phase 3 — Windows + macOS agents.** Same agent, platform collectors + service installers
(`.msi`, `.pkg`).
→ verify: a Windows and a macOS host report correctly.

**Phase 4 — Alerting + plugins.** Alert states/dedup/escalation; email + Slack + webhook;
Nagios-plugin runner + custom-plugin manifest.
→ verify: a simulated outage fires an alert; a custom plugin runs and shows in the UI.

**Phase 5 — RBAC, MFA, audit, multi-tenant.** Admin can create users/groups, granular
permissions, enforce MFA; immutable audit log; tenant isolation for the hosted offering.
→ verify: scoped roles enforced on every endpoint; MFA required for admins.

**Phase 6 — Website, docs, pricing, SEO.** Marketing site (Cloudflare Pages), extensive
plain-English docs, self-host vs hosted pricing, SEO (sitemap, structured data, performance).
→ verify: Lighthouse SEO/perf ≥ 95; docs cover install→alerting end-to-end.

**Phase 7 — Sightline integration.** servmonitor feeds asset/inventory/posture signals into
Sightline as evidence (e.g., disk encryption, patch status, running services → control checks).
→ verify: a servmonitor-sourced finding appears in a Sightline assessment.

**Phase 8 — Hardening & compliance evidence.** Threat model, pen-test pass, SBOM, dependency
scanning, the NIST/HIPAA/SOC2/GDPR control-mapping + evidence pack.
→ verify: security review checklist complete; control mapping documented.

*Nagios-addon parity (NRPE/NSCA/NCPA equivalents, NagVis-style maps, business-process/SLA views,
reporting/trends, config wizards) is folded into Phases 4–6 as concrete features, not a separate
rewrite.*

---

## 6. Security model (summary)

- Agents **outbound-only**, mTLS, per-host short-lived credentials, signed releases.
- Server-side: validate/sanitize all agent input; least-privilege DB; encrypted at rest +
  in transit; secrets in a vault, never in code/logs.
- Auth: SSO/OIDC + **MFA (TOTP/passkeys)**; granular RBAC (role → permission → scope by
  group/host/check); immutable audit log of privileged actions.
- Supply chain: pinned deps, SBOM, SCA + SAST + secret-scan in CI, reproducible builds.
- Tenant isolation for the hosted product (per-tenant data partitioning).

## 7. Sightline integration

servmonitor already collects exactly the evidence Sightline needs (devices, OS, patch status,
encryption, running services, accounts). We expose a read-only connector so Sightline ingests
servmonitor inventory/posture as control evidence — turning live monitoring into compliance
signal. (Direction confirmed in Phase 7; thin connector, not a coupling.)

## 8. Website, pricing, SEO (high level)

- **Site:** fast static site on Cloudflare Pages, same quality bar as sightline-site.
- **Audience:** non-technical ("set it up in 10 minutes, know your servers are OK") *and*
  technical (architecture, plugins, API).
- **Docs:** extensive, plain-English, task-based (install, add a server, set an alert, write a
  plugin), with copy-paste commands per OS.
- **Pricing:** self-host = free/open-source; **hosted = paid** (likely per-monitored-host tiers).
  Numbers TBD with you.
- **SEO:** semantic HTML, metadata + Open Graph, JSON-LD structured data, XML sitemap, fast
  Core Web Vitals, a content/keyword plan around "server monitoring", "Nagios alternative",
  "self-hosted monitoring".

## 9. Open decisions (need your call before we build)

1. **Name + rebrand.** Pick from §10 (or your own). Drives repo, site, agent identity.
2. **Dashboard architecture:** Option A (containerized core + Cloudflare front — *recommended*)
   vs Option B (Cloudflare-native). This determines the Cloudflare setup doc and the engine.
3. **License:** open-source self-host like Sightline? If so, which (AGPL-3.0 to protect the
   hosted business, or permissive)?
4. **Scope of THIS engagement:** do you want me to (a) start building Phase 1 (the agent), or
   (b) build the website + docs first, or (c) stay in planning and refine this further?
5. **Languages:** Go for agent + control plane (recommended). Confirm, or a preference?
6. **Hosted pricing model:** per-host tiers? Free tier? (We can mirror the Sightline pattern.)

## 10. Name options (rebrand candidates)

Working name `servmonitor` is descriptive but generic. Stronger options (verify domain +
trademark before committing):

- **Sentry Grid / SentryGrid** — watchful, infrastructure-wide. (Note: "Sentry" collides with
  the error-monitoring company — check carefully.)
- **Vigil** — "to keep watch." Short, memorable, on-theme.
- **Outpost** — a watch station for your fleet of servers.
- **Beacon** — signals health/alerts; friendly to non-technical users.
- **Northwatch** — "north star" + "watch"; trustworthy, ops-y.
- **Lookout** — plain-English, exactly what it does; great for the non-technical audience.
- **Keep** — as in a fortress keep that stands watch; very short, brandable.

My top three for this audience: **Lookout**, **Vigil**, **Outpost** (plain-English, memorable,
likely brandable). If you like the Dosanjh-Labs house style, I can tune toward that.
