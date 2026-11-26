package access

// C4 — Handler HTTP de pases. Dobles: stubService (stub + spy: un campo func
// por método y contador de llamadas). Reales: platform/errors, getters de
// middleware y chi (funciones puras sin E/S). Peticiones con httptest.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	apperr "controldeaccesos/internal/platform/errors"
	"controldeaccesos/internal/qrsystem"
)

// stubService: STUB + SPY de Service. Un método sin función configurada hace
// fallar la prueba (no se esperaba esa llamada).
type stubService struct {
	t     *testing.T
	calls map[string]int

	createQRCode      func(ctx context.Context, complexID, createdBy uuid.UUID, req CreateQRCodeRequest) (*AccessGrantResponse, error)
	createQRBatch     func(ctx context.Context, complexID, createdBy uuid.UUID, req CreateQRBatchRequest) ([]*AccessGrant, error)
	getQRCodes        func(ctx context.Context, complexID, userID uuid.UUID) ([]*AccessGrant, error)
	grantSummary      func(ctx context.Context, complexID uuid.UUID) (*GrantSummary, error)
	createPreAuth     func(ctx context.Context, complexID, createdBy uuid.UUID, req CreatePreAuthRequest) (*AccessGrant, error)
	getPreAuths       func(ctx context.Context, complexID, userID uuid.UUID) ([]*AccessGrant, error)
	createAccessGrant func(ctx context.Context, complexID, createdBy uuid.UUID, req CreateAccessGrantRequest) (*AccessGrant, error)
	getAccessGrant    func(ctx context.Context, complexID, userID, id uuid.UUID) (*AccessGrant, error)
	cancelAccessGrant func(ctx context.Context, complexID, userID, id uuid.UUID) error
	validateAccess    func(ctx context.Context, req ValidateAccessRequest) (*ValidateAccessResponse, error)
	unitGrants        func(ctx context.Context, unitID uuid.UUID, status *string) ([]*AccessGrant, error)
	qrStatus          func() qrsystem.Public
}

func newStub(t *testing.T) *stubService { return &stubService{t: t, calls: map[string]int{}} }

func (s *stubService) hit(name string, configured bool) {
	s.t.Helper()
	s.calls[name]++
	if !configured {
		s.t.Fatalf("llamada inesperada al servicio: %s", name)
	}
}
func (s *stubService) total() int {
	n := 0
	for _, c := range s.calls {
		n += c
	}
	return n
}
func (s *stubService) CreateQRCode(ctx context.Context, c, u uuid.UUID, r CreateQRCodeRequest) (*AccessGrantResponse, error) {
	s.hit("CreateQRCode", s.createQRCode != nil)
	return s.createQRCode(ctx, c, u, r)
}
func (s *stubService) CreateQRBatch(ctx context.Context, c, u uuid.UUID, r CreateQRBatchRequest) ([]*AccessGrant, error) {
	s.hit("CreateQRBatch", s.createQRBatch != nil)
	return s.createQRBatch(ctx, c, u, r)
}
func (s *stubService) GetQRCodes(ctx context.Context, c, u uuid.UUID) ([]*AccessGrant, error) {
	s.hit("GetQRCodes", s.getQRCodes != nil)
	return s.getQRCodes(ctx, c, u)
}
func (s *stubService) GrantSummary(ctx context.Context, c uuid.UUID) (*GrantSummary, error) {
	s.hit("GrantSummary", s.grantSummary != nil)
	return s.grantSummary(ctx, c)
}
func (s *stubService) CreatePreAuth(ctx context.Context, c, u uuid.UUID, r CreatePreAuthRequest) (*AccessGrant, error) {
	s.hit("CreatePreAuth", s.createPreAuth != nil)
	return s.createPreAuth(ctx, c, u, r)
}
func (s *stubService) GetPreAuthorizations(ctx context.Context, c, u uuid.UUID) ([]*AccessGrant, error) {
	s.hit("GetPreAuthorizations", s.getPreAuths != nil)
	return s.getPreAuths(ctx, c, u)
}
func (s *stubService) CreateAccessGrant(ctx context.Context, c, u uuid.UUID, r CreateAccessGrantRequest) (*AccessGrant, error) {
	s.hit("CreateAccessGrant", s.createAccessGrant != nil)
	return s.createAccessGrant(ctx, c, u, r)
}
func (s *stubService) GetAccessGrant(ctx context.Context, c, u, id uuid.UUID) (*AccessGrant, error) {
	s.hit("GetAccessGrant", s.getAccessGrant != nil)
	return s.getAccessGrant(ctx, c, u, id)
}
func (s *stubService) CancelAccessGrant(ctx context.Context, c, u, id uuid.UUID) error {
	s.hit("CancelAccessGrant", s.cancelAccessGrant != nil)
	return s.cancelAccessGrant(ctx, c, u, id)
}
func (s *stubService) ValidateAccess(ctx context.Context, r ValidateAccessRequest) (*ValidateAccessResponse, error) {
	s.hit("ValidateAccess", s.validateAccess != nil)
	return s.validateAccess(ctx, r)
}
func (s *stubService) GetUnitAccessGrants(ctx context.Context, id uuid.UUID, st *string) ([]*AccessGrant, error) {
	s.hit("GetUnitAccessGrants", s.unitGrants != nil)
	return s.unitGrants(ctx, id, st)
}
func (s *stubService) QRSystemStatus() qrsystem.Public {
	s.hit("QRSystemStatus", s.qrStatus != nil)
	return s.qrStatus()
}

