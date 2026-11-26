package access

// C2 — gestión de pases (service). Dobles: memRepo (fake), noCallsRepo (mock
// estricto), stubDirectory, stubMembership (stub+spy), stubQR, spyAuditor y
// chanNotifier. Sin reloj ni aleatoriedad relevantes.

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"controldeaccesos/internal/audit"
	"controldeaccesos/internal/notifier"
	"controldeaccesos/internal/qrsystem"
)

func (w *world) validUntil() time.Time { return time.Now().Add(2 * time.Hour) }

// createOps son las 4 operaciones de escritura protegidas por checkUnitOwnership.
func (w *world) createOps() map[string]func() error {
	until := w.validUntil()
	return map[string]func() error{
		"CreateQRCode": func() error {
			_, err := w.svc.CreateQRCode(w.ctx(), w.complexID, w.userID, CreateQRCodeRequest{UnitID: w.unitID, ValidUntil: until})
			return err
		},
		"CreateQRBatch": func() error {
			_, err := w.svc.CreateQRBatch(w.ctx(), w.complexID, w.userID, CreateQRBatchRequest{UnitID: w.unitID, Quantity: 2, ValidFrom: time.Now(), ValidUntil: until})
			return err
		},
		"CreatePreAuth": func() error {
			_, err := w.svc.CreatePreAuth(w.ctx(), w.complexID, w.userID, CreatePreAuthRequest{UnitID: w.unitID, VisitorName: "Luis", VisitorIDNumber: "123", VisitorType: VisitorTypeGuest, ValidFrom: time.Now(), ValidUntil: until})
			return err
		},
		"CreateAccessGrant": func() error {
			_, err := w.svc.CreateAccessGrant(w.ctx(), w.complexID, w.userID, CreateAccessGrantRequest{UnitID: w.unitID, Method: MethodQR, VisitorType: VisitorTypeGuest, VisitorName: "Luis", ValidUntil: until})
			return err
		},
	}
}

// assertDeniedGuard verifica la salida de una guarda IDOR: error, repo intacto
// (el mock estricto ya falla si se toca), QR no emitido y auditoría DENIED.
func assertDeniedGuard(t *testing.T, w *world, err error) {
	t.Helper()
	if issued, _ := w.qr.counts(); issued != 0 {
		t.Fatalf("no debe emitirse ningun QR antes de pasar las guardas (emitidos=%d)", issued)
	}
	e := w.aud.only(t)
	if e.Result != audit.ResultDenied || e.Action != ActionCreateGrant || e.Reason != err.Error() {
		t.Fatalf("auditoria inesperada: %+v (err=%v)", e, err)
	}
	if e.ActorID == nil || *e.ActorID != w.userID || e.ComplexID == nil || *e.ComplexID != w.complexID || e.ResourceID == nil || *e.ResourceID != w.unitID {
		t.Fatalf("la auditoria debe identificar actor, complejo y unidad: %+v", e)
	}
}

// Caso GS-01 · unidad inexistente
// Técnica: Tabla de decisión.
// Entrada: missing, en las 4 operaciones de creación
// Debe hacer / esperado: ErrUnitNotFound; repo sin llamadas; sin consulta de membresía; QR no
// emitido; auditoría DENIED (actor, complejo, unidad)
func TestGS01_UnidadInexistente(t *testing.T) {
	for name := range newWorld().createOps() {
		t.Run("GS-01 "+name, func(t *testing.T) {
			w := newWorld()
			w.dir.missing = true
			w.build()
			w.svc.repo = noCallsRepo{t}
			err := w.createOps()[name]()
			if !errors.Is(err, ErrUnitNotFound) {
				t.Fatalf("want ErrUnitNotFound, got %v", err)
			}
			if w.mem.callCount() != 0 {
				t.Fatal("no debe consultarse la membresia de una unidad inexistente")
			}
			assertDeniedGuard(t, w, err)
		})
	}
}

