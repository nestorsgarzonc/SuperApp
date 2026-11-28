package revocation

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newChecker(t *testing.T) (*Redis, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedis(client), mr
}

// Caso RV-01 · sesión revocada o vigente
// Técnica: Partición en clases de equivalencia.
// Entrada: auth escribió auth:revoked:sid:gone; se consulta "gone" y "live"
// Debe hacer / esperado: gone -> true, live -> false, sin error.
func TestRV01_SesionRevocadaOVigente(t *testing.T) {
	r, mr := newChecker(t)
	_ = mr.Set(KeyPrefix+"gone", "1")
	for sid, want := range map[string]bool{"gone": true, "live": false} {
		t.Run("RV-01 "+sid, func(t *testing.T) {
			got, err := r.Revoked(context.Background(), sid)
			if err != nil || got != want {
				t.Fatalf("Revoked(%q) = %v, %v", sid, got, err)
			}
		})
	}
}

// Caso RV-02 · la clave lleva el prefijo exacto que escribe auth
// Técnica: Adivinación de errores.
// Entrada: claves "gone" (sin prefijo) y "auth:revoked:gone" (prefijo incompleto)
// Debe hacer / esperado: ninguna cuenta como revocada; solo auth:revoked:sid:<sid>.
func TestRV02_PrefijoExacto(t *testing.T) {
	r, mr := newChecker(t)
	_ = mr.Set("gone", "1")
	_ = mr.Set("auth:revoked:gone", "1")
	if got, err := r.Revoked(context.Background(), "gone"); err != nil || got {
		t.Fatalf("Revoked = %v, %v", got, err)
	}
	if KeyPrefix != "auth:revoked:sid:" {
		t.Fatalf("KeyPrefix = %q, debe coincidir con el servicio auth", KeyPrefix)
	}
}

// Caso RV-03 · la revocación vence con su TTL
// Técnica: Transición de estados (revocada -> expirada).
// Entrada: clave con TTL de 15 min; el reloj de Redis avanza 15 min
// Debe hacer / esperado: true antes, false después (el token ya habría vencido).
func TestRV03_LaRevocacionVence(t *testing.T) {
	r, mr := newChecker(t)
	_ = mr.Set(KeyPrefix+"s1", "1")
	mr.SetTTL(KeyPrefix+"s1", 15*time.Minute)
	if got, _ := r.Revoked(context.Background(), "s1"); !got {
		t.Fatal("debía estar revocada")
	}
	mr.FastForward(15 * time.Minute)
	if got, _ := r.Revoked(context.Background(), "s1"); got {
		t.Fatal("seguía revocada tras vencer")
	}
}

// Caso RV-04 · Redis caído
// Técnica: Adivinación de errores.
// Entrada: el servidor Redis se cierra
// Debe hacer / esperado: devuelve el error (el gateway decide fallar abierto) y false.
func TestRV04_RedisCaido(t *testing.T) {
	r, mr := newChecker(t)
	mr.Close()
	got, err := r.Revoked(context.Background(), "s1")
	if err == nil || got {
		t.Fatalf("Revoked = %v, %v", got, err)
	}
}
