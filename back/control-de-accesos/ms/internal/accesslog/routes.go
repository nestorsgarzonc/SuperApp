package accesslog

import (
	authmw "controldeaccesos/internal/platform/middleware"

	"github.com/go-chi/chi/v5"
)

// RegisterRoutes mounts /access-logs. Registering and reading the gate log
// are guard/admin operations; the complex always comes from the token.
func RegisterRoutes(r chi.Router, h *Handler, checker authmw.AccessChecker) {
	r.Route("/access-logs", func(r chi.Router) {
		authmw.ApplyEnvironmentAuth(r)
		r.Use(authmw.RequireComplexAccessMiddleware(checker))
		r.Use(authmw.RequireStaffMiddleware(checker, RoleGuard, RoleAdmin))
		r.Post("/", h.Create)
		r.Get("/", h.List)
	})
}

// RegisterResidentRoutes mounts /resident-movements: residents mark that they
// enter/leave the building and see their own history; guards/admins see who is
// currently inside. The complex always comes from the token.
func RegisterResidentRoutes(r chi.Router, h *Handler, checker authmw.AccessChecker) {
	r.Route("/resident-movements", func(r chi.Router) {
		authmw.ApplyEnvironmentAuth(r)
		r.Use(authmw.RequireComplexAccessMiddleware(checker))
		resident := authmw.RequireRoleMiddleware(RoleResident)
		r.With(resident).Post("/", h.ResidentMovement)
		r.With(resident).Get("/me", h.ResidentMe)
		r.With(authmw.RequireStaffMiddleware(checker, RoleGuard, RoleAdmin)).Get("/presence", h.Presence)
	})
}
