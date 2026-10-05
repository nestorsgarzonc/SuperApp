package notifications

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

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

func identity(r *http.Request) (userID, complexID uuid.UUID) {
	id, _ := gatewayauth.FromContext(r.Context())
	return id.UserID, *id.ComplexID
}

func (h *Handler) GetUserNotifications(w http.ResponseWriter, r *http.Request) {
	userID, complexID := identity(r)
	query := r.URL.Query()
	q := InboxQuery{UnreadOnly: query.Get("unread") == "true"}
	q.Limit, _ = strconv.Atoi(query.Get("limit"))
	if raw := query.Get("group"); raw != "" {
		q.Groups = strings.Split(raw, ",")
	}

	out, err := h.service.GetUserNotifications(r.Context(), complexID, userID, q)
	if err != nil {
		apperr.RespondError(w, h.logger, err)
		return
	}
	apperr.RespondOK(w, out)
}

func (h *Handler) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	userID, complexID := identity(r)
	count, err := h.service.CountUnread(r.Context(), complexID, userID)
	if err != nil {
		apperr.RespondError(w, h.logger, err)
		return
	}
	apperr.RespondOK(w, map[string]int{"count": count})
}

func (h *Handler) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apperr.RespondError(w, h.logger, apperr.Field("id", "Invalid UUID format"))
		return
	}
	userID, complexID := identity(r)
	if err := h.service.MarkAsRead(r.Context(), id, complexID, userID); err != nil {
		apperr.RespondError(w, h.logger, err)
		return
	}
	apperr.RespondNoContent(w)
}

func (h *Handler) MarkAllAsRead(w http.ResponseWriter, r *http.Request) {
	userID, complexID := identity(r)
	count, err := h.service.MarkAllAsRead(r.Context(), complexID, userID)
	if err != nil {
		apperr.RespondError(w, h.logger, err)
		return
	}
	apperr.RespondOK(w, map[string]int64{"marked_count": count})
}
