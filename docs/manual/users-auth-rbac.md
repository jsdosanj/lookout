# Users, auth & RBAC

The dashboard sits behind login. This page covers accounts, password + SSO login,
TOTP multi-factor authentication, roles & permissions, org units, sessions, and how
agents authenticate to the control plane.

- [The first account (bootstrap)](#the-first-account-bootstrap)
- [Logging in](#logging-in)
- [Roles & permissions (RBAC)](#roles--permissions-rbac)
- [Managing users](#managing-users)
- [Org units: groups, departments, locations](#org-units-groups-departments-locations)
- [TOTP MFA](#totp-mfa)
- [SSO / OAuth (Google & GitHub)](#sso--oauth-google--github)
- [Sessions, cookies & lockout](#sessions-cookies--lockout)
- [The Notifications page & who can see what](#the-notifications-page--who-can-see-what)
- [Agent identity: TOFU and per-agent tokens](#agent-identity-tofu-and-per-agent-tokens)

---

## The first account (bootstrap)

On a **fresh install (zero users)**, the control plane creates the first **owner**
account from two environment variables:

```bash
LOOKOUT_ADMIN_EMAIL='you@example.com'
LOOKOUT_ADMIN_PASSWORD='a-strong-password'
```

- This happens **only when there are no users yet**. Once any user exists, these
  variables are ignored.
- If you start the server without them on a fresh install you'll see `NOTE: no users
  yet. Set LOOKOUT_ADMIN_EMAIL and LOOKOUT_ADMIN_PASSWORD…` and you won't be able to
  log in until you set them and restart.

Passwords are stored as **bcrypt** hashes (never plaintext). After bootstrap, manage
everyone else from the dashboard.

---

## Logging in

- **Login page:** `/login`. Enter email + password.
- If the user has **MFA enabled**, login then redirects to `/login/mfa` to enter a
  6-digit TOTP code before access is granted.
- **SSO buttons** (Google / GitHub) appear on the login page only if those providers
  are configured (see [SSO / OAuth](#sso--oauth-google--github)).
- **Logout:** the dashboard's sign-out control POSTs to `/logout`.

If credentials are wrong you're redirected back to `/login?err=bad`. After too many
failures you're temporarily locked (`/login?err=locked`) — see
[lockout](#sessions-cookies--lockout).

---

## Roles & permissions (RBAC)

Every user has exactly one **role**, which bundles **permissions**:

| Permission | What it gates |
| --- | --- |
| `view_dashboard` | See the dashboard, server detail, guides, integrations, settings, and the Notifications page. |
| `manage_users` | Create/edit users, change roles, disable users, manage org units (`/admin/*`). |
| `manage_alerts` | Create/edit/delete alert rules and acknowledge/snooze incidents. |
| `manage_agents` | Reserved for agent administration (defined; no dedicated UI route yet). |

The role → permission table:

| Role | view_dashboard | manage_users | manage_alerts | manage_agents |
| --- | :---: | :---: | :---: | :---: |
| **owner** | ✅ | ✅ | ✅ | ✅ |
| **admin** | ✅ | ✅ | ✅ | ✅ |
| **operator** | ✅ | ❌ | ✅ | ✅ |
| **viewer** | ✅ | ❌ | ❌ | ❌ |

Quick guidance:

- **owner / admin** — full administrators (owner and admin currently grant the same
  permissions; `owner` is the bootstrap role).
- **operator** — can run alerting (rules, ack/snooze) and is intended for agent
  management, but **cannot manage users**.
- **viewer** — read-only: sees the fleet but cannot change rules or users, and the
  rule/incident/activity sections of the Notifications page are hidden.

Permission checks happen in middleware on **every** protected route; a user without
the permission gets `403 forbidden — you don't have permission for this`.

---

## Managing users

Admins/owners go to **`/admin/users`** to:

- **Create a user** — email, name, role, and (optionally) a password. A user with no
  password is **SSO-only** (they must sign in via Google/GitHub).
- **Change a role** — changing a role **revokes all of that user's sessions**, so new
  privileges take effect on their next login (no stale elevated sessions).
- **Disable / re-enable a user** — disabling **immediately revokes all their
  sessions**; a disabled user cannot log in (password or SSO).
- **Assign org units** — department, location, and group membership (see below).

Duplicate emails are rejected (`a user with that email already exists`).

---

## Org units: groups, departments, locations

For organizing people (not yet for scoping access), admins can manage three kinds of
org unit:

- **Groups** — `/admin/org/group`
- **Departments** — `/admin/org/department`
- **Locations & buildings** — `/admin/org/location` (the detail field is labelled
  *Address*)

You can create and delete units and assign users to them on the users page. These are
organizational metadata today; per-group/per-host **access scoping** is on the
roadmap.

---

## TOTP MFA

Any user can enable **time-based one-time-password** two-factor auth from
**`/account`**:

1. Click **Set up two-factor**. Lookout generates a secret and shows it plus an
   `otpauth://` URI (labelled with the issuer **"Lookout"**).
2. Scan it into an authenticator app (Google Authenticator, Authy, 1Password, etc.),
   or enter the secret manually.
3. Enter a current 6-digit code to **confirm and enable**. (If the code doesn't match
   you're asked to try again — MFA isn't enabled until you prove a valid code.)
4. To turn it off, use **Disable** on `/account` — this clears the stored secret.

Once enabled, login is two-step: password, then code at `/login/mfa`. The
password→MFA transition **rotates the session token** (prevents session fixation
across the privilege step), and the pre-MFA session is short-lived (10 minutes).

MFA-verify attempts are rate-limited the same way as logins; too many bad codes drop
the half-authenticated session and force a fresh login.

---

## SSO / OAuth (Google & GitHub)

Lookout supports **Google** and **GitHub** as SSO providers. Each is enabled only
when its client ID + secret are present in the environment, plus a base URL so
redirect URIs are correct:

```bash
LOOKOUT_BASE_URL='https://monitor.example.com'

LOOKOUT_OAUTH_GOOGLE_CLIENT_ID='...'
LOOKOUT_OAUTH_GOOGLE_CLIENT_SECRET='...'

LOOKOUT_OAUTH_GITHUB_CLIENT_ID='...'
LOOKOUT_OAUTH_GITHUB_CLIENT_SECRET='...'
```

Set the provider's **redirect/callback URL** to
`<LOOKOUT_BASE_URL>/auth/<provider>/callback`, e.g.
`https://monitor.example.com/auth/google/callback`.

Important behaviours:

- **No silent self-provisioning.** SSO only logs in an account an admin **already
  created** with that email. An unknown SSO email is bounced to
  `/login?err=noaccount`. (Create the user first, with no password, then they sign in
  via SSO.)
- **Verified email only.** Google's `email_verified` claim is enforced; GitHub's
  primary **verified** email is fetched. An unverified address is rejected — this
  stops an attacker binding to someone else's email.
- **CSRF/state-protected.** The OAuth flow uses a signed `state` cookie; a mismatch
  bounces to `/login?err=state`.
- If MFA is enabled on the account, SSO login still requires the TOTP step.

---

## Sessions, cookies & lockout

- **Session cookie:** `lookout_session`, `HttpOnly`, `SameSite=Lax`, `Path=/`.
- **Secure flag:** off by default; set **`LOOKOUT_SECURE_COOKIES=true`** when you run
  behind TLS so the cookie is only sent over HTTPS. The server logs a NOTE on startup
  if this isn't set.
- **Session lifetime:** **12 hours** for a fully authenticated session; **10 minutes**
  for the pre-MFA (password-entered, code-pending) session.
- **Background cleanup:** a goroutine sweeps expired sessions every 10 minutes.
- **HSTS:** `Strict-Transport-Security` is sent automatically when the request
  arrives over TLS.
- **Brute-force lockout:** after **5 failed** password (or MFA) attempts within a
  **15-minute** window, that account+IP is locked out for **15 minutes**
  (`/login?err=locked`). A success clears the counter. This is a per-instance
  in-memory limiter, not distributed.

All state-changing POSTs are protected by a **CSRF synchronizer token** (per session;
a pre-auth token for the login form). A missing/incorrect token yields `invalid or
missing CSRF token` (403).

---

## The Notifications page & who can see what

`/notifications` is visible to anyone with `view_dashboard`, but its contents depend
on `manage_alerts`:

| Section | viewer | operator / admin / owner |
| --- | :---: | :---: |
| Intro + channel cards | ✅ | ✅ |
| Alerting status (Active/Off) | ❌ | ✅ |
| Open incidents + ack/snooze buttons | ❌ | ✅ |
| Active rules + add/delete | ❌ | ✅ |
| Recent alert activity | ❌ | ✅ |

See [Alerting → configuring rules from the dashboard](alerting.md#configuring-rules-from-the-dashboard).

---

## Agent identity: TOFU and per-agent tokens

How agents authenticate to the control plane is part of the security model and worth
understanding:

- **Shared enrollment token (`LOOKOUT_TOKEN`).** Every agent presents this once to
  enroll. It's checked in **constant time** on the server. If unset (dev only),
  reports are unauthenticated and a startup WARNING is logged.
- **Per-agent tokens.** On first `run`, an agent calls `POST /api/v1/agents/enroll`
  with the shared token and gets a **unique per-agent token** bound to a
  server-assigned identity. The agent persists it (`~/.config/lookout-agent/agent-token`,
  `0600`) and uses it thereafter. Per-agent tokens are stored server-side in
  `lookout-agents.json` (`0600`) and matched in constant time.
- **Trust-on-first-use (TOFU) hostname pinning.** The first identity to report a given
  hostname **pins** it. A later report claiming the same hostname from a **different**
  identity is **rejected** (`409 hostname is claimed by another agent`). This stops
  one token holder from overwriting another host's record — even under the shared
  token, where each enrolled agent is a distinct identity and the bare shared token
  reports as a single `shared` identity.
- **Lock down the shared token.** Set **`LOOKOUT_REQUIRE_AGENT_TOKEN=true`** to reject
  the legacy shared token on `/report` entirely — only enrolled per-agent tokens are
  accepted. (Agents must have enrolled first.) The server logs a NOTE when this is on.

> **MVP transport caveat.** Agent reports use a bearer token over whatever transport
> you put them on; the repo serves plain HTTP by default. **Run the control plane
> behind a TLS-terminating proxy** in production. Per-agent mTLS is on the roadmap.
> See [Security & privacy](security-privacy.md).
