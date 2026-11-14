// Package revocation publishes revoked sessions to Redis so the api-gateway,
// which validates access tokens locally, rejects them before they expire.
//
// Contract (docs/api-gateway.md): key "auth:revoked:sid:<sessionId>", any
// value, TTL >= access token lifetime. Access tokens carry the session in
// the "sid" claim.
package revocation

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const KeyPrefix = "auth:revoked:sid:"

type Redis struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedis keeps each marker for accessTTL plus a margin for clock skew.
func NewRedis(client *redis.Client, accessTTL time.Duration) *Redis {
	return &Redis{client: client, ttl: accessTTL + time.Minute}
}

// SessionsRevoked never fails the caller: if Redis is unreachable the
// revocation is still effective for refresh tokens, and access tokens of
// those sessions simply live until they expire (at most the access TTL).
func (r *Redis) SessionsRevoked(ctx context.Context, sessionIDs []string) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	pipe := r.client.Pipeline()
	for _, id := range sessionIDs {
		pipe.Set(ctx, KeyPrefix+id, "1", r.ttl)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("revocation: could not publish %d revoked session(s): %v", len(sessionIDs), err)
	}
}
