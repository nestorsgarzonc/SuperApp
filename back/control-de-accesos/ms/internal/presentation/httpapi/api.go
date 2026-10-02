package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"superapp/control-de-accesos/internal/domain"
)

type Service interface {
	Ready(context.Context) error
	Actor(context.Context, string, string) (domain.Actor, error)
	Context(context.Context) ([]domain.Complex, []domain.Person, error)
	Notifications(context.Context, domain.Actor) ([]domain.Notification, error)
	CreatePass(context.Context, domain.Actor, domain.CreatePass) (*domain.Pass, error)
	GetPass(context.Context, domain.Actor, string) (*domain.Pass, error)
	ListPasses(context.Context, domain.Actor, string, int) ([]domain.Pass, error)
	Checkpoint(context.Context, domain.Actor, int) (*domain.CheckpointFeed, error)
	RevokePass(context.Context, domain.Actor, string) (*domain.Pass, error)
	ExtendPass(context.Context, domain.Actor, string, time.Time) (*domain.Pass, error)
	ValidateToken(context.Context, domain.Actor, string) (*domain.Pass, error)
	CheckOut(context.Context, domain.Actor, string) (*domain.Pass, error)
	CreatePreauth(context.Context, domain.Actor, domain.CreatePreauth) (*domain.Preauth, error)
	ListPreauths(context.Context, domain.Actor) ([]domain.Preauth, error)
	RevokePreauth(context.Context, domain.Actor, string) error
	LookupPlate(context.Context, domain.Actor, string) (*domain.Lookup, error)
	LogAccess(context.Context, domain.Actor, string, string, string) (*domain.AccessLog, error)
	ListAccess(context.Context, domain.Actor) ([]domain.AccessLog, error)
}

type API struct {
	service Service
	mux     *http.ServeMux
}

func New(service Service) *API {
	a := &API{service: service, mux: http.NewServeMux()}
	a.mux.HandleFunc("GET /health", a.health)
	a.mux.HandleFunc("GET /ready", a.ready)
	a.mux.HandleFunc("GET /integration/context", a.context)
	a.mux.HandleFunc("GET /integration/notifications", a.notifications)
	a.mux.HandleFunc("POST /visitor-passes", a.createPass)
	a.mux.HandleFunc("GET /visitor-passes/units/{unitId}", a.listPasses)
	a.mux.HandleFunc("GET /visitor-passes/checkpoint-activity", a.checkpoint)
	a.mux.HandleFunc("POST /visitor-passes/validate", a.validate)
	a.mux.HandleFunc("POST /visitor-passes/check-out", a.checkOut)
	a.mux.HandleFunc("GET /visitor-passes/{id}", a.getPass)
	a.mux.HandleFunc("POST /visitor-passes/{id}/revoke", a.revokePass)
	a.mux.HandleFunc("POST /visitor-passes/{id}/extend", a.extendPass)
	a.mux.HandleFunc("GET /vehicles/lookup", a.lookup)
	a.mux.HandleFunc("POST /vehicles/access-log", a.logAccess)
	a.mux.HandleFunc("GET /vehicles/access-log", a.listAccess)
	a.mux.HandleFunc("POST /vehicles/visitor-preauth", a.createPreauth)
	a.mux.HandleFunc("GET /vehicles/visitor-preauth", a.listPreauths)
	a.mux.HandleFunc("DELETE /vehicles/visitor-preauth/{id}", a.revokePreauth)
	return a
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) { a.mux.ServeHTTP(w, r) }

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	if err := a.service.Ready(r.Context()); err != nil {
		writeError(w, 503, "not_ready", "Access dependencies unavailable")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ready"})
}

func (a *API) context(w http.ResponseWriter, r *http.Request) {
	complexes, people, err := a.service.Context(r.Context())
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"complexes": complexes, "people": people})
}

func (a *API) notifications(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.actor(w, r, "")
	if !ok {
		return
	}
	list, err := a.service.Notifications(r.Context(), actor)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (a *API) createPass(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.actor(w, r, domain.RoleResident)
	if !ok {
		return
	}
	var in struct {
		UnitID      string    `json:"unit_id"`
		VisitorName string    `json:"visitor_name"`
		Document    string    `json:"document"`
		ValidFrom   time.Time `json:"valid_from"`
		ValidUntil  time.Time `json:"valid_until"`
	}
	if !decode(w, r, &in) {
		return
	}
	pass, err := a.service.CreatePass(r.Context(), actor, domain.CreatePass{
		UnitID: in.UnitID, VisitorName: in.VisitorName, Document: in.Document,
		ValidFrom: in.ValidFrom, ValidUntil: in.ValidUntil,
	})
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 201, pass)
}

func (a *API) listPasses(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.actor(w, r, domain.RoleResident)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := a.service.ListPasses(r.Context(), actor, r.PathValue("unitId"), limit)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (a *API) checkpoint(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.actor(w, r, domain.RoleGuard)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	feed, err := a.service.Checkpoint(r.Context(), actor, limit)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, feed)
}

