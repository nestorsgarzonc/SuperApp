package membership

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	user     = "10000000-0000-0000-0000-000000000001"
	complexA = "00000000-0000-0000-0000-00000000000a"
	secret   = "internal-secret"
)

// fakeCA stands in for control-de-accesos' /internal role endpoint (stub + spy):
// it answers what each test configures and records every call.
type fakeCA struct {
	*httptest.Server
	calls   atomic.Int64
	mu      sync.Mutex
	status  int
	body    string
	lastReq *http.Request
	gate    chan struct{} // when set, each call waits until it is closed
}

func newFakeCA(t *testing.T, status int, body string) *fakeCA {
	f := &fakeCA{status: status, body: body}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		f.mu.Lock()
		f.lastReq = r.Clone(context.Background())
		status, body, gate := f.status, f.body, f.gate
		f.mu.Unlock()
		if gate != nil {
			<-gate
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeCA) answer(status int, body string) {
	f.mu.Lock()
	f.status, f.body = status, body
	f.mu.Unlock()
}

// clock is a settable time source (stub) injected through Resolver.now.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newResolver(t *testing.T, f *fakeCA, ttl time.Duration) (*Resolver, *clock) {
	t.Helper()
	base, _ := url.Parse(f.URL)
	r := NewResolver(base, secret, ttl, nil)
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	r.now = c.now
	return r, c
}

const okResident = `{"success":true,"data":{"role":"RESIDENT"}}`

// Caso MB-01 · consulta correcta al servicio interno
// Técnica: Partición (camino feliz) con spy sobre la petición.
// Entrada: control-de-accesos responde 200 con role RESIDENT
// Debe hacer / esperado: role RESIDENT, member=true; GET a /internal/users/<u>/complexes/<c>/role
// con X-Internal-Secret.
func TestMB01_ConsultaCorrecta(t *testing.T) {
	f := newFakeCA(t, 200, okResident)
	r, _ := newResolver(t, f, time.Minute)
	role, member, err := r.Role(context.Background(), user, complexA)
	if err != nil || !member || role != "RESIDENT" {
		t.Fatalf("Role = %q, %v, %v", role, member, err)
	}
	req := f.lastReq
	want := "/internal/users/" + user + "/complexes/" + complexA + "/role"
	if req.Method != http.MethodGet || req.URL.Path != want || req.Header.Get("X-Internal-Secret") != secret {
		t.Fatalf("petición %s %s secret=%q", req.Method, req.URL.Path, req.Header.Get("X-Internal-Secret"))
	}
}

// Caso MB-02 · el rol se normaliza
// Técnica: Partición en clases de equivalencia.
// Entrada: role " admin ", "Guard", "RESIDENT"
// Debe hacer / esperado: ADMIN, GUARD, RESIDENT (mayúsculas y sin espacios).
func TestMB02_ElRolSeNormaliza(t *testing.T) {
	for in, want := range map[string]string{" admin ": "ADMIN", "Guard": "GUARD", "RESIDENT": "RESIDENT"} {
		t.Run("MB-02 "+want, func(t *testing.T) {
			f := newFakeCA(t, 200, fmt.Sprintf(`{"data":{"role":%q}}`, in))
			r, _ := newResolver(t, f, time.Minute)
			if role, _, err := r.Role(context.Background(), user, complexA); err != nil || role != want {
				t.Fatalf("Role = %q, %v", role, err)
			}
		})
	}
}

// Caso MB-03 · respuestas que no permiten decidir fallan cerrado
// Técnica: Tabla de decisión (status x cuerpo) y adivinación de errores.
// Entrada: 200 con rol vacío o solo espacios, 200 con JSON inválido, 401 (secreto mal configurado),
// 403, 500 y 503
// Debe hacer / esperado: ErrUnavailable, nunca "no es miembro" ni un rol vacío.
func TestMB03_RespuestasIndeterminadasFallanCerrado(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"MB-03a rol vacío", 200, `{"data":{"role":""}}`},
		{"MB-03b rol solo espacios", 200, `{"data":{"role":"   "}}`},
		{"MB-03c sin data", 200, `{}`},
		{"MB-03d JSON inválido", 200, `{"data":`},
		{"MB-03e 401 secreto incorrecto", 401, ``},
		{"MB-03f 403", 403, ``},
		{"MB-03g 500", 500, ``},
		{"MB-03h 503", 503, ``},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeCA(t, c.status, c.body)
			r, _ := newResolver(t, f, time.Minute)
			role, member, err := r.Role(context.Background(), user, complexA)
			if !errors.Is(err, ErrUnavailable) || member || role != "" {
				t.Fatalf("Role = %q, %v, %v", role, member, err)
			}
		})
	}
}