// do ejecuta un handler con el contexto dado y parámetros de ruta chi opcionales.
func do(h http.HandlerFunc, method, target, body string, ctx context.Context, params map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if len(params) > 0 {
		rc := chi.NewRouteContext()
		for k, v := range params {
			rc.URLParams.Add(k, v)
		}
		ctx = context.WithValue(ctx, chi.RouteCtxKey, rc)
	}
	rec := httptest.NewRecorder()
	h(rec, req.WithContext(ctx))
	return rec
}

func errBody(t *testing.T, rec *httptest.ResponseRecorder) apperr.ErrorResponse {
	t.Helper()
	var e apperr.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("cuerpo de error no es JSON: %v (%s)", err, rec.Body.String())
	}
	return e
}

func fieldErrs(t *testing.T, rec *httptest.ResponseRecorder, field string) []string {
	t.Helper()
	raw, _ := errBody(t, rec).Error.Details["field_errors"].(map[string]any)
	var out []string
	if list, ok := raw[field].([]any); ok {
		for _, v := range list {
			out = append(out, fmt.Sprint(v))
		}
	}
	return out
}

func wantStatus(t *testing.T, rec *httptest.ResponseRecorder, status int, code apperr.ErrorCode) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status=%d (se esperaba %d), cuerpo=%s", rec.Code, status, rec.Body.String())
	}
	if code != "" {
		if got := errBody(t, rec).Error.Code; got != code {
			t.Fatalf("code=%s (se esperaba %s)", got, code)
		}
	}
}

// Caso AH-01 · mapeo de errores en Validate (a–g)
// Técnica: Tabla de decisión.
// Entrada: ComplexIDRequired; QRCodeRequired; VisitorIDRequired; Unsupported envuelto; otro; JSON
// malformado; request al servicio
// Debe hacer / esperado: 422 con X-Complex-ID / body; 500 sin filtrar la causa; 400 sin llamar al
// servicio; request decodificado llega al servicio
func TestAH01_MapeoDeErroresEnValidate(t *testing.T) {
	for _, tc := range []struct {
		id        string
		err       error
		status    int
		code      apperr.ErrorCode
		field     string
		wantInMsg string
	}{
		{"AH-01a ComplexIDRequired", ErrComplexIDRequired, 422, apperr.ErrCodeValidationFailed, "X-Complex-ID", "Missing or invalid complex ID"},
		{"AH-01b QRCodeRequired", ErrQRCodeRequired, 422, apperr.ErrCodeValidationFailed, "body", ErrQRCodeRequired.Error()},
		{"AH-01c VisitorIDRequired", ErrVisitorIDRequired, 422, apperr.ErrCodeValidationFailed, "body", ErrVisitorIDRequired.Error()},
		{"AH-01d UnsupportedMethod envuelto", fmt.Errorf("%w: FACE", ErrUnsupportedMethod), 422, apperr.ErrCodeValidationFailed, "body", "FACE"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s := newStub(t)
			s.validateAccess = func(context.Context, ValidateAccessRequest) (*ValidateAccessResponse, error) { return nil, tc.err }
			rec := do(NewHandler(s).ValidateAccessGrant, "POST", "/validate", `{"method":"QR"}`, context.Background(), nil)
			wantStatus(t, rec, tc.status, tc.code)
			if got := strings.Join(fieldErrs(t, rec, tc.field), "|"); !strings.Contains(got, tc.wantInMsg) {
				t.Fatalf("field_errors[%s]=%q, se esperaba %q", tc.field, got, tc.wantInMsg)
			}
		})
	}
	t.Run("AH-01e otro error -> 500 generico sin la causa", func(t *testing.T) {
		s := newStub(t)
		s.validateAccess = func(context.Context, ValidateAccessRequest) (*ValidateAccessResponse, error) {
			return nil, errors.New("pq: connection refused to 10.0.0.5")
		}
		rec := do(NewHandler(s).ValidateAccessGrant, "POST", "/validate", `{}`, context.Background(), nil)
		wantStatus(t, rec, 500, apperr.ErrCodeInternalError)
		if strings.Contains(rec.Body.String(), "pq:") || strings.Contains(rec.Body.String(), "10.0.0.5") {
			t.Fatalf("el cuerpo filtra la causa interna: %s", rec.Body.String())
		}
	})
	t.Run("AH-01f JSON malformado -> 400 y el servicio no se llama", func(t *testing.T) {
		s := newStub(t)
		rec := do(NewHandler(s).ValidateAccessGrant, "POST", "/validate", `{"method":`, context.Background(), nil)
		wantStatus(t, rec, 400, apperr.ErrCodeBadRequest)
		if s.total() != 0 {
			t.Fatal("el servicio no debe llamarse")
		}
	})
	t.Run("AH-01g el request decodificado llega al servicio", func(t *testing.T) {
		s := newStub(t)
		var got ValidateAccessRequest
		s.validateAccess = func(_ context.Context, r ValidateAccessRequest) (*ValidateAccessResponse, error) {
			got = r
			return &ValidateAccessResponse{Valid: false, Reason: ReasonNotFound}, nil
		}
		rec := do(NewHandler(s).ValidateAccessGrant, "POST", "/validate", `{"method":"PRE_AUTH","visitor_id_number":"77"}`, context.Background(), nil)
		if rec.Code != 200 || got.Method != MethodPreAuth || got.VisitorIDNumber == nil || *got.VisitorIDNumber != "77" {
			t.Fatalf("status=%d req=%+v", rec.Code, got)
		}
	})
}

