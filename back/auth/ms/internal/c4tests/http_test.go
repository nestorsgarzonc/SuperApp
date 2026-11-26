package c4tests

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	api "superapp/auth/internal"
	"superapp/auth/internal/core/domain"
	authrefresh "superapp/auth/internal/core/usecases/auth/refresh"
	shared "superapp/auth/internal/presentation/shared"
)

// stubRefresh: stub del caso de uso que devuelve el error configurado y cuenta las llamadas.
type stubRefresh struct {
	err   error
	calls int
}

func (s *stubRefresh) Execute(context.Context, authrefresh.Requester) (authrefresh.Presenter, error) {
	s.calls++
	return authrefresh.Presenter{}, s.err
}

func post(h http.Handler, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

const refreshBody = `{"refresh_token":"abc"}`

// ===================== RESPUESTAS HTTP (HT-01 a HT-09) =====================

func TestHT01_CadaErrorDeDominioSeTraduceAlCodigoCorrecto(t *testing.T) {
	cases := []struct {
		err  error
		code int
	}{
		{domain.ErrInvalidToken, http.StatusUnauthorized},
		{domain.ErrInvalidCredentials, http.StatusUnauthorized},
		{domain.ErrForbidden, http.StatusForbidden},
		{domain.ErrAccountDisabled, http.StatusForbidden},
		{domain.ErrNotFound, http.StatusNotFound},
		{domain.ErrConflict, http.StatusConflict},
		{domain.ErrEmailTaken, http.StatusConflict},
		{domain.ErrInvalidArgument, http.StatusBadRequest},
	}
	for _, c := range cases {
		h := api.New(shared.Services{Refresh: &stubRefresh{err: c.err}}, nil)
		if w := post(h, "/api/v1/auth/refresh", refreshBody); w.Code != c.code {
			t.Fatalf("%v: status = %d, se esperaba %d", c.err, w.Code, c.code)
		}
	}
}

func TestHT02_ErrorDesconocidoDa500SinFiltrarDetalles(t *testing.T) {
	secret := errors.New("pq: password authentication failed for user admin")
	h := api.New(shared.Services{Refresh: &stubRefresh{err: secret}}, nil)
	w := post(h, "/api/v1/auth/refresh", refreshBody)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "pq:") || strings.Contains(w.Body.String(), "admin") {
		t.Fatalf("se filtro un detalle interno: %s", w.Body.String())
	}
}

func TestHT03_CuerpoDeMasDe1MBSeRechazaSinLlamarAlCasoDeUso(t *testing.T) {
	stub := &stubRefresh{}
	h := api.New(shared.Services{Refresh: stub}, nil)
	big := `{"refresh_token":"` + strings.Repeat("a", (1<<20)+10) + `"}`
	w := post(h, "/api/v1/auth/refresh", big)
	if w.Code != http.StatusBadRequest || stub.calls != 0 {
		t.Fatalf("status = %d, llamadas = %d", w.Code, stub.calls)
	}
}

func TestHT04_CampoDesconocidoSeRechaza(t *testing.T) {
	stub := &stubRefresh{}
	h := api.New(shared.Services{Refresh: stub}, nil)
	w := post(h, "/api/v1/auth/refresh", `{"refresh_token":"abc","extra":1}`)
	if w.Code != http.StatusBadRequest || stub.calls != 0 {
		t.Fatalf("status = %d, llamadas = %d", w.Code, stub.calls)
	}
}

func TestHT05_DosValoresJSONSeRechazan(t *testing.T) {
	stub := &stubRefresh{}
	h := api.New(shared.Services{Refresh: stub}, nil)
	w := post(h, "/api/v1/auth/refresh", refreshBody+refreshBody)
	if w.Code != http.StatusBadRequest || stub.calls != 0 {
		t.Fatalf("status = %d, llamadas = %d", w.Code, stub.calls)
	}
}

func TestHT06_JSONInvalidoSeRechaza(t *testing.T) {
	stub := &stubRefresh{}
	h := api.New(shared.Services{Refresh: stub}, nil)
	w := post(h, "/api/v1/auth/refresh", `{"refresh_token":`)
	if w.Code != http.StatusBadRequest || stub.calls != 0 {
		t.Fatalf("status = %d, llamadas = %d", w.Code, stub.calls)
	}
}

func TestHT07_MetodoNoSoportadoDa405(t *testing.T) {
	h := api.New(shared.Services{}, nil)
	r := httptest.NewRequest("TRACE", "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestHT08_NombreDeDispositivoDeMasDe128CaracteresSeRechaza(t *testing.T) {
	e := newEnv(thirtyDays)
	e.repo.seedUser(okEmail, okPassword, "ACTIVE")
	h := api.New(shared.Services{Login: e.login}, nil)
	body := `{"email":"` + okEmail + `","password":"` + okPassword + `","device_name":"` + strings.Repeat("d", 129) + `"}`
	w := post(h, "/api/v1/auth/login", body)
	if w.Code != http.StatusBadRequest || e.repo.createSessionCalls != 0 {
		t.Fatalf("status = %d, sesiones = %d", w.Code, e.repo.createSessionCalls)
	}
}

func TestHT09_LoginFallidoDa401GenericoSinNombrarElCorreo(t *testing.T) {
	e := newEnv(thirtyDays)
	h := api.New(shared.Services{Login: e.login}, nil)
	w := post(h, "/api/v1/auth/login", `{"email":"nadie@example.test","password":"x"}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "nadie@example.test") || !strings.Contains(w.Body.String(), "invalid_credentials") {
		t.Fatalf("respuesta inesperada: %s", w.Body.String())
	}
}
