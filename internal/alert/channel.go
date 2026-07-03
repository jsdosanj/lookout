package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

// Format renders a notification as the plain-English line we send to chat
// channels (Slack/Teams both accept {"text": ...}).
func (n Notification) Format() string {
	icon := map[string]string{"warning": "🟠", "critical": "🔴", "stale": "⚪"}[n.Severity]
	switch {
	case n.Resolved:
		return fmt.Sprintf("✅ Lookout: %s recovered (was %s)", n.Server, n.Severity)
	case n.Repeat:
		msg := fmt.Sprintf("%s Lookout (reminder): %s is still %s", icon, n.Server, n.Severity)
		if n.Reason != "" {
			msg += " — " + n.Reason
		}
		return msg
	default:
		msg := fmt.Sprintf("%s Lookout: %s is %s", icon, n.Server, n.Severity)
		if n.Reason != "" {
			msg += " — " + n.Reason
		}
		return msg
	}
}

// WebhookChannel delivers notifications by POSTing JSON to an operator-supplied
// URL (Slack, Teams, PagerDuty, Opsgenie, or any endpoint). This is a real,
// working transport. Every target is SSRF-checked before the request is made.
type WebhookChannel struct {
	id     string
	url    string
	client *http.Client
}

// NewWebhookChannel builds a webhook channel. The URL is validated against the
// SSRF guard up front; an unsafe URL is rejected here so it never reaches Send.
func NewWebhookChannel(id, url string) (*WebhookChannel, error) {
	if err := SafeWebhookURL(url); err != nil {
		return nil, err
	}
	return &WebhookChannel{id: id, url: url, client: safeHTTPClient(10 * time.Second)}, nil
}

func (c *WebhookChannel) ID() string { return c.id }

func (c *WebhookChannel) Send(n Notification) error {
	// Re-check on every send: DNS can re-bind a name to an internal IP between
	// configuration and delivery (DNS-rebinding SSRF).
	if err := SafeWebhookURL(c.url); err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{"text": n.Format()})
	req, err := http.NewRequest(http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook %s returned %d", c.id, resp.StatusCode)
	}
	return nil
}

// EmailChannel is the email transport. The decision logic, payload, and — when
// SMTPConfig is supplied — the live send are all real. A self-hoster sets
// LOOKOUT_SMTP_* to get real email alerts with no dependency on our cloud. The
// boundary is honest: configuring email without SMTP creds returns a clear "not
// configured" error rather than silently dropping or faking a send.
type EmailChannel struct {
	id   string
	to   []string
	smtp *SMTPConfig // nil until the operator supplies credentials
}

// SMTPConfig holds the credentials a live email send needs.
type SMTPConfig struct {
	Host string
	Port int
	User string
	Pass string
	From string
}

// NewEmailChannel builds an email channel. smtp may be nil; if so, Send returns
// an explicit not-configured error (no fake success).
func NewEmailChannel(id string, to []string, smtp *SMTPConfig) *EmailChannel {
	return &EmailChannel{id: id, to: to, smtp: smtp}
}

func (c *EmailChannel) ID() string { return c.id }

// RenderEmail builds the subject and body for a notification. This is pure and
// unit-tested so the email payload is verified independently of the SMTP send.
func (c *EmailChannel) RenderEmail(n Notification) (subject, body string) {
	subject = fmt.Sprintf("[Lookout] %s — %s", n.Server, n.Severity)
	if n.Resolved {
		subject = fmt.Sprintf("[Lookout] %s recovered", n.Server)
	}
	body = n.Format()
	if n.Reason != "" && !n.Resolved {
		body += "\n\nCause: " + n.Reason
	}
	body += fmt.Sprintf("\n\nServer: %s\nSeverity: %s\nTime: %s\n",
		n.Server, n.Severity, n.At.Format(time.RFC3339))
	return subject, body
}

func (c *EmailChannel) Send(n Notification) error {
	if c.smtp == nil {
		// No transport configured. We refuse rather than pretend: the payload is
		// real, but there is nowhere to send it.
		return fmt.Errorf("email channel %q: SMTP not configured", c.id)
	}
	subject, body := c.RenderEmail(n)
	from := c.smtp.From
	if from == "" {
		from = c.smtp.User
	}
	addr := fmt.Sprintf("%s:%d", c.smtp.Host, c.smtp.Port)
	// net/smtp.SendMail negotiates STARTTLS automatically when the server
	// advertises it, and PLAIN auth refuses to send credentials over an
	// unencrypted, non-localhost link — so credentials are never sent in the
	// clear. auth is nil for an unauthenticated relay (User unset).
	var auth smtp.Auth
	if c.smtp.User != "" {
		auth = smtp.PlainAuth("", c.smtp.User, c.smtp.Pass, c.smtp.Host)
	}
	if err := smtp.SendMail(addr, auth, from, c.to, buildEmailMessage(from, c.to, subject, body)); err != nil {
		return fmt.Errorf("email channel %q: %w", c.id, err)
	}
	return nil
}

// buildEmailMessage assembles an RFC-5322 message with CRLF line endings for the
// SMTP DATA phase. The rendered body uses "\n"; SMTP requires "\r\n".
func buildEmailMessage(from string, to []string, subject, body string) []byte {
	headers := "From: " + from + "\r\n" +
		"To: " + strings.Join(to, ", ") + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n"
	return []byte(headers + strings.ReplaceAll(body, "\n", "\r\n"))
}
