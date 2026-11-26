package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

// logger es un DUMMY: llena la firma de New y no se verifica en los casos NT-E*.
var logger = slog.New(slog.DiscardHandler)

// ── dobles ──────────────────────────────────────────────────────────────────

// reply es una respuesta programada de stubTransport: un error de red (err) o un código HTTP.
type reply struct {
	status int
	err    error
}

// sentRequest es la copia (spy) de una peticion que llego al transporte.
type sentRequest struct {
	method string
	url    string
	header http.Header
	body   []byte
	at     time.Time
}

// stubTransport es un STUB + SPY de http.RoundTripper inyectado en c.http.Transport:
// responde en orden con `replies` (la ultima se repite) y registra cada peticion.
// Supuesto (S5): http.Client entrega al Transport lo mismo que recibiria un servidor
// real (sin redirecciones ni proxies); el contrato HTTP real lo cubre NT-S1.
type stubTransport struct {
	mu      sync.Mutex
	replies []reply
	reqs    []sentRequest
}

func newStub(replies ...reply) *stubTransport { return &stubTransport{replies: replies} }

func (s *stubTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
		r.Body.Close()
	}
	s.mu.Lock()
	idx := len(s.reqs)
	s.reqs = append(s.reqs, sentRequest{method: r.Method, url: r.URL.String(), header: r.Header.Clone(), body: body, at: time.Now()})
	rep := s.replies[min(idx, len(s.replies)-1)]
	s.mu.Unlock()
	if rep.err != nil {
		return nil, rep.err
	}
	return &http.Response{StatusCode: rep.status, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}, Request: r}, nil
}

// requests devuelve una copia de lo registrado; usar solo despues de c.Close().
func (s *stubTransport) requests() []sentRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]sentRequest(nil), s.reqs...)
}

// logSpy es un SPY de slog.Handler: guarda nivel, mensaje y atributos de cada registro.
type logSpy struct {
	mu      sync.Mutex
	records []logRecord
}

type logRecord struct {
	level slog.Level
	msg   string
	attrs map[string]any
}

func (l *logSpy) Enabled(context.Context, slog.Level) bool { return true }
func (l *logSpy) Handle(_ context.Context, r slog.Record) error {
	rec := logRecord{level: r.Level, msg: r.Message, attrs: map[string]any{}}
	r.Attrs(func(a slog.Attr) bool { rec.attrs[a.Key] = a.Value.Any(); return true })
	l.mu.Lock()
	l.records = append(l.records, rec)
	l.mu.Unlock()
	return nil
}
func (l *logSpy) WithAttrs([]slog.Attr) slog.Handler { return l }
func (l *logSpy) WithGroup(string) slog.Handler      { return l }
func (l *logSpy) all() []logRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]logRecord(nil), l.records...)
}

const baseURL = "http://notificaciones.test:8083"

// newClient arma un Client con el stub inyectado en el campo c.http del mismo paquete.
func newClient(base, secret string, tr *stubTransport) (*Client, *logSpy) {
	spy := &logSpy{}
	c := New(base, secret, slog.New(spy))
	c.http = &http.Client{Transport: tr, Timeout: requestTimeout}
	return c, spy
}

func event() Event {
	return Event{UserIDs: []uuid.UUID{uuid.New()}, Category: CategoryVisitor, Title: "t", Body: "b"}
}

func decodeEvent(t *testing.T, r sentRequest) Event {
	t.Helper()
	var e Event
	if err := json.Unmarshal(r.body, &e); err != nil {
		t.Fatalf("cuerpo no es JSON de Event: %v (%s)", err, r.body)
	}
	return e
}

// ── casos existentes, migrados a stubTransport ──────────────────────────────

// Caso NT-E1..E4 · 4 existentes migrados de httptest.Server a stubTransport
// Técnica: Partición en clases de equivalencia, Transición de estados.
// Debe hacer / esperado: misma lógica y mismos oráculos
func TestNTE1_EnviaEventoConSecreto(t *testing.T) {
	tr := newStub(reply{status: http.StatusAccepted})
	c, _ := newClient(baseURL+"/", "s3cret", tr)
	user := uuid.New()
	c.Notify(context.Background(), Event{UserIDs: []uuid.UUID{user}, Category: CategoryVisitor, Title: "t", Body: "b"})
	_ = c.Close()

	reqs := tr.requests()
	if len(reqs) != 1 {
		t.Fatalf("peticiones=%d, esperaba 1", len(reqs))
	}
	got := decodeEvent(t, reqs[0])
	if !strings.HasSuffix(reqs[0].url, "/internal/notifications") || reqs[0].header.Get("X-Internal-Secret") != "s3cret" {
		t.Fatalf("url=%q secret=%q", reqs[0].url, reqs[0].header.Get("X-Internal-Secret"))
	}
	if got.Source != Source || got.EventID == "" || len(got.UserIDs) != 1 || got.UserIDs[0] != user {
		t.Fatalf("evento inesperado: %+v", got)
	}
}

