package accesslog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"controldeaccesos/internal/audit"

	"github.com/google/uuid"
)

func residentBody(movement string) map[string]any {
	return map[string]any{"client_id": uuid.New().String(), "movement": movement}
}

// Caso AL-08a1-6 · AlreadyInside/Outside, ClientIDTaken, NotResident, BD, error envuelto
// Técnica: Tabla de decisión.
// Debe hacer / esperado: 409+DENIED, 409, 403, 500
// AlreadyInside/Outside 409 + auditoría DENIED; ClientIDTaken 409 sin auditar; NotResident 403; otro → 500.
func TestAL08a_ErroresDeMovimientoDeResidente(t *testing.T) {
	cases := []struct {
		id        string
		err       error
		status    int
		code      string
		audited   bool
		auditWhat string
	}{
		{"AL-08a1 ya adentro", ErrAlreadyInside, http.StatusConflict, "RES_002", true, "resident_movement.entry"},
		{"AL-08a2 ya afuera", ErrAlreadyOutside, http.StatusConflict, "RES_002", true, "resident_movement.entry"},
		{"AL-08a3 client_id tomado", ErrClientIDTaken, http.StatusConflict, "RES_002", false, ""},
		{"AL-08a4 no residente", ErrNotResident, http.StatusForbidden, "PERM_001", false, ""},
		{"AL-08a5 error de BD", errors.New("pq: deadlock"), http.StatusInternalServerError, "SYS_001", false, ""},
		{"AL-08a6 error envuelto", fmt.Errorf("tx: %w", ErrAlreadyInside), http.StatusConflict, "RES_002", true, "resident_movement.entry"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			f := newFixture()
			f.repo.residentErr = tc.err
			rec := f.do(t, f.ctx(), f.h.ResidentMovement, http.MethodPost, "/x", residentBody(MovementEntry))
			if rec.Code != tc.status {
				t.Fatalf("status=%d, esperaba %d (%s)", rec.Code, tc.status, rec.Body.String())
			}
			if got := decode(t, rec).Error.Code; got != tc.code {
				t.Fatalf("code=%q, esperaba %q", got, tc.code)
			}
			if !tc.audited {
				if len(f.aud.entries) != 0 {
					t.Fatalf("no debia auditar: %+v", f.aud.entries)
				}
				return
			}
			if len(f.aud.entries) != 1 {
				t.Fatalf("auditorias=%d, esperaba 1", len(f.aud.entries))
			}
			e := f.aud.entries[0]
			if e.Result != audit.ResultDenied || e.Action != tc.auditWhat || e.ResourceID != nil || e.Reason == "" {
				t.Fatalf("auditoria DENIED inesperada: %+v", e)
			}
		})
	}
}

// Caso AL-08b1/2 · created true / false
// Técnica: Transición de estados.
// Debe hacer / esperado: 201+ALLOWED / 200 sin auditar
// created=false (client_id repetido) → 200 sin auditar; el repo recibe usuario/complejo del contexto.
func TestAL08b_MovimientoDeResidenteNuevoYRepetido(t *testing.T) {
	t.Run("AL-08b1 nuevo", func(t *testing.T) {
		f := newFixture()
		f.repo.residentLog, f.repo.residentNew = sampleLog(MovementExit, ResultAuthorized), true
		rec := f.do(t, f.ctx(), f.h.ResidentMovement, http.MethodPost, "/x", residentBody(MovementExit))
		if rec.Code != http.StatusCreated {
			t.Fatalf("status=%d (%s)", rec.Code, rec.Body.String())
		}
		if len(f.aud.entries) != 1 {
			t.Fatalf("auditorias=%d", len(f.aud.entries))
		}
		e := f.aud.entries[0]
		if e.Result != audit.ResultAllowed || e.Action != "resident_movement.exit" || e.ResourceID == nil || *e.ResourceID != f.repo.residentLog.ID || e.Reason != "" {
			t.Fatalf("auditoria inesperada: %+v", e)
		}
		if f.repo.gotResidentID != f.userID || f.repo.gotComplexID != f.complexID || f.repo.gotResidentRq.Movement != MovementExit {
			t.Fatalf("argumentos al repo incorrectos")
		}
	})
	t.Run("AL-08b2 repetido", func(t *testing.T) {
		f := newFixture()
		f.repo.residentLog, f.repo.residentNew = sampleLog(MovementEntry, ResultAuthorized), false
		rec := f.do(t, f.ctx(), f.h.ResidentMovement, http.MethodPost, "/x", residentBody(MovementEntry))
		if rec.Code != http.StatusOK || len(f.aud.entries) != 0 {
			t.Fatalf("status=%d auditorias=%d, esperaba 200 y 0", rec.Code, len(f.aud.entries))
		}
	})
}

