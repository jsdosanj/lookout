# FAQ

Common questions, with links to the full answers.

### What is Lookout, in one sentence?

A self-hostable, open-source server-monitoring tool: a small outbound-only **agent**
on each machine reports specs/packages/services to a central **control plane** that
shows plain-English health on a dashboard and alerts you when something breaks.

### Is it free?

Yes — Lookout is open source under the **GNU AGPL-3.0**. Self-host it for free. (A
paid managed-hosting option is part of the broader plan, but the software here is
free.)

### What OSes does the agent support?

Linux (Ubuntu, Debian, RHEL, Rocky, CentOS, AlmaLinux), Windows, and macOS. It's a
single static Go binary per OS/arch with no runtime dependencies.

### Do I have to open ports on my servers?

No. The agent is **outbound-only** — it never opens a listening port. It just needs to
reach the control plane's address (default port 8080) outbound. See
[Security & privacy](security-privacy.md).

### What exactly does the agent collect? Does it read my files or data?

It collects **infrastructure inventory and health telemetry** only: host facts, CPU/
memory/disk/network, installed packages, and services. No file contents, no
application data. Run `lookout-agent report` to see the exact JSON. Detail:
[Monitoring](monitoring.md#what-the-agent-collects).

### How does Lookout decide a host is "warning" or "critical"?

Disk ≥ 80% = warning, disk ≥ 90% = critical, memory ≥ 90% = warning, no report for
5 minutes = stale; worst signal wins, with a plain-English reason. These thresholds
aren't configurable yet, but **which severities alert and where** is — via rules.
Detail: [Monitoring → health model](monitoring.md#the-health--severity-model).

### Can I set custom thresholds or write my own checks (Nagios plugins)?

Not yet. Custom thresholds, per-host overrides, and Nagios-compatible/custom plugins
are on the roadmap. Today the health thresholds are fixed; you control alerting via
[rules](alerting.md#rules). See [Roadmap](roadmap.md).

### How do I get alerts?

Set `LOOKOUT_ALERT_WEBHOOKS` to a Slack/Teams/generic webhook URL and restart — that's
the quickest live path. A default rule then alerts on any host at warning+. Detail:
[Alerting](alerting.md). Recipe: [Set up Slack alerts](how-do-i.md#set-up-slack-or-teams-alerts).

### Does email work?

Live email works **only** through the shared notification service
(`LOOKOUT_NOTIFY_SERVICE_URL` + `_TOKEN`). Direct **SMTP is deferred/not implemented**;
without the notify service the `email` channel returns "SMTP not configured" on send.
Use webhooks for self-contained live alerting. Detail:
[Alerting → email](alerting.md#email-channel-and-the-notify-service).

### Why didn't I get an alert?

Most often: no channel configured (alerting is off), no matching rule, the problem
wasn't confirmed across the flap window, or the channel delivery `failed`. Walk
through [Troubleshooting → no alerts firing](troubleshooting.md#no-alerts-firing-at-all).

### A host went down — will Lookout tell me, even though it stopped reporting?

Yes. A background **sweeper** runs every minute and flags a host as **stale** after 5
minutes of silence, which fires a stale alert (with alerting on and a rule whose
minimum severity is at or below stale — the default qualifies). Detail:
[Alerting → stale-host sweeper](alerting.md#the-stale-host-sweeper).

### My webhook URL was rejected. Why?

The **SSRF guard** blocks URLs that resolve to internal addresses (loopback, private,
link-local incl. cloud metadata, CGNAT) or non-`http(s)` schemes, to stop the server
being tricked into fetching internal services. Use a **public** URL, or front an
internal receiver with a public proxy. Detail:
[SSRF guard](alerting.md#the-ssrf-guard-on-webhooks).

### I acknowledged an incident but got pinged again.

Either the severity **worsened** (which clears the ack by design), a **snooze
expired**, or the **server restarted** (acks are in memory and reset). Detail:
[acknowledge & snooze](alerting.md#acknowledge--snooze) and
[troubleshooting](troubleshooting.md#acknowledging-doesnt-stop-reminders).

### Can I schedule a maintenance window?

Not as a first-class feature yet. Snooze the incident, temporarily narrow/delete the
rule, or stop the agent (mind that it'll go stale). Recipe:
[Schedule maintenance](how-do-i.md#schedule-maintenance-silence-a-host-during-planned-work).

### Who can change alert rules?

Owners, admins, and operators (the `manage_alerts` permission). Viewers can see the
dashboard but not rules/incidents/activity. Detail:
[RBAC](users-auth-rbac.md#roles--permissions-rbac).

### Is the dashboard secure to expose to the internet?

Not on plain HTTP. Run it **behind a TLS proxy**, set `LOOKOUT_SECURE_COOKIES=true`,
use strong passwords + MFA, and lock agents to per-agent tokens. Detail:
[Security & privacy](security-privacy.md).

### Where is my data stored? How do I back it up?

In JSON files (`lookout-*.json`, mode `0600`) in the server's working directory. Stop
the server and copy them; they contain secrets, so protect the backup. Detail:
[Configuration → data files](configuration.md#data-files).

### Does it use a database?

Not yet — the MVP persists to JSON files and keeps ~3h of in-memory/on-disk history
per host. SQLite/Postgres + a real time-series database are on the roadmap. See
[Roadmap](roadmap.md).

### What's the difference between `lookout-agent run` and `lookout-agent collect`?

`run` reports to the **Lookout dashboard** (what this manual is about). `collect` is an
experimental pipeline that ships **signed envelopes** to a separate "Keystone" ingest
plane for the broader DosanjhLabs suite — it does **not** feed this dashboard. Detail:
[Universal Collector](monitoring.md#the-universal-collector-collect-subcommand).

### Is Lookout HIPAA / SOC 2 / NIST / GDPR certified?

No. Lookout builds *toward* alignment with those frameworks and documents its
controls, but certification is an organizational audit process, not a property of the
software — and no one can honestly claim it for you. See
[Security & privacy → design principles](security-privacy.md#design-principles).

### How do I build it / run the tests?

`go build ./cmd/...`, `go test ./...`, `go vet ./...`. Requires Go 1.26+. Detail:
[Getting started](getting-started.md#1-get-the-code--build).