// Caso NT-E1..E4 · 4 existentes migrados de httptest.Server a stubTransport
// Técnica: Partición en clases de equivalencia, Transición de estados.
// Debe hacer / esperado: misma lógica y mismos oráculos
// Espera real: un backoff de 500 ms (R3 no aprobado: no se puede inyectar el sleep).
func TestNTE2_ReintentaErrores5xxConElMismoEventID(t *testing.T) {
	t.Parallel()
	tr := newStub(reply{status: http.StatusInternalServerError}, reply{status: http.StatusAccepted})
	c, _ := newClient(baseURL, "", tr)
	c.Notify(context.Background(), Event{UserIDs: []uuid.UUID{uuid.New()}, Category: CategoryAccess, Title: "t", Body: "b"})
	_ = c.Close()

	reqs := tr.requests()
	if len(reqs) != 2 {
		t.Fatalf("llamadas=%d, esperaba 2", len(reqs))
	}
	if a, b := decodeEvent(t, reqs[0]).EventID, decodeEvent(t, reqs[1]).EventID; a != b || a == "" {
		t.Fatalf("el reintento cambio el event_id: %q != %q", a, b)
	}
}

// Caso NT-E1..E4 · 4 existentes migrados de httptest.Server a stubTransport
// Técnica: Partición en clases de equivalencia, Transición de estados.
// Debe hacer / esperado: misma lógica y mismos oráculos
func TestNTE3_NoReintentaEventosRechazados(t *testing.T) {
	tr := newStub(reply{status: http.StatusUnprocessableEntity})
	c, _ := newClient(baseURL, "", tr)
	c.Notify(context.Background(), Event{UserIDs: []uuid.UUID{uuid.New()}, Category: "NOPE", Title: "t", Body: "b"})
	_ = c.Close()
	if n := len(tr.requests()); n != 1 {
		t.Fatalf("llamadas=%d, esperaba 1", n)
	}
}

// Caso NT-E1..E4 · 4 existentes migrados de httptest.Server a stubTransport
// Técnica: Partición en clases de equivalencia, Transición de estados.
// Debe hacer / esperado: misma lógica y mismos oráculos
func TestNTE4_NoHaceNadaSinURLOSinDestinatarios(t *testing.T) {
	tr := newStub(reply{status: http.StatusOK})
	c, _ := newClient("", "", tr)
	c.Notify(context.Background(), Event{UserIDs: []uuid.UUID{uuid.New()}})
	c2, _ := newClient(baseURL, "", tr)
	c2.Notify(context.Background(), Event{})
	_ = c.Close()
	_ = c2.Close()
	if n := len(tr.requests()); n != 0 {
		t.Fatalf("llamadas=%d, esperaba 0", n)
	}
}

// ── casos nuevos ─────────────────────────────────────────────────────────────

