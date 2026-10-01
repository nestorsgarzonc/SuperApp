package access

import (
	stderrors "errors"
	"net/http"
	"strings"

	"controldeaccesos/internal/platform/errors"
	"controldeaccesos/internal/platform/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler handles HTTP requests for access control.
type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

// resolveComplexID returns the complex id for a request. It prefers the
// explicit ?complex_id= query param (backward compatible) but falls back to
// the X-Complex-ID header that RequireComplexAccessMiddleware injects into the
// context — which is what every authenticated client actually sends. Without
// this fallback these list endpoints returned 422 even though the caller was
// already scoped to a complex.
func resolveComplexID(r *http.Request) (uuid.UUID, error) {
	if s := r.URL.Query().Get("complex_id"); s != "" {
		return uuid.Parse(s)
	}
	return middleware.GetComplexIDAsUUID(r.Context())
}

// respondGrantWriteError maps a grant-write failure: a unit-scope denial is a
// 403, anything else stays an internal error.
func respondGrantWriteError(w http.ResponseWriter, err error) {
	if stderrors.Is(err, ErrUnitAccessDenied) {
		errors.RespondError(w, nil, errors.NewPermissionError(""))
		return
	}
	errors.RespondError(w, nil, errors.NewInternalError(err))
}

// CreateQRCode handles POST /api/v1/access/qr-codes
func (h *Handler) CreateQRCode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	complexID, err := middleware.GetComplexIDAsUUID(ctx)
	if err != nil {
		errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{
			"X-Complex-ID": {"Missing or invalid complex ID"},
		}))
		return
	}

	createdBy, err := middleware.GetUserIDAsUUID(ctx)
	if err != nil {
		errors.RespondError(w, nil, errors.NewAuthError("Authentication required"))
		return
	}

	var req CreateQRCodeRequest
	if err := errors.DecodeJSONLenient(r, &req); err != nil {
		errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{
			"body": {"Invalid request body"},
		}))
		return
	}

	grant, err := h.service.CreateQRCode(ctx, complexID, createdBy, req)
	if err != nil {
		respondGrantWriteError(w, err)
		return
	}

	errors.RespondCreated(w, grant)
}

// CreateQRBatch handles POST /api/v1/access/qr-codes/batch
func (h *Handler) CreateQRBatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	complexID, err := middleware.GetComplexIDAsUUID(ctx)
	if err != nil {
		errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{
			"X-Complex-ID": {"Missing or invalid complex ID"},
		}))
		return
	}

	createdBy, err := middleware.GetUserIDAsUUID(ctx)
	if err != nil {
		errors.RespondError(w, nil, errors.NewAuthError("Authentication required"))
		return
	}

	var req CreateQRBatchRequest
	if err := errors.DecodeJSONLenient(r, &req); err != nil {
		errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{
			"body": {"Invalid request body"},
		}))
		return
	}

	grants, err := h.service.CreateQRBatch(ctx, complexID, createdBy, req)
	if err != nil {
		respondGrantWriteError(w, err)
		return
	}

	errors.RespondCreated(w, grants)
}

// GetQRCodes handles GET /api/v1/access/qr-codes
func (h *Handler) GetQRCodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	complexID, err := resolveComplexID(r)
	if err != nil {
		errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{
			"complex_id": {"Missing or invalid complex ID"},
		}))
		return
	}

	grants, err := h.service.GetQRCodes(ctx, complexID)
	if err != nil {
		errors.RespondError(w, nil, errors.NewInternalError(err))
		return
	}

	errors.RespondOK(w, grants)
}

// CreatePreAuth handles POST /api/v1/access/pre-auth
func (h *Handler) CreatePreAuth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	complexID, err := middleware.GetComplexIDAsUUID(ctx)
	if err != nil {
		errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{
			"X-Complex-ID": {"Missing or invalid complex ID"},
		}))
		return
	}

	createdBy, err := middleware.GetUserIDAsUUID(ctx)
	if err != nil {
		errors.RespondError(w, nil, errors.NewAuthError("Authentication required"))
		return
	}

	var req CreatePreAuthRequest
	if err := errors.DecodeJSONLenient(r, &req); err != nil {
		errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{
			"body": {"Invalid request body"},
		}))
		return
	}

	grant, err := h.service.CreatePreAuth(ctx, complexID, createdBy, req)
	if err != nil {
		respondGrantWriteError(w, err)
		return
	}

	errors.RespondCreated(w, grant)
}

