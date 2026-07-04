# Getting started

This page takes you from nothing to a working Lookout: build the binaries, run the
control plane, create your first login, enroll an agent on a server, and see it
appear on the dashboard. Every command is copy-paste ready.

- [What Lookout is](#what-lookout-is)
- [Prerequisites](#prerequisites)
- [1. Get the code & build](#1-get-the-code--build)
- [2. Run the control plane (server)](#2-run-the-control-plane-server)
- [3. Log in for the first time](#3-log-in-for-the-first-time)
- [4. Deploy & enroll the collector agent](#4-deploy--enroll-the-collector-agent)
- [5. Your first dashboard view](#5-your-first-dashboard-view)
- [Running it as a service (Linux/macOS/Windows)](#running-it-as-a-service)
- [The static demo](#the-static-demo)

---

## What Lookout is

Lookout watches your servers — **Ubuntu, Debian, RHEL, Rocky, CentOS, AlmaLinux,
Windows, and macOS** — and tells you in plain English whether they're healthy, and
warns you *before* something breaks.

You install a small **agent** on each server. It reports the host's system specs,
installed packages, and running services to a central **control plane**, which
computes a plain-English status (ok / warning / critical / stale) and serves a
**dashboard**. You can self-host the whole thing for free (it's Apache-2.0
open-source).

What works **today** (the current MVP + Wave-1 alerting):

- The agent collects a full host report and prints it, or reports it to the control
  plane on a timer.
- The control plane ingests reports, stores the latest state plus a short
  time-series history, and computes health.
- A web dashboard behind **login + RBAC + optional MFA/SSO** shows the fleet and
  per-server detail with charts.
- **Built-in alerting**: a rule engine delivers Slack/Teams/webhook notifications
  with dedupe, flap-damping, escalation reminders, a stale-host sweeper, and
  acknowledge/snooze.

What is **deferred** (documented honestly, not yet wired):

- **Live SMTP email** — the email payload is real and tested, but the actual SMTP
  send is not implemented. Live email today goes through the shared notification
  service instead (see [Alerting](alerting.md#email-channel-and-the-notify-service)).
- **The notification-service *server*** — Lookout speaks the `POST /notify/send`
  contract as a client, but the service itself is a separate platform component.
- **Acknowledgement persistence** — acks/snoozes live in memory and reset if the
  server restarts. See [Roadmap & deferred items](roadmap.md).

---

## Prerequisites

- **Go 1.26 or newer** to build (`go version` to check). Lookout's only third-party
  dependency is `golang.org/x/crypto`; everything else is the Go standard library.
- A machine to run the **control plane** on. It can be the same machine you monitor,
  but in production it's usually separate.
- One or more **servers to monitor**, each of which can reach the control plane over
  the network (outbound from the monitored host to the server's `--addr`, default
  port `8080`).
- **No inbound ports** are needed on the monitored hosts. The agent only dials out.

---

## 1. Get the code & build

```bash
git clone https://github.com/jsdosanj/lookout.git
cd lookout

# Build both binaries for your current OS/architecture.
go build -o lookout-server ./cmd/lookout-server
go build -o lookout-agent  ./cmd/lookout-agent
```

To build the agent for **other** platforms (each is a single static binary with no
dependencies), cross-compile:

```bash
GOOS=linux   GOARCH=amd64 go build -o lookout-agent-linux   ./cmd/lookout-agent
GOOS=linux   GOARCH=arm64 go build -o lookout-agent-linux-arm64 ./cmd/lookout-agent
GOOS=windows GOARCH=amd64 go build -o lookout-agent.exe     ./cmd/lookout-agent
GOOS=darwin  GOARCH=arm64 go build -o lookout-agent-macos   ./cmd/lookout-agent
```

You can stamp a version into the agent build with linker flags:

```bash
go build -ldflags "-X main.version=1.0.0" -o lookout-agent ./cmd/lookout-agent
```

Sanity-check the build:

```bash
./lookout-agent version        # prints: lookout-agent <version>
go test ./...                  # run the unit tests
go vet ./...                   # static checks
```

---

## 2. Run the control plane (server)

The minimum you need is a **shared agent token** (so agents can authenticate) and a
**first owner account** (so you can log in). Set them via environment variables:

```bash
LOOKOUT_TOKEN='choose-a-long-random-secret' \
LOOKOUT_ADMIN_EMAIL='you@example.com' \
LOOKOUT_ADMIN_PASSWORD='a-strong-password' \
  ./lookout-server
```

You'll see startup log lines like:

```
Lookout control plane on :8080 (data: lookout-data.json)
created owner account you@example.com
NOTE: no alert channels configured — set LOOKOUT_ALERT_WEBHOOKS to enable alerting
NOTE: LOOKOUT_SECURE_COOKIES != 'true' — cookies not marked Secure (run behind TLS in production)
```

The server listens on `:8080` by default. Open **http://localhost:8080/** — you'll
be redirected to `/login`.

**Where data is stored.** On first run the server creates a handful of JSON files in
the working directory (all `0600`, owner-only):

| File | Default name | Contents | Override flag |
| --- | --- | --- | --- |
| Server/report store | `lookout-data.json` | Latest report + history per host | `--data` |
| User/session store | `lookout-users.json` | Accounts, sessions, org units | `--auth-data` |
| Agent credentials | `lookout-agents.json` | Per-agent tokens, hostname pins | `--agent-data` |
| Alert rules | `lookout-rules.json` | Persisted, editable alert rules | `--rule-data` |

You can change the listen address and file paths with flags — see the
[Configuration reference](configuration.md#server-command-line-flags). Example:

```bash
./lookout-server --addr :9000 --data /var/lib/lookout/data.json
```

> **Owner bootstrap only happens once.** `LOOKOUT_ADMIN_EMAIL` /
> `LOOKOUT_ADMIN_PASSWORD` create the first owner **only when there are zero users**.
> After that, the variables are ignored — add more users from the dashboard at
> `/admin/users`. If you start the server **without** them on a fresh install, it
> logs `NOTE: no users yet…` and you won't be able to log in until you set them and
> restart.

---

## 3. Log in for the first time

1. Browse to **http://localhost:8080/login**.
2. Enter the email + password you set in `LOOKOUT_ADMIN_EMAIL` /
   `LOOKOUT_ADMIN_PASSWORD`. This account has the **owner** role (full access).
3. After login you land on the **Overview** dashboard (it'll be empty until an agent
   reports).

Optional but recommended:

- Turn on **MFA** for your account at **`/account`** → *Set up two-factor* (scan the
  QR/secret with an authenticator app, then confirm a code). See
  [Users, auth & RBAC](users-auth-rbac.md#totp-mfa).
- Add more users at **`/admin/users`** with appropriate roles.
- If you'll run behind TLS, set `LOOKOUT_SECURE_COOKIES=true`. See
  [Security & privacy](security-privacy.md).

---

## 4. Deploy & enroll the collector agent

Copy the appropriate `lookout-agent` binary to each server you want to monitor.

### Try it locally first (no server needed)

To just see what the agent collects on a machine, run:

```bash
./lookout-agent report
```

This prints the full host report as JSON to stdout and exits. It contacts nothing —
handy for verifying the agent works and seeing exactly what data leaves the host.

### Point the agent at your control plane

On each monitored host, run the agent in `run` mode with the server URL and the same
shared token you set as `LOOKOUT_TOKEN`:

```bash
./lookout-agent run --server http://YOUR_CONTROL_PLANE:8080 --token 'choose-a-long-random-secret'
```

What happens on first run:

1. The agent **enrolls**: it presents the shared token to
   `POST /api/v1/agents/enroll` and receives a **unique per-agent token** bound to a
   server-assigned identity.
2. It **persists** that per-agent token (default
   `~/.config/lookout-agent/agent-token`, mode `0600`) and uses it for all future
   reports. The shared token is only needed for that first enrollment.
3. It then reports the host on a timer (default **every 1 minute**) until you stop
   it (Ctrl-C / SIGTERM).

Useful flags:

```bash
# Report once and exit (good for testing or cron-style runs).
./lookout-agent run --server http://HOST:8080 --token SECRET --once

# Report every 30 seconds instead of the 1-minute default.
./lookout-agent run --server http://HOST:8080 --token SECRET --interval 30s

# Provide the token via the environment instead of --token.
LOOKOUT_TOKEN=SECRET ./lookout-agent run --server http://HOST:8080

# Put the persisted per-agent token somewhere specific.
./lookout-agent run --server http://HOST:8080 --token SECRET --token-file /etc/lookout/agent-token
```

> **Enrollment is migration-safe and falls back gracefully.** If the persisted
> per-agent token already exists, the agent reuses it (no re-enroll). If enrollment
> fails (e.g. an older control plane, or no shared token), the agent prints a notice
> and falls back to reporting with the shared token directly. See
> [Users, auth & RBAC](users-auth-rbac.md#agent-identity-tofu-and-per-agent-tokens)
> and [Security & privacy](security-privacy.md) for the trust model.

> **The `collect` subcommand is different.** `lookout-agent collect --ingest-url …`
> drives the experimental **Universal Collector** pipeline (signed envelopes shipped
> to a separate "Keystone" ingest control plane). It is **not** how you feed the
> Lookout dashboard — use `run` for that. `collect` is documented in
> [Monitoring](monitoring.md#the-universal-collector-collect-subcommand) and
> [Roadmap & deferred items](roadmap.md).

---

## 5. Your first dashboard view

Within one report interval (≤ 1 minute by default), refresh the dashboard at
**http://localhost:8080/**. The host you enrolled appears as a card, color-coded by
health:

- **ok** — all good.
- **warning** — something is trending the wrong way (e.g. a disk over 80% full).
- **critical** — act now (e.g. a disk over 90% full).
- **stale** — the agent hasn't reported in over 5 minutes; the box may be down.

Click any server card to open its **detail page**: CPU / memory / disk usage over
time (charts), plus host facts (OS, kernel, uptime, encryption status), installed
packages, and services.

The other dashboard pages:

- **Overview** (`/`) — every server, plus fleet widgets (OS distribution, disk
  encryption summary).
- **Notifications** (`/notifications`) — alert status, rules, open incidents, recent
  delivery activity. See [Alerting](alerting.md).
- **Integrations** (`/integrations`) — connectors (Slack/Teams/webhook live; others
  in development).
- **Guides** (`/guides`) — built-in plain-English explainers.
- **Settings** (`/settings`) — theme + which Overview widgets to show (saved in your
  browser).
- **Users** (`/admin/users`) — admin user management (owners/admins only).

That's a working Lookout. Next, read [Monitoring](monitoring.md) to understand the
data and health model, then [Alerting](alerting.md) to get notified when something
goes wrong.

---

## Running it as a service

Lookout binaries are plain executables; run them under whatever service manager your
OS uses. The agent is **outbound-only**, so it needs no firewall changes.

### Linux (systemd) — agent example

`/etc/systemd/system/lookout-agent.service`:

```ini
[Unit]
Description=Lookout monitoring agent
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/lookout-agent run --server http://YOUR_CONTROL_PLANE:8080
Environment=LOOKOUT_TOKEN=choose-a-long-random-secret
Restart=on-failure
RestartSec=10
# Least privilege: the agent only reads system info and dials out.
DynamicUser=yes
StateDirectory=lookout-agent
Environment=HOME=/var/lib/lookout-agent

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now lookout-agent
sudo systemctl status lookout-agent
journalctl -u lookout-agent -f   # follow logs
```

> Set `HOME`/`StateDirectory` so the agent has a writable place for its persisted
> per-agent token (`$HOME/.config/lookout-agent/agent-token`), or pass an explicit
> `--token-file`.

### Linux (systemd) — control-plane example

`/etc/systemd/system/lookout-server.service`:

```ini
[Unit]
Description=Lookout control plane
After=network-online.target

[Service]
WorkingDirectory=/var/lib/lookout
ExecStart=/usr/local/bin/lookout-server --addr :8080
Environment=LOOKOUT_TOKEN=choose-a-long-random-secret
Environment=LOOKOUT_ADMIN_EMAIL=you@example.com
Environment=LOOKOUT_ADMIN_PASSWORD=a-strong-password
Environment=LOOKOUT_SECURE_COOKIES=true
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

In production put a **TLS-terminating reverse proxy** (Caddy, nginx, a Cloudflare
Tunnel, etc.) in front of `lookout-server` — see [Security & privacy](security-privacy.md).

### macOS (launchd)

Create a `~/Library/LaunchAgents/com.lookout.agent.plist` that runs
`lookout-agent run --server … --token …` with `RunAtLoad` and `KeepAlive`. Load it
with `launchctl load …`.

### Windows

Use [NSSM](https://nssm.cc/) or `sc.exe` to register `lookout-agent.exe run --server
… --token …` as a Windows service, or run the control plane the same way.

---

## The static demo

To preview the dashboard UI without enrolling anything, generate the static demo:

```bash
go run ./cmd/lookout-demo     # writes sample HTML pages into ./docs
```

This writes `docs/index.html`, per-server pages, and the other dashboard pages as
plain HTML driven by sample data. Host it by enabling **GitHub Pages → source:
`/docs`**. (The demo generator only writes its own `.html` files into `docs/` — it
does **not** touch this `docs/manual/` documentation.)
