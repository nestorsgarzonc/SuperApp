package access

// C1 — service.ValidateAccess. Dobles: memRepo (fake), noCallsRepo (mock
// estricto), stubQR, stubDirectory, spyAuditor, chanNotifier. El reloj es el
// real: las ventanas usan fechas relativas (margen de +-1 h respecto de now),
// por lo que los limites exactos (VA-13 b/c, VA-14) quedan sin probar (sin R1).

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"controldeaccesos/internal/audit"
	"controldeaccesos/internal/notifier"
	"controldeaccesos/internal/qrsystem"
)

func qrReq(code string) ValidateAccessRequest {
	return ValidateAccessRequest{Method: MethodQR, QRCode: &code}
}

func preReq(id string) ValidateAccessRequest {
	return ValidateAccessRequest{Method: MethodPreAuth, VisitorIDNumber: &id}
}

func timep(t time.Time) *time.Time { return &t }

func wantDenied(t *testing.T, resp *ValidateAccessResponse, err error, reason string) {
	t.Helper()
	if err != nil {
		t.Fatalf("no se esperaba error, hubo %v", err)
	}
	if resp == nil || resp.Valid || resp.Reason != reason {
		t.Fatalf("se esperaba Valid=false Reason=%s, hubo %+v", reason, resp)
	}
}

// Caso VA-01 · ctx sin complejo
// Técnica: Tabla de decisión.
// Entrada: ctx solo usuario
// Debe hacer / esperado: ErrComplexIDRequired; auditoría DENIED con ComplexID nil; repo sin
// llamadas (mock estricto)
func TestVA01_ContextoSinComplejoEsComplexIDRequired(t *testing.T) {
	w := newWorld()
	w.svc.repo = noCallsRepo{t}
	resp, err := w.svc.ValidateAccess(ctxWith(w.userID, uuid.Nil), qrReq("x"))
	if !errors.Is(err, ErrComplexIDRequired) || resp != nil {
		t.Fatalf("want ErrComplexIDRequired y resp nil, got %v / %+v", err, resp)
	}
	e := w.aud.only(t)
	if e.Result != audit.ResultDenied || e.ComplexID != nil || e.Action != ActionValidateGrant {
		t.Fatalf("auditoria inesperada: %+v", e)
	}
	if e.ActorID == nil || *e.ActorID != w.userID {
		t.Fatalf("el actor debe quedar auditado: %+v", e.ActorID)
	}
}

// Caso VA-02 · método no soportado
// Técnica: Tabla de decisión, Partición en clases de equivalencia.
// Entrada: "", "qr", "FACE"
// Debe hacer / esperado: ErrUnsupportedMethod; DENIED; repo sin llamadas
func TestVA02_MetodoNoSoportado(t *testing.T) {
	for _, m := range []string{"", "qr", "FACE"} {
		t.Run(fmt.Sprintf("VA-02 metodo=%q", m), func(t *testing.T) {
			w := newWorld()
			w.svc.repo = noCallsRepo{t}
			resp, err := w.svc.ValidateAccess(w.ctx(), ValidateAccessRequest{Method: m})
			if !errors.Is(err, ErrUnsupportedMethod) || resp != nil {
				t.Fatalf("want ErrUnsupportedMethod, got %v / %+v", err, resp)
			}
			if e := w.aud.only(t); e.Result != audit.ResultDenied || e.ComplexID == nil || *e.ComplexID != w.complexID {
				t.Fatalf("auditoria inesperada: %+v", e)
			}
		})
	}
}

// Caso VA-03 · QR sin código
// Técnica: Tabla de decisión, Valores límite.
// Entrada: nil, ""
// Debe hacer / esperado: ErrQRCodeRequired; lector y repo sin llamadas
func TestVA03_QRSinCodigo(t *testing.T) {
	empty := ""
	for name, req := range map[string]ValidateAccessRequest{
		"VA-03a nil":   {Method: MethodQR},
		"VA-03b vacio": {Method: MethodQR, QRCode: &empty},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld()
			w.svc.repo = noCallsRepo{t}
			resp, err := w.svc.ValidateAccess(w.ctx(), req)
			if !errors.Is(err, ErrQRCodeRequired) || resp != nil {
				t.Fatalf("want ErrQRCodeRequired, got %v / %+v", err, resp)
			}
			if _, scanned := w.qr.counts(); scanned != 0 {
				t.Fatal("el lector QR no debe consultarse sin codigo")
			}
			if e := w.aud.only(t); e.Result != audit.ResultDenied {
				t.Fatalf("auditoria inesperada: %+v", e)
			}
		})
	}
}

