package alert

import (
	"strings"
	"testing"
)

// TestBuildEmailMessage checks the SMTP DATA payload: required headers are present
// and the body is normalized to CRLF line endings (SMTP requires "\r\n").
func TestBuildEmailMessage(t *testing.T) {
	msg := string(buildEmailMessage("alerts@example.com",
		[]string{"a@example.com", "b@example.com"},
		"[Lookout] db-01 — critical",
		"line one\nline two"))

	for _, want := range []string{
		"From: alerts@example.com\r\n",
		"To: a@example.com, b@example.com\r\n",
		"Subject: [Lookout] db-01 — critical\r\n",
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n",
		"line one\r\nline two",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q\n---\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "line one\nline two") {
		t.Error("body still has bare LF; SMTP needs CRLF")
	}
}

// TestEmailSendNotConfigured confirms the honest boundary: with no SMTP config,
// Send refuses rather than faking success.
func TestEmailSendNotConfigured(t *testing.T) {
	ch := NewEmailChannel("email", []string{"a@example.com"}, nil)
	if err := ch.Send(Notification{Server: "db-01", Severity: "critical"}); err == nil {
		t.Fatal("expected not-configured error, got nil")
	}
}
