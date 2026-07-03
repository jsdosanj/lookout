# Lookout

**Lightweight, self-hosted server monitoring you can stand up in 5 minutes.**

Lookout is the simple alternative to Nagios and Zabbix — not a clone of them. You
run one small **server** (the control plane + dashboard), drop a single static
**agent** binary on each machine you want to watch, and get a plain-English view
of whether your fleet is healthy — with alerts *before* something falls over.

- **Outbound-only agents.** The agent opens no inbound ports. It dials out to your
  server over HTTPS and reports on a timer — nothing to expose on the monitored host.
- **No dependencies.** Both binaries are pure Go standard library (plus
  `golang.org/x/crypto`). No agent runtime, no plugins to install, no database to
  operate for the basics.
- **No shell.** Every metric is gathered with `exec.Command` and fixed arguments —
  there is no shell-injection surface.
- **Multi-OS, today.** Linux (Ubuntu, Debian, RHEL, Rocky, CentOS, AlmaLinux),
  Windows, and macOS, with native collectors for each.

Lookout is **free and open source forever** under the **Apache-2.0** license. Run it
yourself, fork it, ship it — no tiers, no license keys, no phone-home.

> Honesty note: Lookout is young (v0.x). The agent, control plane, dashboard, and
> alert engine below are **real and tested**. Items in [Roadmap](#roadmap) are not
> built yet. Where a capability isn't live, Lookout returns an explicit error rather
> than faking success — see [`docs/manual/roadmap.md`](docs/manual/roadmap.md) for
> the line-by-line ledger.

---

## What it does today

- **Agent** collects a full host report on a timer: hostname, OS/platform/version,
  kernel, architecture, uptime; CPU model/cores/usage, memory, load average, disk
  usage; installed packages (dpkg/rpm, Homebrew/pkgutil, Windows registry); and
  running/stopped services (systemd, launchd, Windows services). Outbound-only,
  no shell, per-agent token enrollment.
- **Control plane** ingests reports, computes plain-English health, keeps ~3 hours
  of per-host history (in a JSON file), and serves the dashboard.
- **Dashboard** shows every host as a color-coded card (ok / warning / critical /
  stale), per-host detail with CPU/memory/disk time-series charts, and a
  needs-attention panel.
- **Accounts & access:** email + password (bcrypt), TOTP MFA, optional Google/GitHub
  SSO, and RBAC (owner / admin / operator / viewer).
- **Alert engine:** a persisted, dashboard-editable rule engine evaluates each report
  by severity, with dedupe, flap-damping, escalation reminders, resolve notices,
  acknowledge/snooze, and a stale-host sweeper that alerts when an agent goes silent.
  Delivery to **Slack / Teams / generic webhooks is live** (each URL is SSRF-guarded);
  **email is live via self-hosted SMTP** (`LOOKOUT_SMTP_*`) or the shared notification
  service, otherwise it returns an explicit "not configured" error.

**What the alert engine watches today:** health is derived from **disk** usage
(>=80% warning, >=90% critical), **memory** usage (>=90% warning), **CPU** usage
(>=85% warning, >=95% critical), **load per core** (>=1.0 warning, >=2.0 critical),
**watched services** (a stopped/absent service is critical), and **staleness** (no
report for 5 minutes). All thresholds and watched services are configurable
per-host/per-group via `--health-config` (see [`examples/`](examples/)); the seeded
rule's 2-observation flap window keeps a single-sample CPU/load spike from paging.

---

## Self-host in 5 minutes

You need either Docker (recommended) or Go 1.26+. Pick one path.

### Option A — Docker Compose (recommended)

```bash
git clone https://github.com/jsdosanj/lookout.git
cd lookout

# Set the agent enrollment token and your first owner login, then bring it up.
LOOKOUT_TOKEN="$(openssl rand -hex 16)" \
LOOKOUT_ADMIN_EMAIL="you@example.com" \
LOOKOUT_ADMIN_PASSWORD="a-strong-password" \
  docker compose up -d
```

Open <http://localhost:8080> and log in with the email/password you set. The token
you generated is what each agent uses to enroll. The compose file builds the server
image from the included [`Dockerfile`](Dockerfile) and persists data to the
`lookout-data` volume. See [docker-compose.yml](docker-compose.yml) for the full set
of `LOOKOUT_*` settings (SSO, alert webhooks, secure cookies, etc.).

### Option B — Download a release binary

Grab the prebuilt `lookout-server` for your OS/arch from the
[Releases page](https://github.com/jsdosanj/lookout/releases) (built in CI for
linux / macOS / windows on amd64 + arm64), then:

```bash
LOOKOUT_TOKEN="$(openssl rand -hex 16)" \
LOOKOUT_ADMIN_EMAIL="you@example.com" \
LOOKOUT_ADMIN_PASSWORD="a-strong-password" \
  ./lookout-server            # dashboard on http://localhost:8080
```

### Option C — Build from source

```bash
go build -o lookout-server ./cmd/lookout-server
LOOKOUT_TOKEN=secret LOOKOUT_ADMIN_EMAIL=you@example.com \
  LOOKOUT_ADMIN_PASSWORD=a-strong-password ./lookout-server
```

> **Production transport:** agent reports authenticate with a bearer token over
> HTTP. Put the control plane **behind a TLS-terminating reverse proxy** (and set
> `LOOKOUT_SECURE_COOKIES=true`) before exposing it. Per-agent mTLS is on the roadmap.

---

## Enroll an agent

On each machine you want to monitor, download or build the `lookout-agent` binary
(also published on the [Releases page](https://github.com/jsdosanj/lookout/releases))
and point it at your server with the same `LOOKOUT_TOKEN`:

```bash
# build it (or use the release binary)
go build -o lookout-agent ./cmd/lookout-agent

# report once to your control plane
./lookout-agent run --server https://monitor.example.com --token secret --once

# run continuously, reporting every minute (default)
./lookout-agent run --server https://monitor.example.com --token secret
```

**How enrollment works.** On first contact the agent presents the shared
`LOOKOUT_TOKEN`, the control plane issues it a unique **per-agent token**, and the
agent persists that token locally (under your user config dir) and uses it for every
subsequent report — so you can later require per-agent tokens and retire the shared
one (`LOOKOUT_REQUIRE_AGENT_TOKEN=true`). Reports are sent **outbound-only**; the
agent never listens for connections and never follows redirects.

To inspect what a host would send without enrolling, run `./lookout-agent report`
to print the JSON report to stdout.

---

## Alerting model

1. **Rules.** A rule matches a host (or `*` for the fleet), a minimum severity
   (warning / critical), the channels to notify, a flap window, and a reminder
   cadence. A fleet-wide "warning and above" rule is seeded on first run. Rules are
   persisted and editable from the dashboard (behind the *manage alerts* permission).
2. **Evaluation.** Every ingested report is scored to ok / warning / critical /
   stale. A background **sweeper** re-evaluates the whole fleet every minute, so a
   host that simply goes silent still fires a *stale* alert.
3. **Delivery.** When a host crosses a rule's threshold, Lookout notifies the rule's
   channels, **deduplicates** the ongoing incident, **damps flapping**, and sends an
   **escalation reminder** on a cadence until the host recovers — then a **resolve**
   notice. Operators can **acknowledge** or **snooze** an open incident to silence
   reminders; a worsening severity re-alerts and recovery still sends the all-clear.
4. **Channels.** Configure with `LOOKOUT_ALERT_WEBHOOKS` (comma-separated Slack /
   Teams / generic incoming-webhook URLs — each validated by an **SSRF guard** before
   any request). `LOOKOUT_ALERT_EMAIL` registers recipients; live email delivery goes
   through a shared notification service when `LOOKOUT_NOTIFY_SERVICE_URL` /
   `LOOKOUT_NOTIFY_SERVICE_TOKEN` are set, otherwise the local email channel returns
   an explicit not-configured error (it never fakes a send).

Full details, every flag and env var, and the security model are in the
[user manual](docs/manual/README.md) — see
[Alerting](docs/manual/alerting.md) and the
[Configuration reference](docs/manual/configuration.md).

---

## Roadmap

These are **not built yet** — tracked here and in
[`docs/manual/roadmap.md`](docs/manual/roadmap.md):

- **PostgreSQL + time-series datastore** — the embedded SQLite store ships today
  (config/state/history via `--data`); Postgres and a proper time-series store for
  high-cardinality metrics are still planned (today: the ~3-hour in-memory window).
- **Per-agent mTLS** — short-lived per-host credentials instead of bearer-over-TLS.
- **Maintenance windows**, persisted ack/snooze + activity log, more integrations,
  and per-group/per-host RBAC scoping.

---

## Documentation

The full self-service manual lives in [`docs/manual/`](docs/manual/README.md):
[Getting started](docs/manual/getting-started.md) ·
[Monitoring](docs/manual/monitoring.md) ·
[Alerting](docs/manual/alerting.md) ·
[Users, auth & RBAC](docs/manual/users-auth-rbac.md) ·
[Configuration](docs/manual/configuration.md) ·
[Security & privacy](docs/manual/security-privacy.md) ·
[Troubleshooting](docs/manual/troubleshooting.md) ·
[Roadmap](docs/manual/roadmap.md).

Architecture lives in [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md); engineering
guidelines in [CLAUDE.md](CLAUDE.md). A static demo of the dashboard is generated
into [`docs/`](docs) with `go run ./cmd/lookout-demo`.

## Contributing

Issues and PRs are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md). The bar is
simply: `go build ./... && go vet ./... && go test ./...` all pass. Please also read
the [Code of Conduct](CODE_OF_CONDUCT.md) and report security issues privately per
[SECURITY.md](SECURITY.md).

## License

Lookout is free and open source under the **Apache License 2.0** — see
[LICENSE](LICENSE). Copyright 2026 Jasvant Dosanjh.