// Caso VA-04 · lector QR caído
// Técnica: Tabla de decisión.
// Entrada: scanErr UnavailableError
// Debe hacer / esperado: ErrUnavailable; repo sin llamadas; razón "qr system unavailable"
func TestVA04_LectorQRCaido(t *testing.T) {
	w := newWorld()
	w.svc.repo = noCallsRepo{t}
	w.qr.scanErr = &qrsystem.UnavailableError{Cause: qrsystem.CauseTimeout, RetryAfter: time.Second}
	resp, err := w.svc.ValidateAccess(w.ctx(), qrReq("x"))
	if !errors.Is(err, qrsystem.ErrUnavailable) || resp != nil {
		t.Fatalf("want ErrUnavailable, got %v / %+v", err, resp)
	}
	e := w.aud.only(t)
	if e.Result != audit.ResultDenied || e.Reason != "qr system unavailable" {
		t.Fatalf("auditoria inesperada: %+v", e)
	}
}

// Caso VA-05 · QR inexistente
// Técnica: Tabla de decisión.
// Entrada: repo vacío
// Debe hacer / esperado: Valid=false, NOT_FOUND, sin pase
func TestVA05_QRInexistenteEsNotFound(t *testing.T) {
	w := newWorld()
	resp, err := w.svc.ValidateAccess(w.ctx(), qrReq("no-existe"))
	wantDenied(t, resp, err, ReasonNotFound)
	if resp.AccessGrant != nil {
		t.Fatal("no debe devolver ningun pase")
	}
	if e := w.aud.only(t); e.Result != audit.ResultDenied || e.Reason != ErrAccessGrantNotFound.Error() {
		t.Fatalf("auditoria inesperada: %+v", e)
	}
}

// Caso VA-06 · QR de otro conjunto
// Técnica: Tabla de decisión, Adivinación de errores.
// Entrada: pase de complejo B, ctx A
// Debe hacer / esperado: respuesta DeepEqual a la de código inexistente; no se consume; auditoría
// con razón real
func TestVA06_QRDeOtroConjuntoEsNotFound(t *testing.T) {
	w := newWorld()
	g := w.grant(func(g *AccessGrant) { g.ComplexID = uuid.New() })
	other, err := w.svc.ValidateAccess(w.ctx(), qrReq(*g.QRCodeHash))
	wantDenied(t, other, err, ReasonNotFound)
	unknown, err := w.svc.ValidateAccess(w.ctx(), qrReq("no-existe"))
	wantDenied(t, unknown, err, ReasonNotFound)
	if !reflect.DeepEqual(other, unknown) {
		t.Fatalf("la respuesta debe ser indistinguible: %+v vs %+v", other, unknown)
	}
	if w.repo.count("TryMarkGrantUsed") != 0 || w.repo.snapshot(g.ID).Status != StatusActive {
		t.Fatal("un pase ajeno no debe consumirse")
	}
	es := w.aud.all()
	if len(es) != 2 || es[0].Reason != "QR belongs to a different complex" || es[0].Result != audit.ResultDenied {
		t.Fatalf("la auditoria debe guardar la razon real: %+v", es)
	}
}

