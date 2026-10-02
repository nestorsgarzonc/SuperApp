package preferences

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Get("/me/notification-preferences", h.GetPreferences)
	r.Put("/me/notification-preferences", h.UpdatePreferences)
}
