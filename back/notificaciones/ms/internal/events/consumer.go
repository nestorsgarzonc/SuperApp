package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"superapp/notificaciones/internal/features/notifications"
	"superapp/notificaciones/internal/platform/apperr"
)

type NotificationRequested = notifications.DispatchInput

const (
	payloadField     = "payload"
	defaultClaimIdle = time.Minute
	blockFor         = 5 * time.Second
	batchSize        = 20

	maxDeliveries = 5
)

type Dispatcher interface {
	Dispatch(ctx context.Context, in notifications.DispatchInput) error
}

type Consumer struct {
	rdb      *redis.Client
	stream   string
	group    string
	consumer string
	svc      Dispatcher
	logger   *slog.Logger

	ClaimIdle time.Duration
}

func NewConsumer(rdb *redis.Client, stream, group, consumer string, svc Dispatcher, logger *slog.Logger) *Consumer {
	return &Consumer{rdb: rdb, stream: stream, group: group, consumer: consumer, svc: svc, logger: logger, ClaimIdle: defaultClaimIdle}
}

func (c *Consumer) Run(ctx context.Context) error {
	err := c.rdb.XGroupCreateMkStream(ctx, c.stream, c.group, "$").Err()
	if err != nil && !redis.HasErrorPrefix(err, "BUSYGROUP") {
		return fmt.Errorf("failed to create consumer group: %w", err)
	}
	c.logger.Info("event consumer started", "stream", c.stream, "group", c.group, "consumer", c.consumer)

	lastClaim := time.Time{}
	for ctx.Err() == nil {
		if time.Since(lastClaim) >= c.ClaimIdle {
			c.reclaim(ctx)
			lastClaim = time.Now()
		}

		streams, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.group,
			Consumer: c.consumer,
			Streams:  []string{c.stream, ">"},
			Count:    batchSize,
			Block:    min(blockFor, c.ClaimIdle),
		}).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			c.logger.Error("XREADGROUP failed", "error", err)
			time.Sleep(time.Second)
			continue
		}
		for _, s := range streams {
			for _, msg := range s.Messages {
				c.handle(ctx, msg)
			}
		}
	}
	return nil
}

func (c *Consumer) reclaim(ctx context.Context) {
	start := "-"
	for {
		pending, err := c.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
			Stream: c.stream, Group: c.group, Idle: c.ClaimIdle,
			Start: start, End: "+", Count: batchSize,
		}).Result()
		if err != nil {
			if ctx.Err() == nil {
				c.logger.Error("XPENDING failed", "error", err)
			}
			return
		}
		for _, p := range pending {

			msgs, err := c.rdb.XClaim(ctx, &redis.XClaimArgs{
				Stream: c.stream, Group: c.group, Consumer: c.consumer,
				MinIdle: c.ClaimIdle, Messages: []string{p.ID},
			}).Result()
			if err != nil || len(msgs) == 0 {
				continue
			}
			if p.RetryCount >= maxDeliveries {
				c.deadLetter(ctx, msgs[0], fmt.Errorf("gave up after %d deliveries", p.RetryCount))
				continue
			}
			c.handle(ctx, msgs[0])
		}
		if len(pending) < batchSize {
			return
		}
		start = "(" + pending[len(pending)-1].ID
	}
}

func (c *Consumer) handle(ctx context.Context, msg redis.XMessage) {
	err := c.process(ctx, msg)
	switch {
	case err == nil:
		c.ack(ctx, msg.ID)
	case apperr.IsValidation(err):

		c.deadLetter(ctx, msg, err)
	default:

		c.logger.Error("failed to process notification event, will retry", "id", msg.ID, "error", err)
	}
}

func (c *Consumer) deadLetter(ctx context.Context, msg redis.XMessage, reason error) {
	c.logger.Warn("moving notification event to DLQ", "id", msg.ID, "error", reason)
	if err := c.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: c.stream + ".dlq",
		Values: map[string]interface{}{payloadField: msg.Values[payloadField], "error": reason.Error(), "original_id": msg.ID},
	}).Err(); err != nil {
		c.logger.Error("failed to write DLQ, leaving pending", "id", msg.ID, "error", err)
		return
	}
	c.ack(ctx, msg.ID)
}

func (c *Consumer) process(ctx context.Context, msg redis.XMessage) error {
	raw, ok := msg.Values[payloadField].(string)
	if !ok {
		return apperr.Field(payloadField, "missing payload field")
	}
	var ev NotificationRequested
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		return apperr.Field(payloadField, "invalid JSON: "+err.Error())
	}
	return c.svc.Dispatch(ctx, ev)
}

func (c *Consumer) ack(ctx context.Context, id string) {
	if err := c.rdb.XAck(ctx, c.stream, c.group, id).Err(); err != nil {
		c.logger.Error("XACK failed", "id", id, "error", err)
	}
}
