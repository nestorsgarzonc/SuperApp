package devices

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

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

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterDeviceRequest
	if err := apperr.DecodeJSON(r, &req); err != nil {
		apperr.RespondError(w, h.logger, err)
		return
	}
	id, _ := gatewayauth.FromContext(r.Context())
	d, err := h.service.Register(r.Context(), id.UserID, req)
	if err != nil {
		apperr.RespondError(w, h.logger, err)
		return
	}
	apperr.RespondCreated(w, d)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	id, _ := gatewayauth.FromContext(r.Context())
	out, err := h.service.List(r.Context(), id.UserID)
	if err != nil {
		apperr.RespondError(w, h.logger, err)
		return
	}
	apperr.RespondOK(w, out)
}

func (h *Handler) Unregister(w http.ResponseWriter, r *http.Request) {
	deviceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apperr.RespondError(w, h.logger, apperr.Field("id", "Invalid UUID format"))
		return
	}
	id, _ := gatewayauth.FromContext(r.Context())
	if err := h.service.Unregister(r.Context(), deviceID, id.UserID); err != nil {
		apperr.RespondError(w, h.logger, err)
		return
	}
	apperr.RespondNoContent(w)
}

func (h *Handler) UnregisterToken(w http.ResponseWriter, r *http.Request) {
	var req UnregisterDeviceRequest
	if err := apperr.DecodeJSON(r, &req); err != nil {
		apperr.RespondError(w, h.logger, err)
		return
	}
	id, _ := gatewayauth.FromContext(r.Context())
	if err := h.service.UnregisterToken(r.Context(), req.PushToken, id.UserID); err != nil {
		apperr.RespondError(w, h.logger, err)
		return
	}
	apperr.RespondNoContent(w)
}
