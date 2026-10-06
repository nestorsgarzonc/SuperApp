// Package membership exposes who belongs to which complex and with what role.
// This service owns that data, so it answers two consumers:
//
//	GET /api/v1/me/memberships                                  the front, to build the session after login
//	GET /internal/users/{userId}/complexes/{complexId}/role     the api-gateway, to inject X-Complex-Role
package membership

import (
	"context"
	"crypto/subtle"
	"net/http"

	"controldeaccesos/internal/directory"
	"controldeaccesos/internal/platform/errors"
	authmw "controldeaccesos/internal/platform/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// HeaderInternalSecret authenticates service-to-service calls (docs/internal-apis.md).
const HeaderInternalSecret = "X-Internal-Secret"

type Directory interface {
	ListMemberships(ctx context.Context, userID uuid.UUID) ([]directory.Membership, error)
	GetUserComplexRole(ctx context.Context, userID, complexID uuid.UUID) (role string, found bool, err error)
	GetUserDisplayName(ctx context.Context, userID uuid.UUID) (string, error)
}

type Handler struct{ dir Directory }

func NewHandler(dir Directory) *Handler { return &Handler{dir: dir} }

type meResponse struct {
	UserID      uuid.UUID              `json:"user_id"`
	Name        string                 `json:"name"`
	Memberships []directory.Membership `json:"memberships"`
}

// Me handles GET /api/v1/me/memberships for the authenticated user.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID, err := authmw.GetUserIDAsUUID(r.Context())
	if err != nil {
		errors.RespondError(w, nil, errors.NewAuthError("authentication required"))
		return
	}
	memberships, err := h.dir.ListMemberships(r.Context(), userID)
	if err != nil {
		errors.RespondError(w, nil, errors.NewInternalError(err))
		return
	}
	// The name only decorates the session: a user unknown to the directory
	// still gets their (possibly empty) memberships.
	name, _ := h.dir.GetUserDisplayName(r.Context(), userID)
	errors.RespondOK(w, meResponse{UserID: userID, Name: name, Memberships: memberships})
}

// Role handles GET /internal/users/{userId}/complexes/{complexId}/role.
func (h *Handler) Role(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "userId"))
	if err != nil {
		errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{"userId": {"must be a UUID"}}))
		return
	}
	complexID, err := uuid.Parse(chi.URLParam(r, "complexId"))
	if err != nil {
		errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{"complexId": {"must be a UUID"}}))
		return
	}
	role, found, err := h.dir.GetUserComplexRole(r.Context(), userID, complexID)
	if err != nil {
		errors.RespondError(w, nil, errors.NewInternalError(err))
		return
	}
	if !found {
		errors.RespondError(w, nil, errors.NewNotFoundError("Membership", userID.String()))
		return
	}
	errors.RespondOK(w, map[string]string{"role": role})
}

// RegisterRoutes mounts /me/memberships under the authenticated API.
func RegisterRoutes(r chi.Router, h *Handler) {
	r.Route("/me", func(r chi.Router) {
		authmw.ApplyEnvironmentAuth(r)
		r.Get("/memberships", h.Me)
	})
}

// RegisterInternalRoutes mounts /internal/*, reachable only with the shared
// X-Internal-Secret. With no secret configured nothing is mounted: the
// endpoints fail closed instead of being open.
func RegisterInternalRoutes(r chi.Router, h *Handler, secret string) {
	if secret == "" {
		return
	}
	r.Route("/internal", func(r chi.Router) {
		r.Use(requireInternalSecret(secret))
		r.Get("/users/{userId}/complexes/{complexId}/role", h.Role)
	})
}

func requireInternalSecret(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get(HeaderInternalSecret)), []byte(secret)) != 1 {
				errors.RespondError(w, nil, errors.NewAuthError("internal secret required"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
