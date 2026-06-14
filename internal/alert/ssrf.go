package alert

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// SafeWebhookURL validates a webhook target before any request is made, to stop
// server-side request forgery (SSRF): a webhook URL is operator-supplied, but it
// is fetched by the control plane, so an attacker who can set it could otherwise
// pivot to internal services (cloud metadata, admin ports, localhost).
//
// We allow only http/https to a hostname (or literal IP) that does not resolve
// to a private, loopback, link-local, or otherwise non-public address. DNS is
// resolved here and every answer is checked, so a public name that maps to an
// internal IP is also rejected.
func SafeWebhookURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme %q not allowed (use http or https)", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("missing host")
	}
	// A literal IP is checked directly; a name is resolved and every answer checked.
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		resolved, err := net.LookupIP(host)
		if err != nil {
			return fmt.Errorf("cannot resolve host %q: %w", host, err)
		}
		ips = resolved
	}
	for _, ip := range ips {
		if blockedIP(ip) {
			return fmt.Errorf("host %q resolves to a non-public address (%s)", host, ip)
		}
	}
	return nil
}

// blockedIP reports whether an address must not be the target of a webhook.
func blockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	// Cloud metadata endpoint (169.254.169.254) is link-local and already caught;
	// also block the IPv4-mapped form and carrier-grade NAT range used internally.
	if v4 := ip.To4(); v4 != nil {
		// 100.64.0.0/10 (CGNAT, RFC 6598) — commonly internal.
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true
		}
	}
	return false
}
