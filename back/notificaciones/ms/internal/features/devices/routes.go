package devices

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Route("/me/devices", func(r chi.Router) {
		r.Post("/", h.Register)
		r.Get("/", h.List)
		r.Post("/unregister", h.UnregisterToken)
		r.Delete("/{id}", h.Unregister)
	})
}