// Caso NT-01 · envío básico
// Técnica: Partición en clases de equivalencia.
// Debe hacer / esperado: POST a {base}/internal/notifications, Content-Type, secreto, source,
// event_id UUIDv7 (versión 7), resto del evento intacto
// source fijo, event_id UUIDv7, y el resto del evento intacto.
func TestNT01_EnvioBasicoPorPOST(t *testing.T) {
	tr := newStub(reply{status: http.StatusAccepted})
	c, _ := newClient(baseURL, "s3cret", tr)
	complexID, user := uuid.New(), uuid.New()
	c.Notify(context.Background(), Event{
		ComplexID: &complexID, UserIDs: []uuid.UUID{user}, Category: CategoryAccess,
		Title: "Titulo", Body: "Cuerpo", Data: map[string]any{"grant_id": "g-1"},
	})
	_ = c.Close()

	reqs := tr.requests()
	if len(reqs) != 1 {
		t.Fatalf("peticiones=%d", len(reqs))
	}
	r := reqs[0]
	if r.method != http.MethodPost || r.url != baseURL+"/internal/notifications" {
		t.Fatalf("metodo/url: %s %s", r.method, r.url)
	}
	if r.header.Get("Content-Type") != "application/json" || r.header.Get("X-Internal-Secret") != "s3cret" {
		t.Fatalf("cabeceras: %v", r.header)
	}
	got := decodeEvent(t, r)
	id, err := uuid.Parse(got.EventID)
	if err != nil || id.Version() != 7 {
		t.Fatalf("event_id=%q no es UUIDv7 (err=%v)", got.EventID, err)
	}
	if got.Source != "control-de-accesos" || got.Category != CategoryAccess || got.Title != "Titulo" || got.Body != "Cuerpo" {
		t.Fatalf("evento: %+v", got)
	}
	if got.ComplexID == nil || *got.ComplexID != complexID || len(got.UserIDs) != 1 || got.UserIDs[0] != user || got.Data["grant_id"] != "g-1" {
		t.Fatalf("evento: %+v", got)
	}
}

// Caso NT-02 · sin secreto
// Técnica: Partición en clases de equivalencia.
// Debe hacer / esperado: cabecera ausente
func TestNT02_SinSecretoNoHayCabecera(t *testing.T) {
	tr := newStub(reply{status: http.StatusOK})
	c, _ := newClient(baseURL, "", tr)
	c.Notify(context.Background(), event())
	_ = c.Close()
	reqs := tr.requests()
	if len(reqs) != 1 {
		t.Fatalf("peticiones=%d", len(reqs))
	}
	if _, present := reqs[0].header["X-Internal-Secret"]; present {
		t.Fatalf("la cabecera no debia enviarse: %v", reqs[0].header)
	}
}

// Caso NT-03a-c · base con 0/1/3 "/" finales
// Técnica: Valores límite.
// Debe hacer / esperado: URL sin "//"
func TestNT03_BaseURLConBarraFinal(t *testing.T) {
	for _, tc := range []struct{ id, base string }{
		{"NT-03a sin barra", baseURL},
		{"NT-03b una barra", baseURL + "/"},
		{"NT-03c varias barras", baseURL + "///"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			tr := newStub(reply{status: http.StatusOK})
			c, _ := newClient(tc.base, "", tr)
			c.Notify(context.Background(), event())
			_ = c.Close()
			reqs := tr.requests()
			if len(reqs) != 1 || reqs[0].url != baseURL+"/internal/notifications" {
				t.Fatalf("url=%v", reqs)
			}
		})
	}
}

// Caso NT-04a-c · sin URL / UserIDs nil / vacío
// Técnica: Partición en clases de equivalencia.
// Debe hacer / esperado: 0 llamadas, 0 avisos
func TestNT04_SinURLOSinDestinatariosNoLlamaAlTransporte(t *testing.T) {
	cases := []struct {
		id   string
		base string
		ev   Event
	}{
		{"NT-04a sin URL", "", event()},
		{"NT-04b UserIDs nil", baseURL, Event{Category: CategoryAccess}},
		{"NT-04c UserIDs vacio", baseURL, Event{UserIDs: []uuid.UUID{}, Category: CategoryAccess}},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			tr := newStub(reply{status: http.StatusOK})
			c, spy := newClient(tc.base, "", tr)
			c.Notify(context.Background(), tc.ev)
			_ = c.Close()
			if n := len(tr.requests()); n != 0 {
				t.Fatalf("llamadas=%d, esperaba 0", n)
			}
			if n := len(spy.all()); n != 0 {
				t.Fatalf("un no-op no debe registrar avisos: %v", spy.all())
			}
		})
	}
}

// failureLog devuelve el aviso "event delivery failed" registrado (o nil si no hubo).
func failureLog(spy *logSpy) *logRecord {
	for _, r := range spy.all() {
		if r.msg == "notifier: event delivery failed" {
			r := r
			return &r
		}
	}
	return nil
}

