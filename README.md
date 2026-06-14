# Lookout

**Open-source IT infrastructure monitoring — built for humans, not just sysadmins.**

Lookout watches your servers (Ubuntu, Debian, RHEL, Rocky, CentOS, AlmaLinux,
Windows, macOS) and tells you, in plain English, whether they're healthy — and
warns you *before* something breaks. You install a small **agent** on each server;
it reports system specs, installed packages, and running services to a central
dashboard. Self-host it for free, or pay us to host it.

## 📖 Documentation

**Full, self-service user manual:
[`docs/manual/`](docs/manual/README.md).** It covers getting started, monitoring,
the alerting engine, users/auth/RBAC, every config/env var, security, and an
extensive troubleshooting + FAQ + recipe index — written so you never have to ask
for help. Jump to:

- [Getting started](docs/manual/getting-started.md) — build, run the server, enroll an agent, first dashboard.
- [Monitoring](docs/manual/monitoring.md) — what's collected and the health model.
- [Alerting](docs/manual/alerting.md) — rules, channels, the stale-host sweeper, ack/snooze, the SSRF guard.
- [Users, auth & RBAC](docs/manual/users-auth-rbac.md) — accounts, MFA, SSO, roles.
- [Configuration reference](docs/manual/configuration.md) — every flag and env var.
- [Security & privacy](docs/manual/security-privacy.md) · [Troubleshooting](docs/manual/troubleshooting.md) · [Error reference](docs/manual/error-reference.md) · [How do I…?](docs/manual/how-do-i.md) · [FAQ](docs/manual/faq.md) · [Roadmap](docs/manual/roadmap.md)

See **[IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md)** for the architecture and
roadmap, and **[CLAUDE.md](CLAUDE.md)** for engineering guidelines.

## Status

**Phase 1–2 — agent + control plane + dashboard (working MVP).** The agent
collects a host report and reports it to the control plane, which computes
plain-English health and serves a dashboard (overview + per-server detail).
**Built-in alerting** now ships: a rule engine evaluates each report and delivers
Slack/Teams/webhook notifications with dedupe, flap-damping, and escalation
reminders (email is scaffolded; live SMTP send is the next step). Plugins and a
SQLite store are the remaining phases.

A **static live demo** of the dashboard is generated into [`docs/`](docs) and
can be hosted on GitHub Pages.

## Agent — build & run

Requires Go 1.26+.

```bash
# build for the current OS
go build -o lookout-agent ./cmd/lookout-agent

# collect this host's report as JSON
./lookout-agent report

# cross-compile (single static binary, no dependencies)
GOOS=linux   GOARCH=amd64 go build -o lookout-agent-linux   ./cmd/lookout-agent
GOOS=windows GOARCH=amd64 go build -o lookout-agent.exe     ./cmd/lookout-agent
GOOS=darwin  GOARCH=arm64 go build -o lookout-agent-macos   ./cmd/lookout-agent
```

What the agent collects:

- **Host:** hostname, OS/platform/version, kernel, architecture, uptime.
- **Specs:** CPU model + cores, memory total/used, load average, disk usage.
- **Packages:** installed packages (dpkg/rpm on Linux, Homebrew/pkgutil on macOS,
  registry on Windows).
- **Services:** running/stopped services (systemd, launchd, Windows services).

Design notes:

- **No external dependencies** — pure Go standard library (smaller attack surface).
- **No shell** — every command runs via `exec.Command` with fixed arguments, so
  there is no shell-injection surface.
- All parsing logic is pure and unit-tested (`go test ./...`).

## Control plane & dashboard

```bash
# build and run the control plane (dashboard at http://localhost:8080)
go build -o lookout-server ./cmd/lookout-server
# create the first owner account on first run, then log in at /login
LOOKOUT_TOKEN=your-secret \
  LOOKOUT_ADMIN_EMAIL=you@example.com LOOKOUT_ADMIN_PASSWORD=a-strong-password \
  ./lookout-server

# on each server, point the agent at it
./lookout-agent run --server http://YOUR_HOST:8080 --token your-secret
```

The dashboard shows every server with a plain-English status (ok / warning /
critical / stale), CPU / memory / disk usage with time-series charts, and a
per-server detail page.

**Accounts & access.** The dashboard is behind login with:

- **Email + password** (bcrypt) and optional **SSO** (Google / GitHub) — set
  `LOOKOUT_OAUTH_GOOGLE_CLIENT_ID/_SECRET`, `LOOKOUT_OAUTH_GITHUB_CLIENT_ID/_SECRET`,
  and `LOOKOUT_BASE_URL=https://monitor.example.com`.
- **TOTP MFA** (authenticator apps) — users enable it at `/account`.
- **RBAC** — roles (owner / admin / operator / viewer); admins manage users at
  `/admin/users`. Set `LOOKOUT_SECURE_COOKIES=true` behind TLS.

**Alerting.** Set `LOOKOUT_ALERT_WEBHOOKS` to one or more incoming-webhook URLs
(comma-separated — Slack, Teams, PagerDuty, or your own); each is validated by an
SSRF guard before any request is made. A seeded rule fires on every server at
**warning** or above, deduplicates ongoing incidents, damps flapping, and sends a
reminder every 30 minutes until the server recovers (then a resolve notice).
Rules are **persisted and editable from the dashboard** (per-server, minimum
severity, channels, flap window, reminder cadence) behind the *manage alerts*
permission. A background **stale-host sweeper** re-evaluates the fleet every
minute, so a host that goes silent fires a *stale* alert even though it has
stopped reporting. Operators can **acknowledge** an open incident (or **snooze**
it) to stop the reminder cascade without waiting for recovery — a worsening
severity still re-alerts, and recovery still sends the all-clear.

`LOOKOUT_ALERT_EMAIL` registers recipients. Live email is delivered through the
shared notification service (`POST /notify/send`) when
`LOOKOUT_NOTIFY_SERVICE_URL` and `LOOKOUT_NOTIFY_SERVICE_TOKEN` are set; the
service URL is SSRF-checked and the payload is secret-free. When the service is
not configured, the local email channel is the fallback and returns an explicit
"not configured" error rather than faking a send (see `internal/alert`). Active
rules, open incidents, and recent delivery activity are visible on the
**Notifications** page to users who can manage alerts.

> **Security note (MVP):** agent reports are authenticated with a shared bearer
> token, checked in constant time, over plain HTTP. Production needs TLS +
> per-agent credentials (mTLS) — tracked in the plan, not yet implemented. Don't
> expose the control plane to the internet without a TLS-terminating proxy.

## Live demo

The static demo under `docs/` is generated from sample data:

```bash
go run ./cmd/lookout-demo     # regenerates ./docs
```

Host it by enabling **GitHub Pages → source: `/docs`** (or a `gh-pages` branch).

## Tests

```bash
go vet ./...
go test ./...
```

## License

Lookout is free and open source under the **GNU AGPL-3.0** — see [LICENSE](LICENSE).

> The original prototype scripts `monitor.sh` / `monitor.ps1` are superseded by
> the Go agent and kept for reference only.