func (a *API) validate(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.actor(w, r, domain.RoleGuard)
	if !ok {
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &in) {
		return
	}
	pass, err := a.service.ValidateToken(r.Context(), actor, in.Token)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, pass)
}

func (a *API) checkOut(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.actor(w, r, domain.RoleGuard)
	if !ok {
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &in) {
		return
	}
	pass, err := a.service.CheckOut(r.Context(), actor, in.Token)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, pass)
}

func (a *API) getPass(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.actor(w, r, domain.RoleResident)
	if !ok {
		return
	}
	pass, err := a.service.GetPass(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, pass)
}

func (a *API) revokePass(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.actor(w, r, domain.RoleResident)
	if !ok {
		return
	}
	pass, err := a.service.RevokePass(r.Context(), actor, r.PathValue("id"))
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, pass)
}

func (a *API) extendPass(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.actor(w, r, domain.RoleResident)
	if !ok {
		return
	}
	var in struct {
		ValidUntil time.Time `json:"valid_until"`
	}
	if !decode(w, r, &in) {
		return
	}
	pass, err := a.service.ExtendPass(r.Context(), actor, r.PathValue("id"), in.ValidUntil)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, pass)
}

func (a *API) lookup(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.actor(w, r, domain.RoleGuard)
	if !ok {
		return
	}
	result, err := a.service.LookupPlate(r.Context(), actor, r.URL.Query().Get("plate"))
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, result)
}

func (a *API) logAccess(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.actor(w, r, domain.RoleGuard)
	if !ok {
		return
	}
	var in struct {
		Plate     string `json:"plate"`
		Direction string `json:"direction"`
		Note      string `json:"note"`
	}
	if !decode(w, r, &in) {
		return
	}
	entry, err := a.service.LogAccess(r.Context(), actor, in.Plate, in.Direction, in.Note)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 201, entry)
}

func (a *API) listAccess(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.actor(w, r, domain.RoleGuard)
	if !ok {
		return
	}
	list, err := a.service.ListAccess(r.Context(), actor)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (a *API) createPreauth(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.actor(w, r, domain.RoleResident)
	if !ok {
		return
	}
	var in struct {
		UnitID      string    `json:"unit_id"`
		Plate       string    `json:"plate"`
		VisitorName string    `json:"visitor_name"`
		ValidFrom   time.Time `json:"valid_from"`
		ValidUntil  time.Time `json:"valid_until"`
	}
	if !decode(w, r, &in) {
		return
	}
	item, err := a.service.CreatePreauth(r.Context(), actor, domain.CreatePreauth{
		UnitID: in.UnitID, Plate: in.Plate, VisitorName: in.VisitorName,
		ValidFrom: in.ValidFrom, ValidUntil: in.ValidUntil,
	})
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 201, item)
}

func (a *API) listPreauths(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.actor(w, r, domain.RoleResident)
	if !ok {
		return
	}
	list, err := a.service.ListPreauths(r.Context(), actor)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (a *API) revokePreauth(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.actor(w, r, domain.RoleResident)
	if !ok {
		return
	}
	if err := a.service.RevokePreauth(r.Context(), actor, r.PathValue("id")); err != nil {
		a.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) actor(w http.ResponseWriter, r *http.Request, role string) (domain.Actor, bool) {
	actor, err := a.service.Actor(r.Context(), r.Header.Get("X-Complex-Id"), r.Header.Get("X-User-Id"))
	if err != nil {
		a.fail(w, err)
		return domain.Actor{}, false
	}
	if role != "" && actor.Role != role {
		a.fail(w, domain.ErrForbidden)
		return domain.Actor{}, false
	}
	return actor, true
}

func (a *API) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrUnauthorized):
		writeError(w, 401, "unauthorized", err.Error())
	case errors.Is(err, domain.ErrForbidden), errors.Is(err, domain.ErrUnitNotInComplex), errors.Is(err, domain.ErrUnitAccessDenied):
		writeError(w, 403, "forbidden", err.Error())
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrInvalidToken), errors.Is(err, domain.ErrPreauthNotFound):
		writeError(w, 404, "not_found", err.Error())
	case errors.Is(err, domain.ErrPassNotActive), errors.Is(err, domain.ErrPassExpired), errors.Is(err, domain.ErrPassNotYetValid), errors.Is(err, domain.ErrPassNotCheckedIn), errors.Is(err, domain.ErrPassNotExtendable):
		writeError(w, 409, "conflict", err.Error())
	case errors.Is(err, domain.ErrInvalidExtension), errors.Is(err, domain.ErrVisitorNameRequired), errors.Is(err, domain.ErrInvalidWindow), errors.Is(err, domain.ErrInvalidPlate), errors.Is(err, domain.ErrInvalidDirection):
		writeError(w, 400, "invalid_request", err.Error())
	default:
		slog.Error("access internal error", "error", err)
		writeError(w, 500, "internal_error", "Internal server error")
	}
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, 400, "invalid_json", "Invalid request body")
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, 400, "invalid_json", "Request must contain one JSON value")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
