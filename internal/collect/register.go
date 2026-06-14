// Package collect's register.go provides Wire: the single entry point the
// agent's cmd uses to assemble the Universal Collector stack — a safe-exec
// runner allow-listing exactly the binaries the enabled collectors need, the
// three wave0 reference collectors registered into the shared registry, and a
// scheduler wired to a shipper.
//
// NOTE: this file lives in package collect (the existing legacy package) only
// to provide a stable top-level wiring symbol under the assigned directory. It
// defines no symbols that collide with the legacy Report/Collect API.
package collect

import (
	"github.com/jsdosanj/lookout/internal/collect/exec"
	"github.com/jsdosanj/lookout/internal/collect/inventory"
	"github.com/jsdosanj/lookout/internal/collect/posture"
	"github.com/jsdosanj/lookout/internal/collect/software"
	"github.com/jsdosanj/lookout/internal/collector"
)

// RegisterReferenceCollectors builds a safe-exec runner that allow-lists the
// union of the three reference collectors' required binaries, constructs each
// collector with that runner, and registers them into the shared
// collector.Default() registry. It returns the runner so the caller can reuse
// it for additional collectors. Call exactly once at startup.
func RegisterReferenceCollectors() *exec.Runner {
	allow := dedupStrings(
		inventory.DefaultAllow(),
		software.DefaultAllow(),
		posture.DefaultAllow(),
	)
	runner := exec.NewRunner(allow)

	collector.Register(inventory.New(runner))
	collector.Register(software.New(runner))
	collector.Register(posture.New(runner))
	return runner
}

// dedupStrings flattens and de-duplicates allow-list slices.
func dedupStrings(lists ...[]string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, l := range lists {
		for _, s := range l {
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}