// Caso VA-07 · fallo de BD (a: QR, b: PRE_AUTH)
// Técnica: Tabla de decisión, Adivinación de errores.
// Entrada: errOn
// Debe hacer / esperado: error propagado, sin resp
func TestVA07_FalloDeBDSePropaga(t *testing.T) {
	boom := errors.New("db down")
	for name, tc := range map[string]struct {
		method string
		req    ValidateAccessRequest
	}{
		"VA-07a QR":       {"GetAccessGrantByQRHash", qrReq("x")},
		"VA-07b PRE_AUTH": {"GetActiveAccessGrantsByVisitorID", preReq("123")},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld()
			w.repo.errOn[tc.method] = boom
			resp, err := w.svc.ValidateAccess(w.ctx(), tc.req)
			if !errors.Is(err, boom) || resp != nil {
				t.Fatalf("el error de BD debe propagarse (no Valid=false): %v / %+v", err, resp)
			}
		})
	}
}

// Caso VA-08 · camino feliz (a QR, b PRE_AUTH, c sin lector cableado)
// Técnica: Tabla de decisión, Transición de estados.
// Entrada: pase ACTIVE vigente
// Debe hacer / esperado: Valid=true + unidad; TryMark 1 vez con usedBy=actor; estado USED;
// auditoría ALLOWED; evento grant_validated categoría VISITOR
func TestVA08_CaminoFelizConsumeElPase(t *testing.T) {
	member := uuid.New()
	for name, withQR := range map[string]bool{"VA-08a QR": true, "VA-08c QR sin lector cableado": false} {
		t.Run(name, func(t *testing.T) {
			w := newWorld()
			w.dir.members = []uuid.UUID{member}
			n := w.withNotifier()
			if !withQR {
				w.svc.qr = nil
			}
			g := w.grant(nil)
			resp, err := w.svc.ValidateAccess(w.ctx(), qrReq(*g.QRCodeHash))
			if err != nil || resp == nil || !resp.Valid {
				t.Fatalf("se esperaba Valid=true, hubo %+v / %v", resp, err)
			}
			if resp.UnitName == nil || *resp.UnitName != "A-101" || resp.UnitID == nil || *resp.UnitID != w.unitID ||
				resp.AccessGrant == nil || resp.AccessGrant.ID != g.ID || resp.VisitorName == nil || *resp.VisitorName != "Juan" {
				t.Fatalf("respuesta incompleta: %+v", resp)
			}
			if c := w.repo.count("TryMarkGrantUsed"); c != 1 || w.repo.usedByArgs[0] != w.userID {
				t.Fatalf("TryMarkGrantUsed debia llamarse 1 vez con el actor: calls=%d args=%v", c, w.repo.usedByArgs)
			}
			if s := w.repo.snapshot(g.ID); s.Status != StatusUsed || s.UsedBy == nil || *s.UsedBy != w.userID {
				t.Fatalf("el pase debe quedar USED por el actor: %+v", s)
			}
			e := w.aud.only(t)
			if e.Result != audit.ResultAllowed || e.ResourceID == nil || *e.ResourceID != g.ID {
				t.Fatalf("auditoria inesperada: %+v", e)
			}
			ev := waitEvent(t, *n)
			if ev.Category != notifier.CategoryVisitor || ev.Data["event"] != eventGrantValidated || len(ev.UserIDs) != 1 || ev.UserIDs[0] != member {
				t.Fatalf("notificacion inesperada: %+v", ev)
			}
		})
	}
	t.Run("VA-08b PRE_AUTH", func(t *testing.T) {
		w := newWorld()
		g := w.grant(func(g *AccessGrant) { g.Method = MethodPreAuth; g.QRCodeHash = nil; g.VisitorIDNumber = strp("1010") })
		resp, err := w.svc.ValidateAccess(w.ctx(), preReq("1010"))
		if err != nil || !resp.Valid || resp.AccessGrant.ID != g.ID {
			t.Fatalf("se esperaba valido, hubo %+v / %v", resp, err)
		}
		if w.repo.snapshot(g.ID).Status != StatusUsed {
			t.Fatal("el pase PRE_AUTH debe quedar USED")
		}
	})
}

