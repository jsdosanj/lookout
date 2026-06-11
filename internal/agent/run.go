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

	"github.com/jsdosanj/servmonitor/internal/collect"
)

// Config controls how the agent reports.
type Config struct {
	ServerURL string
	Token     string
	Interval  time.Duration
	Once      bool
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
