# Security Policy

Lookout is a monitoring tool that runs inside your infrastructure, so we take
security seriously. Thank you for helping keep it and its users safe.

## Reporting a vulnerability

**Please do not report security vulnerabilities through public GitHub issues,
discussions, or pull requests.**

Instead, report privately using either:

- **GitHub Private Vulnerability Reporting** — go to the repository's **Security**
  tab and click **"Report a vulnerability"** (preferred; keeps the report and fix
  coordination in one place).
- **Email** — send details to **security@dosanjhlabs.com**. If you want to encrypt
  the report, ask for a key in your first message.

Please include:

- A description of the issue and its impact.
- Steps to reproduce or a proof of concept.
- Affected version/commit, OS/arch, and any relevant configuration.

## What to expect

- We aim to acknowledge a report within **3 business days**.
- We will work with you on a fix and a coordinated disclosure timeline, and we will
  credit you in the release notes unless you prefer to remain anonymous.
- Please give us a reasonable window to remediate before any public disclosure.

## Scope

In scope: the `lookout-server` (control plane + dashboard) and `lookout-agent`
binaries and their code in this repository — for example authentication/session
handling, the agent enrollment/report transport, the SSRF guard on outbound
webhook/notify URLs, and input handling on the ingest path.

Out of scope: issues that require a pre-compromised host, social engineering, or the
documented MVP limitations that are already disclosed in the README and
[`docs/manual/security-privacy.md`](docs/manual/security-privacy.md) (for example,
bearer-token-over-HTTP transport when run without the recommended TLS proxy). Those
are tracked roadmap items, not vulnerabilities.

## Supported versions

Lookout is pre-1.0 (v0.x). Security fixes are applied to the latest release and the
default branch.
