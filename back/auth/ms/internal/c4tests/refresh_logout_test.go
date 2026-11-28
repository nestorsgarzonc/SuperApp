package c4tests

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	api "superapp/auth/internal"
	"superapp/auth/internal/core/domain"
	authtokens "superapp/auth/internal/core/usecases/auth"
	authlogin "superapp/auth/internal/core/usecases/auth/login"
	authlogout "superapp/auth/internal/core/usecases/auth/logout"
	authrefresh "superapp/auth/internal/core/usecases/auth/refresh"
	appsessions "superapp/auth/internal/core/usecases/sessions"
	shared "superapp/auth/internal/presentation/shared"
)

func doRefresh(e *env, raw string) (authrefresh.Presenter, error) {
	return e.refresh.Execute(ctx, authrefresh.Requester{RefreshToken: raw})
}

// ===================== RENOVACION (RF-01 a RF-09) =====================

func TestRF01_RenovarRotaElTokenYConsumeElAnterior(t *testing.T) {
	e := newEnv(thirtyDays)
	_, oldRaw := e.loggedIn(t)
	res, err := doRefresh(e, oldRaw)
	if err != nil {
		t.Fatal(err)
	}
	if res.Tokens.AccessToken == "" || res.Tokens.RefreshToken == "" || res.Tokens.RefreshToken == oldRaw {
		t.Fatalf("par de tokens inesperado: %#v", res.Tokens)
	}
	if !e.repo.tokens[authtokens.HashRefreshToken(oldRaw)].consumed {
		t.Fatal("el token anterior no quedo consumido")
	}
	if _, ok := e.repo.tokens[authtokens.HashRefreshToken(res.Tokens.RefreshToken)]; !ok {
		t.Fatal("el token nuevo no se guardo")
	}
}

func TestRF02_ReusarUnTokenConsumidoRevocaLaSesion(t *testing.T) {
	e := newEnv(thirtyDays)
	uid, oldRaw := e.loggedIn(t)
	first, err := doRefresh(e, oldRaw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = doRefresh(e, oldRaw); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("la reutilizacion debia dar token invalido: %v", err)
	}
	if _, err = doRefresh(e, first.Tokens.RefreshToken); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("el token nuevo debia quedar revocado con la sesion: %v", err)
	}
	if e.repo.activeSessions(uid) != 0 {
		t.Fatal("la sesion sigue activa")
	}
}

func TestRF03_TokenRevocadoSeRechaza(t *testing.T) {
	e := newEnv(thirtyDays)
	_, raw := e.loggedIn(t)
	e.repo.tokens[authtokens.HashRefreshToken(raw)].revoked = true
	if _, err := doRefresh(e, raw); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("error = %v", err)
	}
}

func TestRF04_TokenDesconocidoSeRechaza(t *testing.T) {
	e := newEnv(thirtyDays)
	e.loggedIn(t)
	if _, err := doRefresh(e, "token-que-nunca-se-emitio"); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("error = %v", err)
	}
}

func TestRF05_TokenVencidoSeRechaza(t *testing.T) {
	e := newEnv(-time.Hour) // la sesion nace ya vencida
	_, raw := e.loggedIn(t)
	if _, err := doRefresh(e, raw); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("error = %v", err)
	}
}

func TestRF06_UsuarioInactivoNoRenueva(t *testing.T) {
	e := newEnv(thirtyDays)
	uid, raw := e.loggedIn(t)
	e.repo.byID[uid].Status = "SUSPENDED"
	if _, err := doRefresh(e, raw); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("error = %v", err)
	}
}

