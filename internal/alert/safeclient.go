package alert

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// safeHTTPClient is the only client used to deliver operator-supplied webhook
// and notify-service requests. It closes two SSRF-guard bypasses that a static
// up-front SafeWebhookURL check cannot:
//
//   - Redirect bypass: a public endpoint can answer 30x with a Location pointing
//     at an internal host (e.g. http://169.254.169.254/). The default client
//     would follow it with no further validation, so we refuse to follow any
//     redirect at all.
//   - DNS-rebinding TOCTOU: SafeWebhookURL resolves a name and checks the
//     answer, but the client's own dial resolves independently afterwards, so a
//     hostile resolver can answer public for the check and private for the
//     connect. We validate the address the transport actually dials, in the
//     dialer, which is the moment the connection is made — there is no window.
func safeHTTPClient(timeout time.Duration) *http.Client {
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
					if blockedIP(ip) {
						return nil, fmt.Errorf("blocked dial to non-public address %s", ip)
					}
				}
				// Pin to a validated IP so the OS can't re-resolve to a different
				// (unchecked) address between this check and the connect.
				ip := ips[0]
				return d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			},
		},
	}
}
