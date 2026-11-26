package access

// C3 — validate() de las 4 solicitudes, validateDateWindow y checkMaxLen.
// Funciones puras; el único no determinismo es time.Now() en validateDateWindow:
// se usan fechas relativas con margen (+-1 min como mínimo), sin reloj inyectado.

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func okAG() CreateAccessGrantRequest {
	return CreateAccessGrantRequest{
		UnitID: uuid.New(), Method: MethodQR, VisitorType: VisitorTypeGuest, VisitorName: "Ana",
		ValidUntil: time.Now().Add(2 * time.Hour),
	}
}

func okPA() CreatePreAuthRequest {
	return CreatePreAuthRequest{
		UnitID: uuid.New(), VisitorName: "Ana", VisitorIDNumber: "123", VisitorType: VisitorTypeGuest,
		ValidFrom: time.Now(), ValidUntil: time.Now().Add(2 * time.Hour),
	}
}

func okQR() CreateQRCodeRequest {
	return CreateQRCodeRequest{UnitID: uuid.New(), ValidUntil: time.Now().Add(2 * time.Hour)}
}

func okBatch() CreateQRBatchRequest {
	return CreateQRBatchRequest{UnitID: uuid.New(), Quantity: 10, ValidFrom: time.Now(), ValidUntil: time.Now().Add(2 * time.Hour)}
}

// wantField exige que errs tenga error en field (y, si msg != "", que lo contenga) o que no tenga errores.
func wantField(t *testing.T, errs map[string][]string, field string, wantErr bool, msg string) {
	t.Helper()
	got := errs[field]
	if wantErr != (len(got) > 0) {
		t.Fatalf("campo %q: wantErr=%v, errores=%v (todos: %v)", field, wantErr, got, errs)
	}
	if wantErr && msg != "" && !strings.Contains(strings.Join(got, "|"), msg) {
		t.Fatalf("campo %q: se esperaba %q en %v", field, msg, got)
	}
}

// Caso MV-00 · requests base válidos (precondición)
// Técnica: Partición en clases de equivalencia.
// Entrada: los 4
// Debe hacer / esperado: sin errores
func TestMV00_SolicitudesBaseSonValidas(t *testing.T) {
	for name, errs := range map[string]map[string][]string{
		"AccessGrant": okAG().validate(), "PreAuth": okPA().validate(), "QRCode": okQR().validate(), "Batch": okBatch().validate(),
	} {
		if len(errs) != 0 {
			t.Fatalf("%s base debe ser valida: %v", name, errs)
		}
	}
}

// Caso MV-01 · Quantity
// Técnica: Valores límite.
// Entrada: −1, 0, 1, 100, 101
// Debe hacer / esperado: error / error / ok / ok / error ("between 1 and 100")
func TestMV01_CantidadDelLote(t *testing.T) {
	for _, tc := range []struct {
		id      string
		q       int
		wantErr bool
	}{{"MV-01a -1", -1, true}, {"MV-01b 0", 0, true}, {"MV-01c 1", 1, false}, {"MV-01d 100", 100, false}, {"MV-01e 101", 101, true}} {
		t.Run(tc.id, func(t *testing.T) {
			r := okBatch()
			r.Quantity = tc.q
			wantField(t, r.validate(), "quantity", tc.wantErr, "between 1 and 100")
		})
	}
}

// Caso MV-02 · visitor_name
// Técnica: Valores límite.
// Entrada: 200/201 bytes en AccessGrant, PreAuth, QRCode; vacío
// Debe hacer / esperado: ok / "at most 200"; vacío ⇒ "required"
func TestMV02_LongitudDeVisitorName(t *testing.T) {
	for _, tc := range []struct {
		n       int
		wantErr bool
	}{{200, false}, {201, true}} {
		name := strings.Repeat("a", tc.n)
		t.Run("MV-02a AccessGrant "+itoa(tc.n), func(t *testing.T) {
			r := okAG()
			r.VisitorName = name
			wantField(t, r.validate(), "visitor_name", tc.wantErr, "at most 200")
		})
		t.Run("MV-02b PreAuth "+itoa(tc.n), func(t *testing.T) {
			r := okPA()
			r.VisitorName = name
			wantField(t, r.validate(), "visitor_name", tc.wantErr, "at most 200")
		})
		t.Run("MV-02c QRCode "+itoa(tc.n), func(t *testing.T) {
			r := okQR()
			r.VisitorName = &name
			wantField(t, r.validate(), "visitor_name", tc.wantErr, "at most 200")
		})
	}
	t.Run("MV-02d nombre vacio", func(t *testing.T) {
		ag, pa := okAG(), okPA()
		ag.VisitorName, pa.VisitorName = "", ""
		wantField(t, ag.validate(), "visitor_name", true, "required")
		wantField(t, pa.validate(), "visitor_name", true, "required")
	})
}