// Caso VA-09 · USED en/antes/después de ventana
// Técnica: Transición de estados, Tabla de decisión.
// Entrada: 3 ventanas
// Debe hacer / esperado: ALREADY_USED ×3; TryMark 0
func TestVA09_PaseUsadoEsAlreadyUsed(t *testing.T) {
	now := time.Now()
	hour := time.Hour
	for name, win := range map[string][2]time.Time{
		"VA-09a dentro de la ventana": {now.Add(-hour), now.Add(hour)},
		"VA-09b antes de la ventana":  {now.Add(hour), now.Add(2 * hour)},
		"VA-09c ventana vencida":      {now.Add(-2 * hour), now.Add(-hour)},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld()
			g := w.grant(func(g *AccessGrant) { g.Status = StatusUsed; g.ValidFrom = timep(win[0]); g.ValidUntil = timep(win[1]) })
			resp, err := w.svc.ValidateAccess(w.ctx(), qrReq(*g.QRCodeHash))
			wantDenied(t, resp, err, ReasonAlreadyUsed)
			if w.repo.count("TryMarkGrantUsed") != 0 {
				t.Fatal("TryMarkGrantUsed no debe llamarse")
			}
			if e := w.aud.only(t); e.Reason != "status USED" || e.Result != audit.ResultDenied {
				t.Fatalf("auditoria inesperada: %+v", e)
			}
		})
	}
}

// Caso VA-10 · CANCELLED
// Técnica: Transición de estados.
// Entrada: —
// Debe hacer / esperado: CANCELLED; sin cambios
func TestVA10_PaseCanceladoEsCancelled(t *testing.T) {
	w := newWorld()
	g := w.grant(func(g *AccessGrant) { g.Status = StatusCancelled })
	resp, err := w.svc.ValidateAccess(w.ctx(), qrReq(*g.QRCodeHash))
	wantDenied(t, resp, err, ReasonCancelled)
	if w.repo.count("TryMarkGrantUsed") != 0 || w.repo.snapshot(g.ID).Status != StatusCancelled {
		t.Fatal("un pase cancelado no debe consumirse ni cambiar de estado")
	}
}

// Caso VA-13a · antes de valid_from (partición)
// Técnica: Partición en clases de equivalencia.
// Entrada: from=now+1 h
// Debe hacer / esperado: NOT_YET_VALID con fechas; no consume
// Los límites exactos from-1ns / from / from+1ns requieren reloj inyectable (R1): no implementados.
func TestVA13_AntesDeValidFromEsNotYetValid(t *testing.T) {
	w := newWorld()
	from, until := time.Now().Add(time.Hour), time.Now().Add(2*time.Hour)
	g := w.grant(func(g *AccessGrant) { g.ValidFrom, g.ValidUntil = &from, &until })
	resp, err := w.svc.ValidateAccess(w.ctx(), qrReq(*g.QRCodeHash))
	wantDenied(t, resp, err, ReasonNotYetValid)
	if resp.ValidFrom == nil || !resp.ValidFrom.Equal(from) || resp.ValidUntil == nil || !resp.ValidUntil.Equal(until) {
		t.Fatalf("la respuesta debe informar la ventana: %+v", resp)
	}
	if w.repo.count("TryMarkGrantUsed") != 0 || w.repo.snapshot(g.ID).Status != StatusActive {
		t.Fatal("un pase aun no vigente no debe consumirse")
	}
	if e := w.aud.only(t); e.Reason != "not yet valid" {
		t.Fatalf("auditoria inesperada: %+v", e)
	}
}

// Caso VA-14b · después de valid_until (partición)
// Técnica: Partición en clases de equivalencia.
// Entrada: until=now−1 h
// Debe hacer / esperado: EXPIRED con fechas; estado sigue ACTIVE
// El calculo EXPIRED no se persiste. Límites exactos: no implementados (R1).
func TestVA14_DespuesDeValidUntilEsExpiredYNoSeEscribe(t *testing.T) {
	w := newWorld()
	from, until := time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)
	g := w.grant(func(g *AccessGrant) { g.ValidFrom, g.ValidUntil = &from, &until })
	resp, err := w.svc.ValidateAccess(w.ctx(), qrReq(*g.QRCodeHash))
	wantDenied(t, resp, err, ReasonExpired)
	if resp.ValidUntil == nil || !resp.ValidUntil.Equal(until) {
		t.Fatalf("la respuesta debe informar la ventana: %+v", resp)
	}
	if w.repo.snapshot(g.ID).Status != StatusActive || w.repo.count("TryMarkGrantUsed") != 0 {
		t.Fatal("EXPIRED es calculado: no se escribe en BD")
	}
	if e := w.aud.only(t); e.Reason != "expired" {
		t.Fatalf("auditoria inesperada: %+v", e)
	}
}

