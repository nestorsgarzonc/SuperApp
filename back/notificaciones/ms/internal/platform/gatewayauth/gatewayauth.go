package gatewayauth

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

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

// BearerAuth resuelve el usuario de un Bearer token cuando la petición NO
// viene del api-gateway (sin X-Gateway-Secret), p. ej. detrás de AWS API
// Gateway, que no inyecta identidad.
type BearerAuth interface {
	Subject(ctx context.Context, token string) (uuid.UUID, error)
}

// Middleware carga la identidad. Con X-Gateway-Secret se confían los headers
// que inyecta el gateway propio. Sin él, el Bearer se valida con bearer
// (firma, emisor, expiración); con bearer nil se rechaza. Con secret vacío
// (desarrollo) también se acepta X-User-ID suelto.
func Middleware(secret string, bearer BearerAuth) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, err := identify(r, secret, bearer)
			if err != nil {
				apperr.RespondError(w, nil, err)
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

func identify(r *http.Request, secret string, bearer BearerAuth) (uuid.UUID, error) {
	provided := r.Header.Get(HeaderSecret)
	token, hasBearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	switch {
	case provided != "":
		if secret != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(secret)) != 1 {
			return uuid.Nil, apperr.Auth("Request did not come through the gateway")
		}
		return headerUser(r)
	case hasBearer:
		if bearer == nil {
			return uuid.Nil, apperr.Auth("Token verification is not configured")
		}
		userID, err := bearer.Subject(r.Context(), strings.TrimSpace(token))
		if err != nil {
			return uuid.Nil, apperr.Auth("Invalid token")
		}
		return userID, nil
	case secret == "":
		return headerUser(r)
	default:
		return uuid.Nil, apperr.Auth("Authentication required")
	}
}

// Unverified lee `sub` (tokens de auth) o `user_id` (HS256 de pruebas) del
// payload SIN validar firma ni expiración: cualquiera que forje un token se
// hace pasar por otro usuario. Solo con ALLOW_UNVERIFIED_BEARER (desarrollo,
// o despliegues heredados mientras se configura AUTH_JWKS_URL/JWT_SECRET).
type Unverified struct{}

func (Unverified) Subject(_ context.Context, token string) (uuid.UUID, error) {
	userID, ok := tokenSubject(token)
	if !ok {
		return uuid.Nil, apperr.Auth("Invalid token")
	}
	return userID, nil
}

func headerUser(r *http.Request) (uuid.UUID, error) {
	userID, err := uuid.Parse(r.Header.Get(HeaderUserID))
	if err != nil {
		return uuid.Nil, apperr.Auth("Authentication required")
	}
	return userID, nil
}

// tokenSubject lee el sujeto del payload del JWT. No valida nada.
func tokenSubject(token string) (uuid.UUID, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return uuid.Nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return uuid.Nil, false
	}
	var c struct {
		Sub    string `json:"sub"`
		UserID string `json:"user_id"`
	}
	if json.Unmarshal(raw, &c) != nil {
		return uuid.Nil, false
	}
	subject := c.Sub
	if subject == "" {
		subject = c.UserID
	}
	userID, err := uuid.Parse(subject)
	return userID, err == nil
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
