package gatewayauth

import (
	"context"
	"crypto/subtle"
	"net/http"

	"github.com/google/uuid"

	"superapp/notificaciones/internal/platform/apperr"
)

const (
	HeaderSecret    = "X-Gateway-Secret"
	HeaderUserID    = "X-User-ID"
	HeaderComplexID = "X-Complex-ID"
)

type ctxKey int

const identityKey ctxKey = iota

type Identity struct {
	UserID    uuid.UUID
	ComplexID *uuid.UUID
}

func Middleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if secret != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get(HeaderSecret)), []byte(secret)) != 1 {
				apperr.RespondError(w, nil, apperr.Auth("Request did not come through the gateway"))
				return
			}

			userID, err := uuid.Parse(r.Header.Get(HeaderUserID))
			if err != nil {
				apperr.RespondError(w, nil, apperr.Auth("Authentication required"))
				return
			}
			id := Identity{UserID: userID}
			if raw := r.Header.Get(HeaderComplexID); raw != "" {
				complexID, err := uuid.Parse(raw)
				if err != nil {
					apperr.RespondError(w, nil, apperr.Field(HeaderComplexID, "Invalid UUID format"))
					return
				}
				id.ComplexID = &complexID
			}

			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
		})
	}
}

func RequireComplex(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, ok := FromContext(r.Context()); !ok || id.ComplexID == nil {
			apperr.RespondError(w, nil, apperr.Field(HeaderComplexID, "Complex ID is required"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey).(Identity)
	return id, ok
}

func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}
