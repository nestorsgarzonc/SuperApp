package accesslog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"controldeaccesos/internal/audit"
	"controldeaccesos/internal/platform/middleware"

	"github.com/google/uuid"
)

// t0 es el reloj fijo inyectado en h.now (sin tocar produccion: el campo ya existe).
var t0 = time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC)

// stubLogRepo es un STUB + SPY de accesslog.Repository: devuelve valores
// configurables (log, created, err) y captura los argumentos con que se le llama.
// Supuesto: el repositorio real aplica la idempotencia por (complex_id, client_id)
// y devuelve created=false en el reintento; aqui solo se simula esa salida.
type stubLogRepo struct {
	// salidas configurables
	createLog     *AccessLog
	createCreated bool
	createErr     error
	listOut       []*AccessLog
	listErr       error
	residentLog   *AccessLog
	residentNew   bool
	residentErr   error
	historyOut    []*AccessLog
	historyErr    error
	presenceOut   []*Presence
	presenceErr   error

	// capturas (spy)
	createCalls   int
	gotComplexID  uuid.UUID
	gotRegistered uuid.UUID
	gotReq        CreateRequest
	gotOccurredAt time.Time
	listCalls     int
	gotFilter     ListFilter
	residentCalls int
	gotResidentRq ResidentMovementRequest
	gotResidentID uuid.UUID
	historyCalls  int
	gotLimit      int
	presenceCalls int
}

func (s *stubLogRepo) Create(_ context.Context, complexID, registeredBy uuid.UUID, req CreateRequest, occurredAt time.Time) (*AccessLog, bool, error) {
	s.createCalls++
	s.gotComplexID, s.gotRegistered, s.gotReq, s.gotOccurredAt = complexID, registeredBy, req, occurredAt
	return s.createLog, s.createCreated, s.createErr
}

func (s *stubLogRepo) List(_ context.Context, complexID uuid.UUID, f ListFilter) ([]*AccessLog, error) {
	s.listCalls++
	s.gotComplexID, s.gotFilter = complexID, f
	return s.listOut, s.listErr
}

func (s *stubLogRepo) CreateResidentMovement(_ context.Context, complexID, userID uuid.UUID, req ResidentMovementRequest) (*AccessLog, bool, error) {
	s.residentCalls++
	s.gotComplexID, s.gotResidentID, s.gotResidentRq = complexID, userID, req
	return s.residentLog, s.residentNew, s.residentErr
}

func (s *stubLogRepo) ResidentHistory(_ context.Context, complexID, userID uuid.UUID, limit int) ([]*AccessLog, error) {
	s.historyCalls++
	s.gotComplexID, s.gotResidentID, s.gotLimit = complexID, userID, limit
	return s.historyOut, s.historyErr
}

func (s *stubLogRepo) Presence(_ context.Context, complexID uuid.UUID) ([]*Presence, error) {
	s.presenceCalls++
	s.gotComplexID = complexID
	return s.presenceOut, s.presenceErr
}

// spyAuditor es un SPY de Auditor: registra cada entrada escrita y puede devolver
// un error fijo (para probar que la auditoria es "best effort").
type spyAuditor struct {
	entries []audit.Entry
	err     error
}

func (a *spyAuditor) Write(_ context.Context, e audit.Entry) error {
	a.entries = append(a.entries, e)
	return a.err
}

// logSpy es un slog.Handler SPY que captura los mensajes registrados.
type logSpy struct{ msgs []string }

func (l *logSpy) Enabled(context.Context, slog.Level) bool { return true }
func (l *logSpy) Handle(_ context.Context, r slog.Record) error {
	l.msgs = append(l.msgs, r.Message)
	return nil
}
func (l *logSpy) WithAttrs([]slog.Attr) slog.Handler { return l }
func (l *logSpy) WithGroup(string) slog.Handler      { return l }

// dummyLogger es un DUMMY: llena la firma de NewHandler sin verificarse.
func dummyLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type fixture struct {
	h         *Handler
	repo      *stubLogRepo
	aud       *spyAuditor
	complexID uuid.UUID
	userID    uuid.UUID
}

func newFixture() *fixture {
	repo := &stubLogRepo{}
	aud := &spyAuditor{}
	h := NewHandler(repo, aud, dummyLogger())
	h.now = func() time.Time { return t0 }
	return &fixture{h: h, repo: repo, aud: aud, complexID: uuid.New(), userID: uuid.New()}
}