// Caso AH-02 · 503 por QR caído (a–g)
// Técnica: Valores límite.
// Entrada: RetryAfter 1,5 s/1 s/1 ms/0/sin UnavailableError/envuelto 3 s; ruta de escritura
// Debe hacer / esperado: Retry-After 2/1/1/10/10/3; code QR_001; fallback: PRE_AUTH; causa
func TestAH02_QRCaidoDevuelve503ConRetryAfter(t *testing.T) {
	for _, tc := range []struct {
		id    string
		err   error
		retry string
		cause string
	}{
		{"AH-02a 1,5 s -> 2", &qrsystem.UnavailableError{Cause: qrsystem.CauseCircuitOpen, RetryAfter: 1500 * time.Millisecond}, "2", qrsystem.CauseCircuitOpen},
		{"AH-02b 1 s -> 1", &qrsystem.UnavailableError{Cause: qrsystem.CauseTimeout, RetryAfter: time.Second}, "1", qrsystem.CauseTimeout},
		{"AH-02c 1 ms -> 1", &qrsystem.UnavailableError{Cause: qrsystem.CauseOverloaded, RetryAfter: time.Millisecond}, "1", qrsystem.CauseOverloaded},
		{"AH-02d RetryAfter 0 -> 10 por defecto", &qrsystem.UnavailableError{Cause: qrsystem.CauseError}, "10", qrsystem.CauseError},
		{"AH-02e sin UnavailableError -> 10 y causa ERROR", qrsystem.ErrUnavailable, "10", qrsystem.CauseError},
		{"AH-02f envuelto con %w", fmt.Errorf("issue: %w", &qrsystem.UnavailableError{Cause: qrsystem.CauseTimeout, RetryAfter: 3 * time.Second}), "3", qrsystem.CauseTimeout},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s := newStub(t)
			s.validateAccess = func(context.Context, ValidateAccessRequest) (*ValidateAccessResponse, error) { return nil, tc.err }
			rec := do(NewHandler(s).ValidateAccessGrant, "POST", "/validate", `{"method":"QR"}`, context.Background(), nil)
			wantStatus(t, rec, 503, apperr.ErrCodeQRUnavailable)
			if got := rec.Header().Get("Retry-After"); got != tc.retry {
				t.Fatalf("Retry-After=%q, se esperaba %q", got, tc.retry)
			}
			d := errBody(t, rec).Error.Details
			if d["fallback"] != "PRE_AUTH" || d["cause"] != tc.cause || fmt.Sprint(d["retry_after_seconds"]) != tc.retry {
				t.Fatalf("details inesperados: %+v", d)
			}
		})
	}
	t.Run("AH-02g tambien en las rutas de escritura", func(t *testing.T) {
		s := newStub(t)
		down := &qrsystem.UnavailableError{Cause: qrsystem.CauseCircuitOpen, RetryAfter: time.Second}
		s.createQRCode = func(context.Context, uuid.UUID, uuid.UUID, CreateQRCodeRequest) (*AccessGrantResponse, error) {
			return nil, down
		}
		rec := do(NewHandler(s).CreateQRCode, "POST", "/qr", validQRBody(), ctxWith(uuid.New(), uuid.New()), nil)
		wantStatus(t, rec, 503, apperr.ErrCodeQRUnavailable)
	})
}