// Caso GS-02 · unidad de otro conjunto
// Técnica: Tabla de decisión.
// Entrada: complexID distinto
// Debe hacer / esperado: ErrUnitComplexMismatch, igual
func TestGS02_UnidadDeOtroConjunto(t *testing.T) {
	for name := range newWorld().createOps() {
		t.Run("GS-02 "+name, func(t *testing.T) {
			w := newWorld()
			w.dir.complexID = uuid.New()
			w.build()
			w.svc.repo = noCallsRepo{t}
			err := w.createOps()[name]()
			if !errors.Is(err, ErrUnitComplexMismatch) {
				t.Fatalf("want ErrUnitComplexMismatch, got %v", err)
			}
			if w.mem.callCount() != 0 {
				t.Fatal("no debe consultarse la membresia si la unidad es de otro conjunto")
			}
			assertDeniedGuard(t, w, err)
		})
	}
}

// Caso GS-03 · no miembro
// Técnica: Tabla de decisión.
// Entrada: membership ok=false
// Debe hacer / esperado: ErrUnitAccessDenied; checker llamado 1 vez con (user, unit)
func TestGS03_UsuarioNoEsMiembro(t *testing.T) {
	for name := range newWorld().createOps() {
		t.Run("GS-03 "+name, func(t *testing.T) {
			w := newWorld()
			w.mem.ok = false
			w.svc.repo = noCallsRepo{t}
			err := w.createOps()[name]()
			if !errors.Is(err, ErrUnitAccessDenied) {
				t.Fatalf("want ErrUnitAccessDenied, got %v", err)
			}
			if c := w.mem.calls; len(c) != 1 || c[0].user != w.userID || c[0].unit != w.unitID {
				t.Fatalf("la membresia debe consultarse una vez con (usuario, unidad): %+v", c)
			}
			assertDeniedGuard(t, w, err)
		})
	}
}

// Caso GS-04 · falla el checker
// Técnica: Adivinación de errores.
// Entrada: err
// Debe hacer / esperado: se propaga el error original (no ErrUnitAccessDenied)
func TestGS04_FallaElCheckerDeMembresia(t *testing.T) {
	boom := errors.New("membership db down")
	w := newWorld()
	w.mem.err = boom
	w.svc.repo = noCallsRepo{t}
	err := w.createOps()["CreateAccessGrant"]()
	if !errors.Is(err, boom) || errors.Is(err, ErrUnitAccessDenied) {
		t.Fatalf("debe propagarse el error original, no convertirse en denegacion: %v", err)
	}
	assertDeniedGuard(t, w, err)
}

// Caso GS-05 · checker nil
// Técnica: Adivinación de errores.
// Entrada: unitChecker=nil
// Debe hacer / esperado: ErrUnitAccessDenied
func TestGS05_CheckerNilDeniegaPorDefecto(t *testing.T) {
	w := newWorld()
	w.svc.unitChecker = nil
	w.svc.repo = noCallsRepo{t}
	err := w.createOps()["CreatePreAuth"]()
	if !errors.Is(err, ErrUnitAccessDenied) {
		t.Fatalf("want ErrUnitAccessDenied, got %v", err)
	}
	assertDeniedGuard(t, w, err)
}

// Caso GS-06 · falla el directorio
// Técnica: Adivinación de errores.
// Entrada: lookupErr
// Debe hacer / esperado: error envuelto "failed to load unit"; no es ErrUnitNotFound
func TestGS06_FallaElDirectorio(t *testing.T) {
	boom := errors.New("directory down")
	w := newWorld()
	w.dir.lookupErr = boom
	w.build()
	w.svc.repo = noCallsRepo{t}
	err := w.createOps()["CreateQRCode"]()
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "failed to load unit") {
		t.Fatalf("error envuelto 'failed to load unit' esperado, hubo %v", err)
	}
	if errors.Is(err, ErrUnitNotFound) {
		t.Fatal("un fallo del directorio no es 'unidad inexistente'")
	}
	assertDeniedGuard(t, w, err)
}