// ctx arma el contexto que dejaria el middleware de autenticacion (valores, no dependencias).
func (f *fixture) ctx() context.Context {
	ctx := context.WithValue(context.Background(), middleware.UserIDKey, f.userID.String())
	return context.WithValue(ctx, middleware.ComplexIDKey, f.complexID.String())
}

func (f *fixture) do(t *testing.T, ctx context.Context, handler http.HandlerFunc, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, target, rd).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details"`
	} `json:"error"`
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) envelope {
	t.Helper()
	var e envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("cuerpo no es JSON: %v (%s)", err, rec.Body.String())
	}
	return e
}

func sampleLog(movement, result string) *AccessLog {
	return &AccessLog{
		ID: uuid.New(), ClientID: uuid.New(), Movement: movement, Result: result,
		VisitorName: "Ana", UnitLabel: "A-101", OccurredAt: t0,
	}
}

func createBody(clientID uuid.UUID, movement, result string) map[string]any {
	return map[string]any{
		"client_id": clientID.String(), "movement": movement, "result": result,
		"visitor_name": "Ana", "unit_label": "A-101",
	}
}

// Caso AL-05a · created=true ENTRY
// Técnica: Transición de estados.
// Debe hacer / esperado: 201; auditoría access_log.entry ALLOWED con actor, complejo, recurso, IP
// y correlación
// auditoría access_log.entry ALLOWED (con IP y correlación del middleware de metadatos).
func TestAL05a_CrearMovimientoNuevoResponde201YAudita(t *testing.T) {
	f := newFixture()
	f.repo.createLog, f.repo.createCreated = sampleLog(MovementEntry, ResultAuthorized), true

	// WithRequestMeta es una funcion pura sin E/S: se usa real (supuesto declarado).
	h := middleware.WithRequestMeta(http.HandlerFunc(f.h.Create))
	body, _ := json.Marshal(createBody(uuid.New(), MovementEntry, ResultAuthorized))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/access-logs", bytes.NewReader(body)).WithContext(f.ctx())
	req.RemoteAddr = "10.1.2.3:5555"
	req.Header.Set("X-Correlation-ID", "corr-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d, esperaba 201 (%s)", rec.Code, rec.Body.String())
	}
	if e := decode(t, rec); !e.Success {
		t.Fatalf("success=false: %s", rec.Body.String())
	}
	if len(f.aud.entries) != 1 {
		t.Fatalf("auditorias=%d, esperaba 1", len(f.aud.entries))
	}
	e := f.aud.entries[0]
	if e.Action != "access_log.entry" || e.Result != audit.ResultAllowed || e.ResourceType != "access_log" {
		t.Fatalf("auditoria inesperada: %+v", e)
	}
	if e.ActorID == nil || *e.ActorID != f.userID || e.ComplexID == nil || *e.ComplexID != f.complexID {
		t.Fatalf("actor/complejo de auditoria incorrectos: %+v", e)
	}
	if e.ResourceID == nil || *e.ResourceID != f.repo.createLog.ID {
		t.Fatalf("resource id incorrecto: %+v", e.ResourceID)
	}
	if e.IPAddress != "10.1.2.3" || e.CorrelationID != "corr-1" {
		t.Fatalf("ip/correlacion incorrectos: %q %q", e.IPAddress, e.CorrelationID)
	}
	if f.repo.gotComplexID != f.complexID || f.repo.gotRegistered != f.userID {
		t.Fatalf("el repo debe recibir complejo y usuario del contexto")
	}
}

// Caso AL-05b · técnica: PE · condición: ATTEMPT DENIED → 201 y auditoría DENIED con acción access_log.attempt.
func TestAL05b_IntentoDenegadoSeAuditaComoDenied(t *testing.T) {
	f := newFixture()
	f.repo.createLog, f.repo.createCreated = sampleLog(MovementAttempt, ResultDenied), true
	rec := f.do(t, f.ctx(), f.h.Create, http.MethodPost, "/x", createBody(uuid.New(), MovementAttempt, ResultDenied))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d (%s)", rec.Code, rec.Body.String())
	}
	if len(f.aud.entries) != 1 || f.aud.entries[0].Result != audit.ResultDenied || f.aud.entries[0].Action != "access_log.attempt" {
		t.Fatalf("auditoria inesperada: %+v", f.aud.entries)
	}
}