// Caso MV-04 · cédula y detalles
// Técnica: Valores límite.
// Entrada: 50/51; 1000/1001
// Debe hacer / esperado: ok / error
func TestMV04_LongitudDeCedulaYDetalles(t *testing.T) {
	for _, tc := range []struct {
		n       int
		wantErr bool
	}{{50, false}, {51, true}} {
		v := strings.Repeat("9", tc.n)
		t.Run("MV-04a cedula AccessGrant "+itoa(tc.n), func(t *testing.T) {
			r := okAG()
			r.VisitorIDNumber = &v
			wantField(t, r.validate(), "visitor_id_number", tc.wantErr, "at most 50")
		})
		t.Run("MV-04b cedula PreAuth "+itoa(tc.n), func(t *testing.T) {
			r := okPA()
			r.VisitorIDNumber = v
			wantField(t, r.validate(), "visitor_id_number", tc.wantErr, "at most 50")
		})
	}
	for _, tc := range []struct {
		n       int
		wantErr bool
	}{{1000, false}, {1001, true}} {
		d := strings.Repeat("d", tc.n)
		t.Run("MV-04c detalles AccessGrant "+itoa(tc.n), func(t *testing.T) {
			r := okAG()
			r.Details = &d
			wantField(t, r.validate(), "details", tc.wantErr, "at most 1000")
		})
		t.Run("MV-04d detalles PreAuth "+itoa(tc.n), func(t *testing.T) {
			r := okPA()
			r.Details = &d
			wantField(t, r.validate(), "details", tc.wantErr, "at most 1000")
		})
	}
}

// Caso MV-05 (parcial) · PRE_AUTH sin cédula
// Técnica: Partición en clases de equivalencia.
// Entrada: nil, "" (AccessGrant); "" (PreAuth); QR sin cédula; cédula válida
// Debe hacer / esperado: error "required for PRE_AUTH"/"required"; QR ok
func TestMV05_PreAuthSinCedula(t *testing.T) {
	t.Run("MV-05a AccessGrant PRE_AUTH sin cedula (nil)", func(t *testing.T) {
		r := okAG()
		r.Method = MethodPreAuth
		wantField(t, r.validate(), "visitor_id_number", true, "required for PRE_AUTH")
	})
	t.Run("MV-05b AccessGrant PRE_AUTH con cedula vacia", func(t *testing.T) {
		r := okAG()
		r.Method, r.VisitorIDNumber = MethodPreAuth, strp("")
		wantField(t, r.validate(), "visitor_id_number", true, "required for PRE_AUTH")
	})
	t.Run("MV-05c AccessGrant QR no exige cedula", func(t *testing.T) {
		wantField(t, okAG().validate(), "visitor_id_number", false, "")
	})
	t.Run("MV-05d AccessGrant PRE_AUTH con cedula valida", func(t *testing.T) {
		r := okAG()
		r.Method, r.VisitorIDNumber = MethodPreAuth, strp("1010")
		if errs := r.validate(); len(errs) != 0 {
			t.Fatalf("errores inesperados: %v", errs)
		}
	})
	t.Run("MV-05e PreAuth con cedula vacia", func(t *testing.T) {
		r := okPA()
		r.VisitorIDNumber = ""
		wantField(t, r.validate(), "visitor_id_number", true, "required")
	})
}

// Caso MV-06 · visitor_type
// Técnica: Partición en clases de equivalencia.
// Entrada: 5 válidos; "visitor", "", "OTHER"
// Debe hacer / esperado: ok / error
func TestMV06_TipoDeVisitante(t *testing.T) {
	for _, v := range []string{VisitorTypeDelivery, VisitorTypeVisitor, VisitorTypeService, VisitorTypeContractor, VisitorTypeGuest} {
		t.Run("MV-06a valido "+v, func(t *testing.T) {
			ag, pa := okAG(), okPA()
			ag.VisitorType, pa.VisitorType = v, v
			wantField(t, ag.validate(), "visitor_type", false, "")
			wantField(t, pa.validate(), "visitor_type", false, "")
		})
	}
	for _, v := range []string{"visitor", "", "OTHER"} {
		t.Run("MV-06b invalido "+v, func(t *testing.T) {
			ag, pa := okAG(), okPA()
			ag.VisitorType, pa.VisitorType = v, v
			wantField(t, ag.validate(), "visitor_type", true, "must be one of")
			wantField(t, pa.validate(), "visitor_type", true, "must be one of")
		})
	}
}

