package accesslog

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// Caso AL-02a-f · 3 movimientos × 2 resultados
// Técnica: Tabla de decisión.
// Debe hacer / esperado: solo ENTRY+AUTH, EXIT+AUTH, ATTEMPT+DENIED válidos; inválidos dan
// exactamente 1 error en result
// solo ENTRY+AUTHORIZED, EXIT+AUTHORIZED y ATTEMPT+DENIED son válidas.
func TestAL02_TablaMovimientoPorResultado(t *testing.T) {
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		id       string
		movement string
		result   string
		valid    bool
	}{
		{"AL-02a", MovementEntry, ResultAuthorized, true},
		{"AL-02b", MovementEntry, ResultDenied, false},
		{"AL-02c", MovementExit, ResultAuthorized, true},
		{"AL-02d", MovementExit, ResultDenied, false},
		{"AL-02e", MovementAttempt, ResultAuthorized, false},
		{"AL-02f", MovementAttempt, ResultDenied, true},
	}
	for _, tc := range cases {
		t.Run(tc.id+" "+tc.movement+"+"+tc.result, func(t *testing.T) {
			r := validReq()
			r.Movement, r.Result = tc.movement, tc.result
			errs := r.validate(now)
			if tc.valid {
				if len(errs) != 0 {
					t.Fatalf("combinacion valida rechazada: %v", errs)
				}
				return
			}
			if len(errs) != 1 || len(errs["result"]) != 1 {
				t.Fatalf("esperaba exactamente un error en result, obtuve %v", errs)
			}
		})
	}
}

// Caso AL-03a/b · 200 bytes ok / 201 error × 4 campos
// Técnica: Valores límite.
// Debe hacer / esperado: error solo en el campo
// los cuatro campos de texto; 200 es válido y 201 es inválido.
// (La variante multibyte de F10 es solo una sonda desechable.)
func TestAL03_LongitudMaximaDeTextoEnBytes(t *testing.T) {
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	fields := []struct {
		name string
		set  func(r *CreateRequest, v string)
	}{
		{"visitor_name", func(r *CreateRequest, v string) { r.VisitorName = v }},
		{"unit_label", func(r *CreateRequest, v string) { r.UnitLabel = v }},
		{"visitor_type", func(r *CreateRequest, v string) { r.VisitorType = &v }},
		{"vehicle", func(r *CreateRequest, v string) { r.Vehicle = &v }},
	}
	for _, f := range fields {
		t.Run("AL-03a "+f.name+" 200 bytes", func(t *testing.T) {
			r := validReq()
			f.set(&r, strings.Repeat("a", 200))
			if errs := r.validate(now); len(errs) != 0 {
				t.Fatalf("200 bytes debe ser valido: %v", errs)
			}
		})
		t.Run("AL-03b "+f.name+" 201 bytes", func(t *testing.T) {
			r := validReq()
			f.set(&r, strings.Repeat("a", 201))
			errs := r.validate(now)
			if len(errs[f.name]) != 1 || len(errs) != 1 {
				t.Fatalf("201 bytes debe fallar solo en %q, obtuve %v", f.name, errs)
			}
		})
	}
}

// Caso AL-03c · opcionales con UTF-8 inválido
// Técnica: Partición en clases de equivalencia.
// Debe hacer / esperado: error en visitor_type y vehicle
func TestAL03c_OpcionalesConUTF8InvalidoSeRechazan(t *testing.T) {
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	bad := "ab\xff"
	r := validReq()
	r.VisitorType = &bad
	r.Vehicle = &bad
	errs := r.validate(now)
	if len(errs["visitor_type"]) == 0 || len(errs["vehicle"]) == 0 {
		t.Fatalf("esperaba errores en visitor_type y vehicle, obtuve %v", errs)
	}
}

// Caso AL-04a-f · {"",ENTRY,EXIT}×{ENTRY,EXIT} + statusFrom
// Técnica: Transición de estados.
// Debe hacer / esperado: 409 "inside"/"outside"; estado coherente
// de checkTransition unida con statusFrom (el estado derivado explica el conflicto).
func TestAL04_MatrizDeTransicionesYEstado(t *testing.T) {
	cases := []struct {
		id         string
		last, want string
		err        error
		status     string // estado derivado de `last`
		msgPart    string
	}{
		{"AL-04a", "", MovementEntry, nil, StatusUnknown, ""},
		{"AL-04b", "", MovementExit, nil, StatusUnknown, ""},
		{"AL-04c", MovementEntry, MovementEntry, ErrAlreadyInside, StatusInside, "inside"},
		{"AL-04d", MovementEntry, MovementExit, nil, StatusInside, ""},
		{"AL-04e", MovementExit, MovementEntry, nil, StatusOutside, ""},
		{"AL-04f", MovementExit, MovementExit, ErrAlreadyOutside, StatusOutside, "outside"},
	}
	for _, tc := range cases {
		t.Run(tc.id+" "+tc.last+"->"+tc.want, func(t *testing.T) {
			err := checkTransition(tc.last, tc.want)
			if !errors.Is(err, tc.err) {
				t.Fatalf("checkTransition(%q,%q)=%v, esperaba %v", tc.last, tc.want, err, tc.err)
			}
			if tc.msgPart != "" && !strings.Contains(err.Error(), tc.msgPart) {
				t.Fatalf("mensaje %q no contiene %q", err.Error(), tc.msgPart)
			}
			if got := statusFrom(tc.last); got != tc.status {
				t.Fatalf("statusFrom(%q)=%q, esperaba %q", tc.last, got, tc.status)
			}
		})
	}
}
