// Package check runs health checks against monitored targets: a TCP-port reach
// test and an HTTP-endpoint test. It is used by the control plane to probe hosts
// and services, so — like alert delivery — it makes outbound requests to targets
// that are ultimately operator-supplied and must be treated as an SSRF surface.
//
// The HTTP check therefore mirrors the redirect- and DNS-rebinding defenses in
// internal/alert/safeclient.go, but with a deliberately different address policy
// (see httpClient): monitoring loopback and private hosts is the whole point of
// this product, so those are allowed; only addresses that are never a legitimate
// monitor target (link-local, multicast, unspecified) are blocked.
//
// Nothing in this package logs, and no Result ever echoes response bytes,
// headers, or any other secret — only generic reasons.
package check

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// defaultTimeout is used when a Check leaves Timeout unset.
const defaultTimeout = 10 * time.Second

// maxBody caps how much of an HTTP response body we read for the keyword scan.
// The body is attacker-influenced, so this is a denial-of-service guard, not a
// correctness limit.
const maxBody = 1 << 20 // 1 MiB

// Check describes a single health check to run against one target.
type Check struct {
	ID   string
	Name string
	// Type selects the probe: "tcp" or "http".
	Type string
	// Target is "host:port" for tcp, or a full http(s) URL for http.
	Target string
	// ExpectStatus is the required HTTP status code; 0 means "any 2xx is OK".
	ExpectStatus int
	// Keyword, if non-empty, is a substring that must appear in the HTTP body.
	Keyword string
	// Timeout bounds a single run; 0 means defaultTimeout.
	Timeout time.Duration
}

// Result is the outcome of running a Check. Status is "ok" or "critical".
// Reason is a generic, secret-free explanation, suitable for storage and display.
type Result struct {
	Status string
	Reason string
}

const (
	statusOK       = "ok"
	statusCritical = "critical"
)

func ok(reason string) Result       { return Result{Status: statusOK, Reason: reason} }
func critical(reason string) Result { return Result{Status: statusCritical, Reason: reason} }

// timeout returns the effective per-run timeout for c.
func (c Check) timeout() time.Duration {
	if c.Timeout <= 0 {
		return defaultTimeout
	}
	return c.Timeout
}

// Run dispatches on Type and returns the check outcome. An unknown type is a
// critical result rather than an error, so a misconfigured check is visible in
// the dashboard like any other failure.
func (c Check) Run(ctx context.Context) Result {
	switch c.Type {
	case "tcp":
		return c.runTCP(ctx)
	case "http":
		return c.runHTTP(ctx)
	default:
		return critical(fmt.Sprintf("unknown check type %q", c.Type))
	}
}

// runTCP succeeds if a TCP connection to Target can be established within the
// timeout. The connection is closed immediately; we only care that it opened.
func (c Check) runTCP(ctx context.Context) Result {
	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", c.Target)
	if err != nil {
		return critical(fmt.Sprintf("cannot connect to %s", c.Target))
	}
	_ = conn.Close()
	return ok("port reachable")
}

// runHTTP fetches Target and evaluates status and (optionally) a body keyword.
// It is SECURITY-CRITICAL: see httpClient for the SSRF policy. Reasons never
// include response bytes.
func (c Check) runHTTP(ctx context.Context) Result {
	// Only http/https are ever fetched. Reject anything else before a request
	// is made (e.g. file://, ftp://, gopher://), since those are not monitor
	// targets and some are SSRF vectors.
	u, err := url.Parse(strings.TrimSpace(c.Target))
	if err != nil {
		return critical("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return critical(fmt.Sprintf("scheme %q not allowed (use http or https)", u.Scheme))
	}

	timeout := c.timeout()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return critical("invalid URL")
	}

	resp, err := httpClient(timeout).Do(req)
	if err != nil {
		// The transport error (e.g. connection refused, blocked dial, timeout)
		// describes the connection, not the response body, so it is safe to
		// surface and useful for diagnosing the target.
		return critical(fmt.Sprintf("request failed: %v", err))
	}
	defer resp.Body.Close()

	// Status policy: exact match when ExpectStatus is set, otherwise any 2xx.
	// A redirect (30x) satisfies neither and is reported as-is, never followed.
	if c.ExpectStatus > 0 {
		if resp.StatusCode != c.ExpectStatus {
			return critical(fmt.Sprintf("expected status %d, got %d", c.ExpectStatus, resp.StatusCode))
		}
	} else if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return critical(fmt.Sprintf("unexpected status %d", resp.StatusCode))
	}

	// Keyword scan over a bounded read of the body. We never put the body bytes
	// into a Result — only whether the keyword was present.
	if c.Keyword != "" {
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
		if err != nil {
			return critical("request failed: error reading response body")
		}
		if !strings.Contains(string(body), c.Keyword) {
			return critical("keyword not found")
		}
	}

	return ok("")
}

// httpClient builds the dedicated client for HTTP checks. It is modeled on
// internal/alert/safeclient.go (no redirect following; validate the
// actually-dialed IP in DialContext and pin to it to defeat DNS-rebinding
// TOCTOU) but applies a monitoring-appropriate address policy.
//
// Address policy — this is the key difference from alert/ssrf.go's blockedIP:
// a webhook target should always be public, so that code blocks loopback and
// RFC1918 private addresses. Here, monitoring one's own internal hosts and
// localhost is the explicit, legitimate purpose of the product, so those are
// ALLOWED. We block only addresses that are never a legitimate monitor target
// and are the real SSRF danger:
//
//   - link-local (169.254.0.0/16 / fe80::/10) — covers the 169.254.169.254
//     cloud-metadata endpoint, the classic SSRF credential-theft target;
//   - multicast — not a unicast service to probe;
//   - unspecified (0.0.0.0 / ::) — not a real destination.
func httpClient(timeout time.Duration) *http.Client {
	d := &net.Dialer{Timeout: timeout}
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse // never follow redirects to an unvalidated target
		},
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
				if err != nil {
					return nil, err
				}
				for _, ip := range ips {
					if blockedDialIP(ip) {
						return nil, fmt.Errorf("blocked dial to disallowed address %s", ip)
					}
				}
				// Pin to a validated IP so the OS can't re-resolve to a
				// different (unchecked) address between this check and the
				// connect.
				ip := ips[0]
				return d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			},
		},
	}
}

// blockedDialIP reports whether an address must never be the target of an HTTP
// check. Unlike alert.blockedIP, loopback and private addresses are permitted
// (monitoring internal hosts is the point); only the categories that are never a
// legitimate monitor target are refused.
func blockedDialIP(ip net.IP) bool {
	return ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified()
}
