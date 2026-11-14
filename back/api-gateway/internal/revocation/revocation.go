// Package revocation tells whether auth revoked the session of a token.
// auth writes "auth:revoked:sid:<sessionId>" to Redis (TTL >= access token
// lifetime) on logout, logout-all, session revocation, refresh token reuse and
// account deactivation.
package revocation

import (
	"context"

	"github.com/redis/go-redis/v9"
)

const KeyPrefix = "auth:revoked:sid:"

type Redis struct{ client *redis.Client }

func NewRedis(client *redis.Client) *Redis { return &Redis{client: client} }

func (r *Redis) Revoked(ctx context.Context, sessionID string) (bool, error) {
	n, err := r.client.Exists(ctx, KeyPrefix+sessionID).Result()
	return n > 0, err
}