// Caso GS-07 · QR vs PRE_AUTH en CreateAccessGrant (a,b) + error de repo (c)
// Técnica: Partición en clases de equivalencia.
// Entrada: método QR/PRE_AUTH
// Debe hacer / esperado: QR: qrCode del stub llega al repo; PRE_AUTH: Issue 0 y qrCode ""; c:
// error propagado sin auditoría
func TestGS07_QRVersusPreAuthEnCreateAccessGrant(t *testing.T) {
	until := time.Now().Add(time.Hour)
	t.Run("GS-07a QR usa el codigo del sistema QR", func(t *testing.T) {
		w := newWorld()
		w.qr.codes = []string{"CODE-1"}
		g, err := w.svc.CreateAccessGrant(w.ctx(), w.complexID, w.userID, CreateAccessGrantRequest{UnitID: w.unitID, Method: MethodQR, VisitorType: VisitorTypeGuest, VisitorName: "Luis", ValidUntil: until})
		if err != nil {
			t.Fatal(err)
		}
		if len(w.repo.createReqs) != 1 || w.repo.createReqs[0].qrCode != "CODE-1" {
			t.Fatalf("el repo debe recibir el codigo emitido: %+v", w.repo.createReqs)
		}
		if g.QRCodeHash == nil || *g.QRCodeHash != "CODE-1" {
			t.Fatalf("el pase creado debe llevar el codigo: %+v", g.QRCodeHash)
		}
		if e := w.aud.only(t); e.Result != audit.ResultAllowed || e.ResourceID == nil || *e.ResourceID != g.ID {
			t.Fatalf("auditoria inesperada: %+v", e)
		}
	})
	t.Run("GS-07b PRE_AUTH no emite QR", func(t *testing.T) {
		w := newWorld()
		_, err := w.svc.CreateAccessGrant(w.ctx(), w.complexID, w.userID, CreateAccessGrantRequest{UnitID: w.unitID, Method: MethodPreAuth, VisitorType: VisitorTypeGuest, VisitorName: "Luis", VisitorIDNumber: strp("77"), ValidUntil: until})
		if err != nil {
			t.Fatal(err)
		}
		if issued, _ := w.qr.counts(); issued != 0 {
			t.Fatalf("PRE_AUTH no debe llamar a Issue (emitidos=%d)", issued)
		}
		if w.repo.createReqs[0].qrCode != "" {
			t.Fatalf("qrCode debe ir vacio: %q", w.repo.createReqs[0].qrCode)
		}
	})
	t.Run("GS-07c error del repo se propaga sin auditar ALLOWED", func(t *testing.T) {
		w := newWorld()
		boom := errors.New("insert failed")
		w.repo.errOn["CreateAccessGrant"] = boom
		_, err := w.svc.CreateAccessGrant(w.ctx(), w.complexID, w.userID, CreateAccessGrantRequest{UnitID: w.unitID, Method: MethodPreAuth, VisitorType: VisitorTypeGuest, VisitorName: "Luis", VisitorIDNumber: strp("77"), ValidUntil: until})
		if !errors.Is(err, boom) || len(w.aud.all()) != 0 {
			t.Fatalf("err=%v auditoria=%+v", err, w.aud.all())
		}
	})
}

// Caso GS-08 · QR caído en CreateQRCode / CreateAccessGrant
// Técnica: Adivinación de errores.
// Entrada: issueErr
// Debe hacer / esperado: ErrUnavailable; repo sin llamadas; DENIED "qr system unavailable"
func TestGS08_QRCaidoNoEscribeNada(t *testing.T) {
	down := &qrsystem.UnavailableError{Cause: qrsystem.CauseCircuitOpen, RetryAfter: 5 * time.Second}
	for _, name := range []string{"CreateQRCode", "CreateAccessGrant"} {
		t.Run("GS-08 "+name, func(t *testing.T) {
			w := newWorld()
			w.qr.issueErr = down
			w.svc.repo = noCallsRepo{t}
			err := w.createOps()[name]()
			if !errors.Is(err, qrsystem.ErrUnavailable) {
				t.Fatalf("want ErrUnavailable, got %v", err)
			}
			e := w.aud.only(t)
			if e.Result != audit.ResultDenied || e.Reason != "qr system unavailable" || e.Action != ActionCreateGrant {
				t.Fatalf("auditoria inesperada: %+v", e)
			}
		})
	}
}

