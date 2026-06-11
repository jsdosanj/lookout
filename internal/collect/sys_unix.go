//go:build linux || darwin

package collect

import (
	"os/exec"
	"strings"
)

// runCmd runs a command with fixed arguments (never via a shell) and returns
// trimmed stdout. Using exec.Command with explicit args means no shell parsing
// and no injection surface.
func runCmd(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// unixDisks lists mounted filesystems via POSIX `df -kP`.
func unixDisks() []Disk {
	out, err := runCmd("df", "-kP")
	if err != nil {
		return nil
	}
	return parseDf(out)
}

// collectProcesses lists the busiest processes via POSIX `ps`.
func collectProcesses() []Process {
	out, err := runCmd("ps", "-axo", "pid=,pcpu=,pmem=,comm=")
	if err != nil {
		return nil
	}
	return parseProcesses(out)
}