// Caso VA-15 · sin ventana
// Técnica: Partición en clases de equivalencia.
// Entrada: from/until nil
// Debe hacer / esperado: válido
func TestVA15_SinVentanaEsValido(t *testing.T) {
	w := newWorld()
	g := w.grant(func(g *AccessGrant) { g.ValidFrom, g.ValidUntil = nil, nil })
	resp, err := w.svc.ValidateAccess(w.ctx(), qrReq(*g.QRCodeHash))
	if err != nil || !resp.Valid {
		t.Fatalf("se esperaba valido, hubo %+v / %v", resp, err)
	}
	if w.repo.snapshot(g.ID).Status != StatusUsed {
		t.Fatal("debe consumirse")
	}
}

// Caso VA-16 · doble escaneo simultáneo
// Técnica: Transición de estados, Adivinación de errores.
// Entrada: 2 goroutines + barrera, memRepo atómico
// Debe hacer / esperado: exactamente 1 Valid y 1 ALREADY_USED; estado USED
// Dos goroutines liberadas por una barrera; memRepo.TryMarkGrantUsed es atómico. Correr con -race -count=50.
func TestVA16_DobleEscaneoSimultaneoSoloUnoGana(t *testing.T) {
	w := newWorld()
	g := w.grant(nil)
	const n = 2
	start := make(chan struct{})
	resps := make([]*ValidateAccessResponse, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			resps[i], errs[i] = w.svc.ValidateAccess(w.ctx(), qrReq(*g.QRCodeHash))
		}()
	}
	close(start)
	wg.Wait()

	valid, used := 0, 0
	for i := range resps {
		if errs[i] != nil || resps[i] == nil {
			t.Fatalf("goroutine %d: error inesperado %v", i, errs[i])
		}
		switch {
		case resps[i].Valid:
			valid++
		case resps[i].Reason == ReasonAlreadyUsed:
			used++
		}
	}
	if valid != 1 || used != 1 {
		t.Fatalf("se esperaba 1 Valid y 1 ALREADY_USED, hubo valid=%d used=%d", valid, used)
	}
	if w.repo.snapshot(g.ID).Status != StatusUsed {
		t.Fatal("el pase debe quedar USED")
	}
}

// Caso VA-16b · pérdida determinista
// Técnica: Tabla de decisión.
// Entrada: TryMark→false
// Debe hacer / esperado: ALREADY_USED sin datos; auditoría "already used"
func TestVA16b_TryMarkPierdeEsAlreadyUsed(t *testing.T) {
	w := newWorld()
	w.repo.loseTryMark = true
	g := w.grant(nil)
	resp, err := w.svc.ValidateAccess(w.ctx(), qrReq(*g.QRCodeHash))
	wantDenied(t, resp, err, ReasonAlreadyUsed)
	if resp.AccessGrant != nil || resp.UnitName != nil {
		t.Fatalf("no debe revelar datos del pase: %+v", resp)
	}
	if e := w.aud.only(t); e.Reason != "already used" || e.Result != audit.ResultDenied {
		t.Fatalf("auditoria inesperada: %+v", e)
	}
}

// Caso VA-17 · error al consumir
// Técnica: Tabla de decisión.
// Entrada: TryMark→error
// Debe hacer / esperado: error envuelto "failed to mark grant as used"; sin ALLOWED
func TestVA17_ErrorAlConsumirNoPermiteElAcceso(t *testing.T) {
	w := newWorld()
	boom := errors.New("tx aborted")
	w.repo.errOn["TryMarkGrantUsed"] = boom
	g := w.grant(nil)
	resp, err := w.svc.ValidateAccess(w.ctx(), qrReq(*g.QRCodeHash))
	if resp != nil || !errors.Is(err, boom) || !strings.Contains(err.Error(), "failed to mark grant as used") {
		t.Fatalf("error envuelto esperado, hubo %v / %+v", err, resp)
	}
	for _, e := range w.aud.all() {
		if e.Result == audit.ResultAllowed {
			t.Fatalf("no debe auditarse ALLOWED: %+v", e)
		}
	}
}