// Caso GS-09 · valores por defecto de CreateQRCode (a–f)
// Técnica: Partición en clases de equivalencia.
// Entrada: sin nombre, GetUnitName con error, nombre vacío/blanco/error, código de 4000 chars,
// error de repo
// Debe hacer / esperado: "Guest"; data URL PNG válido; "Unknown Unit"; "Self"; sin imagen pero
// creado; error propagado
func TestGS09_ValoresPorDefectoDeCreateQRCode(t *testing.T) {
	until := time.Now().Add(time.Hour)
	run := func(w *world, name *string) *AccessGrantResponse {
		t.Helper()
		resp, err := w.svc.CreateQRCode(w.ctx(), w.complexID, w.userID, CreateQRCodeRequest{UnitID: w.unitID, VisitorName: name, ValidUntil: until})
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	t.Run("GS-09a sin nombre -> Guest, imagen PNG en data URL", func(t *testing.T) {
		w := newWorld()
		w.qr.codes = []string{"ABC123"}
		resp := run(w, nil)
		if w.repo.createReqs[0].VisitorName != "Guest" || resp.VisitorName != "Guest" {
			t.Fatalf("nombre por defecto: %q", resp.VisitorName)
		}
		if w.repo.createReqs[0].Method != MethodQR || w.repo.createReqs[0].VisitorType != VisitorTypeVisitor {
			t.Fatalf("metodo/tipo: %+v", w.repo.createReqs[0])
		}
		const prefix = "data:image/png;base64,"
		if !strings.HasPrefix(resp.QRCodeURL, prefix) {
			t.Fatalf("QRCodeURL sin prefijo: %.30q", resp.QRCodeURL)
		}
		png, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(resp.QRCodeURL, prefix))
		if err != nil || !bytes.HasPrefix(png, []byte("\x89PNG")) {
			t.Fatalf("la imagen debe ser un PNG valido (err=%v)", err)
		}
		if resp.UnitName != "A-101" || resp.CreatorName != "Ana" {
			t.Fatalf("unit/creator: %q / %q", resp.UnitName, resp.CreatorName)
		}
		if e := w.aud.only(t); e.Result != audit.ResultAllowed {
			t.Fatalf("auditoria inesperada: %+v", e)
		}
	})
	t.Run("GS-09b nombre provisto se respeta", func(t *testing.T) {
		w := newWorld()
		resp := run(w, strp("Marta"))
		if resp.VisitorName != "Marta" {
			t.Fatalf("nombre: %q", resp.VisitorName)
		}
	})
	t.Run("GS-09c falla el nombre de unidad -> Unknown Unit", func(t *testing.T) {
		w := newWorld()
		w.dir.unitErr = errors.New("x")
		w.build()
		if resp := run(w, nil); resp.UnitName != "Unknown Unit" {
			t.Fatalf("UnitName: %q", resp.UnitName)
		}
	})
	t.Run("GS-09d creador sin nombre -> Self", func(t *testing.T) {
		for name, dn := range map[string]stubDirectory{
			"vacio":     {displayName: ""},
			"en blanco": {displayName: "   "},
			"con error": {displayName: "Ana", displayErr: errors.New("x")},
		} {
			w := newWorld()
			w.dir.displayName, w.dir.displayErr = dn.displayName, dn.displayErr
			w.build()
			if resp := run(w, nil); resp.CreatorName != "Self" {
				t.Fatalf("%s: CreatorName=%q", name, resp.CreatorName)
			}
		}
	})
	t.Run("GS-09e codigo demasiado largo: sin imagen pero el pase se crea", func(t *testing.T) {
		w := newWorld()
		w.qr.codes = []string{strings.Repeat("A", 4000)}
		resp := run(w, nil)
		if resp.QRCodeURL != "" || resp.ID == uuid.Nil {
			t.Fatalf("QRCodeURL=%.20q id=%v", resp.QRCodeURL, resp.ID)
		}
	})
	t.Run("GS-09f error del repo se propaga", func(t *testing.T) {
		w := newWorld()
		boom := errors.New("insert failed")
		w.repo.errOn["CreateAccessGrant"] = boom
		_, err := w.svc.CreateQRCode(w.ctx(), w.complexID, w.userID, CreateQRCodeRequest{UnitID: w.unitID, ValidUntil: until})
		if !errors.Is(err, boom) {
			t.Fatalf("err=%v", err)
		}
	})
}

