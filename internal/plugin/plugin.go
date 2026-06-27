// Package plugin runs operator-supplied "custom check" executables that follow
// the Nagios plugin convention and maps their exit codes onto Lookout's status
// model.
//
// The Nagios convention is the de-facto standard for monitoring checks: a plugin
// communicates its verdict purely through its process exit code
//
//	0 => OK, 1 => WARNING, 2 => CRITICAL, 3 => UNKNOWN
//
// and prints a one-line human-readable summary on stdout. Thousands of existing
// community checks (check_http, check_disk, check_ssl_cert, ...) speak exactly
// this protocol, so supporting it lets an operator reuse them verbatim.
//
// Security: every check runs through the collector's safe-exec Runner
// (internal/collect/exec). That means no shell is ever involved — Args are
// handed to the OS as discrete argv elements and can never be reinterpreted as
// shell operators, redirections, or sub-commands — the executable must be
// allow-listed by base name, its path is resolved once, and its environment is
// scrubbed. This package adds no execution surface of its own; it only
// classifies the result. No logging, so a noisy plugin can't leak secrets.
//
// Standard library + internal/collect/exec only.
package plugin

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	safeexec "github.com/jsdosanj/lookout/internal/collect/exec"
)

// defaultTimeout bounds a check that doesn't specify its own. Nagios uses 10s by
// default; we allow a little more headroom for network checks.
const defaultTimeout = 30 * time.Second

// Plugin describes a single custom check to run.
type Plugin struct {
	// Name is a human-friendly identifier for this check (for the integrator).
	Name string
	// Command is the allow-listed BASE NAME of the check executable
	// (e.g. "check_http"). It must be on the Runner's allow-list; a path-y value
	// will simply never match and the check resolves to "unknown".
	Command string
	// Args are fixed argv elements passed to the executable VERBATIM. Because the
	// safe-exec Runner uses no shell, these can never be reparsed as shell syntax.
	Args []string

	// Server and Group scope which monitored targets this plugin's result applies
	// to. They are metadata for the integrator; this package does not act on them.
	Server string
	Group  string

	// Interval is how often the integrator should schedule this check. This
	// package does not schedule; it only carries the value.
	Interval time.Duration
	// Timeout bounds a single run. Zero means defaultTimeout.
	Timeout time.Duration
}

// Result is the classified outcome of one plugin run.
type Result struct {
	// Status is one of "ok", "warning", "critical", "unknown".
	Status string
	// Reason is a short human-readable summary (the plugin's first stdout line,
	// per Nagios convention, or a generic fallback).
	Reason string
}

// Status constants for Result.Status.
const (
	StatusOK       = "ok"
	StatusWarning  = "warning"
	StatusCritical = "critical"
	StatusUnknown  = "unknown"
)

// Run executes p through the safe-exec Runner and classifies the result.
//
// The check is injection-safe purely by virtue of the Runner: p.Command is an
// allow-listed base name and p.Args are passed verbatim as argv — there is no
// shell, so no argument can ever be reinterpreted as a shell command.
func Run(ctx context.Context, r *safeexec.Runner, p Plugin) Result {
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := r.Run(ctx, p.Command, p.Args...)
	return classify(out, err)
}

// classify maps a safe-exec Run result onto a Result using the Nagios exit-code
// convention. It is pure (no I/O) so it can be exhaustively unit-tested.
//
// Mapping:
//
//	err == nil (exit 0)        => ok
//	exit 1                     => warning
//	exit 2                     => critical
//	exit 3                     => unknown
//	any other exit code        => unknown
//	non-exit failure           => unknown  (timeout, not-allow-listed, missing
//	                                         binary, spawn failure)
//
// The Reason is the first line of stdout (trimmed), which is where a Nagios
// plugin prints its summary. If stdout is empty we fall back to a generic phrase
// per status. We deliberately do NOT surface the underlying error's text: it can
// contain resolved paths and other internals we don't want to leak.
func classify(out string, err error) Result {
	if err == nil {
		return Result{Status: StatusOK, Reason: reason(out, StatusOK)}
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		// Timeout, ErrNotAllowed, missing binary, or any spawn failure: the check
		// never produced a verdict, so its true state is unknown.
		return Result{Status: StatusUnknown, Reason: "plugin did not run"}
	}

	switch exitErr.ExitCode() {
	case 0:
		// An ExitError with code 0 is not expected (cmd.Run only errors on
		// non-zero), but treat it as success for completeness.
		return Result{Status: StatusOK, Reason: reason(out, StatusOK)}
	case 1:
		return Result{Status: StatusWarning, Reason: reason(out, StatusWarning)}
	case 2:
		return Result{Status: StatusCritical, Reason: reason(out, StatusCritical)}
	case 3:
		return Result{Status: StatusUnknown, Reason: reason(out, StatusUnknown)}
	default:
		// Any other exit code is non-conformant; Nagios treats these as UNKNOWN.
		return Result{Status: StatusUnknown, Reason: reason(out, StatusUnknown)}
	}
}

// reason returns the first non-empty line of stdout (the Nagios summary line),
// or a generic fallback for the given status when stdout is empty.
func reason(out, status string) string {
	if line := firstLine(out); line != "" {
		return line
	}
	switch status {
	case StatusOK:
		return "plugin returned OK"
	case StatusWarning:
		return "plugin returned WARNING"
	case StatusCritical:
		return "plugin returned CRITICAL"
	default:
		return "plugin status unknown"
	}
}

// firstLine returns the first line of s, trimmed of surrounding whitespace.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
