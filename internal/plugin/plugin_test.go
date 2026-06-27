package plugin

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"testing"

	safeexec "github.com/jsdosanj/lookout/internal/collect/exec"
)

// exitErrWithCode runs a real command via the os/exec stdlib (NOT the safe
// Runner) purely to synthesize a genuine *exec.ExitError carrying the given
// exit code, exactly as classify will receive one wrapped by safe-exec at
// runtime. POSIX-only: it relies on `sh -c "exit N"`.
func exitErrWithCode(t *testing.T, code int) error {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("exitErrWithCode helper is POSIX-only")
	}
	cmd := exec.Command("sh", "-c", "exit "+itoa(code))
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit for code %d, got nil", code)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != code {
		t.Fatalf("synthesized exit code = %d, want %d", exitErr.ExitCode(), code)
	}
	// Wrap it so classify's errors.As must unwrap, mirroring safe-exec's %w wrap.
	return errors.Join(errors.New("collect/exec: \"x\" failed"), exitErr)
}

// itoa is a tiny dependency-free int->string for the helper.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// TestClassifyOK: a nil error (exit 0) is "ok", and the first stdout line is the
// reason per Nagios convention.
func TestClassifyOK(t *testing.T) {
	got := classify("HTTP OK: 200 in 0.1s\nperfdata", nil)
	if got.Status != StatusOK {
		t.Fatalf("status = %q, want ok", got.Status)
	}
	if got.Reason != "HTTP OK: 200 in 0.1s" {
		t.Fatalf("reason = %q, want first stdout line", got.Reason)
	}
}

// TestClassifyWarning: exit 1 => warning.
func TestClassifyWarning(t *testing.T) {
	got := classify("DISK WARNING: 85% used", exitErrWithCode(t, 1))
	if got.Status != StatusWarning {
		t.Fatalf("status = %q, want warning", got.Status)
	}
	if got.Reason != "DISK WARNING: 85% used" {
		t.Fatalf("reason = %q", got.Reason)
	}
}

// TestClassifyCritical: exit 2 => critical.
func TestClassifyCritical(t *testing.T) {
	got := classify("DISK CRITICAL: 99% used", exitErrWithCode(t, 2))
	if got.Status != StatusCritical {
		t.Fatalf("status = %q, want critical", got.Status)
	}
	if got.Reason != "DISK CRITICAL: 99% used" {
		t.Fatalf("reason = %q", got.Reason)
	}
}

// TestClassifyUnknownCode3: exit 3 => unknown.
func TestClassifyUnknownCode3(t *testing.T) {
	got := classify("things are weird", exitErrWithCode(t, 3))
	if got.Status != StatusUnknown {
		t.Fatalf("status = %q, want unknown", got.Status)
	}
	if got.Reason != "things are weird" {
		t.Fatalf("reason = %q", got.Reason)
	}
}

// TestClassifyOtherCode: any non-conformant exit code (e.g. 5) => unknown.
func TestClassifyOtherCode(t *testing.T) {
	got := classify("plugin misbehaved", exitErrWithCode(t, 5))
	if got.Status != StatusUnknown {
		t.Fatalf("status = %q, want unknown for code 5", got.Status)
	}
}

// TestClassifyNonExitError: a non-ExitError (timeout / ErrNotAllowed / spawn
// failure) => unknown with a generic, non-leaking reason.
func TestClassifyNonExitError(t *testing.T) {
	got := classify("", errors.New("some internal failure with /secret/path"))
	if got.Status != StatusUnknown {
		t.Fatalf("status = %q, want unknown", got.Status)
	}
	if got.Reason != "plugin did not run" {
		t.Fatalf("reason = %q, want generic non-leaking phrase", got.Reason)
	}
}

// TestClassifyEmptyOutputFallbacks: when stdout is empty, each status falls back
// to its generic phrase rather than an empty reason.
func TestClassifyEmptyOutputFallbacks(t *testing.T) {
	if r := classify("", nil); r.Reason != "plugin returned OK" {
		t.Fatalf("ok fallback = %q", r.Reason)
	}
	if r := classify("", exitErrWithCode(t, 1)); r.Reason != "plugin returned WARNING" {
		t.Fatalf("warning fallback = %q", r.Reason)
	}
	if r := classify("", exitErrWithCode(t, 2)); r.Reason != "plugin returned CRITICAL" {
		t.Fatalf("critical fallback = %q", r.Reason)
	}
	if r := classify("", exitErrWithCode(t, 3)); r.Reason != "plugin status unknown" {
		t.Fatalf("unknown fallback = %q", r.Reason)
	}
}

// TestRunOKAndWarning exercises the full path through the safe Runner using the
// ubiquitous /usr/bin/true (exit 0) and /usr/bin/false (exit 1), mirroring how
// exec_test.go skips when a tool isn't on PATH.
func TestRunOKAndWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on POSIX true/false")
	}
	r := safeexec.NewRunner([]string{"true", "false"})

	if r.Allowed("true") {
		got := Run(context.Background(), r, Plugin{Name: "t", Command: "true"})
		if got.Status != StatusOK {
			t.Fatalf("true => status %q, want ok", got.Status)
		}
	} else {
		t.Log("true not on PATH; skipping ok case")
	}

	if r.Allowed("false") {
		got := Run(context.Background(), r, Plugin{Name: "f", Command: "false"})
		if got.Status != StatusWarning {
			t.Fatalf("false (exit 1) => status %q, want warning", got.Status)
		}
	} else {
		t.Log("false not on PATH; skipping warning case")
	}
}

// TestRunNotAllowed: a command that isn't allow-listed never runs and resolves
// to unknown (ErrNotAllowed is not an ExitError).
func TestRunNotAllowed(t *testing.T) {
	r := safeexec.NewRunner(nil) // allow nothing
	got := Run(context.Background(), r, Plugin{Name: "x", Command: "definitely_not_allowed"})
	if got.Status != StatusUnknown {
		t.Fatalf("status = %q, want unknown for non-allow-listed command", got.Status)
	}
	if got.Reason != "plugin did not run" {
		t.Fatalf("reason = %q", got.Reason)
	}
}
