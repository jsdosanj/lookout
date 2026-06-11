# Lookout

**Open-source IT infrastructure monitoring — built for humans, not just sysadmins.**

Lookout watches your servers (Ubuntu, Debian, RHEL, Rocky, CentOS, AlmaLinux,
Windows, macOS) and tells you, in plain English, whether they're healthy — and
warns you *before* something breaks. You install a small **agent** on each server;
it reports system specs, installed packages, and running services to a central
dashboard. Self-host it for free, or pay us to host it.

See **[IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md)** for the architecture and
roadmap, and **[CLAUDE.md](CLAUDE.md)** for engineering guidelines.

## Status

**Phase 1–2 — agent + control plane + dashboard (working MVP).** The agent
collects a host report and reports it to the control plane, which computes
plain-English health and serves a dashboard (overview + per-server detail).
Alerting, RBAC/MFA, plugins, and a SQLite store are the next phases.

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
LOOKOUT_TOKEN=your-secret ./lookout-server

# on each server, point the agent at it
./lookout-agent run --server http://YOUR_HOST:8080 --token your-secret
```

The dashboard shows every server with a plain-English status (ok / warning /
critical / stale), memory and disk usage, and a per-server detail page.

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
