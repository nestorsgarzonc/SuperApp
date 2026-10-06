// Package notifier publishes notification requests to the notificaciones
// service through the Redis stream `notifications.requested`
// (contract: docs/events.md), best-effort: if Redis is not configured or the
// publish fails, the event is dropped (logged) and the caller is never
// affected.
package notifier

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	// Source identifies this service in the `source` field of every event.
	Source = "control-de-accesos"

	// DefaultStream is the stream owned by notificaciones.
	DefaultStream = "notifications.requested"

	publishTimeout = 3 * time.Second
	// Bounds the stream size (approximate trimming).
	streamMaxLen = 100_000
)

// Categories understood by notificaciones (see docs/events.md#categorías).
const (
	CategoryAccess  = "ACCESS"
	CategoryVisitor = "VISITOR"
)

// Event is the notification request published to notificaciones. Its JSON
// shape is the `notifications.requested` payload; EventID and Source are filled
// in by the client.
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

// Client publishes events to the notifications stream.
type Client struct {
	rdb    *redis.Client
	stream string
	logger *slog.Logger
}

// New returns a client for the Redis at redisURL. An empty URL disables
// publishing (Notify is a no-op).
func New(redisURL, stream string, logger *slog.Logger) (*Client, error) {
	if stream == "" {
		stream = DefaultStream
	}
	c := &Client{stream: stream, logger: logger}
	if redisURL == "" {
		return c, nil
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	c.rdb = redis.NewClient(opts)
	return c, nil
}

// Close releases the Redis connection pool.
func (c *Client) Close() error {
	if c.rdb == nil {
		return nil
	}
	return c.rdb.Close()
}

// Notify publishes the event in the background and never blocks the caller or
// surfaces an error. Events without recipients are skipped.
func (c *Client) Notify(_ context.Context, event Event) {
	if c.rdb == nil || len(event.UserIDs) == 0 {
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

	go func() {
		payload, err := json.Marshal(event)
		if err != nil {
			c.logger.Warn("notifier: failed to encode event", "category", event.Category, "error", err)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
		defer cancel()

		err = c.rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: c.stream,
			MaxLen: streamMaxLen,
			Approx: true,
			Values: map[string]any{"payload": string(payload)},
		}).Err()
		if err != nil {
			c.logger.Warn("notifier: event delivery failed", "event_id", event.EventID, "category", event.Category, "error", err)
		}
	}()
}