// Caso VA-18 · multiuso
// Técnica: Tabla de decisión.
// Entrada: single_use=false ×2
// Debe hacer / esperado: ambas Valid; TryMark 0; sigue ACTIVE
func TestVA18_MultiusoNoSeConsume(t *testing.T) {
	w := newWorld()
	g := w.grant(func(g *AccessGrant) { g.SingleUse = false })
	for i := 0; i < 2; i++ {
		resp, err := w.svc.ValidateAccess(w.ctx(), qrReq(*g.QRCodeHash))
		if err != nil || !resp.Valid {
			t.Fatalf("validacion %d: se esperaba valido, hubo %+v / %v", i, resp, err)
		}
	}
	if w.repo.count("TryMarkGrantUsed") != 0 || w.repo.snapshot(g.ID).Status != StatusActive {
		t.Fatal("un pase multiuso no se consume")
	}
}

// Caso VA-19 · PRE_AUTH sin cédula
// Técnica: Tabla de decisión, Valores límite.
// Entrada: nil, ""
// Debe hacer / esperado: ErrVisitorIDRequired; repo sin llamadas
func TestVA19_PreAuthSinCedula(t *testing.T) {
	empty := ""
	for name, req := range map[string]ValidateAccessRequest{
		"VA-19a nil":   {Method: MethodPreAuth},
		"VA-19b vacia": {Method: MethodPreAuth, VisitorIDNumber: &empty},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld()
			w.svc.repo = noCallsRepo{t}
			resp, err := w.svc.ValidateAccess(w.ctx(), req)
			if !errors.Is(err, ErrVisitorIDRequired) || resp != nil {
				t.Fatalf("want ErrVisitorIDRequired, got %v / %+v", err, resp)
			}
			if e := w.aud.only(t); e.Result != audit.ResultDenied {
				t.Fatalf("auditoria inesperada: %+v", e)
			}
		})
	}
}

// Caso VA-20 · cédula sin pase
// Técnica: Tabla de decisión.
// Entrada: pases de otra cédula/otro complejo
// Debe hacer / esperado: NO_AUTHORIZATION
func TestVA20_CedulaSinPaseEsNoAuthorization(t *testing.T) {
	w := newWorld()
	// Un pase de OTRA cédula y otro de otro complejo no deben contar.
	w.grant(func(g *AccessGrant) { g.Method = MethodPreAuth; g.VisitorIDNumber = strp("999") })
	w.grant(func(g *AccessGrant) {
		g.Method = MethodPreAuth
		g.VisitorIDNumber = strp("123")
		g.ComplexID = uuid.New()
	})
	resp, err := w.svc.ValidateAccess(w.ctx(), preReq("123"))
	wantDenied(t, resp, err, ReasonNoAuthorization)
	if e := w.aud.only(t); e.Reason != "no active authorization for visitor ID" {
		t.Fatalf("auditoria inesperada: %+v", e)
	}
}

func preGrant(w *world, from, until time.Time, created time.Time) AccessGrant {
	return w.grant(func(g *AccessGrant) {
		g.Method, g.QRCodeHash, g.VisitorIDNumber = MethodPreAuth, nil, strp("555")
		g.ValidFrom, g.ValidUntil, g.CreatedAt = &from, &until, created
	})
}