func TestRF07_TokenDeMasDe512CaracteresSeRechazaSinConsultarLaBase(t *testing.T) {
	e := newEnv(thirtyDays)
	handler := api.New(shared.Services{Refresh: e.refresh}, nil)
	body := `{"refresh_token":"` + strings.Repeat("a", 513) + `"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, cuerpo=%s", w.Code, w.Body.String())
	}
	if e.repo.rotateCalls != 0 {
		t.Fatal("se consulto la base con un token demasiado largo")
	}
}

func TestRF08_SoloSeGuardaElHashDelRefreshToken(t *testing.T) {
	e := newEnv(thirtyDays)
	_, raw := e.loggedIn(t)
	hash := authtokens.HashRefreshToken(raw)
	if len(raw) != 43 {
		t.Fatalf("token de %d caracteres, se esperaban 43 (256 bits en base64)", len(raw))
	}
	if len(hash) != 64 || hash == raw {
		t.Fatalf("hash invalido: %q", hash)
	}
	if _, ok := e.repo.tokens[raw]; ok {
		t.Fatal("el token en claro se guardo como llave")
	}
	if _, ok := e.repo.tokens[hash]; !ok {
		t.Fatal("no se guardo el hash SHA-256 del token")
	}
}

func TestRF09_FalloDeBaseDeDatosNoEsTokenInvalido(t *testing.T) {
	e := newEnv(thirtyDays)
	_, raw := e.loggedIn(t)
	boom := errors.New("base de datos caida")
	e.repo.errRotate = boom
	res, err := doRefresh(e, raw)
	if !errors.Is(err, boom) || errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("error = %v", err)
	}
	if res.Tokens.AccessToken != "" || res.Tokens.RefreshToken != "" {
		t.Fatal("se entregaron tokens a pesar del fallo")
	}
}

// ===================== CIERRE DE SESION (LO-01 a LO-04) =====================

func TestLO01_LogoutRevocaLaSesionDelToken(t *testing.T) {
	e := newEnv(thirtyDays)
	uid, raw := e.loggedIn(t)
	if _, err := e.logout.Execute(ctx, authlogout.Requester{RefreshToken: raw}); err != nil {
		t.Fatal(err)
	}
	if !e.repo.tokens[authtokens.HashRefreshToken(raw)].revoked {
		t.Fatal("el token no quedo revocado")
	}
	if e.repo.activeSessions(uid) != 0 {
		t.Fatal("la sesion sigue activa")
	}
}

func TestLO02_LogoutConTokenDesconocidoNoFalla(t *testing.T) {
	e := newEnv(thirtyDays)
	uid, _ := e.loggedIn(t)
	if _, err := e.logout.Execute(ctx, authlogout.Requester{RefreshToken: "no-existe"}); err != nil {
		t.Fatalf("un token desconocido no debe dar error: %v", err)
	}
	if e.repo.activeSessions(uid) != 1 {
		t.Fatal("se afecto una sesion ajena")
	}
}

func TestLO03_LogoutAllRevocaTodasLasSesionesSoloDelUsuario(t *testing.T) {
	e := newEnv(thirtyDays)
	uid, _ := e.loggedIn(t)
	// segunda sesion del mismo usuario
	if _, err := e.login.Execute(ctx, authlogin.Requester{Email: okEmail, Password: okPassword}); err != nil {
		t.Fatal(err)
	}
	other := e.repo.seedUser("otro@example.test", okPassword, "ACTIVE")
	if _, err := e.login.Execute(ctx, authlogin.Requester{Email: "otro@example.test", Password: okPassword}); err != nil {
		t.Fatal(err)
	}
	if e.repo.activeSessions(uid) != 2 {
		t.Fatalf("sesiones activas antes = %d", e.repo.activeSessions(uid))
	}
	svc := appsessions.New(e.repo)
	if _, err := svc.LogoutAll(ctx, appsessions.LogoutAllRequester{UserID: uid}); err != nil {
		t.Fatal(err)
	}
	if e.repo.activeSessions(uid) != 0 {
		t.Fatal("quedaron sesiones activas del usuario")
	}
	if e.repo.activeSessions(other.ID) != 1 {
		t.Fatal("se revoco la sesion de otro usuario")
	}
}

func TestLO04_TrasElLogoutNoSePuedeRenovar(t *testing.T) {
	e := newEnv(thirtyDays)
	_, raw := e.loggedIn(t)
	if _, err := e.logout.Execute(ctx, authlogout.Requester{RefreshToken: raw}); err != nil {
		t.Fatal(err)
	}
	if _, err := doRefresh(e, raw); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("error = %v", err)
	}
}