// Caso NT-05 · 5xx persistente
// Técnica: Transición de estados.
// Debe hacer / esperado: 3 intentos, mismo event_id, huecos ≥500 ms y ≥1 s, total ≥1,5 s, aviso
// attempts=3
// backoff lineal (500 ms, luego 1000 ms); al agotarse se registra 1 aviso con attempts=3.
// Sin R3 no se puede inyectar el sleep: se espera el backoff real (~1,5 s) y se verifica el espaciado
// con COTAS INFERIORES entre peticiones (no hay cotas superiores: serian inestables).
func TestNT05_Persistente5xxAgotaTresIntentos(t *testing.T) {
	t.Parallel()
	tr := newStub(reply{status: http.StatusServiceUnavailable})
	c, spy := newClient(baseURL, "", tr)
	start := time.Now()
	c.Notify(context.Background(), event())
	_ = c.Close()

	reqs := tr.requests()
	if len(reqs) != 3 {
		t.Fatalf("intentos=%d, esperaba exactamente 3", len(reqs))
	}
	ids := map[string]bool{}
	for _, r := range reqs {
		ids[decodeEvent(t, r).EventID] = true
	}
	if len(ids) != 1 {
		t.Fatalf("los reintentos deben conservar el event_id: %v", ids)
	}
	if gap := reqs[1].at.Sub(reqs[0].at); gap < 500*time.Millisecond {
		t.Fatalf("espera entre intento 1 y 2 = %v, minimo 500ms", gap)
	}
	if gap := reqs[2].at.Sub(reqs[1].at); gap < time.Second {
		t.Fatalf("espera entre intento 2 y 3 = %v, minimo 1s (backoff lineal)", gap)
	}
	if time.Since(start) < 1500*time.Millisecond {
		t.Fatalf("duracion total %v < 1,5 s", time.Since(start))
	}
	rec := failureLog(spy)
	if rec == nil || rec.level != slog.LevelWarn || rec.attrs["attempts"] != int64(3) {
		t.Fatalf("aviso de agotamiento incorrecto: %+v", spy.all())
	}
}

// Caso NT-06 · error de red
// Técnica: Adivinación de errores.
// Debe hacer / esperado: 3 intentos, aviso con la causa
// Espera real de ~1,5 s (ver NT-05).
func TestNT06_ErrorDeRedSeReintentaTresVeces(t *testing.T) {
	t.Parallel()
	netErr := errors.New("connection refused")
	tr := newStub(reply{err: netErr})
	c, spy := newClient(baseURL, "", tr)
	c.Notify(context.Background(), event())
	_ = c.Close()

	if n := len(tr.requests()); n != 3 {
		t.Fatalf("intentos=%d, esperaba 3", n)
	}
	rec := failureLog(spy)
	if rec == nil || rec.attrs["attempts"] != int64(3) {
		t.Fatalf("aviso inesperado: %+v", spy.all())
	}
	if err, _ := rec.attrs["error"].(error); err == nil || !errors.Is(err, netErr) {
		t.Fatalf("el aviso debe incluir la causa de red: %v", rec.attrs["error"])
	}
}

// Caso NT-07 · 4xx
// Técnica: Partición en clases de equivalencia.
// Debe hacer / esperado: 1 intento, aviso attempts=1
func TestNT07_Un4xxNoSeReintenta(t *testing.T) {
	tr := newStub(reply{status: http.StatusBadRequest})
	c, spy := newClient(baseURL, "", tr)
	c.Notify(context.Background(), event())
	_ = c.Close()
	if n := len(tr.requests()); n != 1 {
		t.Fatalf("intentos=%d, esperaba 1", n)
	}
	rec := failureLog(spy)
	if rec == nil || rec.attrs["attempts"] != int64(1) {
		t.Fatalf("aviso inesperado: %+v", spy.all())
	}
}

// Caso NT-08a-e · 200, 299, 300, 499, 500→200
// Técnica: Valores límite.
// Debe hacer / esperado: éxito sin aviso (200/299), rechazo con aviso y 1 intento (300/499), 2
// intentos (500)
// 300 y 499 = rechazo (1 intento, con aviso); 500 = se reintenta (2.º intento 200 → éxito).
func TestNT08_LimitesDeCodigoDeEstado(t *testing.T) {
	t.Parallel()
	cases := []struct {
		id       string
		replies  []reply
		attempts int
		warned   bool
	}{
		{"NT-08a 200", []reply{{status: 200}}, 1, false},
		{"NT-08b 299", []reply{{status: 299}}, 1, false},
		{"NT-08c 300", []reply{{status: 300}}, 1, true},
		{"NT-08d 499", []reply{{status: 499}}, 1, true},
		{"NT-08e 500 y luego 200", []reply{{status: 500}, {status: 200}}, 2, false},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			tr := newStub(tc.replies...)
			c, spy := newClient(baseURL, "", tr)
			c.Notify(context.Background(), event())
			_ = c.Close()
			if n := len(tr.requests()); n != tc.attempts {
				t.Fatalf("intentos=%d, esperaba %d", n, tc.attempts)
			}
			if warned := failureLog(spy) != nil; warned != tc.warned {
				t.Fatalf("aviso=%v, esperaba %v (%+v)", warned, tc.warned, spy.all())
			}
		})
	}
}

