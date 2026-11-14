// Package ratelimit implements fixed-window counters. With Redis the limit is
// shared by every gateway replica; without it each replica counts on its own
// (fine for a single instance / local development).
package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type Limiter interface {
	// Allow counts one hit for key in the current window. When the limit is
	// exceeded it returns false and how long until the window resets.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, retryAfter time.Duration, err error)
}

func windowOf(now time.Time, window time.Duration) (start int64, resetIn time.Duration) {
	w := int64(window)
	s := now.UnixNano() / w
	return s, time.Duration((s+1)*w - now.UnixNano())
}

// Redis keeps counters under ratelimit:<key>:<window>.
type Redis struct {
	client *redis.Client
	now    func() time.Time
}

func NewRedis(client *redis.Client) *Redis { return &Redis{client: client, now: time.Now} }

func (l *Redis) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	start, resetIn := windowOf(l.now(), window)
	k := fmt.Sprintf("ratelimit:%s:%d", key, start)
	pipe := l.client.TxPipeline()
	incr := pipe.Incr(ctx, k)
	pipe.ExpireNX(ctx, k, window+time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return true, 0, err
	}
	if incr.Val() > int64(limit) {
		return false, resetIn, nil
	}
	return true, 0, nil
}

// Memory is a per-process limiter.
type Memory struct {
	mu       sync.Mutex
	counters map[string]counter
	now      func() time.Time
}

type counter struct {
	window int64
	hits   int
}

func NewMemory() *Memory { return &Memory{counters: map[string]counter{}, now: time.Now} }

func (l *Memory) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	start, resetIn := windowOf(l.now(), window)
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.counters) > 100_000 {
		for k, c := range l.counters {
			if c.window != start {
				delete(l.counters, k)
			}
		}
	}
	c := l.counters[key]
	if c.window != start {
		c = counter{window: start}
	}
	c.hits++
	l.counters[key] = c
	if c.hits > limit {
		return false, resetIn, nil
	}
	return true, 0, nil
}
