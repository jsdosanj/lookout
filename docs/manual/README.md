# Lookout — User Manual

**Open-source IT infrastructure monitoring — a Go control plane (server) plus a
small collector agent you put on each machine you want to watch.**

This is the complete, self-service documentation for running Lookout. It is
written to be read by a non-expert and still answer every question, so you should
never need to contact support. If something here is wrong or missing, it is a doc
bug — please open an issue.

> **Scope note.** This manual documents what Lookout **actually does today** on the
> `wave1-buildout` line of development. Features that are scaffolded but not yet
> live (live SMTP email, the server side of the shared notification service, and
> on-disk persistence of acknowledgements) are clearly marked **Deferred** wherever
> they come up, so you always know what is real versus planned. See
> [Roadmap & deferred items](roadmap.md) for the full list.

---

## Start here

| If you want to… | Read |
| --- | --- |
| Understand what Lookout is and stand it up for the first time | [Getting started](getting-started.md) |
| Know what the agent collects and how health is decided | [Monitoring](monitoring.md) |
| Set up alerts (Slack/Teams/webhook/email), rules, ack/snooze | [Alerting](alerting.md) |
| Add users, set roles, turn on MFA | [Users, auth & RBAC](users-auth-rbac.md) |
| Look up an environment variable or command-line flag | [Configuration reference](configuration.md) |
| Understand the security model and what data is stored | [Security & privacy](security-privacy.md) |
| Fix a problem (agent not reporting, no alerts, webhook rejected, …) | [Troubleshooting](troubleshooting.md) |
| Look up a log/error message you saw | [Error & log message reference](error-reference.md) |
| Get a short step-by-step recipe ("how do I add a host?") | [How do I…? recipes](how-do-i.md) |
| Find answers to common questions | [FAQ](faq.md) |

## The whole table of contents

1. [Getting started](getting-started.md) — what Lookout is, build, run the server,
   create the first account, enroll a collector agent, see your first dashboard.
2. [Monitoring](monitoring.md) — what's collected, the health/severity model, the
   fleet (overview) view and per-host detail view.
3. [Alerting](alerting.md) — the rule engine, channels (Slack/Teams/webhook, the
   notify-service email channel), the stale-host sweeper, acknowledge/snooze, the
   SSRF guard, the `manage_alerts` permission, and editing rules from the dashboard.
4. [Users, auth & RBAC](users-auth-rbac.md) — accounts, password + SSO login, TOTP
   MFA, roles & permissions, org units, sessions, and the Notifications page.
5. [Configuration reference](configuration.md) — every environment variable and
   command-line flag, with defaults and examples.
6. [Security & privacy](security-privacy.md) — the threat model, what is and isn't
   encrypted, what data is stored where, and hardening checklist.
7. [Troubleshooting](troubleshooting.md) — symptom → cause → fix for every common
   failure.
8. [Error & log message reference](error-reference.md) — every notable log line and
   HTTP error, what it means, and what to do.
9. [How do I…? recipes](how-do-i.md) — short, copy-paste task recipes.
10. [FAQ](faq.md) — frequently asked questions.
11. [Roadmap & deferred items](roadmap.md) — what's planned and what's intentionally
    not wired yet.

## A 60-second mental model

```
   Each monitored machine                 One control plane (the server)            You
   ┌─────────────────────┐  outbound       ┌──────────────────────────────┐
   │  lookout-agent      │  HTTP(S) only   │  lookout-server              │  ◄─►  Web dashboard
   │  - collects specs,  │ ───────────────►│  - ingests reports           │       (login + RBAC + MFA)
   │    packages,        │  (no inbound    │  - computes plain-English    │
   │    services         │   ports on the  │    health                    │  ◄─►  Alerts: Slack / Teams /
   │  - reports on a     │   monitored     │  - evaluates alert rules     │       webhook / email
   │    timer            │   host)         │  - serves the dashboard      │
   └─────────────────────┘                 └──────────────────────────────┘
```

- The **agent** runs on each server you care about. It only makes **outbound**
  connections (it never opens a listening port), and it reports on a timer.
- The **control plane** (`lookout-server`) receives those reports, turns them into a
  plain-English health status, stores the latest state + a short history, evaluates
  your alert rules, and serves the web dashboard behind login.
- **You** open the dashboard in a browser, log in, and see your whole fleet at a
  glance. When something goes wrong, Lookout messages you on the channels you
  configured.

There are two binaries and one optional demo generator:

| Binary | Package | What it is |
| --- | --- | --- |
| `lookout-server` | `cmd/lookout-server` | The control plane + dashboard. You run **one**. |
| `lookout-agent` | `cmd/lookout-agent` | The collector. You run **one per monitored host**. |
| `lookout-demo` | `cmd/lookout-demo` | Generates a static HTML demo of the dashboard into `docs/` (for GitHub Pages). Optional. |

Continue to **[Getting started](getting-started.md)**.
