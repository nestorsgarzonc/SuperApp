package qrsystem

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"sync"
	"testing"
	"time"
)

// stubClient es un STUB de Client: responde con errores fijos, sin red ni espera.
// Supuesto: un Client real respeta ctx; aqui no hace falta porque nunca bloquea.
type stubClient struct {
	issueErr error
	scanErr  error
}

func (c stubClient) Issue(context.Context) (string, error) {
	if c.issueErr != nil {
		return "", c.issueErr
	}
	return "code", nil
}
func (c stubClient) Scan(context.Context) error { return c.scanErr }

// transitionLog es un SPY del callback onChange (seguro ante concurrencia).
type transitionLog struct {
	mu  sync.Mutex
	got []string
}

func (l *transitionLog) record(from, to State) {
	l.mu.Lock()
	l.got = append(l.got, from.String()+">"+to.String())
	l.mu.Unlock()
}

func (l *transitionLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.got...)
}

// newTestSystem arma un System con reloj falso (sys.breaker.now) y un Client stub.
func newTestSystem(c Client, cfg Config) (*System, *clock) {
	if cfg.CallTimeout == 0 {
		cfg.CallTimeout = time.Second
	}
	sys := New(c, cfg, nil)
	clk := &clock{t: time.Unix(1_700_000_000, 0)}
	sys.breaker.now = clk.Now
	return sys, clk
}

// Caso CB-16a-d · Reset desde OPEN / CLOSED con fallos / con probe en vuelo / vía
// System.ResetBreaker
// Técnica: Transición de estados.
// Debe hacer / esperado: CLOSED, failures=0, transición OPEN>CLOSED (spy), sin transición
// CLOSED>CLOSED, probe liberado, aviso en logger
// de onChange registra OPEN>CLOSED; Reset en CLOSED no emite transición; ResetBreaker de System delega.
func TestCB16_ResetCierraElCircuito(t *testing.T) {
	t.Run("CB-16a desde OPEN", func(t *testing.T) {
		b, _, tr := newTestBreaker(1, time.Minute)
		_ = b.Do(context.Background(), fail)
		if b.Status().State != Open {
			t.Fatal("precondicion: debia estar OPEN")
		}
		b.Reset()
		st := b.Status()
		if st.State != Closed || st.Failures != 0 {
			t.Fatalf("tras Reset: %+v", st)
		}
		if got := *tr; len(got) != 2 || got[1] != "OPEN>CLOSED" {
			t.Fatalf("transiciones %v, esperaba [CLOSED>OPEN OPEN>CLOSED]", got)
		}
		if err := b.Do(context.Background(), ok); err != nil {
			t.Fatalf("tras Reset las llamadas deben pasar: %v", err)
		}
	})
	t.Run("CB-16b desde CLOSED con fallos acumulados", func(t *testing.T) {
		b, _, tr := newTestBreaker(3, time.Minute)
		_ = b.Do(context.Background(), fail)
		_ = b.Do(context.Background(), fail)
		b.Reset()
		if st := b.Status(); st.Failures != 0 || st.State != Closed {
			t.Fatalf("tras Reset: %+v", st)
		}
		if len(*tr) != 0 {
			t.Fatalf("CLOSED>CLOSED no es transicion: %v", *tr)
		}
	})
	t.Run("CB-16c libera probe en vuelo", func(t *testing.T) {
		b, clk, _ := newTestBreaker(1, time.Second)
		_ = b.Do(context.Background(), fail)
		clk.Advance(2 * time.Second)
		started, release := make(chan struct{}), make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.Do(context.Background(), func(context.Context) error { close(started); <-release; return nil })
		}()
		<-started
		b.Reset() // operador cierra mientras el probe sigue en vuelo
		if err := b.Do(context.Background(), ok); err != nil {
			t.Fatalf("tras Reset no debe quedar probe retenido: %v", err)
		}
		close(release)
		wg.Wait()
	})
	t.Run("CB-16d System.ResetBreaker registra la transicion en el logger", func(t *testing.T) {
		spy := &logSpy{}
		sys := New(stubClient{issueErr: errBoom}, Config{Failures: 1, OpenFor: time.Minute, CallTimeout: time.Second}, slog.New(spy))
		_, _ = sys.Issue(context.Background())
		sys.ResetBreaker()
		if sys.Availability().State != "CLOSED" {
			t.Fatalf("estado: %+v", sys.Availability())
		}
		if got := spy.msgs(); len(got) != 2 {
			t.Fatalf("esperaba 2 avisos de transicion (CLOSED>OPEN, OPEN>CLOSED), obtuve %v", got)
		}
	})
}

