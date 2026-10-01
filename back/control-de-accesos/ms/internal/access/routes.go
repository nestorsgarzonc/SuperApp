package access

import (
	authmw "controldeaccesos/internal/platform/middleware"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes registers all access control routes.
func RegisterRoutes(r chi.Router, h *Handler, checker authmw.ComplexAccessChecker) {
	r.Route("/access", func(r chi.Router) {
		authmw.ApplyEnvironmentAuth(r)
		r.Use(authmw.RequireComplexAccessMiddleware(checker))

		r.Route("/qr-codes", func(r chi.Router) {
			r.Post("/", h.CreateQRCode)
			r.Post("/batch", h.CreateQRBatch)
			r.Get("/", h.GetQRCodes)
		})

		r.Route("/pre-auth", func(r chi.Router) {
			r.Post("/", h.CreatePreAuth)
			r.Get("/", h.GetPreAuthorizations)
		})
	})

	r.Route("/access-grants", func(r chi.Router) {
		authmw.ApplyEnvironmentAuth(r)
		r.Use(authmw.RequireComplexAccessMiddleware(checker))
		r.Post("/", h.CreateAccessGrant)
		r.Get("/{id}", h.GetAccessGrant)
		r.Patch("/{id}/cancel", h.CancelAccessGrant)
		r.Post("/validate", h.ValidateAccessGrant)
	})
}

// RegisterUnitRoutes registers unit-scoped access endpoints under /units.
func RegisterUnitRoutes(r chi.Router, h *Handler, checker authmw.UnitAccessChecker) {
	r.Group(func(r chi.Router) {
		authmw.ApplyEnvironmentAuth(r)
		r.With(authmw.RequireUnitAccessMiddleware(checker)).Get("/{unitId}/access-grants", h.GetUnitAccessGrants)
	})
}
