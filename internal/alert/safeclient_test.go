package alert

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The safe client's dialer must refuse to connect to a non-public address, even
// when the URL was not (or could not be) screened by SafeWebhookURL first. This
// is the dial-time check that closes the DNS-rebinding TOCTOU window and also
// neutralises a redirect whose Location points at an internal host: the dial to
// that host fails. httptest listens on 127.0.0.1, so a successful Get would mean
// the loopback guard is absent.
func TestSafeClientBlocksNonPublicDial(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := safeHTTPClient(5 * time.Second)
	_, err := c.Get(srv.URL) // http://127.0.0.1:PORT
	if err == nil {
		t.Fatal("SSRF bypass: client dialed a loopback (non-public) address")
	}
	if !strings.Contains(err.Error(), "non-public") {
		t.Fatalf("expected a non-public dial block, got: %v", err)
	}
}

// The safe client must never follow redirects: a public webhook could otherwise
// 30x-bounce the request to an internal target. We assert the policy returns
// ErrUseLastResponse for any redirect.
func TestSafeClientDoesNotFollowRedirects(t *testing.T) {
	c := safeHTTPClient(5 * time.Second)
	if c.CheckRedirect == nil {
		t.Fatal("safe client must set a CheckRedirect policy")
	}
	if err := c.CheckRedirect(nil, nil); err != http.ErrUseLastResponse {
		t.Fatalf("redirect policy = %v, want ErrUseLastResponse", err)
	}
}