// Caso AL-08c · técnica: TD · condición: precondiciones de ResidentMovement; el repo NO se llama.
func TestAL08c_PrecondicionesDeMovimientoDeResidente(t *testing.T) {
	f := newFixture()
	cases := []struct {
		id     string
		ctx    context.Context
		body   any
		status int
	}{
		{"AL-08c1 sin complejo", context.Background(), residentBody(MovementEntry), http.StatusUnprocessableEntity},
		{"AL-08c2 sin usuario", ctxSoloComplejo(f), residentBody(MovementEntry), http.StatusUnauthorized},
		{"AL-08c3 JSON malformado", f.ctx(), "{", http.StatusBadRequest},
		{"AL-08c4 movement ATTEMPT", f.ctx(), residentBody(MovementAttempt), http.StatusUnprocessableEntity},
		{"AL-08c5 sin client_id", f.ctx(), map[string]any{"movement": "ENTRY"}, http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			rec := f.do(t, tc.ctx, f.h.ResidentMovement, http.MethodPost, "/x", tc.body)
			if rec.Code != tc.status {
				t.Fatalf("status=%d, esperaba %d (%s)", rec.Code, tc.status, rec.Body.String())
			}
			if f.repo.residentCalls != 0 || len(f.aud.entries) != 0 {
				t.Fatalf("no debia llamarse al repo ni auditar")
			}
		})
	}
}

func ctxSoloComplejo(f *fixture) context.Context {
	return context.WithValue(context.Background(), complexKey(), f.complexID.String())
}

// Caso AL-08d · técnica: EG · condición: auditoría nil o con error no cambia la respuesta del movimiento.
func TestAL08d_AuditoriaOpcionalEnMovimientoDeResidente(t *testing.T) {
	f := newFixture()
	f.h.auditor = nil
	f.repo.residentErr = ErrAlreadyOutside
	if rec := f.do(t, f.ctx(), f.h.ResidentMovement, http.MethodPost, "/x", residentBody(MovementExit)); rec.Code != http.StatusConflict {
		t.Fatalf("sin auditor: status=%d", rec.Code)
	}

	f = newFixture()
	f.aud.err = errors.New("db caida")
	spy := &logSpy{}
	f.h.logger = newSpyLogger(spy)
	f.repo.residentLog, f.repo.residentNew = sampleLog(MovementEntry, ResultAuthorized), true
	if rec := f.do(t, f.ctx(), f.h.ResidentMovement, http.MethodPost, "/x", residentBody(MovementEntry)); rec.Code != http.StatusCreated {
		t.Fatalf("auditoria fallida: status=%d", rec.Code)
	}
	if len(spy.msgs) != 1 {
		t.Fatalf("debia registrar el fallo de auditoria: %v", spy.msgs)
	}
}

// Caso AL-09a1-6 · limit 0/1/100/101/abc, def. 20
// Técnica: Valores límite.
// Debe hacer / esperado: 200 con limit capturado / 422
func TestAL09a_LimiteDeHistorialDeResidente(t *testing.T) {
	cases := []struct {
		id    string
		query string
		ok    bool
		want  int
	}{
		{"AL-09a1 por defecto", "", true, 20},
		{"AL-09a2 limit=0", "?limit=0", false, 0},
		{"AL-09a3 limit=1", "?limit=1", true, 1},
		{"AL-09a4 limit=100", "?limit=100", true, 100},
		{"AL-09a5 limit=101", "?limit=101", false, 0},
		{"AL-09a6 limit=abc", "?limit=abc", false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			f := newFixture()
			rec := f.do(t, f.ctx(), f.h.ResidentMe, http.MethodGet, "/x"+tc.query, nil)
			if tc.ok {
				if rec.Code != http.StatusOK || f.repo.historyCalls != 1 || f.repo.gotLimit != tc.want {
					t.Fatalf("status=%d calls=%d limit=%d", rec.Code, f.repo.historyCalls, f.repo.gotLimit)
				}
				if f.repo.gotResidentID != f.userID {
					t.Fatalf("solo debe consultar al propio usuario del contexto")
				}
				return
			}
			if rec.Code != http.StatusUnprocessableEntity || f.repo.historyCalls != 0 {
				t.Fatalf("status=%d calls=%d", rec.Code, f.repo.historyCalls)
			}
		})
	}
}