// Caso NT-09 · éxito en 2.º intento
// Técnica: Transición de estados.
// Debe hacer / esperado: 2 llamadas, hueco ≥500 ms, sin aviso
// (backoff real, ver NT-05) y ningún aviso de fallo.
func TestNT09_ExitoEnElSegundoIntento(t *testing.T) {
	t.Parallel()
	tr := newStub(reply{err: errors.New("reset by peer")}, reply{status: http.StatusAccepted})
	c, spy := newClient(baseURL, "", tr)
	c.Notify(context.Background(), event())
	_ = c.Close()

	reqs := tr.requests()
	if len(reqs) != 2 {
		t.Fatalf("intentos=%d, esperaba 2", len(reqs))
	}
	if gap := reqs[1].at.Sub(reqs[0].at); gap < 500*time.Millisecond {
		t.Fatalf("espera entre intentos = %v, minimo 500ms", gap)
	}
	if rec := failureLog(spy); rec != nil {
		t.Fatalf("no debia avisar fallo: %+v", rec)
	}
}

// Caso NT-10 · técnica: PE · condición: un EventID provisto se conserva (idempotencia del llamador);
// Source siempre lo fija el cliente, aunque el llamador envíe otro.
func TestNT10_EventIDProvistoSeConserva(t *testing.T) {
	tr := newStub(reply{status: http.StatusOK})
	c, _ := newClient(baseURL, "", tr)
	ev := event()
	ev.EventID = "evt-fijo-123"
	ev.Source = "otro-servicio"
	c.Notify(context.Background(), ev)
	_ = c.Close()
	reqs := tr.requests()
	if len(reqs) != 1 {
		t.Fatalf("peticiones=%d", len(reqs))
	}
	got := decodeEvent(t, reqs[0])
	if got.EventID != "evt-fijo-123" || got.Source != Source {
		t.Fatalf("event_id=%q source=%q", got.EventID, got.Source)
	}
}

// Caso NT-10b · técnica: PE (propiedad) · condición: sin EventID, dos eventos reciben ids distintos.
func TestNT10b_EventIDGeneradoEsUnico(t *testing.T) {
	tr := newStub(reply{status: http.StatusOK})
	c, _ := newClient(baseURL, "", tr)
	c.Notify(context.Background(), event())
	c.Notify(context.Background(), event())
	_ = c.Close()
	reqs := tr.requests()
	if len(reqs) != 2 {
		t.Fatalf("peticiones=%d", len(reqs))
	}
	if a, b := decodeEvent(t, reqs[0]).EventID, decodeEvent(t, reqs[1]).EventID; a == b {
		t.Fatalf("event_id repetido: %q", a)
	}
}

// Caso NT-11a/b · Close espera la entrega con reintento; Close sin trabajo
// Técnica: Transición de estados.
// Debe hacer / esperado: tras Close hay 2 peticiones; nil, idempotente
// (5xx, luego 202: Close debe esperar el backoff); y sin nada en vuelo retorna de inmediato con nil.
func TestNT11_CloseEsperaLaEntrega(t *testing.T) {
	t.Parallel()
	t.Run("NT-11a espera el reintento", func(t *testing.T) {
		t.Parallel()
		tr := newStub(reply{status: 500}, reply{status: 202})
		c, _ := newClient(baseURL, "", tr)
		c.Notify(context.Background(), event())
		if err := c.Close(); err != nil {
			t.Fatalf("Close=%v", err)
		}
		// Si Close no esperara, aqui habria 0 o 1 peticion.
		if n := len(tr.requests()); n != 2 {
			t.Fatalf("tras Close debia estar terminada la entrega: peticiones=%d", n)
		}
	})
	t.Run("NT-11b sin nada en vuelo", func(t *testing.T) {
		c, _ := newClient(baseURL, "", newStub(reply{status: 200}))
		if err := c.Close(); err != nil {
			t.Fatalf("Close=%v", err)
		}
		if err := c.Close(); err != nil {
			t.Fatalf("Close repetido=%v", err)
		}
	})
}

