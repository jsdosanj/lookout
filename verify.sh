#!/usr/bin/env bash
# verify.sh — Lookout's verification gate. Run before and after any change.
# Checks formatting, static analysis, build, and the full test suite (which
# includes the alert engine: dedupe, flap-damping, escalation, and the SSRF
# guard on webhook targets).
set -euo pipefail
cd "$(dirname "$0")"

echo "== gofmt (alert engine + server) =="
unformatted="$(gofmt -l internal/alert cmd/lookout-server internal/server || true)"
if [ -n "$unformatted" ]; then
  echo "needs gofmt:"; echo "$unformatted"; exit 1
fi

echo "== go vet =="
go vet ./...

echo "== go build =="
go build ./...

echo "== go test =="
go test ./...

echo "== alert coverage (engine + ssrf) =="
go test -run 'Test(Dedupe|Flap|Escalation|Resolve|Repeat|MinSeverity|ServerScoped|SafeWebhook|Email)' ./internal/alert/ -v | tail -40

echo
echo "OK — verify passed."