// Caso AL-09b1-3 · estado según último movimiento
// Técnica: Transición de estados.
// Debe hacer / esperado: UNKNOWN sin since / INSIDE / OUTSIDE
func TestAL09b_EstadoDelResidenteSegunUltimoMovimiento(t *testing.T) {
	type status struct {
		Status    string       `json:"status"`
		Since     *time.Time   `json:"since"`
		Movements []*AccessLog `json:"movements"`
	}
	run := func(t *testing.T, hist []*AccessLog) status {
		f := newFixture()
		f.repo.historyOut = hist
		rec := f.do(t, f.ctx(), f.h.ResidentMe, http.MethodGet, "/x", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d", rec.Code)
		}
		var s status
		if err := json.Unmarshal(decode(t, rec).Data, &s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	t.Run("AL-09b1 sin historial", func(t *testing.T) {
		s := run(t, nil)
		if s.Status != StatusUnknown || s.Since != nil {
			t.Fatalf("estado inesperado: %+v", s)
		}
	})
	t.Run("AL-09b2 ultimo ENTRY", func(t *testing.T) {
		l := sampleLog(MovementEntry, ResultAuthorized)
		l.OccurredAt = t0.Add(-time.Hour)
		s := run(t, []*AccessLog{l, sampleLog(MovementExit, ResultAuthorized)})
		if s.Status != StatusInside || s.Since == nil || !s.Since.Equal(l.OccurredAt) || len(s.Movements) != 2 {
			t.Fatalf("estado inesperado: %+v", s)
		}
	})
	t.Run("AL-09b3 ultimo EXIT", func(t *testing.T) {
		if s := run(t, []*AccessLog{sampleLog(MovementExit, ResultAuthorized)}); s.Status != StatusOutside {
			t.Fatalf("estado=%q", s.Status)
		}
	})
}

// Caso AL-09c · técnica: TD/EG · condición: ResidentMe sin complejo → 422, sin usuario → 401, fallo del repo → 500.
func TestAL09c_PrecondicionesYFalloDeResidentMe(t *testing.T) {
	f := newFixture()
	if rec := f.do(t, context.Background(), f.h.ResidentMe, http.MethodGet, "/x", nil); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("sin complejo: %d", rec.Code)
	}
	if rec := f.do(t, ctxSoloComplejo(f), f.h.ResidentMe, http.MethodGet, "/x", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin usuario: %d", rec.Code)
	}
	if f.repo.historyCalls != 0 {
		t.Fatal("el repo no debia llamarse")
	}
	f.repo.historyErr = errors.New("boom")
	if rec := f.do(t, f.ctx(), f.h.ResidentMe, http.MethodGet, "/x", nil); rec.Code != http.StatusInternalServerError {
		t.Fatalf("fallo repo: %d", rec.Code)
	}
}

// Caso AL-09c, AL-09d · precondiciones/fallo de ResidentMe; Presence
// Técnica: Tabla de decisión, Adivinación de errores.
// Debe hacer / esperado: 422/401/500; lista del complejo
// sin complejo → 422; fallo del repo → 500.
func TestAL09d_Presencia(t *testing.T) {
	f := newFixture()
	f.repo.presenceOut = []*Presence{{UserID: uuid.New(), Name: "Ana", UnitLabel: "A-101", Since: t0}}
	rec := f.do(t, f.ctx(), f.h.Presence, http.MethodGet, "/x", nil)
	if rec.Code != http.StatusOK || f.repo.gotComplexID != f.complexID {
		t.Fatalf("status=%d complejo=%v", rec.Code, f.repo.gotComplexID)
	}
	var got []Presence
	if err := json.Unmarshal(decode(t, rec).Data, &got); err != nil || len(got) != 1 || got[0].Name != "Ana" {
		t.Fatalf("cuerpo inesperado: %v %+v", err, got)
	}

	if rec := f.do(t, context.Background(), f.h.Presence, http.MethodGet, "/x", nil); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("sin complejo: %d", rec.Code)
	}
	f.repo.presenceErr = errors.New("boom")
	if rec := f.do(t, f.ctx(), f.h.Presence, http.MethodGet, "/x", nil); rec.Code != http.StatusInternalServerError {
		t.Fatalf("fallo repo: %d", rec.Code)
	}
}