// Caso AH-04 · respondGrantWriteError
// Técnica: Tabla de decisión.
// Entrada: Denied, Denied envuelto, UnitNotFound, Mismatch, error con "pq:"
// Debe hacer / esperado: 403; 403; 404; 422 (unit_id); 500 sin "pq:" ni nombres de constraint
func TestAH04_RespondGrantWriteError(t *testing.T) {
	for _, tc := range []struct {
		id     string
		err    error
		status int
		code   apperr.ErrorCode
	}{
		{"AH-04a Denied -> 403", ErrUnitAccessDenied, 403, apperr.ErrCodePermissionDenied},
		{"AH-04b Denied envuelto -> 403", fmt.Errorf("x: %w", ErrUnitAccessDenied), 403, apperr.ErrCodePermissionDenied},
		{"AH-04c UnitNotFound -> 404", ErrUnitNotFound, 404, apperr.ErrCodeResourceNotFound},
		{"AH-04d Mismatch -> 422", ErrUnitComplexMismatch, 422, apperr.ErrCodeValidationFailed},
		{"AH-04e otro -> 500 sin filtrar", errors.New("pq: duplicate key value violates constraint access_grants_pkey"), 500, apperr.ErrCodeInternalError},
	} {
		t.Run(tc.id, func(t *testing.T) {
			rec := httptest.NewRecorder()
			respondGrantWriteError(rec, tc.err)
			wantStatus(t, rec, tc.status, tc.code)
			if tc.status == 500 && (strings.Contains(rec.Body.String(), "pq:") || strings.Contains(rec.Body.String(), "access_grants_pkey")) {
				t.Fatalf("el cuerpo filtra la causa: %s", rec.Body.String())
			}
			if tc.status == 422 {
				if got := fieldErrs(t, rec, "unit_id"); len(got) != 1 {
					t.Fatalf("field_errors.unit_id=%v", got)
				}
			}
		})
	}
}

func getGrantCases() []struct {
	id     string
	err    error
	status int
} {
	return []struct {
		id     string
		err    error
		status int
	}{
		{"b1 Denied -> 404 (anti-enumeracion)", ErrUnitAccessDenied, 404},
		{"b2 NotFound -> 404", ErrAccessGrantNotFound, 404},
		{"b3 otro error -> 500", errors.New("db down"), 500},
	}
}

// Caso AH-05 · GetAccessGrant
// Técnica: Partición en clases de equivalencia.
// Entrada: id no UUID; Denied; NotFound; otro; feliz; sin complejo/usuario
// Debe hacer / esperado: 400 sin servicio; 404 con cuerpo idéntico para Denied y NotFound; 500;
// 200 con args correctos; 422 / 401
func TestAH05_GetAccessGrant(t *testing.T) {
	user, cx, id := uuid.New(), uuid.New(), uuid.New()
	ctx := ctxWith(user, cx)
	params := map[string]string{"id": id.String()}

	t.Run("AH-05a id no UUID -> 400 sin llamar al servicio", func(t *testing.T) {
		s := newStub(t)
		rec := do(NewHandler(s).GetAccessGrant, "GET", "/x", "", ctx, map[string]string{"id": "abc"})
		wantStatus(t, rec, 400, apperr.ErrCodeBadRequest)
		if s.total() != 0 {
			t.Fatal("el servicio no debe llamarse")
		}
	})
	var bodies [][]byte
	for _, tc := range getGrantCases() {
		t.Run("AH-05"+tc.id, func(t *testing.T) {
			s := newStub(t)
			s.getAccessGrant = func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*AccessGrant, error) { return nil, tc.err }
			rec := do(NewHandler(s).GetAccessGrant, "GET", "/x", "", ctx, params)
			if rec.Code != tc.status {
				t.Fatalf("status=%d, cuerpo=%s", rec.Code, rec.Body)
			}
			if tc.status == 404 {
				bodies = append(bodies, rec.Body.Bytes())
				d := errBody(t, rec).Error.Details
				if d["resource"] != "AccessGrant" || d["id"] != id.String() {
					t.Fatalf("details: %+v", d)
				}
			}
		})
	}
	t.Run("AH-05c 403 y 404 son indistinguibles", func(t *testing.T) {
		if len(bodies) != 2 || !bytes.Equal(bodies[0], bodies[1]) {
			t.Fatalf("los cuerpos de Denied y NotFound deben ser identicos: %q vs %q", bodies[0], bodies[1])
		}
	})
	t.Run("AH-05d feliz: 200 y el servicio recibe complejo, usuario e id", func(t *testing.T) {
		s := newStub(t)
		s.getAccessGrant = func(_ context.Context, c, u, i uuid.UUID) (*AccessGrant, error) {
			if c != cx || u != user || i != id {
				t.Errorf("args: %v %v %v", c, u, i)
			}
			return &AccessGrant{ID: id, Status: StatusActive}, nil
		}
		rec := do(NewHandler(s).GetAccessGrant, "GET", "/x", "", ctx, params)
		var out struct{ Data AccessGrant }
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &out) != nil || out.Data.ID != id {
			t.Fatalf("status=%d cuerpo=%s", rec.Code, rec.Body)
		}
	})
	t.Run("AH-05e sin complejo -> 422; sin usuario -> 401", func(t *testing.T) {
		s := newStub(t)
		wantStatus(t, do(NewHandler(s).GetAccessGrant, "GET", "/x", "", ctxWith(user, uuid.Nil), params), 422, apperr.ErrCodeValidationFailed)
		wantStatus(t, do(NewHandler(s).GetAccessGrant, "GET", "/x", "", ctxWith(uuid.Nil, cx), params), 401, apperr.ErrCodeAuthFailed)
		if s.total() != 0 {
			t.Fatal("el servicio no debe llamarse")
		}
	})
}