// Caso MB-04 · 404 significa que no es miembro
// Técnica: Partición.
// Entrada: control-de-accesos responde 404
// Debe hacer / esperado: member=false sin error.
func TestMB04_NoEsMiembro(t *testing.T) {
	f := newFakeCA(t, 404, `{"success":false}`)
	r, _ := newResolver(t, f, time.Minute)
	role, member, err := r.Role(context.Background(), user, complexA)
	if err != nil || member || role != "" {
		t.Fatalf("Role = %q, %v, %v", role, member, err)
	}
}

// Caso MB-05 · servicio inalcanzable
// Técnica: Adivinación de errores.
// Entrada: el servidor de control-de-accesos está cerrado
// Debe hacer / esperado: ErrUnavailable.
func TestMB05_ServicioInalcanzable(t *testing.T) {
	f := newFakeCA(t, 200, okResident)
	r, _ := newResolver(t, f, time.Minute)
	f.Close()
	if _, _, err := r.Role(context.Background(), user, complexA); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

// Caso MB-06 · caché de respuestas positivas
// Técnica: Transición de estados (vacía -> cacheada -> vencida) y valores límite del TTL.
// Entrada: TTL 60 s; consultas en t=0, t=60 s - 1 ns y t=60 s; el servicio cambia de respuesta
// después de la primera
// Debe hacer / esperado: 1 llamada hasta el último nanosegundo del TTL; al vencer se vuelve a
// consultar y se ve el nuevo rol.
func TestMB06_CachePositiva(t *testing.T) {
	f := newFakeCA(t, 200, okResident)
	r, c := newResolver(t, f, time.Minute)
	ctx := context.Background()
	r.Role(ctx, user, complexA)
	f.answer(200, `{"data":{"role":"ADMIN"}}`)

	c.add(time.Minute - time.Nanosecond)
	if role, _, _ := r.Role(ctx, user, complexA); role != "RESIDENT" || f.calls.Load() != 1 {
		t.Fatalf("antes de vencer: role=%q llamadas=%d", role, f.calls.Load())
	}
	c.add(time.Nanosecond)
	if role, _, _ := r.Role(ctx, user, complexA); role != "ADMIN" || f.calls.Load() != 2 {
		t.Fatalf("al vencer: role=%q llamadas=%d", role, f.calls.Load())
	}
}

// Caso MB-07 · caché negativa más corta
// Técnica: Valores límite (TTL negativo = TTL/6, mínimo 1 s).
// Entrada: TTL 60 s (negativo 10 s) y TTL 3 s (negativo 1 s); respuesta 404 y luego 200
// Debe hacer / esperado: el "no miembro" se recuerda hasta el último nanosegundo del TTL negativo
// y luego se vuelve a consultar, para no bloquear a un miembro nuevo todo el TTL.
func TestMB07_CacheNegativaMasCorta(t *testing.T) {
	for _, c := range []struct {
		name     string
		ttl, neg time.Duration
	}{
		{"MB-07a TTL 60s -> 10s", time.Minute, 10 * time.Second},
		{"MB-07b TTL 3s -> mínimo 1s", 3 * time.Second, time.Second},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeCA(t, 404, ``)
			r, clk := newResolver(t, f, c.ttl)
			ctx := context.Background()
			r.Role(ctx, user, complexA)
			f.answer(200, okResident)
			clk.add(c.neg - time.Nanosecond)
			if _, member, _ := r.Role(ctx, user, complexA); member || f.calls.Load() != 1 {
				t.Fatalf("antes de vencer: member=%v llamadas=%d", member, f.calls.Load())
			}
			clk.add(time.Nanosecond)
			if _, member, _ := r.Role(ctx, user, complexA); !member || f.calls.Load() != 2 {
				t.Fatalf("al vencer: member=%v llamadas=%d", member, f.calls.Load())
			}
		})
	}
}

// Caso MB-08 · los errores no se cachean
// Técnica: Transición de estados (caído -> recuperado).
// Entrada: primera consulta con 500, segunda con 200
// Debe hacer / esperado: la segunda consulta llega al servicio y devuelve el rol.
func TestMB08_LosErroresNoSeCachean(t *testing.T) {
	f := newFakeCA(t, 500, ``)
	r, _ := newResolver(t, f, time.Minute)
	ctx := context.Background()
	if _, _, err := r.Role(ctx, user, complexA); err == nil {
		t.Fatal("esperaba error")
	}
	f.answer(200, okResident)
	if role, _, err := r.Role(ctx, user, complexA); err != nil || role != "RESIDENT" || f.calls.Load() != 2 {
		t.Fatalf("role=%q err=%v llamadas=%d", role, err, f.calls.Load())
	}
}

