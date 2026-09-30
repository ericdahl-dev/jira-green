// Package alert POSTs stuck-ticket events to user-configured webhooks. The
// transport and signing are ported from coolify-green, so one receiver can
// serve both tools.
package alert

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/ericdahl-dev/jira-green/internal/config"
)

// SignatureHeader carries the HMAC-SHA256 signature of the request body when
// the webhook is configured with a secret. The name is coolify-green's on
// purpose: a receiver verifies both tools the same way.
const SignatureHeader = "X-Coolify-Green-Signature"

// TypeTicketStuck is Event.Type for a card that stayed red too long.
const TypeTicketStuck = "ticket.stuck"

// Event is the JSON payload POSTed to each webhook.
type Event struct {
	Type    string    `json:"type"`
	Key     string    `json:"key"`
	Summary string    `json:"summary"`
	Status  string    `json:"status"`
	URL     string    `json:"url"`
	Reasons []string  `json:"reasons"`
	RedFor  string    `json:"red_for"`
	At      time.Time `json:"at"`
	// Incomplete and DecodeErrors mirror the card: some fields could not be
	// decoded, so the reasons may be missing a cause.
	Incomplete   bool     `json:"incomplete,omitempty"`
	DecodeErrors []string `json:"decode_errors,omitempty"`
}

// Dispatcher sends webhook events to configured endpoints.
type Dispatcher struct {
	hooks  []config.Webhook
	client *http.Client
}

// New creates a Dispatcher for the given webhook configs.
func New(hooks []config.Webhook) *Dispatcher {
	return &Dispatcher{
		hooks:  hooks,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Dispatch POSTs evt to all configured webhooks, giving up when ctx is done.
// Failures are logged at debug level but never returned: a dead endpoint
// must not interrupt polling.
func (d *Dispatcher) Dispatch(ctx context.Context, evt Event) {
	if len(d.hooks) == 0 {
		return
	}
	body, err := json.Marshal(evt)
	if err != nil {
		slog.Debug("webhook marshal failed", "err", err)
		return
	}
	for _, wh := range d.hooks {
		if err := d.post(ctx, wh, body); err != nil {
			slog.Debug("webhook POST failed", "url", wh.URL, "err", err)
		}
	}
}

func (d *Dispatcher) post(ctx context.Context, wh config.Webhook, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wh.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if wh.Secret != "" {
		mac := hmac.New(sha256.New, []byte(wh.Secret))
		mac.Write(body)
		req.Header.Set(SignatureHeader, "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		// Drain so the keep-alive connection is reused.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}
