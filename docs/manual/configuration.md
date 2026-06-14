# Configuration reference

Everything you can configure, in one place. Lookout is configured by **environment
variables** (mostly on the control plane) and a few **command-line flags**. There is
no config file to edit — rules are the one thing edited in the UI and stored as JSON.

- [Server command-line flags](#server-command-line-flags)
- [Agent command-line flags](#agent-command-line-flags)
- [Environment variables — control plane](#environment-variables--control-plane)
  - [Core / auth](#core--auth)
  - [Alerting](#alerting)
  - [SSO / OAuth](#sso--oauth)
- [Environment variables — agent](#environment-variables--agent)
- [Data files](#data-files)
- [Defaults & built-in constants](#defaults--built-in-constants)
- [A complete production example](#a-complete-production-example)

---

## Server command-line flags

`lookout-server [flags]`

| Flag | Default | Meaning |
| --- | --- | --- |
| `--addr` | `:8080` | Listen address (host:port). |
| `--data` | `lookout-data.json` | Path to the server/report store. |
| `--auth-data` | `lookout-users.json` | Path to the users/sessions/org-units store. |
| `--agent-data` | `lookout-agents.json` | Path to the per-agent credentials + hostname-pin store. |
| `--rule-data` | `lookout-rules.json` | Path to the persisted alert-rules file. |

Example:

```bash
./lookout-server --addr 127.0.0.1:9000 \
  --data /var/lib/lookout/data.json \
  --auth-data /var/lib/lookout/users.json \
  --agent-data /var/lib/lookout/agents.json \
  --rule-data /var/lib/lookout/rules.json
```

---

## Agent command-line flags

The agent has subcommands; flags are per-subcommand.

`lookout-agent report` — no flags. Collects and prints the report as JSON.

`lookout-agent run [flags]` — report to the control plane:

| Flag | Default | Meaning |
| --- | --- | --- |
| `--server` | *(required)* | Control-plane URL, e.g. `http://monitor.example.com:8080`. |
| `--token` | `$LOOKOUT_TOKEN` | Shared enrollment token. |
| `--token-file` | `<user config dir>/lookout-agent/agent-token` | Where the per-agent token is persisted/read. |
| `--interval` | `1m` | How often to report (Go duration, e.g. `30s`, `2m`). |
| `--once` | `false` | Report once and exit. |

`lookout-agent collect [flags]` — experimental Universal Collector (ships signed
envelopes to a separate ingest plane; **not** the dashboard path):

| Flag | Default | Meaning |
| --- | --- | --- |
| `--ingest-url` | `$LOOKOUT_INGEST_URL` *(required)* | Keystone ingest base URL, e.g. `https://api.example.com`. |
| `--token` | `$LOOKOUT_TOKEN` | One-time enrollment token (first run only). |
| `--state-dir` | `<user config dir>/lookout-agent` | Directory for the agent identity + spool. |
| `--once` | `false` | Run each collector once and exit. |

`lookout-agent version` (also `-v`, `--version`) — print the agent version.

---

## Environment variables — control plane

### Core / auth

| Variable | Required? | Default | Meaning |
| --- | --- | --- | --- |
| `LOOKOUT_TOKEN` | Strongly recommended | *(unset)* | Shared agent enrollment token. Unset = agent reports are **unauthenticated** (dev only; logs a WARNING). |
| `LOOKOUT_REQUIRE_AGENT_TOKEN` | No | `false` | `true` rejects the legacy shared token on `/report`; only enrolled per-agent tokens are accepted. |
| `LOOKOUT_SECURE_COOKIES` | In production | `false` | `true` marks the session cookie `Secure` (HTTPS only). Set when behind TLS. |
| `LOOKOUT_ADMIN_EMAIL` | First run only | *(unset)* | Email for the first **owner** account (only when there are zero users). |
| `LOOKOUT_ADMIN_PASSWORD` | First run only | *(unset)* | Password for the first owner account. |
| `LOOKOUT_BASE_URL` | If using SSO | *(unset)* | Public base URL of the control plane, used to build OAuth redirect URIs, e.g. `https://monitor.example.com`. |

### Alerting

| Variable | Required for | Default | Meaning |
| --- | --- | --- | --- |
| `LOOKOUT_ALERT_WEBHOOKS` | Webhook alerts | *(unset)* | Comma-separated incoming-webhook URLs (Slack/Teams/generic). Each is SSRF-validated; bad ones are logged & skipped. IDs: `webhook`, `webhook-2`, … |
| `LOOKOUT_ALERT_EMAIL` | Email alerts | *(unset)* | Comma-separated recipient addresses. Registers the `email` channel. |
| `LOOKOUT_NOTIFY_SERVICE_URL` | Live email | *(unset)* | Base URL of the shared notification service (Lookout POSTs to `<URL>/notify/send`). SSRF-checked. |
| `LOOKOUT_NOTIFY_SERVICE_TOKEN` | Live email | *(unset)* | Bearer token authenticating Lookout to that service. Without it, email falls back to the not-yet-live local SMTP channel. |
| `LOOKOUT_SMTP_*` | *(reserved)* | — | **Deferred / not read today.** Reserved for the future direct-SMTP transport. |

How the alerting variables combine:

- **`LOOKOUT_ALERT_WEBHOOKS` only** → live webhook alerting (recommended starting
  point).
- **`LOOKOUT_ALERT_EMAIL` + `LOOKOUT_NOTIFY_SERVICE_URL` + `LOOKOUT_NOTIFY_SERVICE_TOKEN`**
  → live email via the shared notification service.
- **`LOOKOUT_ALERT_EMAIL` only** → an `email` channel exists, but every send is
  recorded as **failed** ("SMTP not configured"). See
  [Alerting → email](alerting.md#email-channel-and-the-notify-service).
- **No channel variables** → alerting is **off** (no rules are seeded).

### SSO / OAuth

| Variable | Meaning |
| --- | --- |
| `LOOKOUT_OAUTH_GOOGLE_CLIENT_ID` | Enables Google SSO when set (with the secret). |
| `LOOKOUT_OAUTH_GOOGLE_CLIENT_SECRET` | Google OAuth client secret. |
| `LOOKOUT_OAUTH_GITHUB_CLIENT_ID` | Enables GitHub SSO when set (with the secret). |
| `LOOKOUT_OAUTH_GITHUB_CLIENT_SECRET` | GitHub OAuth client secret. |
| `LOOKOUT_BASE_URL` | Required for SSO so redirect URIs resolve (see above). |

Set the provider callback URL to `<LOOKOUT_BASE_URL>/auth/<provider>/callback`.

---

## Environment variables — agent

| Variable | Used by | Meaning |
| --- | --- | --- |
| `LOOKOUT_TOKEN` | `run`, `collect` | Shared enrollment token (alternative to `--token`). |
| `LOOKOUT_INGEST_URL` | `collect` | Default Keystone ingest base URL (alternative to `--ingest-url`). |

---

## Data files

All written by the control plane in its working directory (or wherever the flags
point), as JSON, mode **`0600`** (owner-only read/write), updated **atomically** (temp
file + rename):

| File | Flag | Contents |
| --- | --- | --- |
| `lookout-data.json` | `--data` | Latest report + rolling history per server. |
| `lookout-users.json` | `--auth-data` | Users (bcrypt hashes, TOTP secrets), sessions, org units. |
| `lookout-agents.json` | `--agent-data` | Per-agent tokens, hostname→identity pins. |
| `lookout-rules.json` | `--rule-data` | Persisted, dashboard-editable alert rules. |

Agent side: the per-agent token is stored at `<user config dir>/lookout-agent/agent-token`
(override with `--token-file`), mode `0600`. The `collect` pipeline stores its
identity + spool under `--state-dir`.

> **Back these up** (especially `lookout-users.json` — it has your accounts and MFA
> secrets — and `lookout-agents.json` — the agent credentials). They contain secrets;
> protect them like any credential store.

---

## Defaults & built-in constants

These are not configurable today but are useful to know:

| Constant | Value | Where it matters |
| --- | --- | --- |
| Report interval (agent default) | `1m` | How often a host reports (`--interval`). |
| Stale threshold | `5m` | No report for this long → status `stale`. |
| Sweeper cadence | `1m` | Background fleet re-evaluation (stale detection). |
| History kept per server | `180 samples` | ~3h at 1-min interval. |
| Report body cap | `8 MiB` | Max accepted `/report` body (DoS guard). |
| Enroll body cap | `1 MiB` | Max accepted `/enroll` body. |
| Session lifetime | `12h` | Fully authenticated session. |
| Pre-MFA session lifetime | `10m` | Password-entered, code-pending. |
| Session GC interval | `10m` | Expired-session cleanup. |
| Login/MFA lockout | `5 fails / 15m → 15m lock` | Brute-force resistance. |
| Webhook send timeout | `10s` | Per webhook/notify-service delivery. |
| Disk warning / critical | `≥ 80%` / `≥ 90%` | Health thresholds. |
| Memory warning | `≥ 90%` | Health threshold. |
| Seeded rule | fleet `*`, `warning+`, flap `2`, repeat `30m` | First-run alerting. |
| Activity log size | `50` kept, `20` shown | Recent alert deliveries. |

---

## A complete production example

A control plane behind TLS, with webhook + email alerting and Google SSO:

```bash
LOOKOUT_TOKEN='REPLACE-with-a-long-random-secret' \
LOOKOUT_REQUIRE_AGENT_TOKEN='true' \
LOOKOUT_SECURE_COOKIES='true' \
LOOKOUT_ADMIN_EMAIL='you@example.com' \
LOOKOUT_ADMIN_PASSWORD='REPLACE-strong-password' \
LOOKOUT_BASE_URL='https://monitor.example.com' \
LOOKOUT_ALERT_WEBHOOKS='https://hooks.slack.com/services/T000/B000/XXXX' \
LOOKOUT_ALERT_EMAIL='oncall@example.com' \
LOOKOUT_NOTIFY_SERVICE_URL='https://api.dosanjhlabs.com' \
LOOKOUT_NOTIFY_SERVICE_TOKEN='REPLACE-service-token' \
LOOKOUT_OAUTH_GOOGLE_CLIENT_ID='REPLACE' \
LOOKOUT_OAUTH_GOOGLE_CLIENT_SECRET='REPLACE' \
  ./lookout-server --addr 127.0.0.1:8080 --data /var/lib/lookout/data.json \
    --auth-data /var/lib/lookout/users.json \
    --agent-data /var/lib/lookout/agents.json \
    --rule-data /var/lib/lookout/rules.json
```

(Bind to `127.0.0.1` and put a TLS proxy in front; see
[Security & privacy](security-privacy.md).)

Agents:

```bash
LOOKOUT_TOKEN='REPLACE-with-a-long-random-secret' \
  ./lookout-agent run --server https://monitor.example.com
```