// GetPreAuthorizations handles GET /api/v1/access/pre-auth
func (h *Handler) GetPreAuthorizations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	complexID, err := resolveComplexID(r)
	if err != nil {
		errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{
			"complex_id": {"Missing or invalid complex ID"},
		}))
		return
	}

	grants, err := h.service.GetPreAuthorizations(ctx, complexID)
	if err != nil {
		errors.RespondError(w, nil, errors.NewInternalError(err))
		return
	}

	errors.RespondOK(w, grants)
}

// CreateAccessGrant handles POST /api/v1/access-grants
func (h *Handler) CreateAccessGrant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	complexID, err := middleware.GetComplexIDAsUUID(ctx)
	if err != nil {
		errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{
			"X-Complex-ID": {"Missing or invalid complex ID"},
		}))
		return
	}

	createdBy, err := middleware.GetUserIDAsUUID(ctx)
	if err != nil {
		errors.RespondError(w, nil, errors.NewAuthError("Authentication required"))
		return
	}

	var req CreateAccessGrantRequest
	if err := errors.DecodeJSONLenient(r, &req); err != nil {
		errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{
			"body": {"Invalid request body"},
		}))
		return
	}

	grant, err := h.service.CreateAccessGrant(ctx, complexID, createdBy, req)
	if err != nil {
		respondGrantWriteError(w, err)
		return
	}

	errors.RespondCreated(w, grant)
}

// GetAccessGrant handles GET /api/v1/access-grants/{id}
func (h *Handler) GetAccessGrant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	complexID, err := middleware.GetComplexIDAsUUID(ctx)
	if err != nil {
		errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{
			"X-Complex-ID": {"Missing or invalid complex ID"},
		}))
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{
			"id": {"Invalid UUID format"},
		}))
		return
	}

	userID, err := middleware.GetUserIDAsUUID(ctx)
	if err != nil {
		errors.RespondError(w, nil, errors.NewAuthError("Authentication required"))
		return
	}

	grant, err := h.service.GetAccessGrant(ctx, complexID, userID, id)
	if err != nil {
		if stderrors.Is(err, ErrUnitAccessDenied) {
			errors.RespondError(w, nil, errors.NewPermissionError(""))
			return
		}
		errors.RespondError(w, nil, errors.NewNotFoundError("AccessGrant", idStr))
		return
	}

	errors.RespondOK(w, grant)
}

// CancelAccessGrant handles PATCH /api/v1/access-grants/{id}/cancel
func (h *Handler) CancelAccessGrant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	complexID, err := middleware.GetComplexIDAsUUID(ctx)
	if err != nil {
		errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{
			"X-Complex-ID": {"Missing or invalid complex ID"},
		}))
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{
			"id": {"Invalid UUID format"},
		}))
		return
	}

	userID, err := middleware.GetUserIDAsUUID(ctx)
	if err != nil {
		errors.RespondError(w, nil, errors.NewAuthError("Authentication required"))
		return
	}

	if err := h.service.CancelAccessGrant(ctx, complexID, userID, id); err != nil {
		switch {
		case stderrors.Is(err, ErrUnitAccessDenied):
			errors.RespondError(w, nil, errors.NewPermissionError(""))
		case stderrors.Is(err, ErrAccessGrantNotFound) || strings.Contains(err.Error(), "not found"):
			errors.RespondError(w, nil, errors.NewNotFoundError("AccessGrant", idStr))
		default:
			errors.RespondError(w, nil, errors.NewInternalError(err))
		}
		return
	}

	errors.RespondOK(w, map[string]string{"message": "Access grant cancelled successfully"})
}

// ValidateAccessGrant handles POST /api/v1/access-grants/validate
func (h *Handler) ValidateAccessGrant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req ValidateAccessRequest
	if err := errors.DecodeJSONLenient(r, &req); err != nil {
		errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{
			"body": {"Invalid request body"},
		}))
		return
	}

	result, err := h.service.ValidateAccess(ctx, req)
	if err != nil {
		errors.RespondError(w, nil, errors.NewInternalError(err))
		return
	}

	errors.RespondOK(w, result)
}

// GetUnitAccessGrants handles GET /api/v1/units/{unitId}/access-grants
func (h *Handler) GetUnitAccessGrants(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	unitIDStr := chi.URLParam(r, "unitId")
	unitID, err := uuid.Parse(unitIDStr)
	if err != nil {
		errors.RespondError(w, nil, errors.NewValidationError(map[string][]string{
			"unitId": {"Invalid UUID format"},
		}))
		return
	}

	var status *string
	if statusParam := r.URL.Query().Get("status"); statusParam != "" {
		status = &statusParam
	}

	grants, err := h.service.GetUnitAccessGrants(ctx, unitID, status)
	if err != nil {
		errors.RespondError(w, nil, errors.NewInternalError(err))
		return
	}

	errors.RespondOK(w, grants)
}
