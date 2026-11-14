// Package membership asks control-de-accesos which role a user holds in a
// complex (GET /internal/users/{userId}/complexes/{complexId}/role) and
// caches the answer, so X-Complex-Role is always the real role.
package membership

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	headerInternalSecret = "X-Internal-Secret"
	lookupTimeout        = 3 * time.Second
	maxEntries           = 100_000
)

// ErrUnavailable means the role could not be determined; callers must fail
// closed (503) and never forward without a role.
var ErrUnavailable = errors.New("membership lookup unavailable")

type Resolver struct {
	base   *url.URL
	secret string
	client *http.Client
	ttl    time.Duration
	// Negative answers are cached shorter so a newly added member is not
	// locked out for a whole TTL.
	negTTL time.Duration

	mu      sync.Mutex
	entries map[string]entry
	group   singleflight.Group
	now     func() time.Time
}

type entry struct {
	role    string
	member  bool
	expires time.Time
}

func NewResolver(base *url.URL, internalSecret string, ttl time.Duration, client *http.Client) *Resolver {
	if client == nil {
		client = &http.Client{Timeout: lookupTimeout}
	}
	neg := ttl / 6
	if neg < time.Second {
		neg = time.Second
	}
	return &Resolver{base: base, secret: internalSecret, client: client, ttl: ttl, negTTL: neg, entries: map[string]entry{}, now: time.Now}
}

// Role returns the user's role in the complex; member=false when the user does
// not belong to it. IDs must already be canonical UUIDs.
func (r *Resolver) Role(ctx context.Context, userID, complexID string) (role string, member bool, err error) {
	key := userID + "|" + complexID
	r.mu.Lock()
	e, ok := r.entries[key]
	r.mu.Unlock()
	if ok && r.now().Before(e.expires) {
		return e.role, e.member, nil
	}

	// Concurrent misses for the same pair share one lookup. The lookup is
	// detached from any single caller so one cancelled request does not fail
	// the others waiting on it.
	ch := r.group.DoChan(key, func() (any, error) {
		lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), lookupTimeout)
		defer cancel()
		res, err := r.fetch(lctx, userID, complexID)
		if err != nil {
			return nil, err
		}
		ttl := r.ttl
		if !res.member {
			ttl = r.negTTL
		}
		res.expires = r.now().Add(ttl)
		r.store(key, res)
		return res, nil
	})
	select {
	case <-ctx.Done():
		return "", false, ctx.Err()
	case out := <-ch:
		if out.Err != nil {
			return "", false, out.Err
		}
		res := out.Val.(entry)
		return res.role, res.member, nil
	}
}

func (r *Resolver) store(key string, e entry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.entries) >= maxEntries {
		now := r.now()
		for k, v := range r.entries {
			if !now.Before(v.expires) {
				delete(r.entries, k)
			}
		}
		if len(r.entries) >= maxEntries { // still full: start over rather than grow unbounded
			r.entries = map[string]entry{}
		}
	}
	r.entries[key] = e
}

func (r *Resolver) fetch(ctx context.Context, userID, complexID string) (entry, error) {
	u := r.base.JoinPath("internal", "users", userID, "complexes", complexID, "role")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return entry{}, err
	}
	req.Header.Set(headerInternalSecret, r.secret)
	resp, err := r.client.Do(req)
	if err != nil {
		return entry{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var body struct {
			Data struct {
				Role string `json:"role"`
			} `json:"data"`
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body); err != nil {
			return entry{}, fmt.Errorf("%w: decode: %v", ErrUnavailable, err)
		}
		role := strings.ToUpper(strings.TrimSpace(body.Data.Role))
		if role == "" {
			return entry{}, fmt.Errorf("%w: empty role", ErrUnavailable)
		}
		return entry{role: role, member: true}, nil
	case http.StatusNotFound:
		return entry{member: false}, nil
	default:
		// 401 here means INTERNAL_SECRET does not match: a misconfiguration,
		// which must look like an outage (503), never like "not a member".
		return entry{}, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}
}