// Caso AH-06 · CancelAccessGrant (a, b, d, e)
// Técnica: Partición en clases de equivalencia.
// Entrada: igual
// Debe hacer / esperado: 400; 404/404/500; 200 con mensaje; 422/401
func TestAH06_CancelAccessGrant(t *testing.T) {
	user, cx, id := uuid.New(), uuid.New(), uuid.New()
	ctx := ctxWith(user, cx)
	params := map[string]string{"id": id.String()}

	t.Run("AH-06a id no UUID -> 400", func(t *testing.T) {
		s := newStub(t)
		wantStatus(t, do(NewHandler(s).CancelAccessGrant, "PATCH", "/x", "", ctx, map[string]string{"id": "x"}), 400, apperr.ErrCodeBadRequest)
		if s.total() != 0 {
			t.Fatal("el servicio no debe llamarse")
		}
	})
	for _, tc := range getGrantCases() {
		t.Run("AH-06"+tc.id, func(t *testing.T) {
			s := newStub(t)
			s.cancelAccessGrant = func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error { return tc.err }
			rec := do(NewHandler(s).CancelAccessGrant, "PATCH", "/x", "", ctx, params)
			if rec.Code != tc.status {
				t.Fatalf("status=%d, cuerpo=%s", rec.Code, rec.Body)
			}
		})
	}
	t.Run("AH-06d feliz: 200 con mensaje", func(t *testing.T) {
		s := newStub(t)
		s.cancelAccessGrant = func(_ context.Context, c, u, i uuid.UUID) error {
			if c != cx || u != user || i != id {
				t.Errorf("args: %v %v %v", c, u, i)
			}
			return nil
		}
		rec := do(NewHandler(s).CancelAccessGrant, "PATCH", "/x", "", ctx, params)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Access grant cancelled successfully") {
			t.Fatalf("status=%d cuerpo=%s", rec.Code, rec.Body)
		}
	})
	t.Run("AH-06e sin complejo -> 422; sin usuario -> 401", func(t *testing.T) {
		s := newStub(t)
		wantStatus(t, do(NewHandler(s).CancelAccessGrant, "PATCH", "/x", "", ctxWith(user, uuid.Nil), params), 422, apperr.ErrCodeValidationFailed)
		wantStatus(t, do(NewHandler(s).CancelAccessGrant, "PATCH", "/x", "", ctxWith(uuid.Nil, cx), params), 401, apperr.ErrCodeAuthFailed)
		if s.total() != 0 {
			t.Fatal("el servicio no debe llamarse")
		}
	})
}

func validQRBody() string {
	return fmt.Sprintf(`{"unit_id":%q,"valid_until":%q}`, uuid.New(), time.Now().Add(2*time.Hour).Format(time.RFC3339))
}

