package accesslog

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"controldeaccesos/internal/audit"
	"controldeaccesos/internal/platform/errors"
	"controldeaccesos/internal/platform/middleware"

	"github.com/google/uuid"
)

// Auditor persists an append-only audit trail entry.
type Auditor interface {
	Write(ctx context.Context, e audit.Entry) error
}

type Handler struct {
	repo    Repository
	auditor Auditor
	logger  *slog.Logger
	now     func() time.Time
}

func NewHandler(repo Repository, auditor Auditor, logger *slog.Logger) *Handler {
	return &Handler{repo: repo, auditor: auditor, logger: logger, now: time.Now}
}

func missingComplex() error {
	return errors.NewValidationError(map[string][]string{"X-Complex-ID": {"Missing or invalid complex ID"}})
}

// Create handles POST /api/v1/access-logs (GUARD/ADMIN).
// 201 when new, 200 when the same client_id was already recorded.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
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

	var req CreateRequest
	if err := errors.DecodeJSONLenient(w, r, &req); err != nil {
		errors.RespondError(w, nil, err)
		return
	}
	now := h.now()
	if fieldErrs := req.validate(now); len(fieldErrs) > 0 {
		errors.RespondError(w, nil, errors.NewValidationError(fieldErrs))
		return
	}
	req.VisitorName = strings.TrimSpace(req.VisitorName)
	req.UnitLabel = strings.TrimSpace(req.UnitLabel)

	occurredAt := now
	if req.OccurredAt != nil {
		occurredAt = *req.OccurredAt
	}

	entry, created, err := h.repo.Create(ctx, complexID, userID, req, occurredAt)
	if err != nil {
		errors.RespondError(w, nil, errors.NewInternalError(err))
		return
	}
	if !created {
		errors.RespondOK(w, entry)
		return
	}

	h.audit(ctx, userID, complexID, entry)
	errors.RespondCreated(w, entry)
}

// List handles GET /api/v1/access-logs?from=&to=&movement=&result=&limit= (GUARD/ADMIN).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	complexID, err := middleware.GetComplexIDAsUUID(ctx)
	if err != nil {
		errors.RespondError(w, nil, missingComplex())
		return
	}

	q := r.URL.Query()
	f := ListFilter{Movement: q.Get("movement"), Result: q.Get("result"), Subject: q.Get("subject"), Limit: defaultListLimit}
	fieldErrs := map[string][]string{}

	parseTime := func(field string) *time.Time {
		raw := q.Get(field)
		if raw == "" {
			return nil
		}
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			fieldErrs[field] = []string{"must be an RFC3339 timestamp"}
			return nil
		}
		return &t
	}
	f.From = parseTime("from")
	f.To = parseTime("to")

	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxListLimit {
			fieldErrs["limit"] = []string{"must be between 1 and 500"}
		} else {
			f.Limit = n
		}
	}
	for k, v := range f.validate() {
		fieldErrs[k] = v
	}
	if len(fieldErrs) > 0 {
		errors.RespondError(w, nil, errors.NewValidationError(fieldErrs))
		return
	}

	logs, err := h.repo.List(ctx, complexID, f)
	if err != nil {
		errors.RespondError(w, nil, errors.NewInternalError(err))
		return
	}
	errors.RespondOK(w, logs)
}

// audit is best-effort: a failure is logged but never fails the request.
func (h *Handler) audit(ctx context.Context, actor, complexID uuid.UUID, l *AccessLog) {
	if h.auditor == nil {
		return
	}
	result := audit.ResultAllowed
	if l.Result == ResultDenied {
		result = audit.ResultDenied
	}
	err := h.auditor.Write(ctx, audit.Entry{
		ActorID:       &actor,
		ComplexID:     &complexID,
		ResourceType:  "access_log",
		ResourceID:    &l.ID,
		Action:        "access_log." + strings.ToLower(l.Movement),
		Result:        result,
		IPAddress:     middleware.GetRequestIP(ctx),
		CorrelationID: middleware.GetCorrelationID(ctx),
	})
	if err != nil && h.logger != nil {
		h.logger.Error("audit write failed", "error", err, "action", "access_log."+strings.ToLower(l.Movement))
	}
}
