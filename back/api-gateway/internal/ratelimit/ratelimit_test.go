package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// fixedClock is a settable clock (stub) injected through the limiter's now field.
type fixedClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fixedClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fixedClock) set(t time.Time) {
	c.mu.Lock()
	c.t = t
	c.mu.Unlock()
}

// windowStart is the beginning of a one-minute window.
var windowStart = time.Unix(1_800_000_000, 0).Truncate(time.Minute)

func newMemory(clock *fixedClock) *Memory {
	m := NewMemory()
	m.now = clock.now
	return m
}

func newRedis(t *testing.T, clock *fixedClock) (*Redis, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	l := NewRedis(client)
	l.now = clock.now
	return l, mr
}

// both runs the same check against the in-memory and the Redis limiter.
func both(t *testing.T, id string, fn func(t *testing.T, l Limiter, clock *fixedClock)) {
	t.Run(id+" memoria", func(t *testing.T) {
		clock := &fixedClock{t: windowStart}
		fn(t, newMemory(clock), clock)
	})
	t.Run(id+" redis", func(t *testing.T) {
		clock := &fixedClock{t: windowStart}
		l, _ := newRedis(t, clock)
		fn(t, l, clock)
	})
}

func hit(t *testing.T, l Limiter, key string, limit int) (bool, time.Duration) {
	t.Helper()
	ok, retry, err := l.Allow(context.Background(), key, limit, time.Minute)
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	return ok, retry
}

// Caso RL-01 · borde del límite
// Técnica: Valores límite (limit-1, limit, limit+1), con ambos backends.
// Entrada: limit=3, cuatro peticiones con la misma clave en la misma ventana
// Debe hacer / esperado: las tres primeras pasan con retryAfter 0; la cuarta se niega.
func TestRL01_BordeDelLimite(t *testing.T) {
	both(t, "RL-01", func(t *testing.T, l Limiter, _ *fixedClock) {
		for i := 1; i <= 3; i++ {
			if ok, retry := hit(t, l, "user:a", 3); !ok || retry != 0 {
				t.Fatalf("petición %d: ok=%v retry=%v", i, ok, retry)
			}
		}
		if ok, _ := hit(t, l, "user:a", 3); ok {
			t.Fatal("la petición limit+1 pasó")
		}
	})
}

// Caso RL-02 · límite 1
// Técnica: Valores límite (mínimo permitido por la configuración).
// Entrada: limit=1, dos peticiones
// Debe hacer / esperado: la primera pasa, la segunda se niega.
func TestRL02_LimiteUno(t *testing.T) {
	both(t, "RL-02", func(t *testing.T, l Limiter, _ *fixedClock) {
		if ok, _ := hit(t, l, "ip:1.2.3.4", 1); !ok {
			t.Fatal("primera negada")
		}
		if ok, _ := hit(t, l, "ip:1.2.3.4", 1); ok {
			t.Fatal("segunda permitida")
		}
	})
}

// Caso RL-03 · claves independientes
// Técnica: Partición en clases de equivalencia (misma clave / otra clave).
// Entrada: limit=1, una petición con "user:a" y otra con "user:b"
// Debe hacer / esperado: el consumo de una clave no afecta a la otra.
func TestRL03_ClavesIndependientes(t *testing.T) {
	both(t, "RL-03", func(t *testing.T, l Limiter, _ *fixedClock) {
		hit(t, l, "user:a", 1)
		if ok, _ := hit(t, l, "user:a", 1); ok {
			t.Fatal("user:a debía estar agotado")
		}
		if ok, _ := hit(t, l, "user:b", 1); !ok {
			t.Fatal("user:b heredó el consumo de user:a")
		}
	})
}

// Caso RL-04 · la ventana se reinicia
// Técnica: Transición de estados (agotado -> nueva ventana) y valores límite del reloj.
// Entrada: limit=1 agotado; reloj en el último nanosegundo de la ventana y luego en el primero
// de la siguiente
// Debe hacer / esperado: sigue negado hasta el último nanosegundo; en la ventana nueva pasa.
func TestRL04_LaVentanaSeReinicia(t *testing.T) {
	both(t, "RL-04", func(t *testing.T, l Limiter, clock *fixedClock) {
		hit(t, l, "k", 1)
		clock.set(windowStart.Add(time.Minute - time.Nanosecond))
		if ok, _ := hit(t, l, "k", 1); ok {
			t.Fatal("pasó antes de terminar la ventana")
		}
		clock.set(windowStart.Add(time.Minute))
		if ok, _ := hit(t, l, "k", 1); !ok {
			t.Fatal("la ventana nueva no reinició el contador")
		}
	})
}

// Caso RL-05 · tiempo hasta el reinicio
// Técnica: Valores límite (inicio, mitad y final de la ventana).
// Entrada: limit=0 (siempre negado) en t=0 s, 20 s y 60 s - 1 ns de la ventana
// Debe hacer / esperado: retryAfter = tiempo restante hasta el fin de la ventana (60 s, 40 s, 1 ns).
func TestRL05_TiempoHastaElReinicio(t *testing.T) {
	both(t, "RL-05", func(t *testing.T, l Limiter, clock *fixedClock) {
		for _, c := range []struct{ at, want time.Duration }{
			{0, time.Minute},
			{20 * time.Second, 40 * time.Second},
			{time.Minute - time.Nanosecond, time.Nanosecond},
		} {
			clock.set(windowStart.Add(c.at))
			ok, retry := hit(t, l, fmt.Sprintf("k%d", c.at), 0)
			if ok || retry != c.want {
				t.Fatalf("en %v: ok=%v retry=%v, esperaba %v", c.at, ok, retry, c.want)
			}
		}
	})
}