// logSpy es un SPY de slog.Handler que registra "from>to" de cada transicion del breaker.
type logSpy struct {
	mu   sync.Mutex
	list []string
}

func (l *logSpy) Enabled(context.Context, slog.Level) bool { return true }
func (l *logSpy) Handle(_ context.Context, r slog.Record) error {
	var from, to string
	r.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "from":
			from = a.Value.String()
		case "to":
			to = a.Value.String()
		}
		return true
	})
	l.mu.Lock()
	l.list = append(l.list, from+">"+to)
	l.mu.Unlock()
	return nil
}
func (l *logSpy) WithAttrs([]slog.Attr) slog.Handler { return l }
func (l *logSpy) WithGroup(string) slog.Handler      { return l }
func (l *logSpy) msgs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.list...)
}

// Caso CB-17a-d · ceros, negativos, límites 1, valores propios
// Técnica: Partición en clases de equivalencia, Valores límite.
// Debe hacer / esperado: 3/10 s/800 ms/32 o se conservan
// y conserva los valores >=1 (límites 0, 1 y -1).
func TestCB17_ConfigPorDefecto(t *testing.T) {
	def := Config{Failures: 3, OpenFor: 10 * time.Second, CallTimeout: 800 * time.Millisecond, MaxConcurrent: 32}
	cases := []struct {
		id   string
		in   Config
		want Config
	}{
		{"CB-17a ceros", Config{}, def},
		{"CB-17b negativos", Config{Failures: -1, OpenFor: -time.Second, CallTimeout: -time.Second, MaxConcurrent: -5}, def},
		{"CB-17c limite inferior valido", Config{Failures: 1, OpenFor: time.Nanosecond, CallTimeout: time.Nanosecond, MaxConcurrent: 1},
			Config{Failures: 1, OpenFor: time.Nanosecond, CallTimeout: time.Nanosecond, MaxConcurrent: 1}},
		{"CB-17d valores propios", Config{Failures: 7, OpenFor: time.Minute, CallTimeout: 2 * time.Second, MaxConcurrent: 4},
			Config{Failures: 7, OpenFor: time.Minute, CallTimeout: 2 * time.Second, MaxConcurrent: 4}},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			if got := tc.in.withDefaults(); got != tc.want {
				t.Fatalf("withDefaults(%+v) = %+v, esperaba %+v", tc.in, got, tc.want)
			}
		})
	}
}

// Caso CB-18 · Error(), Is, State.String (incl. desconocido)
// Técnica: Partición en clases de equivalencia.
// Debe hacer / esperado: "qr system unavailable (TIMEOUT)"; Is(ErrUnavailable) true, Is(otro)
// false
func TestCB18_UnavailableErrorYEstados(t *testing.T) {
	e := &UnavailableError{Cause: CauseTimeout}
	if got := e.Error(); got != "qr system unavailable (TIMEOUT)" {
		t.Fatalf("Error()=%q", got)
	}
	if !errors.Is(e, ErrUnavailable) {
		t.Fatal("debe coincidir con ErrUnavailable")
	}
	if errors.Is(e, errors.New("otro")) || errors.Is(e, context.Canceled) {
		t.Fatal("no debe coincidir con otros errores")
	}
	if !errors.Is(errors.Join(errBoom, e), ErrUnavailable) {
		t.Fatal("debe poder detectarse dentro de un error envuelto")
	}
	for s, want := range map[State]string{Closed: "CLOSED", Open: "OPEN", HalfOpen: "HALF_OPEN", State(99): "CLOSED"} {
		if s.String() != want {
			t.Errorf("State(%d).String()=%q, esperaba %q", int(s), s.String(), want)
		}
	}
}