// Caso AL-05c · client_id repetido
// Técnica: Transición de estados.
// Debe hacer / esperado: 200, cuerpo original, 0 auditorías
// original y CERO auditorías (la interacción nula ES el comportamiento: idempotencia).
func TestAL05c_ClientIDRepetidoResponde200SinAuditar(t *testing.T) {
	f := newFixture()
	orig := sampleLog(MovementEntry, ResultAuthorized)
	f.repo.createLog, f.repo.createCreated = orig, false
	rec := f.do(t, f.ctx(), f.h.Create, http.MethodPost, "/x", createBody(orig.ClientID, MovementEntry, ResultAuthorized))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, esperaba 200", rec.Code)
	}
	var got AccessLog
	if err := json.Unmarshal(decode(t, rec).Data, &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != orig.ID {
		t.Fatalf("debe devolver el registro original: %v vs %v", got.ID, orig.ID)
	}
	if len(f.aud.entries) != 0 {
		t.Fatalf("un reintento idempotente no debe auditar: %+v", f.aud.entries)
	}
}

// Caso AL-05d1/2 · occurred_at nil / provisto
// Técnica: Partición en clases de equivalencia.
// Debe hacer / esperado: repo recibe h.now() / el provisto
func TestAL05d_OccurredAtPorDefectoEsElRelojInyectado(t *testing.T) {
	t.Run("AL-05d1 nil usa now", func(t *testing.T) {
		f := newFixture()
		f.repo.createLog, f.repo.createCreated = sampleLog(MovementEntry, ResultAuthorized), true
		f.do(t, f.ctx(), f.h.Create, http.MethodPost, "/x", createBody(uuid.New(), MovementEntry, ResultAuthorized))
		if !f.repo.gotOccurredAt.Equal(t0) {
			t.Fatalf("occurredAt=%v, esperaba %v", f.repo.gotOccurredAt, t0)
		}
	})
	t.Run("AL-05d2 provisto se conserva", func(t *testing.T) {
		f := newFixture()
		f.repo.createLog, f.repo.createCreated = sampleLog(MovementEntry, ResultAuthorized), true
		when := t0.Add(-2 * time.Hour)
		b := createBody(uuid.New(), MovementEntry, ResultAuthorized)
		b["occurred_at"] = when.Format(time.RFC3339)
		f.do(t, f.ctx(), f.h.Create, http.MethodPost, "/x", b)
		if !f.repo.gotOccurredAt.Equal(when) {
			t.Fatalf("occurredAt=%v, esperaba %v", f.repo.gotOccurredAt, when)
		}
	})
}

// Caso AL-05e1-5 · sin complejo 422, sin usuario 401, usuario no-UUID 401, JSON malo 400, inválido
// 422
// Técnica: Tabla de decisión.
// Debe hacer / esperado: repo y auditor sin llamadas
// JSON malformado → 400, request inválido → 422); en todas el repo NO se llama.
func TestAL05e_PrecondicionesDeCreateNoTocanElRepo(t *testing.T) {
	f := newFixture()
	onlyUser := context.WithValue(context.Background(), middleware.UserIDKey, f.userID.String())
	onlyComplex := context.WithValue(context.Background(), middleware.ComplexIDKey, f.complexID.String())
	badUUID := context.WithValue(f.ctx(), middleware.UserIDKey, "no-es-uuid")
	valid := createBody(uuid.New(), MovementEntry, ResultAuthorized)
	invalid := createBody(uuid.New(), MovementEntry, ResultDenied)

	cases := []struct {
		id     string
		ctx    context.Context
		body   any
		status int
	}{
		{"AL-05e1 sin complejo", onlyUser, valid, http.StatusUnprocessableEntity},
		{"AL-05e2 sin usuario", onlyComplex, valid, http.StatusUnauthorized},
		{"AL-05e3 usuario no UUID", badUUID, valid, http.StatusUnauthorized},
		{"AL-05e4 JSON malformado", f.ctx(), "{no-json", http.StatusBadRequest},
		{"AL-05e5 request invalido", f.ctx(), invalid, http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			rec := f.do(t, tc.ctx, f.h.Create, http.MethodPost, "/x", tc.body)
			if rec.Code != tc.status {
				t.Fatalf("status=%d, esperaba %d (%s)", rec.Code, tc.status, rec.Body.String())
			}
			if f.repo.createCalls != 0 || len(f.aud.entries) != 0 {
				t.Fatalf("no debia llamarse al repo ni auditar (repo=%d aud=%d)", f.repo.createCalls, len(f.aud.entries))
			}
		})
	}
}

