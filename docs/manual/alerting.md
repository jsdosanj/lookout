# Alerting

This is the deepest part of Lookout. It turns a host's health into **actionable
notifications** — Slack, Teams, a generic webhook, or email — with the behaviour
real operators expect: one message per ongoing problem (not one per report),
flap-damping, escalation reminders, recovery notices, a sweeper that catches silent
hosts, and acknowledge/snooze to stop the nagging.

- [How alerting works end to end](#how-alerting-works-end-to-end)
- [Turning alerting on](#turning-alerting-on)
- [Severity (how health maps to an alert level)](#severity-how-health-maps-to-an-alert-level)
- [Rules](#rules)
  - [What a rule is](#what-a-rule-is)
  - [The seeded default rule](#the-seeded-default-rule)
  - [Dedupe](#dedupe-one-problem-one-alert)
  - [Flap-damping (the flap window)](#flap-damping-the-flap-window)
  - [Repeat / escalation reminders](#repeat--escalation-reminders)
  - [Resolve notices](#resolve-notices)
- [Channels](#channels)
  - [Webhook (Slack / Teams / generic)](#webhook-channel-slack--teams--generic)
  - [Email & the notify-service channel](#email-channel-and-the-notify-service)
- [The SSRF guard on webhooks](#the-ssrf-guard-on-webhooks)
- [The stale-host sweeper](#the-stale-host-sweeper)
- [Acknowledge & snooze](#acknowledge--snooze)
- [Configuring rules from the dashboard](#configuring-rules-from-the-dashboard)
- [The `manage_alerts` permission](#the-manage_alerts-permission)
- [What the messages look like](#what-the-messages-look-like)
- [Recent alert activity](#recent-alert-activity)

---

## How alerting works end to end

```
report arrives ─┐
                ├─►  health computed (ok/warning/critical/stale)
sweeper tick ───┘            │
                             ▼
                    Engine.Observe(server, status, reason)
                             │
            ┌────────────────┴──────────────────┐
            ▼                                    ▼
   for each matching rule:               (rules you set; one is
     flap-damp → dedupe →                 seeded on first run)
     fire / repeat / resolve
            │
            ▼
   deliver to the rule's channels  ──►  Slack / Teams / webhook / email
            │
            ▼
   record the delivery in the in-memory activity log (shown on /notifications)
```

Two things feed the engine:

1. **Every agent report** — when a host reports, its health is recomputed and
   observed.
2. **A background sweeper every minute** — re-evaluates the whole fleet, which is the
   only way a **silent** host (which stops sending reports) can ever alert.

The engine is the brain: it decides **what** to send and **when** (rules, dedupe,
flap-damping, escalation), then hands a finished notification to one or more
**channels**, which do the actual network delivery.

---

## Turning alerting on

Alerting is **off** until you configure at least one channel. The simplest way is a
webhook:

```bash
LOOKOUT_TOKEN=... LOOKOUT_ADMIN_EMAIL=... LOOKOUT_ADMIN_PASSWORD=... \
LOOKOUT_ALERT_WEBHOOKS='https://hooks.slack.com/services/T000/B000/XXXX' \
  ./lookout-server
```

On startup you'll see:

```
alerting enabled: 1 channel(s), 1 rule(s)
```

If you don't set any channel, you'll instead see:

```
NOTE: no alert channels configured — set LOOKOUT_ALERT_WEBHOOKS to enable alerting
```

…and the Notifications page shows alerting as **Off**. Channels are configured by
**environment variables on the control plane** (not in the UI); rules are then
editable in the UI. The full variable list is in the
[Configuration reference](configuration.md#alerting).

---

## Severity (how health maps to an alert level)

Alerting has its own ordered **severity** derived from the health status:

| Health status | Alert severity | Notes |
| --- | --- | --- |
| ok | *none* | Nothing to alert on. |
| warning | **warning** | |
| critical | **critical** | |
| stale | **stale** | Treated as the **most severe** level — a silent host is the worst case. |

Severity is ordered `none < warning < critical < stale`. A rule's **minimum
severity** is a floor: anything below it is treated as healthy by that rule.

---

## Rules

### What a rule is

A **rule** decides *when a server's state is worth alerting on* and *where to send
it*. A rule has:

| Field | Meaning |
| --- | --- |
| **Name** | Human label shown in the UI and (indirectly) in messages. |
| **Server** | Which hosts it applies to. Blank or `*` = **all servers**; otherwise an **exact** server ID (the hostname). |
| **Minimum severity** | The floor: fire only at or above `warning`, `critical`, or `stale`. |
| **Channels** | One or more channel IDs to deliver to (e.g. `webhook`, `email`). |
| **Flap window** | How many consecutive same-severity observations are required before the engine acts. `1` = act immediately; higher = damp flapping. |
| **Repeat every** | How often to re-notify an unresolved, unacknowledged incident (the escalation reminder). `0` = never repeat. |

Multiple rules can match the same host (e.g. a fleet-wide warning rule plus a
critical-only rule for your databases). Each matching rule evaluates independently
and tracks its own incident state per server.

Rules are **persisted** to `lookout-rules.json` and **editable from the dashboard** —
see [Configuring rules from the dashboard](#configuring-rules-from-the-dashboard).

### The seeded default rule

On a **fresh install with at least one channel configured**, Lookout seeds one rule
so you have working alerting out of the box:

| Field | Seeded value |
| --- | --- |
| Name | `Fleet: warning and above` |
| Server | `*` (all servers) |
| Minimum severity | `warning` |
| Channels | **all** configured channels |
| Flap window | `2` observations |
| Repeat every | `30 minutes` |

So by default: any host that reaches warning or worse, confirmed over 2
observations, alerts on all your channels and reminds you every 30 minutes until it
recovers. You can edit or delete this rule like any other.

### Dedupe (one problem, one alert)

The engine tracks an **incident** per (rule, server). While a problem is ongoing at
the same severity, it fires **once** — not on every report. You won't get a message a
minute for the same full disk. (Reminders are separate and cadence-controlled; see
[Repeat](#repeat--escalation-reminders).)

If the severity **changes** while the host is still unhealthy (e.g. warning →
critical, or critical → warning), that's a new state, so the engine fires again to
reflect the change. A worsening change also **clears any acknowledgement** so a
problem getting worse always re-alerts even if you'd silenced the earlier level.

### Flap-damping (the flap window)

A value bouncing across a threshold (89% → 91% → 89% …) would otherwise spam you. The
**flap window** requires *N* consecutive observations of the **same** severity before
the engine treats it as the confirmed state and acts on it.

- `FlapWindow = 1` → act immediately, no damping.
- `FlapWindow = 2` (the default) → a new severity must be seen **twice in a row**
  before it fires or resolves.
- While the window hasn't been satisfied, the engine **holds steady** — it neither
  fires a new alert nor resolves an open one. This applies to *recovery* too: a host
  must look healthy for the full window before its incident resolves, so a brief dip
  back to ok doesn't prematurely send the all-clear.

### Repeat / escalation reminders

An unacknowledged incident that stays open re-notifies on the rule's **Repeat every**
cadence. Reminders are clearly marked (the message says `(reminder)` / "is still …").

- `RepeatEvery = 0` → never repeat (fire once, resolve once).
- `RepeatEvery = 30m` (the default) → remind every 30 minutes until the host recovers
  or you acknowledge.

Reminders stop when:

1. the host **recovers** (you get a resolve notice instead), **or**
2. someone **acknowledges or snoozes** the incident (see
   [Acknowledge & snooze](#acknowledge--snooze)).

### Resolve notices

When a host recovers (drops below the rule's minimum severity, confirmed over the
flap window), the engine sends **one** resolve notification (`✅ … recovered`) and
closes the incident. Acknowledgement does **not** suppress resolve notices — ack
stops the nagging, not the all-clear.

---

## Channels

A **channel** is one delivery destination. Channels are configured by environment
variables on the control plane; rules then reference them by **ID**.

| Channel type | Default ID(s) | Status | Configured by |
| --- | --- | --- | --- |
| Webhook (Slack/Teams/generic) | `webhook`, `webhook-2`, … | **Live** | `LOOKOUT_ALERT_WEBHOOKS` |
| Email via notify service | `email` | **Live** (when the notify service is configured) | `LOOKOUT_ALERT_EMAIL` + `LOOKOUT_NOTIFY_SERVICE_URL` + `LOOKOUT_NOTIFY_SERVICE_TOKEN` |
| Email, local SMTP | `email` | **Deferred** (fallback; returns "not configured") | `LOOKOUT_ALERT_EMAIL` only |

### Webhook channel (Slack / Teams / generic)

Set `LOOKOUT_ALERT_WEBHOOKS` to one or more incoming-webhook URLs, **comma-separated**.
The first becomes channel `webhook`, the second `webhook-2`, the third `webhook-3`,
and so on.

```bash
# one webhook
LOOKOUT_ALERT_WEBHOOKS='https://hooks.slack.com/services/T000/B000/XXXX'

# multiple (Slack + Teams + your own)
LOOKOUT_ALERT_WEBHOOKS='https://hooks.slack.com/...,https://outlook.office.com/webhook/...,https://ops.example.com/hook'
```

Each webhook delivers a JSON body of the form `{"text": "<the message>"}`, which
**Slack and Microsoft Teams both accept** as incoming-webhook text. Generic
receivers (PagerDuty, Opsgenie, your own endpoint) just receive that JSON POST.

- Each URL is **validated by the SSRF guard up front** (at startup). An unsafe or
  unresolvable URL is **logged and skipped** — the others still load. You'll see
  `alert webhook rejected (webhook-2): …` in the log.
- A webhook send has a **10-second timeout**, and a response status `≥ 300` is
  treated as a failed delivery (recorded in the activity log).
- The URL is **re-checked on every send** (defends against DNS-rebinding — a name
  that pointed somewhere safe at startup but later resolves to an internal IP).

#### Where to get a webhook URL

- **Slack:** create an *Incoming Webhook* app for the target channel; copy the
  `https://hooks.slack.com/services/…` URL.
- **Teams:** add an *Incoming Webhook* connector to a channel; copy the
  `https://…webhook…` URL.
- **Anything else:** any HTTPS endpoint that accepts a JSON POST works.

### Email channel and the notify service

Set `LOOKOUT_ALERT_EMAIL` to one or more recipient addresses (comma-separated) to
register the `email` channel. **How it actually delivers depends on whether the
shared notification service is configured:**

**Live delivery — via the shared notification service (the supported path today).**
Set both:

```bash
LOOKOUT_ALERT_EMAIL='oncall@example.com,owner@example.com'
LOOKOUT_NOTIFY_SERVICE_URL='https://api.dosanjhlabs.com'
LOOKOUT_NOTIFY_SERVICE_TOKEN='the-service-bearer-token'
```

Lookout then delivers email by POSTing a **secret-free payload** to
`<URL>/notify/send` (the platform's `POST /notify/send` contract) with a bearer
token. The notification *service* owns the real provider key, dedupe, retry, and
audit log; Lookout only hands it template variables (subject, body, server,
severity, reason). On startup you'll see:

```
alert email: live delivery via shared notification service
```

- The composed `<URL>/notify/send` endpoint is **SSRF-checked** up front and on every
  send, exactly like a webhook.
- If the token is missing, the channel **refuses to register** rather than faking a
  send.
- Lookout includes a stable `dedupeKey` so the service collapses repeats of one
  incident-state within its dedupe window.

> **Deferred: the notify-service *server* itself.** Lookout is the *client* of `POST
> /notify/send`. The service that receives that call (and holds the email provider
> key) is a separate platform component, not part of this repo. If you point
> `LOOKOUT_NOTIFY_SERVICE_URL` at a service that doesn't implement the contract, sends
> fail and are recorded as failed in the activity log.

**Fallback — local SMTP channel (Deferred / not yet live).** If you set
`LOOKOUT_ALERT_EMAIL` **without** the notify-service variables, Lookout still
registers an `email` channel so rules and the UI can reference it — but its `Send`
returns an explicit **"SMTP not configured"** error rather than silently dropping or
faking a send. You'll see:

```
NOTE: alert email recipients set but notify service not configured — set
LOOKOUT_NOTIFY_SERVICE_URL/_TOKEN for live delivery
```

The email **payload** (subject + body rendering) is real and unit-tested; only the
live SMTP transport is intentionally unimplemented in this wave. `LOOKOUT_SMTP_*`
variables are reserved for that future work but are **not read today**. See
[Roadmap & deferred items](roadmap.md).

> **Net:** for live email today, use the notify service. Without it, the `email`
> channel exists but every delivery is recorded as **failed** ("SMTP not
> configured"). Webhooks are the fully self-contained, live option.

---

## The SSRF guard on webhooks

Webhook URLs (and the notify-service URL) are **operator-supplied but fetched by the
control plane**. Without protection, someone who can set a webhook could point it at
internal services — cloud metadata (`169.254.169.254`), localhost admin ports,
private IPs — and make the server fetch them on their behalf. That's **Server-Side
Request Forgery (SSRF)**.

Lookout's guard (`SafeWebhookURL`) enforces:

- **Scheme** must be `http` or `https` (no `file://`, `gopher://`, etc.).
- A **host** must be present.
- The host is **resolved** (a literal IP is checked directly; a name is looked up and
  **every** returned address is checked). The URL is rejected if any address is:
  - **loopback** (`127.0.0.0/8`, `::1`),
  - **private** (`10/8`, `172.16/12`, `192.168/16`, ULA),
  - **link-local** (`169.254/16` — this also covers the cloud metadata endpoint
    `169.254.169.254`),
  - **unspecified** (`0.0.0.0`, `::`),
  - **multicast**, or
  - **carrier-grade NAT** (`100.64.0.0/10`, commonly used internally).

The check runs **at configuration time** (so a bad URL is rejected before it's ever
used and is logged + skipped) **and again on every send** (so DNS-rebinding — a name
that maps to a safe IP at startup but flips to an internal IP later — is also
blocked).

**What you'll see if a URL is rejected:**

```
alert webhook rejected (webhook): host "10.0.0.5" resolves to a non-public address (10.0.0.5)
alert webhook rejected (webhook-2): scheme "file" not allowed (use http or https)
alert webhook rejected (webhook-3): cannot resolve host "typo.invalid": ...
```

If you genuinely need to deliver to an internal endpoint, **put a public-facing
relay/proxy in front of it** and point Lookout at the public address. The guard is
deliberately strict — see [Troubleshooting](troubleshooting.md#webhook-rejected-by-the-ssrf-guard).

---

## The stale-host sweeper

Here's the subtle one. Alerts normally fire when a **report arrives**. But a host
that **goes silent** (crashes, loses network, agent killed) sends *no* reports — so on
the report path alone, **a dead host would never alert**, which is exactly the case
you most want to know about.

The **sweeper** fixes that. The control plane runs a background loop **every minute**
that re-evaluates **every known server's** health and feeds it to the engine. A host
that hasn't reported in over 5 minutes evaluates to **stale**, and the sweep observes
that — so the engine fires a stale alert (subject to your rule's minimum severity and
flap window) even though the host has stopped talking.

Because the engine **dedupes**, a sweep that finds nothing newly wrong is a no-op —
it won't re-alert healthy hosts. The sweeper only ever surfaces *new* problems or
drives reminders/recovery for existing ones.

**For a stale host to actually page you**, a matching rule's **minimum severity** must
be at or below `stale`. The seeded default (`warning` and above) covers stale, since
stale outranks warning. If you set a rule to `critical` only, note that `stale`
outranks `critical` too, so it still fires; a rule set to fire on `stale` only will
*only* fire on silence, not on warning/critical.

---

## Acknowledge & snooze

When an incident is open and reminding you, you can **acknowledge** it (or **snooze**
it) from the Notifications page to **stop the reminder cascade** without waiting for
recovery. This is for "yes, I'm on it — stop paging me."

- **Acknowledge** — silences reminders for the **life of this incident**. The incident
  stays open (still listed, still resolves normally).
- **Snooze (e.g. 1 hour)** — silences reminders **until that time passes**, then
  reminders resume if the problem is still open.

Important behaviours:

- Acknowledgement is **per incident** (per rule+server). A *different* host, or the
  same host under a *different* rule, is unaffected.
- A **worsening severity re-alerts even if acknowledged.** If you ack a warning and it
  becomes critical, the ack is cleared and you're alerted to the escalation. (You're
  saying "I've got this warning," not "ignore this host forever.")
- **Recovery still notifies.** Ack stops reminders, not the all-clear — you always get
  the `✅ recovered` message.

> **Deferred: ack persistence.** Acknowledgements and snoozes are tracked **in
> memory**. If the control plane **restarts**, open incidents are rebuilt from the
> next observation but their ack/snooze state is **lost**, so reminders can resume.
> On-disk persistence of acks is on the roadmap. Plan around it: a server restart can
> re-page you for a still-open, previously-acked incident.

How to do it: see [Configuring rules from the dashboard](#configuring-rules-from-the-dashboard)
and the recipe [Snooze an incident](how-do-i.md#snooze-or-acknowledge-an-incident).

---

## Configuring rules from the dashboard

Go to **Notifications** (`/notifications`). If you have the
[`manage_alerts`](#the-manage_alerts-permission) permission you'll see, in order:

1. **Alerting status** — `Active` (rules are live and delivering) or `Off` (no
   channels configured).
2. **Open incidents** — a table of currently-active incidents with **Acknowledge** and
   **Snooze 1h** buttons.
3. **Active rules** — every rule: name, which servers, fires-at severity, flap window,
   repeat cadence, channels, and a **Delete** button.
4. **Add or update a rule** — a form to create or edit a rule:
   - **Name** (required).
   - **Server** — blank or `*` for all, or an exact server id (hostname).
   - **Fire at** — `warning and above` / `critical and above` / `stale only`.
   - **Flap window** — observations before acting (default 2).
   - **Reminder cadence (minutes)** — 0 = never (default 30).
   - **Channels** — tick one or more (required: at least one). The list is exactly the
     channels your environment configured.
5. **Recent alert activity** — the last ~20 delivery attempts.

Saving a rule **persists** it to `lookout-rules.json` and immediately pushes the new
rule set into the live engine — no restart needed. Editing rules preserves in-flight
incident and acknowledgement state, so an ongoing problem keeps its dedupe and ack
across the edit.

> To **edit** an existing rule, give the form the same details (there isn't a separate
> "edit" button beyond Delete). Creating a rule with a fresh name adds a new rule;
> the form always upserts. Channels are **not** edited here — they come from the
> control-plane environment.

All rule/ack actions are **POSTs protected by both the `manage_alerts` permission and
CSRF**.

---

## The `manage_alerts` permission

Editing alerting is privileged. The relevant routes are gated by the **`manage_alerts`**
permission and CSRF protection:

| Route | What it does | Required permission |
| --- | --- | --- |
| `GET /notifications` | View the page; rules/incidents/activity only render for managers | `view_dashboard` to see the page; `manage_alerts` to see/manage rules |
| `POST /notifications/rules/save` | Create/update a rule | `manage_alerts` (+ CSRF) |
| `POST /notifications/rules/delete` | Delete a rule | `manage_alerts` (+ CSRF) |
| `POST /notifications/ack` | Acknowledge/snooze an incident | `manage_alerts` (+ CSRF) |

Which roles have it:

| Role | `manage_alerts`? |
| --- | --- |
| owner | ✅ |
| admin | ✅ |
| operator | ✅ |
| viewer | ❌ (can view the dashboard, but rules/incidents/activity are hidden) |

A viewer who opens `/notifications` sees the channel list and the intro copy, but not
the live rules, open incidents, or activity table. See
[Users, auth & RBAC](users-auth-rbac.md).

---

## What the messages look like

The plain-English line Lookout sends (to chat channels, and as the email body):

| Situation | Message |
| --- | --- |
| New problem | `🟠 Lookout: web-01 is warning — disk /data is 84% full` |
| Critical | `🔴 Lookout: db-02 is critical — disk / is 94% full` |
| Stale | `⚪ Lookout: app-03 is stale` |
| Reminder | `🟠 Lookout (reminder): web-01 is still warning — disk /data is 84% full` |
| Recovered | `✅ Lookout: web-01 recovered (was warning)` |

For email, the subject is `[Lookout] <server> — <severity>` (or `[Lookout] <server>
recovered`), and the body adds the cause, server, severity, and timestamp.

---

## Recent alert activity

The **Recent alert activity** table on `/notifications` shows the last ~20 delivery
attempts (newest first): time, server, state (severity / `resolved` / `(reminder)`),
channel, and result (`sent` or `failed`). It's the fastest way to confirm "did my
alert actually go out?" and to spot a misconfigured channel (a row of `failed`).

> **In memory.** The activity log keeps the last 50 deliveries in memory and shows up
> to 20. It is **not persisted** — a control-plane restart clears it.

For things that aren't working, jump to
[Troubleshooting → alerting](troubleshooting.md#alerting-problems).
