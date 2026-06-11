// Package notify delivers alert messages to webhook endpoints (Slack, Microsoft
// Teams, or any generic webhook — they all accept a JSON {"text": "..."} body).
//
// Email and SMS channels are intentionally not here yet: they need an SMTP
// server / SMS provider (e.g. Twilio) and the operator's credentials.
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Event describes a health-state change worth alerting on.
type Event struct {
	Server    string
	OldStatus string
	NewStatus string
	Reason    string
}

// Notifier posts alert messages to one or more webhook URLs.
type Notifier struct {
	webhooks []string
	client   *http.Client
}

// New returns a Notifier for the given webhook URLs (empty = no-op).
func New(webhooks []string) *Notifier {
	return &Notifier{webhooks: webhooks, client: &http.Client{Timeout: 10 * time.Second}}
}

// Enabled reports whether any channel is configured.
func (n *Notifier) Enabled() bool { return n != nil && len(n.webhooks) > 0 }

// Notify sends one event to every configured webhook (best-effort, non-blocking
// per webhook). Slack and Teams both accept the {"text": ...} shape.
func (n *Notifier) Notify(e Event) {
	if !n.Enabled() {
		return
	}
	icon := map[string]string{"warning": "🟠", "critical": "🔴", "stale": "⚪"}[e.NewStatus]
	msg := fmt.Sprintf("%s Lookout: %s is %s", icon, e.Server, e.NewStatus)
	if e.Reason != "" {
		msg += " — " + e.Reason
	}
	body, _ := json.Marshal(map[string]string{"text": msg})
	for _, url := range n.webhooks {
		go n.post(url, body)
	}
}

func (n *Notifier) post(url string, body []byte) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if resp, err := n.client.Do(req); err == nil {
		_ = resp.Body.Close()
	}
}
