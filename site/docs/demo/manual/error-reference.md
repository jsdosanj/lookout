# Error & log message reference

Look up an exact message you saw in a log or in the browser, what it means, and what
to do. Grouped by where it appears.

- [Control-plane startup log lines](#control-plane-startup-log-lines)
- [Control-plane runtime log lines](#control-plane-runtime-log-lines)
- [HTTP responses from the control plane](#http-responses-from-the-control-plane)
- [Agent messages](#agent-messages)
- [Alert delivery (activity log) results](#alert-delivery-activity-log-results)
- [Browser / login redirects](#browser--login-redirects)

---

## Control-plane startup log lines

| Message | Meaning | Action |
| --- | --- | --- |
| `Lookout control plane on :8080 (data: lookout-data.json)` | Server started normally. | None. |
| `created owner account you@example.com` | First-run bootstrap created the owner. | Log in with it. |
| `NOTE: no users yet. Set LOOKOUT_ADMIN_EMAIL and LOOKOUT_ADMIN_PASSWORD to create the first owner.` | Fresh install, no admin vars set. | Set both vars, restart. |
| `could not create owner: …` | Bootstrap failed (e.g. bad role, write error). | Check the error; verify file permissions/email. |
| `WARNING: LOOKOUT_TOKEN not set — agent reports are unauthenticated (dev only)` | No shared token → anyone can POST reports. | Set `LOOKOUT_TOKEN` for anything but local dev. |
| `NOTE: LOOKOUT_REQUIRE_AGENT_TOKEN=true — legacy shared token is rejected on /report (per-agent tokens only)` | Shared token disabled for reporting. | Ensure all agents enrolled first. |
| `NOTE: LOOKOUT_SECURE_COOKIES != 'true' — cookies not marked Secure (run behind TLS in production)` | Session cookie isn't `Secure`. | Set `LOOKOUT_SECURE_COOKIES=true` behind TLS. |
| `alerting enabled: N channel(s), M rule(s)` | Alerting is on. | None. |
| `NOTE: no alert channels configured — set LOOKOUT_ALERT_WEBHOOKS to enable alerting` | Alerting is off (no channels). | Set `LOOKOUT_ALERT_WEBHOOKS` (and/or email vars). |
| `alert webhook rejected (webhook[-N]): <reason>` | A webhook URL failed the SSRF guard (or didn't resolve). That channel is **skipped**. | Use a public URL / fix DNS. See [SSRF](alerting.md#the-ssrf-guard-on-webhooks). |
| `alert email via notify service rejected: <reason>` | The notify-service email channel couldn't be built (bad/SSRF-blocked URL, missing token). | Fix `LOOKOUT_NOTIFY_SERVICE_URL`/`_TOKEN`. |
| `alert email: live delivery via shared notification service` | Live email is wired via the notify service. | None. |
| `NOTE: alert email recipients set but notify service not configured — set LOOKOUT_NOTIFY_SERVICE_URL/_TOKEN for live delivery` | `LOOKOUT_ALERT_EMAIL` set but no notify service → fallback channel returns "not configured" on send. | Configure the notify service for live email. |
| `open store: …` / `open auth store: …` / `open agent store: …` | A data file couldn't be opened/parsed (fatal). | Check the path, permissions, and that the JSON isn't corrupt. |
| `open rule store: …` | The rules file couldn't be opened/parsed (fatal). | Check `lookout-rules.json`. |

---

## Control-plane runtime log lines

| Message | Meaning | Action |
| --- | --- | --- |
| `report rejected: hostname "X" is pinned to a different agent identity (identity=…)` | TOFU conflict: a different agent tried to report an already-pinned hostname. | See [hostname claimed](troubleshooting.md#hostname-is-claimed-by-another-agent-409). |

---

## HTTP responses from the control plane

| Status & body | Endpoint | Meaning | Action |
| --- | --- | --- | --- |
| `200 ok` | `GET /healthz` | Server is alive. | Liveness probe. |
| `401 unauthorized` | `/api/v1/agents/enroll`, `/api/v1/agents/report` | Token not accepted. | Check `LOOKOUT_TOKEN` / per-agent token / `LOOKOUT_REQUIRE_AGENT_TOKEN`. |
| `400 bad request: …` | enroll/report | Malformed JSON body. | Fix the client; ensure it sends valid report JSON. |
| `400 missing host.hostname` | report | Report had no hostname. | The agent should always send one; check the agent build/version. |
| `409 hostname is claimed by another agent` | report | TOFU pin conflict. | See troubleshooting. |
| `413` / body too large | report (>8 MiB) / enroll (>1 MiB) | Body exceeded the cap. | Don't send oversized bodies; this is a DoS guard. |
| `403 forbidden — you don't have permission for this` | any RBAC-gated route | Your role lacks the permission. | Get a higher role; see [RBAC](users-auth-rbac.md). |
| `403 invalid or missing CSRF token` | any state-changing POST | CSRF token missing/stale. | Reload the page, resubmit; re-login if it persists. |
| `503 rule editing not available` | `/notifications/rules/*` | The rule store isn't configured (alerting off). | Enable alerting (configure a channel). |
| `400 rule needs a name and at least one channel` | `/notifications/rules/save` | Form missing name or channels. | Provide both. |
| `404 SSO provider not configured` | `/auth/{provider}/…` | That OAuth provider isn't enabled. | Set its `LOOKOUT_OAUTH_*` vars. |

---

## Agent messages

| Message | Meaning | Action |
| --- | --- | --- |
| `lookout-agent: <error>` | A fatal agent error (printed to stderr, exit 1). | Read the specific error. |
| `report error: <error>` | A transient report failure (non-fatal; the agent keeps looping). | Usually network or a server-side rejection; check reachability/token. |
| `control plane returned 401 Unauthorized` | Token rejected by the server. | Fix the token; ensure enrollment if `REQUIRE_AGENT_TOKEN`. |
| `control plane returned <status>` | Server returned a non-2xx on report. | Match the status to the table above. |
| `lookout-agent: enroll failed, using shared token: <error>` | First-run enrollment failed; the agent falls back to the shared token. | Often harmless; check the error if you require per-agent tokens. |
| `lookout-agent: could not persist agent token: <error>` | The per-agent token couldn't be written. | Ensure the `--token-file` dir is writable (set `HOME`/`StateDirectory`). |
| `enroll: --token is required on first run` | (`collect`) No enrollment token on first run. | Pass `--token`. |
| `collect: --ingest-url (or LOOKOUT_INGEST_URL) is required` | (`collect`) No ingest URL. | Pass `--ingest-url` or set the env var. |
| `enroll: control plane returned <code>: <msg>` | (`collect`) Ingest enrollment rejected. | Check the token/URL against the ingest plane. |

---

## Alert delivery (activity log) results

On `/notifications`, each delivery row shows a result:

| Result | Meaning |
| --- | --- |
| `sent` | The channel accepted the delivery (HTTP `< 300`). |
| `failed` | Delivery errored. Common causes: webhook returned `≥ 300`, URL/SSRF/resolve error, notify-service rejected it, or the email channel is the not-configured fallback ("SMTP not configured"). |

The underlying error text is captured internally; cross-reference the
[troubleshooting](troubleshooting.md#alerting-problems) section for the channel type.

---

## Browser / login redirects

| URL you land on | Meaning | Action |
| --- | --- | --- |
| `/login` | Not logged in (or session expired). | Log in. |
| `/login?err=bad` | Wrong email/password. | Re-enter. |
| `/login?err=locked` | Too many failed attempts; locked 15 min. | Wait 15 minutes. |
| `/login/mfa` | Password OK, TOTP code required. | Enter your authenticator code. |
| `/login/mfa?err=bad` | Wrong TOTP code. | Try again (watch the lockout). |
| `/login?err=noaccount` | SSO email has no Lookout account. | Create the user first (admin). |
| `/login?err=state` | OAuth state mismatch. | Restart SSO; check `LOOKOUT_BASE_URL`/callback. |
| `/login?err=sso` | OAuth exchange/email failed (incl. unverified email). | Check client id/secret; verify the email at the provider. |
