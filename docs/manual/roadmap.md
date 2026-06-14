# Roadmap & deferred items

This page is the honest ledger of **what works today**, **what is scaffolded but not
yet wired**, and **what is planned**. Lookout's engineering norm is to never fake a
capability — where something isn't live, it returns an explicit error rather than
pretending to succeed. The rest of this manual marks these inline; here they are in
one place.

## Works today (current MVP + Wave-1 alerting)

- **Agent**: collects a full host report (`report`), reports to the control plane on a
  timer (`run`), per-agent token enrollment, outbound-only, no shell, pure-stdlib
  collection.
- **Control plane**: ingests reports, computes plain-English health, keeps ~3h of
  per-host history, serves the dashboard, atomic JSON persistence.
- **Dashboard**: Overview (fleet + OS/encryption widgets), per-host detail with
  charts, Guides, Integrations, Settings, Notifications.
- **Auth**: email+password (bcrypt), TOTP MFA, Google/GitHub SSO (verified email, no
  self-provisioning), RBAC (owner/admin/operator/viewer), sessions with GC, CSRF,
  brute-force lockout, org units.
- **Alerting**: rule engine (server/severity/channels/flap-window/repeat), dedupe,
  flap-damping, escalation reminders, resolve notices, the stale-host sweeper,
  acknowledge/snooze, dashboard rule editing, recent-activity log.
- **Channels**: webhook (Slack/Teams/generic) — **live**; email via the shared
  notification service — **live when configured**.
- **Security**: SSRF guard on outbound webhook/notify URLs (with DNS-rebinding
  re-check), constant-time token checks, TOFU hostname pinning, hardening headers,
  `0600` data files.

## Deferred (scaffolded, intentionally not yet live)

| Item | Status today | Where it's discussed |
| --- | --- | --- |
| **Live SMTP email** | Payload/rendering is real and unit-tested; the SMTP send is not implemented. The local `email` channel returns "SMTP not configured" / "live SMTP send not yet implemented" rather than faking. `LOOKOUT_SMTP_*` is reserved but not read. Use the notify service for live email. | [Alerting → email](alerting.md#email-channel-and-the-notify-service) |
| **The notify-service *server*** | Lookout speaks the `POST /notify/send` client contract; the service that receives it (and holds the provider key, dedupe, retry, audit) is a separate platform component, not in this repo. | [Alerting → email](alerting.md#email-channel-and-the-notify-service) |
| **Acknowledge/snooze persistence** | Acks and snoozes are tracked **in memory**; a control-plane restart loses them, so reminders can resume on a still-open incident. | [Alerting → acknowledge & snooze](alerting.md#acknowledge--snooze) |
| **Recent-activity persistence** | The alert activity log is in memory (last 50, shows 20); cleared on restart. | [Alerting → recent activity](alerting.md#recent-alert-activity) |
| **Universal Collector (`collect`)** | Experimental: signs and ships envelopes to a separate "Keystone" ingest plane; uses a static local capability grant with a `TODO` to fetch a signed policy bundle from `/v1/policy`. Does **not** feed the Lookout dashboard. | [Monitoring → Universal Collector](monitoring.md#the-universal-collector-collect-subcommand) |

## Planned (not yet built)

From the project's [implementation plan](../../IMPLEMENTATION_PLAN.md):

- **Configurable thresholds & custom checks** — per-host overrides and
  Nagios-compatible + simple custom plugins (drop a script + manifest).
- **Scheduled maintenance windows** — first-class silencing for planned work (today:
  snooze / narrow the rule / stop the agent).
- **Per-agent mTLS** — replace the bearer-token-over-TLS-proxy MVP transport with
  mutual TLS and short-lived per-host credentials.
- **Real datastore** — SQLite/PostgreSQL for config/state and a proper time-series
  database for metrics/history (replacing the JSON store and 180-sample window).
- **More integrations** — the Integrations page lists many connectors (Jira,
  ServiceNow, Jamf, Intune, Kandji, JumpCloud, Active Directory, Sightline, SMS, …)
  marked **In development**; only Slack/Teams/webhook are live today.
- **Per-group / per-host access scoping** — org units exist as metadata; scoping RBAC
  by group/host is planned.
- **SMS alerting** — listed as in development on the Notifications page.
- **Sightline integration** — feed Lookout inventory/posture into Sightline as
  compliance evidence.
- **Compliance evidence pack** — threat model, SBOM, dependency scanning, and a
  NIST/HIPAA/SOC2/GDPR control mapping (alignment + evidence, not certification).

> If you build against Lookout, treat anything in **Deferred** or **Planned** as not
> guaranteed. Everything in **Works today** is exercised by the test suite
> (`go test ./...`).