// Caso CB-19a-h · quedan 2 s/1,5 s/1 s+1 ns/1 s/1 ms/1 ns; CLOSED; HALF_OPEN
// Técnica: Valores límite.
// Debe hacer / esperado: 2/2/2/1/1/1 en Availability y Detail; 0 si no OPEN
// en Detail (system.go:128 y :143): 2 s→2, 1,5 s→2, 1 s+1 ns→2, 1 s→1, 1 ms→1; 0 si no está OPEN.
func TestCB19_RedondeoDeRetryAfterSeconds(t *testing.T) {
	cases := []struct {
		id   string
		left time.Duration
		want int
	}{
		{"CB-19a quedan 2 s", 2 * time.Second, 2},
		{"CB-19b quedan 1,5 s", 1500 * time.Millisecond, 2},
		{"CB-19c quedan 1 s + 1 ns", time.Second + 1, 2},
		{"CB-19d quedan 1 s", time.Second, 1},
		{"CB-19e quedan 1 ms", time.Millisecond, 1},
		{"CB-19f quedan 1 ns", 1, 1},
	}
	const openFor = 10 * time.Second
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			sys, clk := newTestSystem(stubClient{issueErr: errBoom}, Config{Failures: 1, OpenFor: openFor})
			_, _ = sys.Issue(context.Background())
			clk.Advance(openFor - tc.left)
			a := sys.Availability()
			if a.Available || a.State != "OPEN" || a.Fallback != "PRE_AUTH" {
				t.Fatalf("debia estar OPEN con fallback: %+v", a)
			}
			if a.RetryAfterSeconds != tc.want {
				t.Fatalf("Availability.RetryAfterSeconds=%d, esperaba %d", a.RetryAfterSeconds, tc.want)
			}
			if d := sys.Detail(); d.RetryAfterSeconds != tc.want {
				t.Fatalf("Detail.RetryAfterSeconds=%d, esperaba %d", d.RetryAfterSeconds, tc.want)
			}
		})
	}
	t.Run("CB-19g CLOSED no informa espera", func(t *testing.T) {
		sys, _ := newTestSystem(stubClient{}, Config{Failures: 1})
		a := sys.Availability()
		if !a.Available || a.State != "CLOSED" || a.RetryAfterSeconds != 0 || a.Fallback != "" {
			t.Fatalf("%+v", a)
		}
		if d := sys.Detail(); d.RetryAfterSeconds != 0 || d.State != "CLOSED" {
			t.Fatalf("%+v", d)
		}
	})
	t.Run("CB-19h vencido el enfriamiento Availability informa HALF_OPEN disponible", func(t *testing.T) {
		sys, clk := newTestSystem(stubClient{issueErr: errBoom}, Config{Failures: 1, OpenFor: openFor})
		_, _ = sys.Issue(context.Background())
		clk.Advance(openFor)
		a := sys.Availability()
		if !a.Available || a.State != "HALF_OPEN" || a.RetryAfterSeconds != 0 || a.Fallback != "" {
			t.Fatalf("%+v", a)
		}
	})
}

// Caso CB-20a-d · Simulator UP/DOWN, Local, fallos acumulados
// Técnica: Partición en clases de equivalencia.
// Debe hacer / esperado: Simulated, Mode, Simulator()
// Simulator() solo devuelve el simulador cuando el cliente lo es; Failures se refleja.
func TestCB20_Detail(t *testing.T) {
	t.Run("CB-20a Simulator UP", func(t *testing.T) {
		sys, _ := newTestSystem(&Simulator{}, Config{Failures: 5})
		d := sys.Detail()
		if !d.Simulated || d.Mode != ModeUp || d.State != "CLOSED" || d.Failures != 0 {
			t.Fatalf("%+v", d)
		}
		if sim, ok := sys.Simulator(); !ok || sim == nil {
			t.Fatal("Simulator() debe devolver el simulador")
		}
	})
	t.Run("CB-20b Simulator DOWN", func(t *testing.T) {
		sim := &Simulator{}
		sim.SetMode(ModeDown)
		sys, _ := newTestSystem(sim, Config{Failures: 5})
		if d := sys.Detail(); !d.Simulated || d.Mode != ModeDown {
			t.Fatalf("%+v", d)
		}
	})
	t.Run("CB-20c Local", func(t *testing.T) {
		sys, _ := newTestSystem(Local{}, Config{Failures: 5})
		d := sys.Detail()
		if d.Simulated || d.Mode != "" {
			t.Fatalf("%+v", d)
		}
		if sim, ok := sys.Simulator(); ok || sim != nil {
			t.Fatal("Simulator() debe ser (nil,false) con Local")
		}
	})
	t.Run("CB-20d refleja fallos acumulados", func(t *testing.T) {
		sys, _ := newTestSystem(stubClient{scanErr: errBoom}, Config{Failures: 5})
		_ = sys.Scan(context.Background())
		_ = sys.Scan(context.Background())
		if d := sys.Detail(); d.Failures != 2 || d.State != "CLOSED" {
			t.Fatalf("%+v", d)
		}
	})
}

