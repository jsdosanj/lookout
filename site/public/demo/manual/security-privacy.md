# Security & privacy

Security is a first-class requirement for Lookout, not an afterthought. This page
describes the actual security model **as implemented today**, what data is stored
and where, what is and isn't encrypted, and a hardening checklist. It is honest about
the MVP's limits.

- [Design principles](#design-principles)
- [What data Lookout handles](#what-data-lookout-handles)
- [Transport & the TLS caveat](#transport--the-tls-caveat)
- [Agent authentication & anti-spoofing](#agent-authentication--anti-spoofing)
- [Dashboard authentication](#dashboard-authentication)
- [SSRF protection](#ssrf-protection)
- [Browser hardening headers](#browser-hardening-headers)
- [Secrets handling](#secrets-handling)
- [The agent's footprint on monitored hosts](#the-agents-footprint-on-monitored-hosts)
- [Hardening checklist](#hardening-checklist)
- [Honest limitations (MVP)](#honest-limitations-mvp)

---

## Design principles

- **Agents are outbound-only.** The agent never opens a listening port on a monitored
  host. It only dials out to the control plane. (More secure than agent designs that
  listen on a port.)
- **No shell, fixed arguments.** Every OS command runs via `exec.Command` with explicit
  arguments — no `sh -c`, so no shell-injection surface.
- **Read-only collection.** The agent reads system info; it does not modify the host.
- **Least third-party dependency.** The only external module is `golang.org/x/crypto`
  (for bcrypt); everything else is the Go standard library — a small, auditable
  surface.
- **Validate everything server-side.** Report bodies are size-capped and validated;
  hostnames are pinned; tokens are compared in constant time.
- **Honest about claims.** Lookout builds *toward* NIST CSF / HIPAA / SOC 2 / GDPR
  alignment and documents controls; it does **not** claim certification (that's an
  audit process) or "zero vulnerabilities."

---

## What data Lookout handles

**Collected from each host and sent to the control plane** (see
[Monitoring](monitoring.md#what-the-agent-collects)): hostname, OS/platform/version,
kernel, architecture, uptime, virtualization, disk-encryption status, CPU/memory/load,
disk usage, network interfaces (name/IPv4/MAC), the busiest processes, installed
packages (name+version), and services (name+status). This is **infrastructure
inventory and health telemetry** — no file contents, no user data, no application
payloads.

**Stored on the control plane** (JSON files, mode `0600`, atomic writes):

| File | Sensitive contents |
| --- | --- |
| `lookout-data.json` | Host reports + ~3h history. Inventory/telemetry. |
| `lookout-users.json` | **bcrypt password hashes, TOTP secrets**, sessions, org units. |
| `lookout-agents.json` | **Per-agent tokens** (raw, file is `0600`), hostname pins. |
| `lookout-rules.json` | Alert rules (no secrets). |

**Held in memory only** (lost on restart): open incidents and their
acknowledge/snooze state, and the recent-alert activity log.

**Sent outward by the control plane:** alert messages to your configured webhooks /
notify service. These contain the **hostname, severity, and plain-English reason**
(e.g. "disk /data is 94% full"). They do **not** contain Lookout secrets — the
notify-service payload is deliberately secret-free.

---

## Transport & the TLS caveat

> **Read this.** By default `lookout-server` serves **plain HTTP**. Agent reports are
> authenticated with a bearer token compared in constant time, but on plain HTTP that
> token (and the dashboard session cookie) is exposed to anyone who can sniff the
> network.

**In production, always run the control plane behind a TLS-terminating reverse
proxy** (Caddy, nginx, a Cloudflare Tunnel, etc.), and:

- Bind `lookout-server` to `127.0.0.1` (or a private interface) so only the proxy
  reaches it.
- Set **`LOOKOUT_SECURE_COOKIES=true`** so session cookies are HTTPS-only. (Lookout
  also emits HSTS automatically when it sees a TLS request.)
- Point agents at the **`https://`** URL of the proxy.

Per-agent **mTLS** (mutual TLS with per-host certificates) is the planned end state;
the token-over-TLS-proxy model is the MVP. See
[Honest limitations](#honest-limitations-mvp).

---

## Agent authentication & anti-spoofing

- **Shared enrollment token** (`LOOKOUT_TOKEN`) — checked in **constant time**;
  enrollment is gated on it.
- **Per-agent tokens** — issued at enrollment, stored `0600`, matched in constant time
  across the whole candidate set so a match doesn't leak via timing.
- **TOFU hostname pinning** — the first identity to report a hostname pins it; a
  different identity claiming the same hostname is rejected (`409`). This blocks
  cross-host overwrite/spoofing even under a shared token.
- **`LOOKOUT_REQUIRE_AGENT_TOKEN=true`** — hard-disables the shared token on `/report`,
  so only enrolled per-agent tokens are accepted. Recommended once your fleet has
  enrolled.

See [Users, auth & RBAC → agent identity](users-auth-rbac.md#agent-identity-tofu-and-per-agent-tokens).

---

## Dashboard authentication

- **bcrypt** password hashing; a dummy-hash compare runs even for unknown users to
  keep login timing uniform (no user-enumeration via timing).
- **TOTP MFA** with session-token rotation on the password→MFA transition.
- **SSO** (Google/GitHub) requires a **pre-created** account and a **verified** email;
  no silent self-provisioning.
- **RBAC** enforced in middleware on every protected route.
- **CSRF** synchronizer tokens on every state-changing POST.
- **Brute-force lockout** (5 fails / 15 min → 15-min lock) on login and MFA.
- **Session fixation defenses** — discard any presented session at login; rotate the
  token at MFA completion; revoke all sessions on role change / disable.

See [Users, auth & RBAC](users-auth-rbac.md) for the full detail.

---

## SSRF protection

Operator-supplied URLs that the **server** fetches — webhook targets and the
notify-service URL — are validated by an SSRF guard that blocks loopback, private,
link-local (incl. cloud metadata `169.254.169.254`), unspecified, multicast, and
CGNAT addresses, allows only `http`/`https`, resolves names and checks **every**
answer, and **re-checks on every send** (DNS-rebinding defense). Full details in
[Alerting → the SSRF guard](alerting.md#the-ssrf-guard-on-webhooks).

---

## Browser hardening headers

The control plane sends baseline hardening headers on every response:

- `Content-Security-Policy` — pins script sources to self + the Chart.js CDN
  (`cdn.jsdelivr.net`), restricts styles/images, forbids framing (`frame-ancestors
  'none'`), pins `base-uri 'self'`.
- `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`,
  `Referrer-Policy: no-referrer`.
- `Strict-Transport-Security` when the request is over TLS.

---

## Secrets handling

- Secrets (tokens, OAuth client secrets, admin password) are supplied via
  **environment variables**, not committed to code.
- Data files are mode **`0600`**. Password hashes are bcrypt; the notify-service
  payload is secret-free.
- **No secrets are logged.** Startup logs mention which features are on/off, never the
  values.
- **You** are responsible for protecting the environment and the data files (they hold
  password hashes, MFA secrets, and agent tokens). Back them up securely.

---

## The agent's footprint on monitored hosts

- **No inbound ports.** Nothing to firewall on the monitored host.
- **Outbound only**, to the single control-plane URL you configure.
- **Least privilege.** The agent only needs to read system info and (for packages/
  services) run fixed read-only commands. Run it as an unprivileged user where
  possible (e.g. systemd `DynamicUser=yes`); some package/service collectors surface
  more when run with elevated rights, but the agent never requires write access to
  the host.
- The `collect` (Universal Collector) pipeline additionally **disables HTTP
  redirects** on its outbound client (outbound-only discipline) and ships **signed**
  envelopes.

---

## Hardening checklist

- [ ] Set a **long, random `LOOKOUT_TOKEN`**.
- [ ] Run the control plane **behind TLS**; bind it to localhost/private and proxy.
- [ ] Set **`LOOKOUT_SECURE_COOKIES=true`**.
- [ ] Set a **strong admin password**; enable **MFA** on every admin/owner account.
- [ ] Use **least-privilege roles** (viewers for read-only people).
- [ ] After enrollment, set **`LOOKOUT_REQUIRE_AGENT_TOKEN=true`**.
- [ ] Restrict file permissions / ownership on the data directory; **back up**
      `lookout-users.json` and `lookout-agents.json` securely.
- [ ] Keep webhook URLs to **public** endpoints (the SSRF guard enforces this).
- [ ] Keep Go and `golang.org/x/crypto` up to date; rebuild on security releases.
- [ ] Run the agent as an unprivileged service user.

---

## Honest limitations (MVP)

These are documented openly so you can plan around them:

- **Plain HTTP by default** — the control plane needs a TLS proxy in front for
  production. Per-agent **mTLS** is roadmap, not implemented.
- **JSON file store** — the MVP persists to JSON files, not a database. Fine for small
  fleets; SQLite/Postgres + a real TSDB are roadmap.
- **In-memory ack/snooze and activity log** — lost on restart (see
  [Alerting → acknowledge & snooze](alerting.md#acknowledge--snooze)).
- **Live SMTP email is deferred** — use the notify service for live email.
- **The notify-service *server* is a separate component** — Lookout is only its client.
- **No certification claims** — Lookout aligns with security frameworks and documents
  controls; it is not certified, and no software can promise "zero vulnerabilities."

See [Roadmap & deferred items](roadmap.md) for the planned trajectory.
