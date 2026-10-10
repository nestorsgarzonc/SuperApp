package notifications

import (
	"github.com/go-chi/chi/v5"

	"superapp/notificaciones/internal/platform/gatewayauth"
)

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Group(func(r chi.Router) {
		r.Use(gatewayauth.RequireComplex)
		r.Get("/users/me/notifications", h.GetUserNotifications)
		r.Get("/users/me/notifications/unread-count", h.GetUnreadCount)
		r.Post("/notifications/mark-all-read", h.MarkAllAsRead)
		r.Patch("/notifications/{id}/read", h.MarkAsRead)
	})
}

// RegisterInternalRoutes monta las rutas que solo llaman otros servicios; el
// router que las recibe ya exige X-Internal-Secret.
func RegisterInternalRoutes(r chi.Router, h *Handler) {
	r.Post("/internal/notifications", h.Dispatch)
}