// Caso GS-10 · CreatePreAuth feliz (+error de repo)
// Técnica: Partición en clases de equivalencia.
// Entrada: miembros [actor, vecino]
// Debe hacer / esperado: Method PRE_AUTH, cédula/tipo/detalles al repo; ALLOWED; notify
// grant_created solo al vecino
func TestGS10_CreatePreAuthFeliz(t *testing.T) {
	w := newWorld()
	neighbour := uuid.New()
	w.dir.members = []uuid.UUID{w.userID, neighbour}
	n := w.withNotifier()
	from, until := time.Now(), time.Now().Add(3*time.Hour)
	detail := "entrega"
	g, err := w.svc.CreatePreAuth(w.ctx(), w.complexID, w.userID, CreatePreAuthRequest{UnitID: w.unitID, VisitorName: "Luis", VisitorIDNumber: "C-123", VisitorType: VisitorTypeDelivery, ValidFrom: from, ValidUntil: until, Details: &detail})
	if err != nil {
		t.Fatal(err)
	}
	r := w.repo.createReqs[0]
	if r.Method != MethodPreAuth || r.VisitorIDNumber == nil || *r.VisitorIDNumber != "C-123" || r.VisitorType != VisitorTypeDelivery || r.Details == nil || *r.Details != detail {
		t.Fatalf("peticion al repo inesperada: %+v", r)
	}
	if issued, _ := w.qr.counts(); issued != 0 || g.QRCodeHash != nil {
		t.Fatal("PRE_AUTH no usa el sistema QR")
	}
	if e := w.aud.only(t); e.Result != audit.ResultAllowed || e.Action != ActionCreateGrant {
		t.Fatalf("auditoria inesperada: %+v", e)
	}
	ev := waitEvent(t, *n)
	if ev.Data["event"] != eventGrantCreated || len(ev.UserIDs) != 1 || ev.UserIDs[0] != neighbour || ev.Category != notifier.CategoryAccess {
		t.Fatalf("la notificacion debe ir al vecino y no al actor: %+v", ev)
	}
	t.Run("GS-10b error del repo", func(t *testing.T) {
		w := newWorld()
		boom := errors.New("x")
		w.repo.errOn["CreateAccessGrant"] = boom
		if _, err := w.svc.CreatePreAuth(w.ctx(), w.complexID, w.userID, CreatePreAuthRequest{UnitID: w.unitID}); !errors.Is(err, boom) {
			t.Fatalf("err=%v", err)
		}
	})
}

// Caso GS-11 · Get de otro conjunto
// Técnica: Tabla de decisión.
// Entrada: pase de otro complejo
// Debe hacer / esperado: ErrUnitAccessDenied; membresía no consultada; razón "cross-tenant access
// attempt"
func TestGS11_GetAccessGrantDeOtroConjunto(t *testing.T) {
	w := newWorld()
	g := w.grant(func(g *AccessGrant) { g.ComplexID = uuid.New() })
	got, err := w.svc.GetAccessGrant(w.ctx(), w.complexID, w.userID, g.ID)
	if !errors.Is(err, ErrUnitAccessDenied) || got != nil {
		t.Fatalf("want ErrUnitAccessDenied y nil, got %v / %+v", err, got)
	}
	if w.mem.callCount() != 0 {
		t.Fatal("no debe consultarse la membresia de un pase de otro conjunto")
	}
	e := w.aud.only(t)
	if e.Result != audit.ResultDenied || e.Action != ActionGetGrant || e.Reason != "cross-tenant access attempt" {
		t.Fatalf("auditoria inesperada: %+v", e)
	}
}