// Caso AL-05f · técnica: PE · condición: sin auditor (nil) el alta funciona igual (auditoría opcional).
func TestAL05f_SinAuditorElAltaFunciona(t *testing.T) {
	f := newFixture()
	f.h.auditor = nil
	f.repo.createLog, f.repo.createCreated = sampleLog(MovementEntry, ResultAuthorized), true
	rec := f.do(t, f.ctx(), f.h.Create, http.MethodPost, "/x", createBody(uuid.New(), MovementEntry, ResultAuthorized))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d", rec.Code)
	}
}

// Caso AL-05g · técnica: EG · condición: si la auditoría falla, la petición NO falla (best-effort) y se registra el error.
func TestAL05g_FalloDeAuditoriaNoRompeLaPeticion(t *testing.T) {
	f := newFixture()
	f.aud.err = errors.New("db caida")
	spy := &logSpy{}
	f.h.logger = slog.New(spy)
	f.repo.createLog, f.repo.createCreated = sampleLog(MovementEntry, ResultAuthorized), true
	rec := f.do(t, f.ctx(), f.h.Create, http.MethodPost, "/x", createBody(uuid.New(), MovementEntry, ResultAuthorized))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d, esperaba 201", rec.Code)
	}
	if len(spy.msgs) != 1 || spy.msgs[0] != "audit write failed" {
		t.Fatalf("debia registrar 'audit write failed': %v", spy.msgs)
	}
}

// Caso AL-06 · repo falla
// Técnica: Adivinación de errores.
// Debe hacer / esperado: 500 "An unexpected error occurred" sin filtrar causa; sin auditoría
func TestAL06_FalloDelRepoResponde500Generico(t *testing.T) {
	f := newFixture()
	f.repo.createErr = errors.New(`pq: relation "access_logs" does not exist`)
	rec := f.do(t, f.ctx(), f.h.Create, http.MethodPost, "/x", createBody(uuid.New(), MovementEntry, ResultAuthorized))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, esperaba 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "access_logs") || strings.Contains(rec.Body.String(), "pq:") {
		t.Fatalf("la respuesta filtra la causa interna: %s", rec.Body.String())
	}
	if e := decode(t, rec); e.Error.Message != "An unexpected error occurred" {
		t.Fatalf("mensaje=%q", e.Error.Message)
	}
	if len(f.aud.entries) != 0 {
		t.Fatalf("no debe auditar si el repo falla")
	}
}

// Caso AL-07a1-7 · limit por defecto/0/1/500/501/abc/−1
// Técnica: Valores límite.
// Debe hacer / esperado: 100 al repo; inválidos 422 sin llamar al repo
func TestAL07a_LimiteDeListado(t *testing.T) {
	cases := []struct {
		id    string
		query string
		ok    bool
		want  int
	}{
		{"AL-07a1 por defecto", "", true, 100},
		{"AL-07a2 limit=0", "limit=0", false, 0},
		{"AL-07a3 limit=1", "limit=1", true, 1},
		{"AL-07a4 limit=500", "limit=500", true, 500},
		{"AL-07a5 limit=501", "limit=501", false, 0},
		{"AL-07a6 limit=abc", "limit=abc", false, 0},
		{"AL-07a7 limit negativo", "limit=-1", false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			f := newFixture()
			rec := f.do(t, f.ctx(), f.h.List, http.MethodGet, "/x?"+tc.query, nil)
			if tc.ok {
				if rec.Code != http.StatusOK || f.repo.listCalls != 1 || f.repo.gotFilter.Limit != tc.want {
					t.Fatalf("status=%d calls=%d limit=%d, esperaba 200/1/%d", rec.Code, f.repo.listCalls, f.repo.gotFilter.Limit, tc.want)
				}
				return
			}
			if rec.Code != http.StatusUnprocessableEntity || f.repo.listCalls != 0 {
				t.Fatalf("status=%d calls=%d, esperaba 422 sin llamar al repo", rec.Code, f.repo.listCalls)
			}
			if !hasFieldError(decode(t, rec), "limit") {
				t.Fatalf("detalle debe senalar 'limit': %s", rec.Body.String())
			}
		})
	}
}

