package alert

import (
	"strings"
	"testing"
	"time"
)

func TestSafeWebhookURLBlocksInternal(t *testing.T) {
	bad := []string{
		"http://localhost/hook",
		"http://127.0.0.1/hook",
		"https://127.0.0.1:8080/x",
		"http://10.0.0.5/hook",        // private
		"http://192.168.1.10/hook",    // private
		"http://172.16.4.4/hook",      // private
		"http://169.254.169.254/meta", // cloud metadata (link-local)
		"http://100.100.0.1/x",        // CGNAT
		"http://[::1]/hook",           // IPv6 loopback
		" file:///etc/passwd",         // non-http scheme
		"ftp://example.com/x",         // non-http scheme
		"http:///nohost",              // missing host
	}
	for _, u := range bad {
		if err := SafeWebhookURL(u); err == nil {
			t.Errorf("SafeWebhookURL(%q) = nil, want error", u)
		}
	}
}

func TestSafeWebhookURLAllowsPublic(t *testing.T) {
	good := []string{
		"https://hooks.slack.com/services/T000/B000/xxx",
		"https://1.1.1.1/hook",
		"http://8.8.8.8/notify",
	}
	for _, u := range good {
		if err := SafeWebhookURL(u); err != nil {
			t.Errorf("SafeWebhookURL(%q) = %v, want nil", u, err)
		}
	}
}

func TestNewWebhookChannelRejectsUnsafe(t *testing.T) {
	if _, err := NewWebhookChannel("w", "http://127.0.0.1/hook"); err == nil {
		t.Error("NewWebhookChannel should reject a loopback URL")
	}
}

// Email payload is real and verified even though SMTP isn't wired.
func TestEmailRenderAndNotConfigured(t *testing.T) {
	ch := NewEmailChannel("email", []string{"ops@example.com"}, nil)
	n := Notification{Server: "h1", Severity: "critical", Reason: "disk /data is 95% full", At: time.Now()}
	subj, body := ch.RenderEmail(n)
	if !strings.Contains(subj, "h1") || !strings.Contains(subj, "critical") {
		t.Errorf("subject missing fields: %q", subj)
	}
	if !strings.Contains(body, "disk /data is 95% full") {
		t.Errorf("body missing cause: %q", body)
	}
	// Without SMTP creds, Send must refuse honestly (no fake success).
	if err := ch.Send(n); err == nil {
		t.Error("email Send without SMTP config should return an error")
	}
}

func TestNotificationFormat(t *testing.T) {
	cases := []struct {
		n    Notification
		want string
	}{
		{Notification{Server: "h1", Severity: "critical", Reason: "disk 95%"}, "h1 is critical — disk 95%"},
		{Notification{Server: "h1", Severity: "warning", Resolved: true}, "h1 recovered"},
		{Notification{Server: "h1", Severity: "critical", Repeat: true, Reason: "disk 95%"}, "still critical"},
	}
	for _, c := range cases {
		if got := c.n.Format(); !strings.Contains(got, c.want) {
			t.Errorf("Format()=%q, want substring %q", got, c.want)
		}
	}
}