// Caso AH-07 · precondiciones de las 4 escrituras (a–h)
// Técnica: Tabla de decisión.
// Entrada: sin complejo; sin usuario; JSON malformado; request inválido; cuerpo >1 MiB; lecturas;
// errores del servicio
// Debe hacer / esperado: 422; 401; 400; 422 con campos y servicio 0 llamadas; 413; traducción de
// errores
func TestAH07_PrecondicionesDeEscritura(t *testing.T) {
	user, cx := uuid.New(), uuid.New()
	handlers := map[string]func(h *Handler) http.HandlerFunc{
		"CreateQRCode":      func(h *Handler) http.HandlerFunc { return h.CreateQRCode },
		"CreateQRBatch":     func(h *Handler) http.HandlerFunc { return h.CreateQRBatch },
		"CreatePreAuth":     func(h *Handler) http.HandlerFunc { return h.CreatePreAuth },
		"CreateAccessGrant": func(h *Handler) http.HandlerFunc { return h.CreateAccessGrant },
	}
	invalid := `{"unit_id":"` + uuid.NewString() + `"}` // sin valid_until -> falla validate()
	for name, pick := range handlers {
		t.Run("AH-07a sin complejo -> 422 "+name, func(t *testing.T) {
			s := newStub(t)
			rec := do(pick(NewHandler(s)), "POST", "/x", validQRBody(), ctxWith(user, uuid.Nil), nil)
			wantStatus(t, rec, 422, apperr.ErrCodeValidationFailed)
			if s.total() != 0 {
				t.Fatal("el servicio no debe llamarse")
			}
		})
		t.Run("AH-07b sin usuario -> 401 "+name, func(t *testing.T) {
			s := newStub(t)
			rec := do(pick(NewHandler(s)), "POST", "/x", validQRBody(), ctxWith(uuid.Nil, cx), nil)
			wantStatus(t, rec, 401, apperr.ErrCodeAuthFailed)
			if s.total() != 0 {
				t.Fatal("el servicio no debe llamarse")
			}
		})
		t.Run("AH-07c JSON malformado -> 400 "+name, func(t *testing.T) {
			s := newStub(t)
			rec := do(pick(NewHandler(s)), "POST", "/x", `{"unit_id":`, ctxWith(user, cx), nil)
			wantStatus(t, rec, 400, apperr.ErrCodeBadRequest)
			if s.total() != 0 {
				t.Fatal("el servicio no debe llamarse")
			}
		})
		t.Run("AH-07d request invalido -> 422 y el servicio NO se llama "+name, func(t *testing.T) {
			s := newStub(t)
			rec := do(pick(NewHandler(s)), "POST", "/x", invalid, ctxWith(user, cx), nil)
			wantStatus(t, rec, 422, apperr.ErrCodeValidationFailed)
			if len(fieldErrs(t, rec, "valid_until")) == 0 && len(fieldErrs(t, rec, "quantity")) == 0 && len(fieldErrs(t, rec, "visitor_name")) == 0 {
				t.Fatalf("debe detallar los campos invalidos: %s", rec.Body)
			}
			if s.total() != 0 {
				t.Fatal("el servicio no debe llamarse con un request invalido")
			}
		})
		t.Run("AH-07e cuerpo mayor al limite -> 413 "+name, func(t *testing.T) {
			s := newStub(t)
			big := `{"visitor_name":"` + strings.Repeat("x", int(apperr.MaxRequestBodyBytes)+10) + `"}`
			wantStatus(t, do(pick(NewHandler(s)), "POST", "/x", big, ctxWith(user, cx), nil), 413, apperr.ErrCodePayloadTooLarge)
			if s.total() != 0 {
				t.Fatal("el servicio no debe llamarse")
			}
		})
	}
	t.Run("AH-07f lecturas sin complejo -> 422 / sin usuario -> 401", func(t *testing.T) {
		reads := map[string]func(h *Handler) http.HandlerFunc{
			"GetQRCodes":           func(h *Handler) http.HandlerFunc { return h.GetQRCodes },
			"GetPreAuthorizations": func(h *Handler) http.HandlerFunc { return h.GetPreAuthorizations },
		}
		for name, pick := range reads {
			s := newStub(t)
			wantStatus(t, do(pick(NewHandler(s)), "GET", "/x", "", ctxWith(user, uuid.Nil), nil), 422, apperr.ErrCodeValidationFailed)
			wantStatus(t, do(pick(NewHandler(s)), "GET", "/x", "", ctxWith(uuid.Nil, cx), nil), 401, apperr.ErrCodeAuthFailed)
			if s.total() != 0 {
				t.Fatalf("%s: el servicio no debe llamarse", name)
			}
		}
		s := newStub(t)
		wantStatus(t, do(NewHandler(s).GetGrantSummary, "GET", "/x", "", ctxWith(user, uuid.Nil), nil), 422, apperr.ErrCodeValidationFailed)
	})
	t.Run("AH-07h error del servicio en CreatePreAuth y CreateAccessGrant se traduce", func(t *testing.T) {
		s := newStub(t)
		s.createPreAuth = func(context.Context, uuid.UUID, uuid.UUID, CreatePreAuthRequest) (*AccessGrant, error) {
			return nil, ErrUnitNotFound
		}
		s.createAccessGrant = func(context.Context, uuid.UUID, uuid.UUID, CreateAccessGrantRequest) (*AccessGrant, error) {
			return nil, ErrUnitComplexMismatch
		}
		until := time.Now().Add(time.Hour).Format(time.RFC3339)
		pre := fmt.Sprintf(`{"unit_id":%q,"visitor_name":"L","visitor_id_number":"1","visitor_type":"GUEST","valid_from":%q,"valid_until":%q}`, uuid.New(), time.Now().Format(time.RFC3339), until)
		ag := fmt.Sprintf(`{"unit_id":%q,"method":"QR","visitor_type":"GUEST","visitor_name":"L","valid_until":%q}`, uuid.New(), until)
		wantStatus(t, do(NewHandler(s).CreatePreAuth, "POST", "/x", pre, ctxWith(user, cx), nil), 404, apperr.ErrCodeResourceNotFound)
		wantStatus(t, do(NewHandler(s).CreateAccessGrant, "POST", "/x", ag, ctxWith(user, cx), nil), 422, apperr.ErrCodeValidationFailed)
	})
	t.Run("AH-07g error del servicio en las escrituras se traduce", func(t *testing.T) {
		s := newStub(t)
		s.createQRBatch = func(context.Context, uuid.UUID, uuid.UUID, CreateQRBatchRequest) ([]*AccessGrant, error) {
			return nil, ErrUnitAccessDenied
		}
		body := fmt.Sprintf(`{"unit_id":%q,"quantity":3,"valid_from":%q,"valid_until":%q}`, uuid.New(), time.Now().Format(time.RFC3339), time.Now().Add(time.Hour).Format(time.RFC3339))
		wantStatus(t, do(NewHandler(s).CreateQRBatch, "POST", "/x", body, ctxWith(user, cx), nil), 403, apperr.ErrCodePermissionDenied)
	})
}