// Caso GS-12 · Get no miembro / inexistente / feliz
// Técnica: Tabla de decisión.
// Entrada: —
// Debe hacer / esperado: denegado + auditoría; ErrAccessGrantNotFound; devuelve el pase
func TestGS12_GetAccessGrantSinSerMiembro(t *testing.T) {
	t.Run("GS-12a no miembro", func(t *testing.T) {
		w := newWorld()
		w.mem.ok = false
		g := w.grant(nil)
		got, err := w.svc.GetAccessGrant(w.ctx(), w.complexID, w.userID, g.ID)
		if !errors.Is(err, ErrUnitAccessDenied) || got != nil {
			t.Fatalf("got %v / %+v", err, got)
		}
		if e := w.aud.only(t); e.Result != audit.ResultDenied || e.Reason != ErrUnitAccessDenied.Error() {
			t.Fatalf("auditoria inesperada: %+v", e)
		}
	})
	t.Run("GS-12b pase inexistente", func(t *testing.T) {
		w := newWorld()
		if _, err := w.svc.GetAccessGrant(w.ctx(), w.complexID, w.userID, uuid.New()); !errors.Is(err, ErrAccessGrantNotFound) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("GS-12c miembro recibe el pase", func(t *testing.T) {
		w := newWorld()
		g := w.grant(nil)
		got, err := w.svc.GetAccessGrant(w.ctx(), w.complexID, w.userID, g.ID)
		if err != nil || got.ID != g.ID {
			t.Fatalf("got %v / %+v", err, got)
		}
	})
}

// Caso GS-13 · Cancel otro conjunto / no miembro
// Técnica: Tabla de decisión.
// Entrada: —
// Debe hacer / esperado: denegado; CancelAccessGrant del repo 0 llamadas; pase sigue ACTIVE
func TestGS13_CancelDeneg(t *testing.T) {
	for name, setup := range map[string]func(w *world) AccessGrant{
		"GS-13a otro conjunto": func(w *world) AccessGrant { return w.grant(func(g *AccessGrant) { g.ComplexID = uuid.New() }) },
		"GS-13b no miembro": func(w *world) AccessGrant {
			w.mem.ok = false
			return w.grant(nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld()
			g := setup(w)
			err := w.svc.CancelAccessGrant(w.ctx(), w.complexID, w.userID, g.ID)
			if !errors.Is(err, ErrUnitAccessDenied) {
				t.Fatalf("want ErrUnitAccessDenied, got %v", err)
			}
			if w.repo.count("CancelAccessGrant") != 0 || w.repo.snapshot(g.ID).Status != StatusActive {
				t.Fatal("el pase no debe cancelarse")
			}
			if e := w.aud.only(t); e.Result != audit.ResultDenied || e.Action != ActionCancelGrant {
				t.Fatalf("auditoria inesperada: %+v", e)
			}
		})
	}
}

// Caso GS-14 · Cancel feliz (+inexistente, +fallo de escritura)
// Técnica: Transición de estados.
// Entrada: pase ACTIVE
// Debe hacer / esperado: ACTIVE→CANCELLED; ALLOWED; notify grant_cancelled al vecino; fallo ⇒ sin
// ALLOWED
func TestGS14_CancelFeliz(t *testing.T) {
	w := newWorld()
	neighbour := uuid.New()
	w.dir.members = []uuid.UUID{w.userID, neighbour}
	n := w.withNotifier()
	g := w.grant(nil)
	if err := w.svc.CancelAccessGrant(w.ctx(), w.complexID, w.userID, g.ID); err != nil {
		t.Fatal(err)
	}
	if w.repo.snapshot(g.ID).Status != StatusCancelled {
		t.Fatal("el pase debe quedar CANCELLED")
	}
	if e := w.aud.only(t); e.Result != audit.ResultAllowed || e.Action != ActionCancelGrant || e.ResourceID == nil || *e.ResourceID != g.ID {
		t.Fatalf("auditoria inesperada: %+v", e)
	}
	ev := waitEvent(t, *n)
	if ev.Data["event"] != eventGrantCancelled || len(ev.UserIDs) != 1 || ev.UserIDs[0] != neighbour {
		t.Fatalf("notificacion inesperada: %+v", ev)
	}
	t.Run("GS-14b pase inexistente", func(t *testing.T) {
		w := newWorld()
		if err := w.svc.CancelAccessGrant(w.ctx(), w.complexID, w.userID, uuid.New()); !errors.Is(err, ErrAccessGrantNotFound) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("GS-14c falla la escritura: sin auditoria ALLOWED", func(t *testing.T) {
		w := newWorld()
		boom := errors.New("update failed")
		w.repo.errOn["CancelAccessGrant"] = boom
		g := w.grant(nil)
		if err := w.svc.CancelAccessGrant(w.ctx(), w.complexID, w.userID, g.ID); !errors.Is(err, boom) || len(w.aud.all()) != 0 {
			t.Fatalf("err=%v aud=%+v", err, w.aud.all())
		}
	})
}

// Caso GS-16 · lote
// Técnica: Valores límite.
// Entrada: falla 1.er código; n=1; n=100; falla de tx
// Debe hacer / esperado: 0 escrituras (mock estricto); 1 y 100 pases en 1 transacción con 1/100
// códigos emitidos; error "failed to create QR batch"
func TestGS16_LoteLimites(t *testing.T) {
	until := time.Now().Add(2 * time.Hour)
	batch := func(w *world, n int) ([]*AccessGrant, error) {
		return w.svc.CreateQRBatch(w.ctx(), w.complexID, w.userID, CreateQRBatchRequest{UnitID: w.unitID, Quantity: n, ValidFrom: time.Now(), ValidUntil: until})
	}
	t.Run("GS-16a falla el primer codigo: 0 escrituras", func(t *testing.T) {
		w := newWorld()
		w.qr.issueErr = &qrsystem.UnavailableError{Cause: qrsystem.CauseError}
		w.svc.repo = noCallsRepo{t}
		if _, err := batch(w, 5); !errors.Is(err, qrsystem.ErrUnavailable) {
			t.Fatalf("err=%v", err)
		}
		if e := w.aud.only(t); e.Result != audit.ResultDenied || e.Reason != "qr system unavailable" {
			t.Fatalf("auditoria inesperada: %+v", e)
		}
	})
	for _, n := range []int{1, 100} {
		t.Run("GS-16b cantidad "+itoa(n), func(t *testing.T) {
			w := newWorld()
			grants, err := batch(w, n)
			if err != nil || len(grants) != n {
				t.Fatalf("se esperaban %d pases, hubo %d (%v)", n, len(grants), err)
			}
			if w.repo.count("CreateAccessGrantsTx") != 1 || w.repo.count("CreateAccessGrant") != 0 || len(w.repo.txReqs[0]) != n {
				t.Fatalf("todo debe ir en UNA transaccion de %d", n)
			}
			if issued, _ := w.qr.counts(); issued != n {
				t.Fatalf("codigos emitidos=%d", issued)
			}
			if e := w.aud.only(t); e.Result != audit.ResultAllowed || e.Reason != "batch of "+itoa(n) {
				t.Fatalf("auditoria inesperada: %+v", e)
			}
		})
	}
	t.Run("GS-16c falla la transaccion: error envuelto", func(t *testing.T) {
		w := newWorld()
		boom := errors.New("tx failed")
		w.repo.errOn["CreateAccessGrantsTx"] = boom
		_, err := batch(w, 3)
		if !errors.Is(err, boom) || !strings.Contains(err.Error(), "failed to create QR batch") || len(w.aud.all()) != 0 {
			t.Fatalf("err=%v aud=%+v", err, w.aud.all())
		}
	})
}

func itoa(n int) string { return strconv.Itoa(n) }

// Caso GS-17 · QRSystemStatus
// Técnica: Partición en clases de equivalencia.
// Entrada: qr nil / stub
// Debe hacer / esperado: {true,"CLOSED"} / lo del stub
func TestGS17_QRSystemStatus(t *testing.T) {
	w := newWorld()
	w.svc.qr = nil
	if got := w.svc.QRSystemStatus(); got != (qrsystem.Public{Available: true, State: "CLOSED"}) {
		t.Fatalf("sin QR: %+v", got)
	}
	w.qr.avail = qrsystem.Public{Available: false, State: "OPEN", RetryAfterSeconds: 7}
	w.svc.qr = w.qr
	if got := w.svc.QRSystemStatus(); got != w.qr.avail {
		t.Fatalf("con QR: %+v", got)
	}
}

// Caso GS-18 · ramas auxiliares de notify (extra)
// Técnica: Partición en clases de equivalencia, Adivinación de errores.
// Entrada: nombres vacíos; error de miembros; evento desconocido; sin notifier / sin pase
// Debe hacer / esperado: textos por defecto; no se notifica
func TestGS18_NotifyRamasAuxiliares(t *testing.T) {
	grant := func(w *world, name string) *AccessGrant {
		return &AccessGrant{ID: uuid.New(), ComplexID: w.complexID, UnitID: w.unitID, VisitorName: name, Method: MethodQR}
	}
	t.Run("GS-18a sin nombre de visitante ni de unidad usa los textos por defecto", func(t *testing.T) {
		w := newWorld()
		w.dir.members, w.dir.unitName = []uuid.UUID{uuid.New()}, "  "
		n := w.withNotifier()
		w.svc.notify(w.ctx(), eventGrantCreated, grant(w, "  "), nil)
		ev := waitEvent(t, *n)
		if !strings.Contains(ev.Body, "Un visitante") || !strings.Contains(ev.Body, "tu unidad") {
			t.Fatalf("cuerpo: %q", ev.Body)
		}
	})
	t.Run("GS-18b el nombre de la unidad aparece en el cuerpo (cancelacion)", func(t *testing.T) {
		w := newWorld()
		w.dir.members = []uuid.UUID{uuid.New()}
		n := w.withNotifier()
		w.svc.notify(w.ctx(), eventGrantCancelled, grant(w, "Juan"), nil)
		ev := waitEvent(t, *n)
		if ev.Title != "Acceso cancelado" || !strings.Contains(ev.Body, "Juan") || !strings.Contains(ev.Body, "A-101") {
			t.Fatalf("evento: %+v", ev)
		}
	})
	t.Run("GS-18c falla al resolver miembros: no se notifica", func(t *testing.T) {
		w := newWorld()
		w.dir.membersErr = errors.New("directory down")
		n := w.withNotifier()
		w.svc.notify(w.ctx(), eventGrantCreated, grant(w, "Juan"), nil)
		select {
		case ev := <-n.events:
			t.Fatalf("no se esperaba notificacion: %+v", ev)
		case <-time.After(200 * time.Millisecond):
		}
	})
	t.Run("GS-18d evento desconocido: no se notifica", func(t *testing.T) {
		w := newWorld()
		w.dir.members = []uuid.UUID{uuid.New()}
		n := w.withNotifier()
		w.svc.notify(w.ctx(), "otro_evento", grant(w, "Juan"), nil)
		select {
		case ev := <-n.events:
			t.Fatalf("no se esperaba notificacion: %+v", ev)
		case <-time.After(200 * time.Millisecond):
		}
	})
	t.Run("GS-18e sin notifier o sin pase: no hace nada", func(t *testing.T) {
		w := newWorld()
		w.svc.notify(w.ctx(), eventGrantCreated, grant(w, "Juan"), nil) // notifier nil
		n := w.withNotifier()
		w.svc.notify(w.ctx(), eventGrantCreated, nil, nil)
		select {
		case ev := <-n.events:
			t.Fatalf("no se esperaba notificacion: %+v", ev)
		case <-time.After(100 * time.Millisecond):
		}
	})
}

// Caso GS-19 · delegaciones y QR no cableado (extra)
// Técnica: Partición en clases de equivalencia.
// Entrada: GetUnitAccessGrants, GrantSummary, qr=nil
// Debe hacer / esperado: argumentos reenviados; qrCode vacío
func TestGS19_DelegacionesYQRNoCableado(t *testing.T) {
	t.Run("GS-19a GetUnitAccessGrants filtra por unidad y estado", func(t *testing.T) {
		w := newWorld()
		w.svc.repo = unitGrantsRepo{Repository: w.repo, want: w.unitID, t: t}
		st := StatusActive
		if _, err := w.svc.GetUnitAccessGrants(w.ctx(), w.unitID, &st); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("GS-19b GrantSummary devuelve lo del repo", func(t *testing.T) {
		w := newWorld()
		w.svc.repo = summaryRepo{Repository: w.repo, sum: &GrantSummary{Total: 9, Active: 4}}
		got, err := w.svc.GrantSummary(w.ctx(), w.complexID)
		if err != nil || got.Total != 9 || got.Active != 4 {
			t.Fatalf("got %+v / %v", got, err)
		}
	})
	t.Run("GS-19c sin sistema QR el repo deriva el codigo (qrCode vacio)", func(t *testing.T) {
		w := newWorld()
		w.svc.qr = nil
		if _, err := w.svc.CreateAccessGrant(w.ctx(), w.complexID, w.userID, CreateAccessGrantRequest{UnitID: w.unitID, Method: MethodQR, VisitorType: VisitorTypeGuest, VisitorName: "Luis", ValidUntil: w.validUntil()}); err != nil {
			t.Fatal(err)
		}
		if w.repo.createReqs[0].qrCode != "" {
			t.Fatalf("qrCode debia ir vacio: %q", w.repo.createReqs[0].qrCode)
		}
	})
}

// unitGrantsRepo / summaryRepo: STUBs de una sola respuesta sobre memRepo (el resto delega).
type unitGrantsRepo struct {
	Repository
	want uuid.UUID
	t    *testing.T
}

func (r unitGrantsRepo) GetUnitAccessGrants(_ context.Context, unit uuid.UUID, status *string) ([]*AccessGrant, error) {
	if unit != r.want || status == nil || *status != StatusActive {
		r.t.Errorf("args inesperados: %v %v", unit, status)
	}
	return nil, nil
}

type summaryRepo struct {
	Repository
	sum *GrantSummary
}

func (r summaryRepo) GrantSummary(context.Context, uuid.UUID) (*GrantSummary, error) {
	return r.sum, nil
}