// Caso RL-06 · cálculo de la ventana
// Técnica: Valores límite.
// Entrada: instantes en el borde de dos ventanas de un minuto
// Debe hacer / esperado: el último nanosegundo pertenece a la ventana anterior y el siguiente
// abre una nueva, con el tiempo restante correcto.
func TestRL06_CalculoDeLaVentana(t *testing.T) {
	s1, r1 := windowOf(windowStart.Add(-time.Nanosecond), time.Minute)
	s2, r2 := windowOf(windowStart, time.Minute)
	if s2 != s1+1 || r1 != time.Nanosecond || r2 != time.Minute {
		t.Fatalf("s1=%d r1=%v s2=%d r2=%v", s1, r1, s2, r2)
	}
}

// Caso RL-07 · Redis guarda el contador con vencimiento
// Técnica: Partición (efecto observable en el backend compartido).
// Entrada: una petición con clave "user:a"
// Debe hacer / esperado: existe ratelimit:user:a:<ventana> = 1 con TTL = ventana + 1 s, para que
// Redis lo borre solo y no crezca.
func TestRL07_RedisGuardaElContadorConVencimiento(t *testing.T) {
	clock := &fixedClock{t: windowStart}
	l, mr := newRedis(t, clock)
	hit(t, l, "user:a", 5)
	start, _ := windowOf(windowStart, time.Minute)
	key := fmt.Sprintf("ratelimit:user:a:%d", start)
	if v, err := mr.Get(key); err != nil || v != "1" {
		t.Fatalf("contador %q err %v", v, err)
	}
	if ttl := mr.TTL(key); ttl != time.Minute+time.Second {
		t.Fatalf("TTL %v", ttl)
	}
	// Un segundo hit no extiende el vencimiento (ExpireNX).
	mr.FastForward(10 * time.Second)
	hit(t, l, "user:a", 5)
	if ttl := mr.TTL(key); ttl != 51*time.Second {
		t.Fatalf("TTL tras segundo hit %v, esperaba 51s", ttl)
	}
}

// Caso RL-08 · Redis caído deja pasar
// Técnica: Adivinación de errores.
// Entrada: el servidor Redis se cierra antes de la petición
// Debe hacer / esperado: allowed=true y el error se devuelve (el gateway lo registra y deja pasar).
func TestRL08_RedisCaidoDejaPasar(t *testing.T) {
	clock := &fixedClock{t: windowStart}
	l, mr := newRedis(t, clock)
	mr.Close()
	ok, retry, err := l.Allow(context.Background(), "k", 1, time.Minute)
	if !ok || retry != 0 || err == nil {
		t.Fatalf("ok=%v retry=%v err=%v", ok, retry, err)
	}
}

// Caso RL-09 · limpieza de contadores viejos en memoria
// Técnica: Valores límite (100 000 y 100 001 claves).
// Entrada: 100 001 contadores de una ventana anterior y una petición nueva
// Debe hacer / esperado: con más de 100 000 claves se borran las de otras ventanas; con
// exactamente 100 000 no se limpia.
func TestRL09_LimpiezaDeContadoresViejos(t *testing.T) {
	fill := func(m *Memory, n int, window int64) {
		for i := 0; i < n; i++ {
			m.counters[fmt.Sprintf("old%d", i)] = counter{window: window, hits: 1}
		}
	}
	clock := &fixedClock{t: windowStart}
	current, _ := windowOf(windowStart, time.Minute)

	t.Run("RL-09a 100001 claves: limpia", func(t *testing.T) {
		m := newMemory(clock)
		fill(m, 100_001, current-1)
		m.counters["live"] = counter{window: current, hits: 1}
		hit(t, m, "new", 10)
		if len(m.counters) != 2 || m.counters["live"].hits != 1 {
			t.Fatalf("quedaron %d contadores", len(m.counters))
		}
	})
	t.Run("RL-09b 100000 claves: no limpia", func(t *testing.T) {
		m := newMemory(clock)
		fill(m, 100_000, current-1)
		hit(t, m, "new", 10)
		if len(m.counters) != 100_001 {
			t.Fatalf("quedaron %d contadores", len(m.counters))
		}
	})
}

// Caso RL-10 · concurrencia en memoria
// Técnica: Adivinación de errores (condición de carrera).
// Entrada: 200 goroutines contra la misma clave con limit=50
// Debe hacer / esperado: exactamente 50 pasan (se ejecuta con -race).
func TestRL10_ConcurrenciaEnMemoria(t *testing.T) {
	m := newMemory(&fixedClock{t: windowStart})
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _, _ := m.Allow(context.Background(), "k", 50, time.Minute); ok {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed != 50 {
		t.Fatalf("pasaron %d, esperaba 50", allowed)
	}
}