// Caso CB-21 · probe cancelado en HALF_OPEN
// Técnica: Transición de estados, Adivinación de errores.
// Debe hacer / esperado: no cuenta fallo; el siguiente llamado es probe y cierra
// `probing`: el siguiente llamado vuelve a ser probe (se ejecuta), no se rechaza como CIRCUIT_OPEN.
func TestCB21_CancelacionDelProbeLiberaElProbing(t *testing.T) {
	b, clk, tr := newTestBreaker(1, 10*time.Second)
	_ = b.Do(context.Background(), fail)
	clk.Advance(11 * time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := b.Do(ctx, func(c context.Context) error { return c.Err() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("el probe cancelado debe devolver Canceled, obtuve %v", err)
	}
	if st := b.Status(); st.Failures != 1 || st.State != HalfOpen {
		t.Fatalf("cancelar el probe no debe contar como fallo ni cerrar: %+v", st)
	}

	calls := 0
	if err := b.Do(context.Background(), func(context.Context) error { calls++; return nil }); err != nil {
		t.Fatalf("el siguiente llamado debia ser un probe valido, obtuve %v", err)
	}
	if calls != 1 || b.Status().State != Closed {
		t.Fatalf("calls=%d state=%v", calls, b.Status().State)
	}
	want := []string{"CLOSED>OPEN", "OPEN>HALF_OPEN", "HALF_OPEN>CLOSED"}
	if got := *tr; len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("transiciones %v, esperaba %v", got, want)
	}
}

// Caso CB-22a/b · inflight del probe (desde OPEN y desde HALF_OPEN)
// Técnica: Adivinación de errores.
// Debe hacer / esperado: MaxConcurrent=1: inflight=0 tras probe; 1 llamada bloqueada → la 2.ª
// OVERLOADED
// exitoso el contador inflight vuelve a 0: una llamada bloqueada ocupa el único cupo y la 2.ª es OVERLOADED.
// Si el probe no incrementara inflight, release() lo dejaría en -1 y la 2.ª pasaría.
func TestCB22_InflightDelProbe(t *testing.T) {
	overloadedAfterBlockedCall := func(t *testing.T, b *Breaker) {
		t.Helper()
		started, release := make(chan struct{}), make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.Do(context.Background(), func(context.Context) error { close(started); <-release; return nil })
		}()
		<-started
		err := b.Do(context.Background(), ok)
		var ue *UnavailableError
		if !errors.As(err, &ue) || ue.Cause != CauseOverloaded {
			t.Errorf("la 2.ª llamada debia ser OVERLOADED, obtuve %v", err)
		}
		close(release)
		wg.Wait()
	}
	newOpen := func() (*Breaker, *clock) {
		clk := &clock{t: time.Unix(1_700_000_000, 0)}
		b := NewBreaker(Config{Failures: 1, OpenFor: 10 * time.Second, CallTimeout: time.Second, MaxConcurrent: 1}, nil)
		b.now = clk.Now
		_ = b.Do(context.Background(), fail)
		clk.Advance(11 * time.Second)
		return b, clk
	}

	t.Run("CB-22a probe desde OPEN (breaker.go:154)", func(t *testing.T) {
		b, _ := newOpen()
		if err := b.Do(context.Background(), ok); err != nil {
			t.Fatalf("probe: %v", err)
		}
		b.mu.Lock()
		in := b.inflight
		b.mu.Unlock()
		if in != 0 {
			t.Fatalf("inflight=%d tras el probe, esperaba 0", in)
		}
		overloadedAfterBlockedCall(t, b)
	})
	t.Run("CB-22b probe desde HALF_OPEN tras cancelacion (rama HalfOpen)", func(t *testing.T) {
		b, _ := newOpen()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = b.Do(ctx, func(c context.Context) error { return c.Err() }) // probe cancelado, estado HALF_OPEN
		if err := b.Do(context.Background(), ok); err != nil {
			t.Fatalf("probe: %v", err)
		}
		overloadedAfterBlockedCall(t, b)
	})
}

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Caso CB-23 · Local.Issue
// Técnica: Partición en clases de equivalencia.
// Debe hacer / esperado: ^[0-9a-f]{64}$, dos llamadas distintas, Scan nil
// producen códigos distintos (el aleatorio se verifica por propiedad, no por valor); Local.Scan es nil.
func TestCB23_LocalIssueEsAleatorioYBienFormado(t *testing.T) {
	var l Local
	a, err := l.Issue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, err := l.Issue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !hex64.MatchString(a) || !hex64.MatchString(b) {
		t.Fatalf("formato invalido: %q %q", a, b)
	}
	if a == b {
		t.Fatalf("dos codigos consecutivos no pueden coincidir: %q", a)
	}
	if err := l.Scan(context.Background()); err != nil {
		t.Fatalf("Local.Scan=%v", err)
	}
}

