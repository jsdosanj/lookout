//go:build linux

package software

import (
	"context"
	"strings"
)

// gather lists installed packages from dpkg (Debian/Ubuntu) and/or rpm
// (RHEL/Fedora/SUSE), using allow-listed argv-only queries with explicit
// machine-readable format strings.
func (c *Collector) gather(ctx context.Context) []Package {
	if c.runner == nil {
		return nil
	}
	var pkgs []Package

	// dpkg: tab-separated name, version, arch for installed packages only.
	if c.runner.Allowed("dpkg-query") {
		if out, err := c.runner.Run(ctx, "dpkg-query", "-W",
			"-f=${Package}\t${Version}\t${Architecture}\t${Status}\n"); err == nil {
			pkgs = append(pkgs, parseDpkg(out)...)
		}
	}

	// rpm: explicit queryformat, name version arch.
	if c.runner.Allowed("rpm") {
		if out, err := c.runner.Run(ctx, "rpm", "-qa",
			"--queryformat", "%{NAME}\t%{VERSION}-%{RELEASE}\t%{ARCH}\n"); err == nil {
			pkgs = append(pkgs, parseRPM(out)...)
		}
	}
	return pkgs
}

// parseDpkg parses dpkg-query tab output; keeps only "install ok installed".
func parseDpkg(out string) []Package {
	var pkgs []Package
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 4 {
			continue
		}
		if !strings.Contains(f[3], "installed") {
			continue
		}
		pkgs = append(pkgs, Package{Name: f[0], Version: f[1], Arch: f[2], Source: "dpkg"})
	}
	return pkgs
}

// parseRPM parses the rpm queryformat tab output.
func parseRPM(out string) []Package {
	var pkgs []Package
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 3 {
			continue
		}
		pkgs = append(pkgs, Package{Name: f[0], Version: f[1], Arch: f[2], Source: "rpm"})
	}
	return pkgs
}
