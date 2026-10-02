package accesslog

import (
	stderrors "errors"
	"net/http"
	"strconv"
	"strings"

	"controldeaccesos/internal/audit"
	"controldeaccesos/internal/platform/errors"
	"controldeaccesos/internal/platform/middleware"

	"github.com/google/uuid"
)

func respondResidentError(w http.ResponseWriter, err error) {
	switch {
	case stderrors.Is(err, ErrAlreadyInside), stderrors.Is(err, ErrAlreadyOutside), stderrors.Is(err, ErrClientIDTaken):
		errors.RespondError(w, nil, errors.NewConflictError(err.Error()))
	case stderrors.Is(err, ErrNotResident):
		errors.RespondError(w, nil, errors.NewPermissionError(RoleResident))
	default:
		errors.RespondError(w, nil, errors.NewInternalError(err))
	}
}

// ResidentMovement handles POST /api/v1/resident-movements (RESIDENT).
// 201 when new, 200 when the same client_id was already recorded,
// 409 when it contradicts the current state (already inside / outside).
func (h *Handler) ResidentMovement(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	complexID, err := middleware.GetComplexIDAsUUID(ctx)
	if err != nil {
		errors.RespondError(w, nil, missingComplex())
		return
	}
	userID, err := middleware.GetUserIDAsUUID(ctx)
	if err != nil {
		errors.RespondError(w, nil, errors.NewAuthError("Authentication required"))
		return
	}

	var req ResidentMovementRequest
	if err := errors.DecodeJSONLenient(w, r, &req); err != nil {
		errors.RespondError(w, nil, err)
		return
	}
	if fieldErrs := req.validate(); len(fieldErrs) > 0 {
		errors.RespondError(w, nil, errors.NewValidationError(fieldErrs))
		return
	}

	entry, created, err := h.repo.CreateResidentMovement(ctx, complexID, userID, req)
	if err != nil {
		if stderrors.Is(err, ErrAlreadyInside) || stderrors.Is(err, ErrAlreadyOutside) {
			h.auditResident(r, userID, complexID, nil, "resident_movement."+strings.ToLower(req.Movement), audit.ResultDenied, err.Error())
		}
		respondResidentError(w, err)
		return
	}
	if !created {
		errors.RespondOK(w, entry)
		return
	}

	h.auditResident(r, userID, complexID, entry, "resident_movement."+strings.ToLower(entry.Movement), audit.ResultAllowed, "")
	errors.RespondCreated(w, entry)
}

// ResidentMe handles GET /api/v1/resident-movements/me?limit= (RESIDENT):
// the caller's own current state and recent movements — never anyone else's.
func (h *Handler) ResidentMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	complexID, err := middleware.GetComplexIDAsUUID(ctx)
	if err != nil {
		errors.RespondError(w, nil, missingComplex())
		return
	}
	userID, err := middleware.GetUserIDAsUUID(ctx)
	if err != nil {
		errors.RespondError(w, nil, errors.NewAuthError("Authentication required"))
		return
	}

	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{"limit": {"must be between 1 and 100"}}))
			return
		}
		limit = n
	}

	movements, err := h.repo.ResidentHistory(ctx, complexID, userID, limit)
	if err != nil {
		errors.RespondError(w, nil, errors.NewInternalError(err))
		return
	}
	status := ResidentStatus{Status: StatusUnknown, Movements: movements}
	if len(movements) > 0 {
		status.Status = statusFrom(movements[0].Movement)
		since := movements[0].OccurredAt
		status.Since = &since
	}
	errors.RespondOK(w, status)
}

// Presence handles GET /api/v1/resident-movements/presence (GUARD/ADMIN):
// residents currently inside the building of the token's complex.
func (h *Handler) Presence(w http.ResponseWriter, r *http.Request) {
	complexID, err := middleware.GetComplexIDAsUUID(r.Context())
	if err != nil {
		errors.RespondError(w, nil, missingComplex())
		return
	}
	inside, err := h.repo.Presence(r.Context(), complexID)
	if err != nil {
		errors.RespondError(w, nil, errors.NewInternalError(err))
		return
	}
	errors.RespondOK(w, inside)
}

func (h *Handler) auditResident(r *http.Request, actor, complexID uuid.UUID, l *AccessLog, action, result, reason string) {
	if h.auditor == nil {
		return
	}
	e := audit.Entry{
		ActorID:       &actor,
		ComplexID:     &complexID,
		ResourceType:  "access_log",
		Action:        action,
		Result:        result,
		Reason:        reason,
		IPAddress:     middleware.GetRequestIP(r.Context()),
		CorrelationID: middleware.GetCorrelationID(r.Context()),
	}
	if l != nil {
		e.ResourceID = &l.ID
	}
	if err := h.auditor.Write(r.Context(), e); err != nil && h.logger != nil {
		h.logger.Error("audit write failed", "error", err, "action", action)
	}
}
