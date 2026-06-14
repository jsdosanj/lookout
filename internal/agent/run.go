// Package agent ships collected reports to the Lookout control plane.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jsdosanj/lookout/internal/collect"
)

// Config controls how the agent reports.
type Config struct {
	ServerURL string
	Token     string // per-agent token if enrolled, else the shared enrollment token
	Interval  time.Duration
	Once      bool
}

// enrollResponse is the control plane's reply to POST /api/v1/agents/enroll.
type enrollResponse struct {
	AgentID    string `json:"agent_id"`
	AgentToken string `json:"agent_token"`
}

// Enroll exchanges the shared enrollment token for a per-agent token bound to a
// server-assigned identity. The caller persists the returned token and reports
// with it thereafter. serverURL and sharedToken are required.
func Enroll(ctx context.Context, serverURL, sharedToken, hostname string) (agentID, agentToken string, err error) {
	if serverURL == "" {
		return "", "", fmt.Errorf("--server is required to enroll")
	}
	body, err := json.Marshal(map[string]string{"hostname": hostname})
	if err != nil {
		return "", "", err
	}
	url := strings.TrimRight(serverURL, "/") + "/api/v1/agents/enroll"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if sharedToken != "" {
		req.Header.Set("Authorization", "Bearer "+sharedToken)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("enroll: control plane returned %s", resp.Status)
	}
	var er enrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		return "", "", fmt.Errorf("enroll: decode response: %w", err)
	}
	return er.AgentID, er.AgentToken, nil
}

// Run reports once (Once) or on Interval until ctx is cancelled. Transient
// report errors are logged, not fatal — a flaky network shouldn't kill the agent.
func Run(ctx context.Context, cfg Config) error {
	if cfg.ServerURL == "" {
		return fmt.Errorf("--server is required")
	}
	if cfg.Once {
		return send(ctx, cfg)
	}
	if err := send(ctx, cfg); err != nil {
		fmt.Println("report error:", err)
	}
	t := time.NewTicker(cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if err := send(ctx, cfg); err != nil {
				fmt.Println("report error:", err)
			}
		}
	}
}

func send(ctx context.Context, cfg Config) error {
	rep, err := collect.Collect()
	if err != nil {
		return err
	}
	body, err := json.Marshal(rep)
	if err != nil {
		return err
	}
	url := strings.TrimRight(cfg.ServerURL, "/") + "/api/v1/agents/report"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("control plane returned %s", resp.Status)
	}
	return nil
}
