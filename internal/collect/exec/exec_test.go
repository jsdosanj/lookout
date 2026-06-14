package exec

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
)

// TestNotAllowed verifies a non-allow-listed binary is refused before exec.
func TestNotAllowed(t *testing.T) {
	r := NewRunner(nil) // empty allow-list
	_, err := r.Run(context.Background(), "echo", "hi")
	if !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("want ErrNotAllowed, got %v", err)
	}
}

// TestAllowListRejectsPaths ensures path-y allow-list entries can't smuggle an
// absolute binary past base-name resolution.
func TestAllowListRejectsPaths(t *testing.T) {
	r := NewRunner([]string{"/bin/echo", `..\evil`})
	if r.Allowed("/bin/echo") || r.Allowed("echo") {
		t.Fatal("path-y allow-list entry should not have been accepted")
	}
}

// TestNoShellInjection proves args are passed as argv, never reparsed by a
// shell. We run a real allow-listed binary on POSIX with an argument that WOULD
// be dangerous under a shell ("; rm -rf") and confirm it is treated as a
// literal string, not executed.
func TestNoShellInjection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-only shell-injection assertion")
	}
	r := NewRunner([]string{"echo"})
	if !r.Allowed("echo") {
		t.Skip("echo not present on PATH")
	}
	const payload = "hello; rm -rf /tmp/should-not-run"
	out, err := r.Run(context.Background(), "echo", payload)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// The literal payload (incl. the "; rm -rf" text) must come back verbatim —
	// proof it was an argv element, not shell-interpreted.
	if !strings.Contains(out, payload) {
		t.Fatalf("argument was not passed verbatim; got %q", out)
	}
}

// TestOutputCap ensures captured output is bounded.
func TestOutputCap(t *testing.T) {
	var buf bytes.Buffer
	w := &capWriter{buf: &buf, max: 4}
	n, _ := w.Write([]byte("123456789"))
	if n != 9 {
		t.Fatalf("Write must report full length to avoid EPIPE, got %d", n)
	}
	if buf.Len() != 4 {
		t.Fatalf("buffer should cap at 4, got %d", buf.Len())
	}
}
