// Package notifier sends notification requests to the notificaciones service
// through POST /internal/notifications (contract: docs/events.md), best-effort:
// if the URL is not configured or the request keeps failing, the event is
// dropped (logged) and the caller is never affected.
package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// Source identifies this service in the `source` field of every event.
	Source = "control-de-accesos"

	path         = "/internal/notifications"
	secretHeader = "X-Internal-Secret"

	requestTimeout = 3 * time.Second
	// Retries on network errors and 5xx; safe because notificaciones is
	// idempotent on event_id.
	maxAttempts = 3
	retryDelay  = 500 * time.Millisecond
)

// Categories understood by notificaciones (see docs/events.md#categorías).
const (
	CategoryAccess  = "ACCESS"
	CategoryVisitor = "VISITOR"
)

// Event is the notification request sent to notificaciones. Its JSON shape is
// the /internal/notifications body; EventID and Source are filled in by the
// client.
type Event struct {
	EventID   string         `json:"event_id"`
	Source    string         `json:"source"`
	ComplexID *uuid.UUID     `json:"complex_id"`
	UserIDs   []uuid.UUID    `json:"user_ids"`
	Category  string         `json:"category"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	Data      map[string]any `json:"data,omitempty"`
}

// Client sends events to notificaciones.
type Client struct {
	url      string
	secret   string
	http     *http.Client
	logger   *slog.Logger
	inflight sync.WaitGroup
}

// New returns a client for the notificaciones service at baseURL (e.g.
// http://10.0.1.20:8083). An empty URL disables sending (Notify is a no-op).
func New(baseURL, secret string, logger *slog.Logger) *Client {
	c := &Client{secret: secret, http: &http.Client{Timeout: requestTimeout}, logger: logger}
	if baseURL != "" {
		c.url = strings.TrimRight(baseURL, "/") + path
	}
	return c
}

// Close waits for in-flight requests (each bounded by its timeouts and retries).
func (c *Client) Close() error {
	c.inflight.Wait()
	return nil
}

// Notify sends the event in the background and never blocks the caller or
// surfaces an error. Events without recipients are skipped.
func (c *Client) Notify(_ context.Context, event Event) {
	if c.url == "" || len(event.UserIDs) == 0 {
		return
	}

	event.Source = Source
	if event.EventID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			c.logger.Warn("notifier: failed to generate event id", "category", event.Category, "error", err)
			return
		}
		event.EventID = id.String()
	}

	payload, err := json.Marshal(event)
	if err != nil {
		c.logger.Warn("notifier: failed to encode event", "category", event.Category, "error", err)
		return
	}

	c.inflight.Add(1)
	go func() {
		defer c.inflight.Done()
		for attempt := 1; ; attempt++ {
			retry, err := c.send(payload)
			if err == nil {
				return
			}
			if !retry || attempt == maxAttempts {
				c.logger.Warn("notifier: event delivery failed", "event_id", event.EventID, "category", event.Category, "attempts", attempt, "error", err)
				return
			}
			time.Sleep(retryDelay * time.Duration(attempt))
		}
	}()
}

// send makes one attempt; retry reports whether a later attempt may succeed.
func (c *Client) send(payload []byte) (retry bool, err error) {
	req, err := http.NewRequest(http.MethodPost, c.url, bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		req.Header.Set(secretHeader, c.secret)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return true, err
	}
	resp.Body.Close()
	switch {
	case resp.StatusCode < 300:
		return false, nil
	case resp.StatusCode >= 500:
		return true, fmt.Errorf("notificaciones responded %d", resp.StatusCode)
	default:
		return false, fmt.Errorf("notificaciones rejected the event: %d", resp.StatusCode)
	}
}