// Caso CB-24 · Simulator UP/DOWN/modo desconocido
// Técnica: Partición en clases de equivalencia.
// Debe hacer / esperado: sin bloqueo; DOWN se libera con ctx cancelado (Canceled)
// Mode refleja SetMode y un modo desconocido equivale a UP; DOWN se libera al cancelar el contexto del llamador.
func TestCB24_SimuladorUP(t *testing.T) {
	within := func(t *testing.T, f func()) {
		t.Helper()
		done := make(chan struct{})
		go func() { f(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second): // cota de fallo, no espera fija
			t.Fatal("el simulador UP no debe bloquear")
		}
	}
	sim := &Simulator{}
	if sim.Mode() != ModeUp {
		t.Fatalf("modo inicial=%q", sim.Mode())
	}
	within(t, func() {
		code, err := sim.Issue(context.Background())
		if err != nil || !hex64.MatchString(code) {
			t.Errorf("Issue UP: %q %v", code, err)
		}
		if err := sim.Scan(context.Background()); err != nil {
			t.Errorf("Scan UP: %v", err)
		}
	})
	sim.SetMode(ModeDown)
	if sim.Mode() != ModeDown {
		t.Fatalf("modo=%q", sim.Mode())
	}
	sim.SetMode("cualquier-cosa")
	if sim.Mode() != ModeUp {
		t.Fatalf("un modo desconocido debe equivaler a UP, obtuve %q", sim.Mode())
	}

	// DOWN: bloquea hasta que el llamador cancela y devuelve el error del contexto.
	sim.SetMode(ModeDown)
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 2)
	go func() { _, err := sim.Issue(ctx); errc <- err }()
	go func() { errc <- sim.Scan(ctx) }()
	cancel()
	for i := 0; i < 2; i++ {
		select {
		case err := <-errc:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("DOWN debia devolver Canceled, obtuve %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("DOWN no se libero al cancelar el contexto")
		}
	}
}

// Caso CB-25 · System.Issue/Scan delegan y traducen errores
// Técnica: Partición en clases de equivalencia.
// Debe hacer / esperado: código emitido; error → UnavailableError{ERROR, RetryAfter=OpenFor}
// el código emitido y los errores se traducen en *UnavailableError{ERROR}.
func TestCB25_SystemDelegaEnElClient(t *testing.T) {
	sys, _ := newTestSystem(stubClient{}, Config{Failures: 3})
	if code, err := sys.Issue(context.Background()); err != nil || code != "code" {
		t.Fatalf("Issue: %q %v", code, err)
	}
	if err := sys.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	sys, _ = newTestSystem(stubClient{issueErr: errBoom, scanErr: errBoom}, Config{Failures: 9})
	var ue *UnavailableError
	if code, err := sys.Issue(context.Background()); code != "" || !errors.As(err, &ue) || ue.Cause != CauseError {
		t.Fatalf("Issue con error: %q %v", code, err)
	}
	if err := sys.Scan(context.Background()); !errors.As(err, &ue) || ue.Cause != CauseError || ue.RetryAfter != 10*time.Second {
		t.Fatalf("Scan con error: %v", err)
	}
}
