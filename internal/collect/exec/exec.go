// Package exec is the Lookout collector's safe command-execution layer.
//
// Every collector that shells out to an OS tool MUST go through Runner. The
// design eliminates shell-injection surface entirely:
//
//   - No shell. We call exec.Command with an explicit argv. There is never a
//     "sh -c" / "cmd /c" indirection, so argument values can never be reparsed
//     as shell syntax.
//   - Allow-list. Only binaries an operator has explicitly allow-listed (by
//     base name) may run; anything else is refused before exec.
//   - Full-path resolution. The allow-listed base name is resolved to an
//     absolute path via the OS path lookup once, so a planted binary earlier on
//     PATH at call time can't be substituted after the allow-list check.
//   - Env scrub. The child runs with a minimal, fixed environment (no inherited
//     LD_PRELOAD, IFS, PATH tricks, etc.).
//   - Bounded. A context deadline and an output cap prevent a wedged or
//     chatty tool from hanging or exhausting memory.
//
// Standard library only.
package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// maxOutputBytes caps captured stdout to keep a misbehaving tool from
// exhausting agent memory. Collectors that need large output should stream to
// the spool/R2 path, not buffer it here.
const maxOutputBytes = 8 << 20 // 8 MiB

// ErrNotAllowed is returned when a command's base name is not on the allow-list.
var ErrNotAllowed = errors.New("collect/exec: command not allow-listed")

// Runner executes allow-listed binaries with no shell and a scrubbed env.
// A zero Runner allows nothing; build one with NewRunner.
type Runner struct {
	// allowed maps an allow-listed base name (e.g. "df", "trivy") to its
	// resolved absolute path. Resolution happens once at construction.
	allowed map[string]string
	// baseEnv is the fixed, minimal environment handed to every child.
	baseEnv []string
}

// NewRunner builds a Runner from a list of allow-listed binary base names.
// Names that cannot be resolved on the current host are skipped (the collector
// that needs them will get ErrNotAllowed and degrade gracefully), so a Runner
// never fails to construct just because an optional tool is absent.
func NewRunner(allow []string) *Runner {
	r := &Runner{
		allowed: make(map[string]string, len(allow)),
		baseEnv: scrubbedEnv(),
	}
	for _, name := range allow {
		name = strings.TrimSpace(name)
		if name == "" || strings.ContainsAny(name, `/\`) {
			// Reject path-y entries: the allow-list is by base name only, so a
			// caller can't slip an absolute path past resolution.
			continue
		}
		if abs, err := exec.LookPath(name); err == nil {
			r.allowed[name] = abs
		}
	}
	return r
}

// Allowed reports whether name is allow-listed and resolvable on this host.
func (r *Runner) Allowed(name string) bool {
	_, ok := r.allowed[name]
	return ok
}

// Run executes the allow-listed binary `name` with explicit args (never a
// shell) and returns trimmed stdout. ctx bounds the wall-clock time. args are
// passed verbatim as argv elements; because there is no shell, they cannot be
// reinterpreted as operators, redirections, or sub-commands.
func (r *Runner) Run(ctx context.Context, name string, args ...string) (string, error) {
	abs, ok := r.allowed[name]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrNotAllowed, name)
	}

	cmd := exec.CommandContext(ctx, abs, args...)
	cmd.Env = r.baseEnv // scrubbed, fixed environment

	var stdout bytes.Buffer
	// Cap stdout so a runaway tool can't exhaust memory.
	cmd.Stdout = &capWriter{buf: &stdout, max: maxOutputBytes}
	cmd.Stderr = nil // never capture/leak stderr into structured output

	err := cmd.Run()
	out := strings.TrimSpace(stdout.String())
	if err != nil {
		// Return whatever we captured plus the error; some tools exit non-zero
		// but still print useful structured output the caller may tolerate.
		if ctx.Err() != nil {
			return out, fmt.Errorf("collect/exec: %q timed out: %w", name, ctx.Err())
		}
		return out, fmt.Errorf("collect/exec: %q failed: %w", name, err)
	}
	return out, nil
}

// capWriter discards writes once max bytes have been buffered.
type capWriter struct {
	buf *bytes.Buffer
	max int
}

func (w *capWriter) Write(p []byte) (int, error) {
	remaining := w.max - w.buf.Len()
	if remaining <= 0 {
		return len(p), nil // swallow overflow; report full length so the child isn't EPIPE'd
	}
	if len(p) > remaining {
		w.buf.Write(p[:remaining])
		return len(p), nil
	}
	return w.buf.Write(p)
}

// scrubbedEnv returns a minimal, fixed environment for child processes. We do
// NOT inherit the agent's environment: that strips LD_PRELOAD/DYLD_*, custom
// IFS, attacker-controlled PATH, and any leaked secrets. A conservative system
// PATH is set so allow-listed tools that themselves shell out to absolute
// system paths still function.
func scrubbedEnv() []string {
	path := "/usr/bin:/bin:/usr/sbin:/sbin"
	env := []string{"LC_ALL=C", "LANG=C"}
	if runtime.GOOS == "windows" {
		// On Windows, PATH casing and SystemRoot matter for resolving system
		// DLLs; keep it minimal but functional. TODO(wave0): pull SystemRoot
		// from the live env rather than assuming C:\Windows.
		return []string{
			`SystemRoot=C:\Windows`,
			`Path=C:\Windows\System32;C:\Windows`,
		}
	}
	env = append(env, "PATH="+path)
	return env
}