// Caso AL-07b1-8 · filtros válidos llegan al repo; from/to mal formados, from≥to, enumerados
// inválidos
// Técnica: Partición en clases de equivalencia.
// Debe hacer / esperado: 422 por campo / filtro capturado
func TestAL07b_FiltrosDeListado(t *testing.T) {
	from := t0.Add(-time.Hour)
	to := t0
	enc := func(ts time.Time) string { return url.QueryEscape(ts.Format(time.RFC3339)) }

	t.Run("AL-07b1 filtros validos llegan al repo", func(t *testing.T) {
		f := newFixture()
		f.repo.listOut = []*AccessLog{sampleLog(MovementExit, ResultAuthorized)}
		q := "from=" + enc(from) + "&to=" + enc(to) + "&movement=EXIT&result=AUTHORIZED&subject=VISITOR"
		rec := f.do(t, f.ctx(), f.h.List, http.MethodGet, "/x?"+q, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d (%s)", rec.Code, rec.Body.String())
		}
		g := f.repo.gotFilter
		if g.From == nil || !g.From.Equal(from) || g.To == nil || !g.To.Equal(to) ||
			g.Movement != MovementExit || g.Result != ResultAuthorized || g.Subject != SubjectVisitor {
			t.Fatalf("filtro recibido incorrecto: %+v", g)
		}
		if f.repo.gotComplexID != f.complexID {
			t.Fatalf("el listado debe acotarse al complejo del contexto")
		}
		var logs []*AccessLog
		if err := json.Unmarshal(decode(t, rec).Data, &logs); err != nil || len(logs) != 1 {
			t.Fatalf("cuerpo inesperado: %v %s", err, rec.Body.String())
		}
	})

	bad := []struct {
		id    string
		query string
		field string
	}{
		{"AL-07b2 from no RFC3339", "from=ayer", "from"},
		{"AL-07b3 to no RFC3339", "to=2026-13-45", "to"},
		{"AL-07b4 from igual a to", "from=" + enc(t0) + "&to=" + enc(t0), "to"},
		{"AL-07b5 from posterior a to", "from=" + enc(t0) + "&to=" + enc(from), "to"},
		{"AL-07b6 movement invalido", "movement=IN", "movement"},
		{"AL-07b7 result invalido", "result=OK", "result"},
		{"AL-07b8 subject invalido", "subject=ROBOT", "subject"},
	}
	for _, tc := range bad {
		t.Run(tc.id, func(t *testing.T) {
			f := newFixture()
			rec := f.do(t, f.ctx(), f.h.List, http.MethodGet, "/x?"+tc.query, nil)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status=%d, esperaba 422 (%s)", rec.Code, rec.Body.String())
			}
			if !hasFieldError(decode(t, rec), tc.field) {
				t.Fatalf("detalle debe senalar %q: %s", tc.field, rec.Body.String())
			}
			if f.repo.listCalls != 0 {
				t.Fatal("el repo no debia llamarse")
			}
		})
	}
}

// Caso AL-07c · List sin complejo / repo falla
// Técnica: Tabla de decisión, Adivinación de errores.
// Debe hacer / esperado: 422 / 500
func TestAL07c_ListSinComplejoYFalloDelRepo(t *testing.T) {
	f := newFixture()
	rec := f.do(t, context.Background(), f.h.List, http.MethodGet, "/x", nil)
	if rec.Code != http.StatusUnprocessableEntity || f.repo.listCalls != 0 {
		t.Fatalf("sin complejo: status=%d calls=%d", rec.Code, f.repo.listCalls)
	}
	f.repo.listErr = errors.New("boom")
	rec = f.do(t, f.ctx(), f.h.List, http.MethodGet, "/x", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, esperaba 500", rec.Code)
	}
}

// Caso AL-10 · trim
// Técnica: Partición en clases de equivalencia.
// Debe hacer / esperado: " Ana "→"Ana", "\tA-101 "→"A-101"
func TestAL10_RecortaEspaciosAntesDeGuardar(t *testing.T) {
	f := newFixture()
	f.repo.createLog, f.repo.createCreated = sampleLog(MovementEntry, ResultAuthorized), true
	b := createBody(uuid.New(), MovementEntry, ResultAuthorized)
	b["visitor_name"] = " Ana "
	b["unit_label"] = "\tA-101  "
	rec := f.do(t, f.ctx(), f.h.Create, http.MethodPost, "/x", b)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d", rec.Code)
	}
	if f.repo.gotReq.VisitorName != "Ana" || f.repo.gotReq.UnitLabel != "A-101" {
		t.Fatalf("repo recibio %q / %q", f.repo.gotReq.VisitorName, f.repo.gotReq.UnitLabel)
	}
}

func complexKey() any { return middleware.ComplexIDKey }

func newSpyLogger(h slog.Handler) *slog.Logger { return slog.New(h) }

// hasFieldError dice si la respuesta 422 senala el campo en details.field_errors.
func hasFieldError(e envelope, field string) bool {
	fe, ok := e.Error.Details["field_errors"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = fe[field]
	return ok
}
