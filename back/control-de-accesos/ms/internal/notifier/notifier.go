// Package notifier is the outbound edge of the "evento de acceso" arrow in
// SuperApp/docs/architecture.md: control-de-accesos fires access events at
// the notificaciones microservice. That service doesn't exist yet, so this
// client is best-effort by design — if NOTIFICATIONS_URL is unset or the
// request fails, the event is dropped (logged) and the access operation that
// triggered it is never affected.
package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Event types fired by the access feature.
const (
	EventQRCreated      = "access.qr_created"
	EventPreAuthCreated = "access.pre_auth_created"
	EventGrantCreated   = "access.grant_created"
	EventGrantValidated = "access.grant_validated"
	EventGrantCancelled = "access.grant_cancelled"
)

// Event is the payload POSTed to the notifications service.
type Event struct {
	Type        string    `json:"type"`
	ComplexID   uuid.UUID `json:"complex_id"`
	UnitID      uuid.UUID `json:"unit_id"`
	GrantID     uuid.UUID `json:"grant_id,omitempty"`
	VisitorName string    `json:"visitor_name,omitempty"`
	Method      string    `json:"method,omitempty"`
}

// Client posts access events to the notifications service.
type Client struct {
	url        string
	httpClient *http.Client
	logger     *slog.Logger
}

func New(url string, logger *slog.Logger) *Client {
	return &Client{
		url:        url,
		httpClient: &http.Client{Timeout: 3 * time.Second},
		logger:     logger,
	}
}

// Notify fires the event in the background and never blocks the caller or
// surfaces an error — notificaciones being absent or unreachable is expected
// until that microservice is built.
func (c *Client) Notify(ctx context.Context, event Event) {
	if c.url == "" {
		return
	}

	go func() {
		body, err := json.Marshal(event)
		if err != nil {
			c.logger.Warn("notifier: failed to encode event", "type", event.Type, "error", err)
			return
		}

		sendCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(sendCtx, http.MethodPost, c.url, bytes.NewReader(body))
		if err != nil {
			c.logger.Warn("notifier: failed to build request", "type", event.Type, "error", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			c.logger.Warn("notifier: event delivery failed", "type", event.Type, "error", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 300 {
			c.logger.Warn("notifier: event rejected", "type", event.Type, "status", resp.StatusCode)
		}
	}()
}