// Caso VA-21 · varios pases, uno vigente (no el más nuevo)
// Técnica: Partición en clases de equivalencia.
// Entrada: [futuro nuevo, vigente]
// Debe hacer / esperado: elige el vigente y lo consume
func TestVA21_VariosPasesEligeElVigente(t *testing.T) {
	w := newWorld()
	now := time.Now()
	preGrant(w, now.Add(time.Hour), now.Add(2*time.Hour), now.Add(-time.Minute)) // futuro, el más nuevo
	ok := preGrant(w, now.Add(-time.Hour), now.Add(time.Hour), now.Add(-time.Hour))
	resp, err := w.svc.ValidateAccess(w.ctx(), preReq("555"))
	if err != nil || !resp.Valid || resp.AccessGrant.ID != ok.ID {
		t.Fatalf("debia elegir el pase vigente %s, hubo %+v / %v", ok.ID, resp, err)
	}
	if w.repo.snapshot(ok.ID).Status != StatusUsed {
		t.Fatal("el pase elegido debe consumirse")
	}
}

// Caso VA-22 · varios, ninguno vigente
// Técnica: Partición en clases de equivalencia.
// Entrada: [vencido nuevo, futuro]
// Debe hacer / esperado: reporta el más nuevo (EXPIRED con su fecha)
func TestVA22_NingunoVigenteReportaElMasNuevo(t *testing.T) {
	w := newWorld()
	now := time.Now()
	newest := preGrant(w, now.Add(-3*time.Hour), now.Add(-2*time.Hour), now.Add(-time.Minute)) // vencido, el más nuevo
	preGrant(w, now.Add(time.Hour), now.Add(2*time.Hour), now.Add(-time.Hour))                 // futuro
	resp, err := w.svc.ValidateAccess(w.ctx(), preReq("555"))
	wantDenied(t, resp, err, ReasonExpired)
	if resp.ValidUntil == nil || !resp.ValidUntil.Equal(*newest.ValidUntil) {
		t.Fatalf("debe reportar el pase más nuevo: %+v", resp)
	}
}

// Caso VA-25 · falla nombre de unidad
// Técnica: Adivinación de errores.
// Entrada: GetUnitName→error
// Debe hacer / esperado: Valid=true, UnitName nil
func TestVA25_FallaElNombreDeLaUnidadNoBloqueaElAcceso(t *testing.T) {
	w := newWorld()
	w.dir.unitErr = errors.New("directory down")
	w.build()
	g := w.grant(nil)
	resp, err := w.svc.ValidateAccess(w.ctx(), qrReq(*g.QRCodeHash))
	if err != nil || !resp.Valid || resp.UnitName != nil {
		t.Fatalf("se esperaba Valid=true con UnitName nil, hubo %+v / %v", resp, err)
	}
	if resp.UnitID == nil || *resp.UnitID != w.unitID {
		t.Fatalf("UnitID debe seguir presente: %+v", resp)
	}
}

// Caso VA-26 · falla el auditor
// Técnica: Adivinación de errores.
// Entrada: auditor→error
// Debe hacer / esperado: respuesta idéntica a la del caso sin fallo
func TestVA26_FalloDelAuditorNoAfectaLaRespuesta(t *testing.T) {
	run := func(auditErr error) (*ValidateAccessResponse, *spyAuditor) {
		w := newWorld()
		w.aud.err = auditErr
		g := w.grant(func(g *AccessGrant) {
			g.ID = uuid.MustParse("00000000-0000-0000-0000-0000000000aa")
			hash := "qr-fijo"
			g.QRCodeHash = &hash
		})
		resp, err := w.svc.ValidateAccess(w.ctx(), qrReq(*g.QRCodeHash))
		if err != nil {
			t.Fatalf("error inesperado: %v", err)
		}
		return resp, w.aud
	}
	base, _ := run(nil)
	failing, aud := run(errors.New("audit db down"))
	if len(aud.all()) != 1 {
		t.Fatal("el auditor debia intentarse una vez")
	}
	// Los UnitID/ventanas difieren entre mundos (ids aleatorios); se compara lo observable por el guardia.
	sig := func(r *ValidateAccessResponse) string {
		return fmt.Sprintf("%v|%s|%s|%s|%s|%s|%s", r.Valid, r.Message, r.Reason, *r.UnitName, r.AccessGrant.ID, *r.VisitorName, *r.VisitorType)
	}
	if !base.Valid || sig(base) != sig(failing) {
		t.Fatalf("la respuesta debe ser identica: %s vs %s", sig(base), sig(failing))
	}
}
