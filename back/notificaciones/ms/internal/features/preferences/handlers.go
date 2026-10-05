package preferences

import (
	"log/slog"
	"net/http"

	"superapp/notificaciones/internal/platform/apperr"
	"superapp/notificaciones/internal/platform/gatewayauth"
)

type Handler struct {
	service Service
	logger  *slog.Logger
}

func NewHandler(service Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

func (h *Handler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	id, _ := gatewayauth.FromContext(r.Context())
	resp, err := h.service.GetPreferences(r.Context(), id.UserID)
	if err != nil {
		apperr.RespondError(w, h.logger, err)
		return
	}
	apperr.RespondOK(w, resp)
}

func (h *Handler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	var req UpdatePreferencesRequest
	if err := apperr.DecodeJSON(r, &req); err != nil {
		apperr.RespondError(w, h.logger, err)
		return
	}
	id, _ := gatewayauth.FromContext(r.Context())

	if err := h.service.UpdatePreferences(r.Context(), id.UserID, req); err != nil {
		apperr.RespondError(w, h.logger, err)
		return
	}
	h.GetPreferences(w, r)
}