// Caso AH-08 · GetUnitAccessGrants (chi.RouteContext)
// Técnica: Partición en clases de equivalencia.
// Entrada: unitId inválido; con/sin status; error
// Debe hacer / esperado: 400; puntero/nil; 500
func TestAH08_GetUnitAccessGrants(t *testing.T) {
	unit := uuid.New()
	t.Run("AH-08a unitId invalido -> 400", func(t *testing.T) {
		s := newStub(t)
		wantStatus(t, do(NewHandler(s).GetUnitAccessGrants, "GET", "/x", "", context.Background(), map[string]string{"unitId": "no-uuid"}), 400, apperr.ErrCodeBadRequest)
		if s.total() != 0 {
			t.Fatal("el servicio no debe llamarse")
		}
	})
	t.Run("AH-08b sin status -> nil; con status -> puntero", func(t *testing.T) {
		for _, tc := range []struct {
			target string
			want   *string
		}{{"/x", nil}, {"/x?status=ACTIVE", strp("ACTIVE")}} {
			s := newStub(t)
			var gotStatus *string
			s.unitGrants = func(_ context.Context, id uuid.UUID, st *string) ([]*AccessGrant, error) {
				if id != unit {
					t.Errorf("unit=%v", id)
				}
				gotStatus = st
				return []*AccessGrant{{ID: uuid.New()}}, nil
			}
			rec := do(NewHandler(s).GetUnitAccessGrants, "GET", tc.target, "", context.Background(), map[string]string{"unitId": unit.String()})
			if rec.Code != 200 || (gotStatus == nil) != (tc.want == nil) || (tc.want != nil && *gotStatus != *tc.want) {
				t.Fatalf("%s: status=%d got=%v", tc.target, rec.Code, gotStatus)
			}
		}
	})
	t.Run("AH-08c error del servicio -> 500", func(t *testing.T) {
		s := newStub(t)
		s.unitGrants = func(context.Context, uuid.UUID, *string) ([]*AccessGrant, error) { return nil, errors.New("db") }
		wantStatus(t, do(NewHandler(s).GetUnitAccessGrants, "GET", "/x", "", context.Background(), map[string]string{"unitId": unit.String()}), 500, apperr.ErrCodeInternalError)
	})
}

