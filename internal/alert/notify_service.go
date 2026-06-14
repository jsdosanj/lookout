package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// NotifyServiceChannel delivers email via the shared DosanjhLabs notification
// service (the platform "POST /notify/send" contract, see
// plans/10-notifications-runner.md). This is the production email transport: the
// service owns the provider key, dedupe, retry, and the audit log; Lookout only
// hands it a secret-free payload. The local EmailChannel remains the offline
// fallback when this service is not configured.
//
// The boundary is honest: if no bearer token is configured the channel refuses
// rather than faking a send, and the base URL is SSRF-checked up front and again
// on every send (DNS-rebinding defense), exactly like WebhookChannel.
type NotifyServiceChannel struct {
	id       string
	endpoint string   // full URL: <baseURL>/notify/send
	token    string   // bearer auth for the service (tenant derived server-side)
	to       []string // recipient email addresses
	template string   // registered template id (product-namespaced)
	client   *http.Client
}

// NewNotifyServiceChannel builds the channel. baseURL is the notification
// service root (e.g. https://api.dosanjhlabs.com); token authenticates Lookout to
// it; to are the email recipients. The composed endpoint is SSRF-validated up
// front so an unsafe target never reaches Send.
func NewNotifyServiceChannel(id, baseURL, token string, to []string) (*NotifyServiceChannel, error) {
	if token == "" {
		return nil, fmt.Errorf("notify service channel %q: no service token configured", id)
	}
	endpoint := baseURL
	for len(endpoint) > 0 && endpoint[len(endpoint)-1] == '/' {
		endpoint = endpoint[:len(endpoint)-1]
	}
	endpoint += "/notify/send"
	if err := SafeWebhookURL(endpoint); err != nil {
		return nil, err
	}
	return &NotifyServiceChannel{
		id: id, endpoint: endpoint, token: token, to: to,
		template: "lookout.alert",
		client:   safeHTTPClient(10 * time.Second),
	}, nil
}

func (c *NotifyServiceChannel) ID() string { return c.id }

// notifyRequest is the POST /notify/send body (platform contract). data carries
// only template variables — never secrets — so the payload is safe to log/store.
type notifyRequest struct {
	Channel   string         `json:"channel"`
	Target    string         `json:"target"`
	Template  string         `json:"template"`
	Data      map[string]any `json:"data"`
	DedupeKey string         `json:"dedupeKey,omitempty"`
}

func (c *NotifyServiceChannel) Send(n Notification) error {
	// Re-check on every send: a name can re-bind to an internal IP between
	// configuration and delivery (DNS-rebinding SSRF).
	if err := SafeWebhookURL(c.endpoint); err != nil {
		return err
	}
	subject, body := (&EmailChannel{}).RenderEmail(n)
	// Collapse repeats of one incident-state within the service's dedupe window.
	dedupe := fmt.Sprintf("lookout|%s|%s|%s|%t|%t", n.RuleID, n.Server, n.Severity, n.Resolved, n.Repeat)

	var lastErr error
	for _, addr := range c.to {
		req := notifyRequest{
			Channel:  "email",
			Target:   addr,
			Template: c.template,
			Data: map[string]any{
				"subject":  subject,
				"body":     body,
				"server":   n.Server,
				"severity": n.Severity,
				"resolved": n.Resolved,
				"reason":   n.Reason,
			},
			DedupeKey: dedupe,
		}
		if err := c.post(req); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (c *NotifyServiceChannel) post(body notifyRequest) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, c.endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("notify service %s returned %d", c.id, resp.StatusCode)
	}
	return nil
}