// Caso MV-07 · ventana de fechas en los 4 requests (a–h)
// Técnica: Partición en clases de equivalencia, Valores límite.
// Entrada: until−1 min; from==until; from>until; from<until; until cero; from puntero a cero; from
// nil; from cero obligatorio en Batch/PreAuth
// Debe hacer / esperado: "must not be in the past"; "must be after valid_from"; ok; exactamente
// [required]; ok; ok; "required"
func TestMV07_VentanaDeFechas(t *testing.T) {
	now := time.Now()
	type setter func(from *time.Time, until time.Time) map[string][]string
	reqs := map[string]setter{
		"AccessGrant": func(f *time.Time, u time.Time) map[string][]string {
			r := okAG()
			r.ValidFrom, r.ValidUntil = f, u
			return r.validate()
		},
		"QRCode": func(f *time.Time, u time.Time) map[string][]string {
			r := okQR()
			r.ValidFrom, r.ValidUntil = f, u
			return r.validate()
		},
		"Batch": func(f *time.Time, u time.Time) map[string][]string {
			r := okBatch()
			if f != nil {
				r.ValidFrom = *f
			}
			r.ValidUntil = u
			return r.validate()
		},
		"PreAuth": func(f *time.Time, u time.Time) map[string][]string {
			r := okPA()
			if f != nil {
				r.ValidFrom = *f
			}
			r.ValidUntil = u
			return r.validate()
		},
	}
	for name, run := range reqs {
		t.Run("MV-07a until en el pasado "+name, func(t *testing.T) {
			past := now.Add(-time.Minute)
			from := past.Add(-time.Hour)
			wantField(t, run(&from, past), "valid_until", true, "must not be in the past")
		})
		t.Run("MV-07b from == until "+name, func(t *testing.T) {
			at := now.Add(time.Hour)
			wantField(t, run(&at, at), "valid_until", true, "must be after valid_from")
		})
		t.Run("MV-07c from > until "+name, func(t *testing.T) {
			from, until := now.Add(3*time.Hour), now.Add(time.Hour)
			wantField(t, run(&from, until), "valid_until", true, "must be after valid_from")
		})
		t.Run("MV-07d from < until "+name, func(t *testing.T) {
			from, until := now.Add(time.Minute), now.Add(time.Hour)
			wantField(t, run(&from, until), "valid_until", false, "")
		})
		t.Run("MV-07e until cero: solo 'required', sin duplicar "+name, func(t *testing.T) {
			from := now
			errs := run(&from, time.Time{})
			if got := errs["valid_until"]; len(got) != 1 || got[0] != "required" {
				t.Fatalf("valid_until debe ser exactamente [required], hubo %v", got)
			}
		})
	}
	t.Run("MV-07f valid_from puntero a cero se ignora (AccessGrant)", func(t *testing.T) {
		zero := time.Time{}
		wantField(t, reqs["AccessGrant"](&zero, now.Add(time.Hour)), "valid_until", false, "")
	})
	t.Run("MV-07g valid_from nil es opcional (AccessGrant/QRCode)", func(t *testing.T) {
		wantField(t, reqs["AccessGrant"](nil, now.Add(time.Hour)), "valid_until", false, "")
		wantField(t, reqs["QRCode"](nil, now.Add(time.Hour)), "valid_until", false, "")
	})
	t.Run("MV-07h valid_from cero es obligatorio (Batch/PreAuth)", func(t *testing.T) {
		b, p := okBatch(), okPA()
		b.ValidFrom, p.ValidFrom = time.Time{}, time.Time{}
		wantField(t, b.validate(), "valid_from", true, "required")
		wantField(t, p.validate(), "valid_from", true, "required")
	})
}

// Caso MV-08 · method
// Técnica: Partición en clases de equivalencia.
// Entrada: QR, PRE_AUTH, "", "qr"
// Debe hacer / esperado: ok / ok / error / error
func TestMV08_Metodo(t *testing.T) {
	for _, tc := range []struct {
		m       string
		wantErr bool
	}{{MethodQR, false}, {MethodPreAuth, false}, {"", true}, {"qr", true}} {
		t.Run("MV-08 method="+tc.m, func(t *testing.T) {
			r := okAG()
			r.Method = tc.m
			if tc.m == MethodPreAuth {
				r.VisitorIDNumber = strp("1")
			}
			wantField(t, r.validate(), "method", tc.wantErr, "must be QR or PRE_AUTH")
		})
	}
}

// Caso MV-10 · unit_id nil
// Técnica: Partición en clases de equivalencia.
// Entrada: 4 requests
// Debe hacer / esperado: error "required" en unit_id
func TestMV10_UnitIDNil(t *testing.T) {
	ag, pa, qr, b := okAG(), okPA(), okQR(), okBatch()
	ag.UnitID, pa.UnitID, qr.UnitID, b.UnitID = uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil
	for name, errs := range map[string]map[string][]string{"AccessGrant": ag.validate(), "PreAuth": pa.validate(), "QRCode": qr.validate(), "Batch": b.validate()} {
		t.Run("MV-10 "+name, func(t *testing.T) { wantField(t, errs, "unit_id", true, "required") })
	}
}