// Caso AH-09 · casos felices (a–i)
// Técnica: Partición en clases de equivalencia.
// Entrada: las 4 creaciones, validate, listados, summary, qr-system
// Debe hacer / esperado: 201 / 200 con el cuerpo del stub; validate de negocio denegado = 200;
// listados con error = 500
func TestAH09_CasosFelices(t *testing.T) {
	user, cx := uuid.New(), uuid.New()
	ctx := ctxWith(user, cx)
	id := uuid.New()
	grant := &AccessGrant{ID: id, Status: StatusActive}
	list := func(context.Context, uuid.UUID, uuid.UUID) ([]*AccessGrant, error) { return []*AccessGrant{grant}, nil }

	wantData := func(t *testing.T, rec *httptest.ResponseRecorder, status int) {
		t.Helper()
		if rec.Code != status || !strings.Contains(rec.Body.String(), id.String()) || !strings.Contains(rec.Body.String(), `"success":true`) {
			t.Fatalf("status=%d (esperado %d) cuerpo=%s", rec.Code, status, rec.Body)
		}
	}
	t.Run("AH-09a CreateQRCode -> 201", func(t *testing.T) {
		s := newStub(t)
		s.createQRCode = func(_ context.Context, c, u uuid.UUID, r CreateQRCodeRequest) (*AccessGrantResponse, error) {
			if c != cx || u != user || r.UnitID == uuid.Nil {
				t.Errorf("args: %v %v %+v", c, u, r)
			}
			return &AccessGrantResponse{AccessGrant: *grant, UnitName: "A-101"}, nil
		}
		wantData(t, do(NewHandler(s).CreateQRCode, "POST", "/x", validQRBody(), ctx, nil), 201)
	})
	t.Run("AH-09b CreateQRBatch -> 201", func(t *testing.T) {
		s := newStub(t)
		s.createQRBatch = func(_ context.Context, _, _ uuid.UUID, r CreateQRBatchRequest) ([]*AccessGrant, error) {
			if r.Quantity != 5 {
				t.Errorf("quantity=%d", r.Quantity)
			}
			return []*AccessGrant{grant}, nil
		}
		body := fmt.Sprintf(`{"unit_id":%q,"quantity":5,"valid_from":%q,"valid_until":%q}`, uuid.New(), time.Now().Format(time.RFC3339), time.Now().Add(time.Hour).Format(time.RFC3339))
		wantData(t, do(NewHandler(s).CreateQRBatch, "POST", "/x", body, ctx, nil), 201)
	})
	t.Run("AH-09c CreatePreAuth -> 201", func(t *testing.T) {
		s := newStub(t)
		s.createPreAuth = func(context.Context, uuid.UUID, uuid.UUID, CreatePreAuthRequest) (*AccessGrant, error) {
			return grant, nil
		}
		body := fmt.Sprintf(`{"unit_id":%q,"visitor_name":"Luis","visitor_id_number":"1","visitor_type":"GUEST","valid_from":%q,"valid_until":%q}`, uuid.New(), time.Now().Format(time.RFC3339), time.Now().Add(time.Hour).Format(time.RFC3339))
		wantData(t, do(NewHandler(s).CreatePreAuth, "POST", "/x", body, ctx, nil), 201)
	})
	t.Run("AH-09d CreateAccessGrant -> 201", func(t *testing.T) {
		s := newStub(t)
		s.createAccessGrant = func(context.Context, uuid.UUID, uuid.UUID, CreateAccessGrantRequest) (*AccessGrant, error) {
			return grant, nil
		}
		body := fmt.Sprintf(`{"unit_id":%q,"method":"QR","visitor_type":"GUEST","visitor_name":"Luis","valid_until":%q}`, uuid.New(), time.Now().Add(time.Hour).Format(time.RFC3339))
		wantData(t, do(NewHandler(s).CreateAccessGrant, "POST", "/x", body, ctx, nil), 201)
	})
	t.Run("AH-09e ValidateAccessGrant -> 200 con el cuerpo del stub", func(t *testing.T) {
		s := newStub(t)
		s.validateAccess = func(context.Context, ValidateAccessRequest) (*ValidateAccessResponse, error) {
			return &ValidateAccessResponse{Valid: true, Message: "Access granted", AccessGrant: grant}, nil
		}
		rec := do(NewHandler(s).ValidateAccessGrant, "POST", "/x", `{"method":"QR","qr_code":"abc"}`, ctx, nil)
		wantData(t, rec, 200)
		if !strings.Contains(rec.Body.String(), `"valid":true`) || !strings.Contains(rec.Body.String(), "Access granted") {
			t.Fatalf("cuerpo: %s", rec.Body)
		}
	})
	t.Run("AH-09f Validate con denegacion de negocio sigue siendo 200", func(t *testing.T) {
		s := newStub(t)
		s.validateAccess = func(context.Context, ValidateAccessRequest) (*ValidateAccessResponse, error) {
			return &ValidateAccessResponse{Valid: false, Reason: ReasonExpired}, nil
		}
		rec := do(NewHandler(s).ValidateAccessGrant, "POST", "/x", `{"method":"QR","qr_code":"abc"}`, ctx, nil)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), ReasonExpired) {
			t.Fatalf("status=%d cuerpo=%s", rec.Code, rec.Body)
		}
	})
	t.Run("AH-09g listados -> 200", func(t *testing.T) {
		s := newStub(t)
		s.getQRCodes, s.getPreAuths = list, list
		s.grantSummary = func(_ context.Context, c uuid.UUID) (*GrantSummary, error) {
			if c != cx {
				t.Errorf("complex=%v", c)
			}
			return &GrantSummary{Total: 3, Active: 2}, nil
		}
		wantData(t, do(NewHandler(s).GetQRCodes, "GET", "/x", "", ctx, nil), 200)
		wantData(t, do(NewHandler(s).GetPreAuthorizations, "GET", "/x", "", ctx, nil), 200)
		if rec := do(NewHandler(s).GetGrantSummary, "GET", "/x", "", ctx, nil); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"total":3`) {
			t.Fatalf("summary: %d %s", rec.Code, rec.Body)
		}
	})
	t.Run("AH-09h errores de listado -> 500", func(t *testing.T) {
		s := newStub(t)
		boom := func(context.Context, uuid.UUID, uuid.UUID) ([]*AccessGrant, error) { return nil, errors.New("db") }
		s.getQRCodes, s.getPreAuths = boom, boom
		s.grantSummary = func(context.Context, uuid.UUID) (*GrantSummary, error) { return nil, errors.New("db") }
		wantStatus(t, do(NewHandler(s).GetQRCodes, "GET", "/x", "", ctx, nil), 500, apperr.ErrCodeInternalError)
		wantStatus(t, do(NewHandler(s).GetPreAuthorizations, "GET", "/x", "", ctx, nil), 500, apperr.ErrCodeInternalError)
		wantStatus(t, do(NewHandler(s).GetGrantSummary, "GET", "/x", "", ctx, nil), 500, apperr.ErrCodeInternalError)
	})
	t.Run("AH-09i QRSystemStatus -> 200 con el estado", func(t *testing.T) {
		s := newStub(t)
		s.qrStatus = func() qrsystem.Public {
			return qrsystem.Public{Available: false, State: "OPEN", RetryAfterSeconds: 4, Fallback: "PRE_AUTH"}
		}
		rec := do(NewHandler(s).QRSystemStatus, "GET", "/x", "", ctx, nil)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"state":"OPEN"`) || !strings.Contains(rec.Body.String(), `"fallback":"PRE_AUTH"`) {
			t.Fatalf("status=%d cuerpo=%s", rec.Code, rec.Body)
		}
	})
}
