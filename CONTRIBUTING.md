# Contributing to Lookout

Thanks for helping make Lookout better. It is free and open source under
[Apache-2.0](LICENSE), and contributions of all sizes are welcome — bug reports,
docs, and code.

## Ground rules

- Be honest about capabilities. Lookout's engineering norm is to **never fake a
  feature** — if something isn't wired up, it returns an explicit error rather than
  pretending to succeed. Keep it that way (see [`docs/manual/roadmap.md`](docs/manual/roadmap.md)).
- Security is a first-class requirement. Agents are **outbound-only**, there is **no
  shell** (every command uses `exec.Command` with fixed arguments), and **no secrets**
  belong in code, logs, or commits.
- Keep changes surgical and the standard-library-only spirit intact. The only runtime
  dependency is `golang.org/x/crypto`; don't add dependencies without discussion.

## Prerequisites

- **Go 1.26+** (`go version`). Docker is optional (for the compose workflow).

## Build, vet, and test

Before opening a PR, this must pass clean:

```bash
go build ./...
go vet ./...
go test ./...
```

Useful while developing:

```bash
# run the control plane locally (dashboard on http://localhost:8080)
LOOKOUT_TOKEN=dev LOOKOUT_ADMIN_EMAIL=you@example.com \
  LOOKOUT_ADMIN_PASSWORD=devpassword go run ./cmd/lookout-server

# print a host report
go run ./cmd/lookout-agent report

# regenerate the static dashboard demo under ./docs
go run ./cmd/lookout-demo
```

## Pull requests

1. Fork the repo and create a branch from the default branch.
2. Make focused changes — every changed line should trace to the issue/feature.
3. Add or update tests. New behavior in `internal/...` should be covered; parsing and
   alert logic are pure and unit-tested by design.
4. Run the build/vet/test trio above.
5. Open a PR with a clear description of *what* and *why*. Link any related issue.
   CI runs the same checks; PRs must be green to merge.

By contributing you agree your contributions are licensed under Apache-2.0.

## Reporting bugs and requesting features

Open a GitHub issue with steps to reproduce, the OS/arch involved, and the relevant
log lines (with secrets redacted). For **security vulnerabilities, do not open a
public issue** — follow [SECURITY.md](SECURITY.md).
