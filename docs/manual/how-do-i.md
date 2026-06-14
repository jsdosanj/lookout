# How do I…? — recipe index

Short, copy-paste task recipes. Each links to the fuller explanation.

- [Add a host (monitor a new server)](#add-a-host-monitor-a-new-server)
- [Set up Slack (or Teams) alerts](#set-up-slack-or-teams-alerts)
- [Set up a generic webhook / PagerDuty / Opsgenie](#set-up-a-generic-webhook--pagerduty--opsgenie)
- [Set up live email alerts](#set-up-live-email-alerts)
- [Create an alert rule for one server only](#create-an-alert-rule-for-one-server-only)
- [Make alerts less noisy](#make-alerts-less-noisy)
- [Snooze or acknowledge an incident](#snooze-or-acknowledge-an-incident)
- [Schedule maintenance (silence a host during planned work)](#schedule-maintenance-silence-a-host-during-planned-work)
- [Add a user / change a role](#add-a-user--change-a-role)
- [Turn on MFA for myself](#turn-on-mfa-for-myself)
- [Turn on Google/GitHub SSO](#turn-on-googlegithub-sso)
- [Lock the fleet to per-agent tokens](#lock-the-fleet-to-per-agent-tokens)
- [Re-enroll or replace an agent](#re-enroll-or-replace-an-agent)
- [Run behind TLS](#run-behind-tls)
- [Back up / move my Lookout install](#back-up--move-my-lookout-install)
- [See exactly what data a host sends](#see-exactly-what-data-a-host-sends)

---

## Add a host (monitor a new server)

1. Copy the right `lookout-agent` binary to the server.
2. Run it pointed at your control plane with the shared token:
   ```bash
   ./lookout-agent run --server http://CONTROL_PLANE:8080 --token YOUR_SHARED_TOKEN
   ```
3. Within ~1 minute it appears on the Overview. Make it permanent with a service unit
   ([systemd example](getting-started.md#running-it-as-a-service)).

Full detail: [Getting started → deploy & enroll](getting-started.md#4-deploy--enroll-the-collector-agent).

---

## Set up Slack (or Teams) alerts

1. In Slack, create an **Incoming Webhook** for the target channel and copy its
   `https://hooks.slack.com/services/…` URL. (Teams: add an **Incoming Webhook**
   connector and copy its URL.)
2. Set it on the control plane and restart:
   ```bash
   LOOKOUT_ALERT_WEBHOOKS='https://hooks.slack.com/services/T000/B000/XXXX' ./lookout-server
   ```
3. You'll see `alerting enabled: 1 channel(s), 1 rule(s)`. The seeded rule alerts on
   any host at warning+ across this channel. Verify on `/notifications` → **Recent
   alert activity** the next time something crosses a threshold.

Detail: [Alerting → webhook channel](alerting.md#webhook-channel-slack--teams--generic).

---

## Set up a generic webhook / PagerDuty / Opsgenie

Same as above — any **public** HTTPS endpoint that accepts a JSON `{"text": "…"}` POST
works. Comma-separate multiple targets:

```bash
LOOKOUT_ALERT_WEBHOOKS='https://hooks.slack.com/...,https://events.pagerduty.com/...,https://ops.example.com/hook'
```

They become channels `webhook`, `webhook-2`, `webhook-3`. Each is SSRF-validated;
internal addresses are rejected ([SSRF guard](alerting.md#the-ssrf-guard-on-webhooks)).

---

## Set up live email alerts

Live email goes through the shared notification service (local SMTP is deferred):

```bash
LOOKOUT_ALERT_EMAIL='oncall@example.com,owner@example.com' \
LOOKOUT_NOTIFY_SERVICE_URL='https://api.example.com' \
LOOKOUT_NOTIFY_SERVICE_TOKEN='the-service-token' \
  ./lookout-server
```

Look for `alert email: live delivery via shared notification service`. Then add the
`email` channel to a rule on `/notifications`. Without the service vars, the `email`
channel exists but every send is recorded **failed** ("SMTP not configured") — use
webhooks instead. Detail:
[Alerting → email](alerting.md#email-channel-and-the-notify-service).

---

## Create an alert rule for one server only

On **`/notifications`** (you need `manage_alerts`) → **Add or update a rule**:

- **Name:** e.g. `db-01 critical`.
- **Server:** the exact host id, e.g. `db-01`.
- **Fire at:** `critical and above`.
- **Flap window / Reminder:** as you like (e.g. 2 / 15).
- **Channels:** tick the ones to use.
- **Save rule.** It goes live immediately (no restart).

Detail: [Alerting → configuring rules](alerting.md#configuring-rules-from-the-dashboard).

---

## Make alerts less noisy

On the rule (edit via the same form, same name):

- **Raise the flap window** (e.g. 3–4) so flapping must persist before firing.
- **Increase the reminder cadence** minutes, or set it to **0** (never remind).
- **Raise minimum severity** to `critical and above` so warnings don't page.

Detail: [Troubleshooting → too many alerts](troubleshooting.md#too-many-alerts--flapping).

---

## Snooze or acknowledge an incident

On **`/notifications`** → **Open incidents**:

- **Acknowledge** — stops reminders for the life of this incident.
- **Snooze 1h** — stops reminders for one hour, then they resume if still open.

A worsening severity re-alerts even if acked; recovery still notifies. Acks are
**in memory** and reset on a server restart. Detail:
[Alerting → acknowledge & snooze](alerting.md#acknowledge--snooze).

---

## Schedule maintenance (silence a host during planned work)

There is **no dedicated maintenance-window feature yet**. To avoid paging during
planned work, use one of these:

- **Snooze the incident** once it opens (e.g. Snooze 1h), re-snoozing as needed. Best
  when work is short.
- **Temporarily narrow the rule** — e.g. raise its minimum severity, or `Delete` the
  host-specific rule before the work and re-create it after. (Rule edits are
  instant.)
- **Stop the agent on that host** during the work — but note it will then go **stale**
  after 5 minutes, which itself can alert; pair this with snoozing/removing the rule.

A first-class scheduled-maintenance window is on the roadmap; see
[Roadmap](roadmap.md).

---

## Add a user / change a role

As owner/admin, go to **`/admin/users`**:

- **Create user:** email, name, role, optional password (omit for SSO-only).
- **Change role:** pick a new role — this revokes their sessions so it takes effect on
  next login.
- **Disable:** instantly revokes their sessions; they can't log in.

Roles: [RBAC](users-auth-rbac.md#roles--permissions-rbac).

---

## Turn on MFA for myself

Go to **`/account`** → **Set up two-factor** → scan the QR/secret into an authenticator
app → enter a code to confirm. Next login becomes two-step. Detail:
[TOTP MFA](users-auth-rbac.md#totp-mfa).

---

## Turn on Google/GitHub SSO

Set the provider's client id/secret plus your public base URL, and configure the
provider's callback to `<BASE_URL>/auth/<provider>/callback`:

```bash
LOOKOUT_BASE_URL='https://monitor.example.com' \
LOOKOUT_OAUTH_GOOGLE_CLIENT_ID='...' LOOKOUT_OAUTH_GOOGLE_CLIENT_SECRET='...' \
  ./lookout-server
```

Then pre-create each user (same email, no password). Detail:
[SSO / OAuth](users-auth-rbac.md#sso--oauth-google--github).

---

## Lock the fleet to per-agent tokens

After every agent has enrolled (each has a persisted per-agent token), set:

```bash
LOOKOUT_REQUIRE_AGENT_TOKEN=true ./lookout-server
```

The shared token is then rejected on `/report`; only enrolled per-agent tokens work.
Detail: [agent identity](users-auth-rbac.md#agent-identity-tofu-and-per-agent-tokens).

---

## Re-enroll or replace an agent

If you re-imaged a host (new identity) and hit `409 hostname is claimed by another
agent`:

1. Stop the control plane.
2. Edit `lookout-agents.json`: remove the `hostname_pins` entry for that hostname
   (and optionally the old agent).
3. Restart. The new agent's next report re-pins the hostname.

On the host, deleting `~/.config/lookout-agent/agent-token` forces a fresh enrollment
on the next run (needs a valid shared token, and `LOOKOUT_REQUIRE_AGENT_TOKEN` must
not be blocking that re-enroll). Detail:
[Troubleshooting](troubleshooting.md#hostname-is-claimed-by-another-agent-409).

---

## Run behind TLS

1. Bind the server to localhost/private: `./lookout-server --addr 127.0.0.1:8080`.
2. Put a TLS proxy (Caddy/nginx/Cloudflare Tunnel) in front, terminating HTTPS and
   forwarding to `127.0.0.1:8080`.
3. Set `LOOKOUT_SECURE_COOKIES=true`.
4. Point agents at the `https://` proxy URL.

Detail: [Security & privacy → transport](security-privacy.md#transport--the-tls-caveat).

---

## Back up / move my Lookout install

Stop the server and copy the data files (default in the working directory):
`lookout-data.json`, `lookout-users.json`, `lookout-agents.json`, `lookout-rules.json`.
They're `0600` and contain secrets (password hashes, MFA secrets, agent tokens) —
**protect the backup**. Restore by placing them where the flags point and starting the
server. Detail: [Configuration → data files](configuration.md#data-files).

---

## See exactly what data a host sends

On the host, run:

```bash
./lookout-agent report
```

It prints the full report JSON and contacts nothing. Detail:
[Monitoring → the report on the wire](monitoring.md#the-report-on-the-wire).