// Caso NT-12 · URL no construible
// Técnica: Adivinación de errores.
// Debe hacer / esperado: 0 llamadas al transporte, 1 intento, aviso
// 1 intento fallido sin transporte, sin reintento y con aviso (rama de http.NewRequest).
func TestNT12_URLInvalidaNoSeReintenta(t *testing.T) {
	tr := newStub(reply{status: 200})
	c, spy := newClient("http://host malo", "", tr)
	c.Notify(context.Background(), event())
	_ = c.Close()
	if n := len(tr.requests()); n != 0 {
		t.Fatalf("no debia llegar al transporte: %d", n)
	}
	rec := failureLog(spy)
	if rec == nil || rec.attrs["attempts"] != int64(1) {
		t.Fatalf("aviso inesperado: %+v", spy.all())
	}
}

// Caso NT-13 · Data no serializable
// Técnica: Adivinación de errores.
// Debe hacer / esperado: 0 llamadas, aviso "failed to encode event"
// event", sin llamar al transporte y sin bloquear al llamador.
func TestNT13_EventoNoSerializableSeDescartaConAviso(t *testing.T) {
	tr := newStub(reply{status: 200})
	c, spy := newClient(baseURL, "", tr)
	ev := event()
	ev.Data = map[string]any{"bad": make(chan int)}
	c.Notify(context.Background(), ev)
	_ = c.Close()
	if n := len(tr.requests()); n != 0 {
		t.Fatalf("llamadas=%d, esperaba 0", n)
	}
	recs := spy.all()
	if len(recs) != 1 || recs[0].msg != "notifier: failed to encode event" || recs[0].attrs["category"] != CategoryVisitor {
		t.Fatalf("aviso inesperado: %+v", recs)
	}
}

// Caso NT-14 · configuración por defecto
// Técnica: Partición en clases de equivalencia.
// Debe hacer / esperado: timeout 3 s, URL normalizada
func TestNT14_ConfiguracionPorDefecto(t *testing.T) {
	c := New("", "x", logger)
	if c.url != "" || c.http.Timeout != 3*time.Second || c.secret != "x" {
		t.Fatalf("cliente inesperado: %+v", c)
	}
	if c := New(baseURL+"//", "", logger); c.url != baseURL+"/internal/notifications" {
		t.Fatalf("url=%q", c.url)
	}
}

// Caso NT-S1 · contrato HTTP real
// Técnica: Partición en clases de equivalencia.
// Debe hacer / esperado: método, ruta, cabeceras y cuerpo vistos por un servidor loopback; sin
// reintento en 202
// método, ruta, cabeceras y cuerpo llegan tal como los lee un servidor real, y un 202 no se reintenta.
func TestNTS1_ContratoHTTPContraServidorReal(t *testing.T) {
	var calls atomic.Int32
	var (
		mu      sync.Mutex
		method  string
		path    string
		secret  string
		ctype   string
		rawBody []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		method, path, secret, ctype, rawBody = r.Method, r.URL.Path, r.Header.Get("X-Internal-Secret"), r.Header.Get("Content-Type"), b
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	c := New(srv.URL+"/", "s3cret", logger)
	user := uuid.New()
	c.Notify(context.Background(), Event{UserIDs: []uuid.UUID{user}, Category: CategoryVisitor, Title: "t", Body: "b"})
	_ = c.Close()

	mu.Lock()
	defer mu.Unlock()
	if calls.Load() != 1 || method != http.MethodPost || path != "/internal/notifications" || secret != "s3cret" || ctype != "application/json" {
		t.Fatalf("calls=%d method=%s path=%s secret=%q ctype=%q", calls.Load(), method, path, secret, ctype)
	}
	var got Event
	if err := json.NewDecoder(bytes.NewReader(rawBody)).Decode(&got); err != nil {
		t.Fatalf("cuerpo: %v", err)
	}
	if got.Source != Source || got.EventID == "" || len(got.UserIDs) != 1 || got.UserIDs[0] != user {
		t.Fatalf("evento: %+v", got)
	}
}