// Caso MB-09 · la caché es por pareja usuario-conjunto
// Técnica: Partición en clases de equivalencia.
// Entrada: misma persona en dos conjuntos y dos personas en el mismo conjunto
// Debe hacer / esperado: cada pareja se consulta por separado (3 llamadas).
func TestMB09_CachePorPareja(t *testing.T) {
	f := newFakeCA(t, 200, okResident)
	r, _ := newResolver(t, f, time.Minute)
	ctx := context.Background()
	r.Role(ctx, user, complexA)
	r.Role(ctx, user, "00000000-0000-0000-0000-00000000000b")
	r.Role(ctx, "10000000-0000-0000-0000-000000000002", complexA)
	r.Role(ctx, user, complexA)
	if n := f.calls.Load(); n != 3 {
		t.Fatalf("llamadas %d, esperaba 3", n)
	}
}

// Caso MB-10 · consultas simultáneas comparten una sola llamada
// Técnica: Adivinación de errores (estampida sobre la caché vacía).
// Entrada: 20 goroutines piden la misma pareja mientras la respuesta está retenida
// Debe hacer / esperado: control-de-accesos recibe 1 llamada y todas obtienen RESIDENT.
func TestMB10_ConsultasSimultaneasCompartenLlamada(t *testing.T) {
	f := newFakeCA(t, 200, okResident)
	f.gate = make(chan struct{})
	r, _ := newResolver(t, f, time.Minute)
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if role, _, err := r.Role(context.Background(), user, complexA); err != nil || role != "RESIDENT" {
				errs <- fmt.Errorf("role=%q err=%v", role, err)
			}
		}()
	}
	waitFor(t, func() bool { return f.calls.Load() == 1 })
	time.Sleep(20 * time.Millisecond) // deja que el resto se encole en la misma llamada
	close(f.gate)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if n := f.calls.Load(); n != 1 {
		t.Fatalf("llamadas %d, esperaba 1", n)
	}
}

// Caso MB-11 · un llamador cancelado no arrastra a los demás
// Técnica: Adivinación de errores.
// Entrada: dos consultas simultáneas; el contexto de la primera se cancela antes de la respuesta
// Debe hacer / esperado: la primera recibe context.Canceled; la segunda obtiene el rol y queda en
// caché.
func TestMB11_LlamadorCanceladoNoArrastraALosDemas(t *testing.T) {
	f := newFakeCA(t, 200, okResident)
	f.gate = make(chan struct{})
	r, _ := newResolver(t, f, time.Minute)

	ctx1, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { _, _, err := r.Role(ctx1, user, complexA); first <- err }()
	waitFor(t, func() bool { return f.calls.Load() == 1 })

	second := make(chan string, 1)
	go func() { role, _, _ := r.Role(context.Background(), user, complexA); second <- role }()
	time.Sleep(20 * time.Millisecond)

	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("primera: %v", err)
	}
	close(f.gate)
	if role := <-second; role != "RESIDENT" {
		t.Fatalf("segunda: %q", role)
	}
	if role, _, _ := r.Role(context.Background(), user, complexA); role != "RESIDENT" || f.calls.Load() != 1 {
		t.Fatalf("no quedó en caché: llamadas=%d", f.calls.Load())
	}
}

// Caso MB-12 · límite de la caché
// Técnica: Valores límite (maxEntries) y transición de estados.
// Entrada: caché llena con 100 000 entradas (a) vencidas o (b) vigentes, y una consulta nueva
// Debe hacer / esperado: (a) se borran las vencidas y queda solo la nueva; (b) se vacía y empieza
// de nuevo con la nueva, sin crecer por encima del máximo.
func TestMB12_LimiteDeLaCache(t *testing.T) {
	for _, c := range []struct {
		name    string
		expired bool
	}{
		{"MB-12a entradas vencidas", true},
		{"MB-12b entradas vigentes", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeCA(t, 200, okResident)
			r, clk := newResolver(t, f, time.Minute)
			exp := clk.now().Add(time.Hour)
			if c.expired {
				exp = clk.now()
			}
			for i := 0; i < maxEntries; i++ {
				r.entries[fmt.Sprintf("u%d|c", i)] = entry{member: true, role: "X", expires: exp}
			}
			r.Role(context.Background(), user, complexA)
			if len(r.entries) != 1 {
				t.Fatalf("quedaron %d entradas", len(r.entries))
			}
			if _, ok := r.entries[user+"|"+complexA]; !ok {
				t.Fatal("la nueva entrada no se guardó")
			}
		})
	}
}

// Caso MB-13 · cliente HTTP por defecto
// Técnica: Partición (parámetro nil).
// Entrada: NewResolver con client nil y TTL 60 s
// Debe hacer / esperado: cliente con timeout de 3 s y TTL negativo de 10 s.
func TestMB13_ClientePorDefecto(t *testing.T) {
	base, _ := url.Parse("http://ca:8080")
	r := NewResolver(base, secret, time.Minute, nil)
	if r.client == nil || r.client.Timeout != lookupTimeout || r.negTTL != 10*time.Second {
		t.Fatalf("client=%v negTTL=%v", r.client, r.negTTL)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condición no alcanzada")
		}
		time.Sleep(time.Millisecond)
	}
}
