# Monitoring

This page explains exactly **what the agent collects**, **how Lookout decides a
host's health**, and **what the dashboard views show you**.

- [What the agent collects](#what-the-agent-collects)
- [The report on the wire](#the-report-on-the-wire)
- [The health & severity model](#the-health--severity-model)
- [The fleet (Overview) view](#the-fleet-overview-view)
- [The per-host detail view](#the-per-host-detail-view)
- [History & time-series](#history--time-series)
- [How often data updates](#how-often-data-updates)
- [The Universal Collector (`collect` subcommand)](#the-universal-collector-collect-subcommand)

---

## What the agent collects

Each time the agent reports, it gathers a full **host report**. There is no
configuration of *what* to collect — the agent reports a fixed, sensible set:

**Host facts**

| Field | Meaning |
| --- | --- |
| `hostname` | The machine's hostname (this is also its **server ID** in Lookout). |
| `os` | `linux`, `darwin` (macOS), or `windows`. |
| `platform` | Distribution/OS family: `ubuntu`, `debian`, `rocky`, `rhel`, `macos`, `windows`, … |
| `version` | OS version string. |
| `arch` | CPU architecture (e.g. `amd64`, `arm64`). |
| `kernel` | Kernel version (where available). |
| `uptime_seconds` | How long the host has been up. |
| `virtualization` | `physical`, `kvm`, `vmware`, `hyperv`, … (best effort). |
| `encryption` | Disk encryption status: `on`, `off`, or empty/unknown — FileVault (macOS), BitLocker (Windows), or LUKS (Linux). |

**Specs (point-in-time resource usage)**

| Field | Meaning |
| --- | --- |
| `cpu_model`, `cpu_cores` | CPU model name and core count. |
| `cpu_percent` | Current CPU utilization (%). |
| `mem_total_mb`, `mem_used_mb` | Total and used memory in MB. |
| `load_avg` | Load average (on platforms that report it). |
| `disks[]` | Each mounted filesystem: `mount`, `fs`, `total_mb`, `used_mb`. |
| `network[]` | Up, non-loopback interfaces with an IPv4 address: `name`, `ipv4`, `mac`. |
| `processes[]` | The busiest few processes: `pid`, `name`, `cpu_pct`, `mem_pct`. |

**Installed packages** (`packages[]`) — name + version, sourced from the native
package manager:

- **Linux:** `dpkg` (Debian/Ubuntu) or `rpm` (RHEL/Rocky/CentOS/AlmaLinux).
- **macOS:** Homebrew and `pkgutil`.
- **Windows:** the registry's installed-software list.

**Services** (`services[]`) — name + status (`running` / `stopped`):

- **Linux:** `systemd`.
- **macOS:** `launchd`.
- **Windows:** Windows services.

> **Best-effort, never fatal.** Host facts and specs are required for a valid
> report. Packages and services are **best-effort**: a host without `systemd`, or
> where listing packages is denied, still reports successfully — those sections are
> just empty. The agent never fails a whole report because one optional collector
> couldn't run.

### How collection is done (and why it's safe)

- **Pure Go standard library** for the core report — no external runtime
  dependencies, a small attack surface.
- **No shell.** Every OS command runs via `exec.Command` with fixed arguments —
  there is no `sh -c`, so there is no shell-injection surface.
- **Read-only.** Collectors read files (`/proc`, etc.) or run fixed read-only
  commands. The agent does not change the system it monitors.
- **Outbound-only.** The agent opens no listening port; it only dials out to the
  control plane.

---

## The report on the wire

`lookout-agent report` prints the exact JSON the agent would send. Abbreviated
example:

```json
{
  "schema_version": "1",
  "collected_at": "2026-06-14T18:03:11Z",
  "host": {
    "hostname": "web-01",
    "os": "linux",
    "platform": "ubuntu",
    "version": "24.04",
    "arch": "amd64",
    "kernel": "6.8.0-31-generic",
    "uptime_seconds": 824113,
    "virtualization": "kvm",
    "encryption": "on"
  },
  "specs": {
    "cpu_model": "Intel(R) Xeon(R)",
    "cpu_cores": 4,
    "cpu_percent": 12.4,
    "mem_total_mb": 7976,
    "mem_used_mb": 5210,
    "load_avg": [0.42, 0.51, 0.48],
    "disks": [{ "mount": "/", "fs": "ext4", "total_mb": 80000, "used_mb": 41200 }],
    "network": [{ "name": "eth0", "ipv4": "10.0.0.5", "mac": "..." }],
    "processes": [{ "pid": 1234, "name": "nginx", "cpu_pct": 3.1, "mem_pct": 1.2 }]
  },
  "packages": [{ "name": "nginx", "version": "1.24.0" }],
  "services":  [{ "name": "nginx", "status": "running" }]
}
```

`schema_version` lets the control plane evolve the format safely; it is `"1"` today.

The control plane caps an incoming report body at **8 MiB** (a DoS guard) and
requires `host.hostname` to be present, otherwise it rejects the report. See
[Error & log message reference](error-reference.md).

---

## The health & severity model

The control plane converts each report into a **health status** anyone can read.
This logic lives server-side (you don't configure thresholds today; the defaults are
chosen to be sensible). The four statuses, worst wins:

| Status | When | Severity rank |
| --- | --- | --- |
| **ok** | Nothing is wrong. | 0 |
| **warning** | A disk is **≥ 80%** full, **or** memory is **≥ 90%** used. | 1 |
| **critical** | A disk is **≥ 90%** full. | 2 |
| **stale** | No report received for more than **5 minutes** (`StaleAfter`). | 3 (treated as most severe) |

Rules of the model:

- **The worst signal wins.** If one disk is at 82% (warning) and another is at 95%
  (critical), the host is **critical**.
- **Every status carries a plain-English reason**, e.g. `disk /data is 94% full` or
  `memory 91% used` or `no report in 7m`. The dashboard and alerts show this reason.
- **Stale overrides everything.** A host that stops reporting becomes **stale** after
  5 minutes regardless of its last known numbers — because a silent host might be
  down. This is the signal the [stale-host sweeper](alerting.md#the-stale-host-sweeper)
  turns into an alert.

The exact thresholds, for reference:

```
disk used ≥ 90%   → critical   ("disk <mount> is NN% full")
disk used ≥ 80%   → warning    ("disk <mount> is NN% full")
memory used ≥ 90% → warning    ("memory NN% used")
no report > 5 min → stale      ("no report in <duration>")
```

> **Not yet configurable.** Custom thresholds, per-host overrides, and arbitrary
> custom checks (Nagios-style plugins) are on the roadmap but not implemented today.
> What you *can* tune today is **which severities alert and where** — that's done via
> [alert rules](alerting.md), not by changing these health thresholds.

---

## The fleet (Overview) view

The **Overview** page (`/`) is your at-a-glance fleet health. It shows:

- **One card per server**, color-coded by status (ok / warning / critical / stale),
  with its hostname, key resource usage, and the plain-English reason if it isn't ok.
- **OS distribution** — the mix of operating systems across your fleet.
- **Disk-encryption summary** — how many machines report disk encryption
  (FileVault / BitLocker / LUKS) turned on. A quick compliance read.

You can show/hide the OS-distribution and disk-encryption widgets from
**Settings** (`/settings`); those preferences are saved in your browser.

The same data is available as JSON for tooling at **`GET /api/v1/servers`** (behind
the dashboard's view permission). Each entry includes the stored server plus its
computed `health` object.

---

## The per-host detail view

Click any server card (or browse `/server/{hostname}`) to open its detail page:

- **Resource charts** — CPU, memory, and disk usage **over time** (time-series). Watch
  for sustained climbs — they're your early warning before a disk fills or a box
  falls over.
- **Host facts** — OS, platform, version, kernel, architecture, uptime,
  virtualization, and encryption status.
- **Disks** — each mounted filesystem with used/total.
- **Network** — interfaces and IPv4 addresses.
- **Packages** — installed software (name + version).
- **Services** — services and whether each is running or stopped.

---

## History & time-series

The control plane keeps a rolling **time-series history** per server so the detail
charts have something to draw:

- On every report it appends one **sample** containing the timestamp, CPU %, memory
  %, and the busiest disk's %.
- It keeps the **last 180 samples** per server (older ones are dropped). At the
  default 1-minute report interval that's roughly **3 hours** of history.

This is the MVP store (latest state + a short rolling history, persisted to JSON).
Long-term retention and a real time-series database are on the roadmap — see
[Roadmap & deferred items](roadmap.md).

---

## How often data updates

- The **agent** reports on its `--interval` (default **1 minute**; configurable, e.g.
  `--interval 30s`).
- A host is considered **stale** after **5 minutes** without a report
  (`StaleAfter`). With the default 1-minute interval, a host has to miss ~5 reports
  before it's flagged stale.
- The control plane also runs a background **sweeper every minute** that
  re-evaluates the whole fleet, so a host that goes silent gets flagged (and can
  alert) even though it stopped sending data. See
  [Alerting → the stale-host sweeper](alerting.md#the-stale-host-sweeper).

---

## The Universal Collector (`collect` subcommand)

`lookout-agent` also ships an **experimental** second pipeline behind the `collect`
subcommand:

```bash
lookout-agent collect --ingest-url https://api.example.com --token ONE_TIME_TOKEN
```

This is the **Universal Collector**: it generates a signed agent identity, registers
a set of reference collectors (system inventory, software, posture), and ships each
record as a **versioned, signed envelope** to a separate "Keystone" ingest control
plane over outbound HTTPS only (redirects disabled). It uses a fail-closed
capability model and a budgeted scheduler.

**This is not how you feed the Lookout dashboard.** The dashboard is fed by
`lookout-agent run` (see [Getting started](getting-started.md#4-deploy--enroll-the-collector-agent)).
The `collect` pipeline targets the broader DosanjhLabs suite ingest and is still
being built out; treat it as a preview. See [Roadmap & deferred items](roadmap.md)
for its status.
