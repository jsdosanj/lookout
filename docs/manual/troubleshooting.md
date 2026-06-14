# Troubleshooting

Find your symptom, read the likely cause, apply the fix. For exact log/error strings
see the [Error & log message reference](error-reference.md). For step-by-step task
recipes see [How do I…?](how-do-i.md).

- [First: confirm the basics](#first-confirm-the-basics)
- [Server problems](#server-problems)
  - [Can't log in / "no users yet"](#cant-log-in--no-users-yet)
  - [Locked out (`?err=locked`)](#locked-out-errlocked)
  - [SSO won't sign me in (`?err=noaccount` / `?err=state`)](#sso-wont-sign-me-in)
  - [`forbidden` / `invalid or missing CSRF token`](#forbidden--invalid-or-missing-csrf-token)
- [Agent problems](#agent-problems)
  - [Agent not reporting / host never appears](#agent-not-reporting--host-never-appears)
  - [`hostname is claimed by another agent` (409)](#hostname-is-claimed-by-another-agent-409)
  - [`unauthorized` (401) on report/enroll](#unauthorized-401-on-reportenroll)
  - [Host shows as stale](#host-shows-as-stale)
- [Alerting problems](#alerting-problems)
  - [No alerts firing at all](#no-alerts-firing-at-all)
  - [Webhook rejected by the SSRF guard](#webhook-rejected-by-the-ssrf-guard)
  - [Webhook configured but messages don't arrive](#webhook-configured-but-messages-dont-arrive)
  - [Email not sending / "SMTP not configured"](#email-not-sending--smtp-not-configured)
  - [Acknowledging doesn't stop reminders](#acknowledging-doesnt-stop-reminders)
  - [Too many alerts / flapping](#too-many-alerts--flapping)
  - [A worsening problem re-alerted after I acked it](#a-worsening-problem-re-alerted-after-i-acked-it)

---

## First: confirm the basics

Before deep-diving, check these three:

1. **Is the server up?** `curl -s http://CONTROL_PLANE:8080/healthz` should print
   `ok`.
2. **Can the agent reach the server?** From the monitored host:
   `curl -s http://CONTROL_PLANE:8080/healthz`. If this fails, it's a network/DNS/
   firewall issue, not Lookout.
3. **What do the logs say?** The control plane and the agent both log to stderr.
   Watch them (`journalctl -u lookout-server -f`, `journalctl -u lookout-agent -f`,
   or just the terminal). Most problems announce themselves there.

---

## Server problems

### Can't log in / "no users yet"

**Symptom:** `/login` rejects your credentials, or the log says `NOTE: no users yet`.

**Cause:** The first owner account is created **only on a fresh install** from
`LOOKOUT_ADMIN_EMAIL` + `LOOKOUT_ADMIN_PASSWORD`. If those weren't set on first run,
no account exists.

**Fix:** Set both variables and **restart** the server. They only take effect when
there are **zero** users — if `lookout-users.json` already has users, add accounts
from `/admin/users` instead. If you've lost the only admin and have no other way in,
you can stop the server, remove/rename `lookout-users.json`, restart with the admin
variables to recreate the owner, and then re-add users (you'll lose existing accounts
and sessions).

### Locked out (`?err=locked`)

**Symptom:** Login redirects to `/login?err=locked`.

**Cause:** 5 failed password (or MFA) attempts from your account+IP within 15 minutes
triggers a **15-minute lockout**.

**Fix:** Wait 15 minutes, then log in correctly. The counter clears on success and
the window rolls over. (This is an in-memory, per-instance limiter; restarting the
server also clears it, but don't rely on that in production.)

### SSO won't sign me in

- **`/login?err=noaccount`** — SSO only logs in accounts that **already exist**.
  Create the user first at `/admin/users` (with the **same email** the provider
  returns, no password needed), then sign in via SSO.
- **`/login?err=state`** — the OAuth `state` cookie didn't match (often a stale tab,
  third-party-cookie blocking, or a misconfigured `LOOKOUT_BASE_URL`). Start the SSO
  flow fresh; confirm `LOOKOUT_BASE_URL` matches the URL you actually browse to and
  the provider's callback is `<LOOKOUT_BASE_URL>/auth/<provider>/callback`.
- **`/login?err=sso`** — token exchange or the userinfo/email fetch failed, or the
  provider returned an **unverified** email (rejected on purpose). Re-check the client
  ID/secret and that the account's email is verified at the provider.

### `forbidden` / `invalid or missing CSRF token`

- **`forbidden — you don't have permission for this`** — your role lacks the required
  permission. Editing alert rules/acks needs `manage_alerts`; user admin needs
  `manage_users`. A viewer can't do either. Ask an owner/admin to grant a higher
  role (see [RBAC](users-auth-rbac.md#roles--permissions-rbac)).
- **`invalid or missing CSRF token`** — usually a **stale page/session**. Reload the
  page so the form gets a current token, then resubmit. If it persists, log out and
  back in.

---

## Agent problems

### Agent not reporting / host never appears

Work through these in order:

1. **Network reachability** — from the monitored host, `curl http://SERVER:8080/healthz`
   must return `ok`. If not, fix DNS/routing/firewall first. The agent is
   outbound-only, so this is purely an outbound path.
2. **The `--server` URL** — must be the control plane's address including scheme and
   port, e.g. `http://monitor.example.com:8080`. A missing scheme or wrong port means
   the agent can't connect.
3. **The token** — `--token` (or `$LOOKOUT_TOKEN`) must match the server's
   `LOOKOUT_TOKEN`. A mismatch yields `control plane returned 401 Unauthorized` in the
   agent log.
4. **Is it actually running/looping?** Without `--once`, the agent reports every
   `--interval` (default 1m). Run once in the foreground to see errors directly:
   `./lookout-agent run --server http://SERVER:8080 --token SECRET --once`.
5. **Watch the agent log.** Transient errors print as `report error: …` but are
   non-fatal (a flaky network won't kill the agent). A persistent error there tells
   you exactly what's failing.
6. **Give it a minute.** The dashboard updates when a report lands; with a 1-minute
   interval, allow up to a minute after the agent starts.
7. **Hostname conflict** — if the host is enrolling under a hostname already pinned to
   another identity, reports are rejected with a 409 (next item).

### `hostname is claimed by another agent` (409)

**Symptom:** Agent log shows the report rejected; server log shows `report rejected:
hostname "X" is pinned to a different agent identity`.

**Cause:** TOFU hostname pinning. The hostname was first reported by a **different**
agent identity, and Lookout refuses to let a second identity overwrite that host's
record.

**Fixes:**

- If you **re-imaged/replaced** the host and want the new agent to own that hostname,
  remove the stale pin: stop the server, edit `lookout-agents.json` to delete the
  `hostname_pins` entry (and the old agent if you like), restart.
- If two **different** machines genuinely share a hostname, **rename one** — Lookout
  keys servers by hostname, so duplicates can't coexist.
- If you deleted the persisted per-agent token on a host so it re-enrolled as a *new*
  identity, that new identity won't match the old pin. Either restore the token or
  clear the pin as above.

### `unauthorized` (401) on report/enroll

**Cause:** The presented token isn't accepted. Either the shared token doesn't match
`LOOKOUT_TOKEN`, or `LOOKOUT_REQUIRE_AGENT_TOKEN=true` is set and the agent is still
using the shared token (it hasn't enrolled).

**Fix:** Confirm the token matches. If you've locked down to per-agent tokens, ensure
each agent has **enrolled** (it must successfully call `/api/v1/agents/enroll` once
with the shared token, then persist the per-agent token). To re-enroll, delete the
agent's `agent-token` file and run again with a valid shared token *before* you flip
`LOOKOUT_REQUIRE_AGENT_TOKEN=true`.

### Host shows as stale

**Symptom:** A host's card is **stale**; reason `no report in Nm`.

**Cause:** No report has arrived in over **5 minutes**. The host may be down, the
agent may have stopped, or the network path broke.

**Fix:** Check whether the box is actually up. If it is, check the agent
(running? logging `report error`?) and reachability (`curl …/healthz`). Stale is the
*correct* signal for a silent host — if the host is genuinely down, the
[stale-host sweeper](alerting.md#the-stale-host-sweeper) will (with alerting on) page
you about it.

---

## Alerting problems

### No alerts firing at all

Check, in order:

1. **Is a channel configured?** Alerting is **off** with no channels. The startup log
   shows `NOTE: no alert channels configured…` and `/notifications` shows **Off**. Set
   `LOOKOUT_ALERT_WEBHOOKS` (simplest) and restart. On success you'll see `alerting
   enabled: N channel(s), M rule(s)`.
2. **Is there a matching rule?** On `/notifications` (as a manager), confirm a rule
   matches the host (server `*` or its exact id) with a **minimum severity** at or
   below the problem's severity. The seeded `Fleet: warning and above` covers most
   cases.
3. **Is the problem severe enough and confirmed?** A warning only fires if it reaches
   the rule's floor, and a flap window of 2 means it must be seen **twice in a row**
   first. Brief blips below the window are intentionally suppressed.
4. **Did a delivery attempt happen?** Look at **Recent alert activity**. If you see
   `failed` rows, the rule fired but the channel is misconfigured (next items). If you
   see *nothing*, no rule fired — revisit steps 2–3.
5. **For a silent host:** stale alerts come from the sweeper (every minute) and need a
   rule whose minimum severity is at or below `stale` (the default qualifies).

### Webhook rejected by the SSRF guard

**Symptom:** Startup log: `alert webhook rejected (webhook): host "…" resolves to a
non-public address (…)` (or `scheme "…" not allowed`, or `cannot resolve host …`).
That channel is **skipped**; others still load.

**Cause:** The [SSRF guard](alerting.md#the-ssrf-guard-on-webhooks) refuses URLs that
resolve to loopback/private/link-local/CGNAT/metadata addresses, non-`http(s)`
schemes, or names that don't resolve.

**Fixes:**

- Use a **public** webhook URL. Slack/Teams/PagerDuty/Opsgenie URLs are public and
  pass.
- If your receiver is **internal**, put a public-facing relay/proxy in front of it and
  point Lookout at the public address. The guard is deliberately strict and cannot be
  disabled.
- For `cannot resolve host`, fix the typo / DNS — the control plane must be able to
  resolve the name.
- Remember the check **re-runs on every send**, so a URL that passes at startup but
  later resolves internally will start failing (recorded as `failed` in activity).

### Webhook configured but messages don't arrive

The rule fired (you see a row in **Recent alert activity**) but it's `failed`, or
nothing arrives:

- **`failed` with a status code** — the webhook returned `≥ 300`. The URL is wrong or
  expired (regenerate it in Slack/Teams), or the receiver rejected the payload.
- **`failed` with an SSRF/resolve error** — see the previous item.
- **`sent` but you see nothing** — check the *destination* (right Slack channel? Teams
  connector enabled? Did the receiver filter it?). Lookout sends `{"text": "…"}`,
  which Slack and Teams accept; a custom receiver must handle that shape.
- **10-second timeout** — a slow receiver can time out; the attempt is recorded as
  failed.

### Email not sending / "SMTP not configured"

**Symptom:** Email deliveries show `failed`, or startup logs `NOTE: alert email
recipients set but notify service not configured…`.

**Cause:** You set `LOOKOUT_ALERT_EMAIL` but **not** the notify-service variables.
Live local SMTP is **deferred/not implemented**, so the fallback `email` channel
returns an explicit "SMTP not configured" error rather than faking a send.

**Fix:** For live email, configure the shared notification service:

```bash
LOOKOUT_NOTIFY_SERVICE_URL='https://api.example.com'
LOOKOUT_NOTIFY_SERVICE_TOKEN='the-service-token'
```

…and restart. You should then see `alert email: live delivery via shared notification
service`. If sends still `fail`, the service URL must implement `POST /notify/send`,
the token must be valid, and the composed URL must pass the SSRF guard (must be a
public address). Until you have the service, **use webhooks** for live alerting. See
[Alerting → email](alerting.md#email-channel-and-the-notify-service).

### Acknowledging doesn't stop reminders

Things to check:

- **Did the ack POST succeed?** It needs the `manage_alerts` permission and a valid
  CSRF token. If you got `forbidden` or a CSRF error, the ack didn't apply — reload
  and retry as a manager.
- **Did the server restart?** Acks are **in memory** (deferred persistence). A restart
  loses ack state, so reminders can resume on a still-open incident. This is expected;
  re-acknowledge.
- **Did the severity change?** A worsening severity **clears** the ack by design (next
  item).
- **Snooze expired?** A snooze (e.g. 1h) only silences until it elapses; then
  reminders resume if the problem is still open. Re-ack/re-snooze if needed.

### Too many alerts / flapping

- **Raise the flap window** on the rule (e.g. from 2 to 3–4 observations) so a value
  bouncing across a threshold must hold the new state longer before it fires/resolves.
- **Lengthen the repeat cadence** (or set it to `0` to never remind) so an open
  incident nags less often.
- **Raise the minimum severity** (e.g. `critical and above`) so warnings don't page.
- **Acknowledge/snooze** the specific incident you're already handling.

### A worsening problem re-alerted after I acked it

**This is intentional.** Acknowledgement silences reminders for the **current**
severity. If the problem **worsens** (warning → critical, or a host goes stale), that's
a new, more serious state, so the ack is cleared and you're re-alerted. You
acknowledged "I'm handling this warning," not "ignore this host." Recovery still sends
the all-clear regardless.
