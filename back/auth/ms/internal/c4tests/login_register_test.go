package c4tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"superapp/auth/internal/core/domain"
	authtokens "superapp/auth/internal/core/usecases/auth"
	authlogin "superapp/auth/internal/core/usecases/auth/login"
	authregister "superapp/auth/internal/core/usecases/auth/register"
)

var ctx = context.Background()

const thirtyDays = 30 * 24 * time.Hour

func doLogin(e *env, email, password string) (authlogin.Presenter, error) {
	return e.login.Execute(ctx, authlogin.Requester{Email: email, Password: password, Device: "pruebas"})
}

// ===================== LOGIN (LG-01 a LG-08) =====================

func TestLG01_CredencialesCorrectasCreanSesionYTokens(t *testing.T) {
	e := newEnv(thirtyDays)
	u := e.repo.seedUser(okEmail, okPassword, "ACTIVE")
	res, err := doLogin(e, okEmail, okPassword)
	if err != nil {
		t.Fatal(err)
	}
	if res.User.ID != u.ID || res.User.Email != okEmail {
		t.Fatalf("usuario inesperado: %#v", res.User)
	}
	if res.Tokens.AccessToken == "" || res.Tokens.RefreshToken == "" {
		t.Fatalf("faltan tokens: %#v", res.Tokens)
	}
	if res.Tokens.ExpiresIn != 900 {
		t.Fatalf("ExpiresIn = %d, se esperaba 900", res.Tokens.ExpiresIn)
	}
	if e.repo.createSessionCalls != 1 || e.repo.setLastLoginCalls != 1 {
		t.Fatalf("sesion=%d ultimo_login=%d", e.repo.createSessionCalls, e.repo.setLastLoginCalls)
	}
}

func TestLG02_CorreoInexistenteDa401GenericoYGastaBcrypt(t *testing.T) {
	e := newEnv(thirtyDays)
	_, err := doLogin(e, "nadie@example.test", okPassword)
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("error = %v", err)
	}
	if e.pw.burned != 1 {
		t.Fatalf("Burn llamado %d veces, se esperaba 1", e.pw.burned)
	}
	if e.repo.createSessionCalls != 0 {
		t.Fatal("se creo una sesion para un correo inexistente")
	}
}

func TestLG03_ContrasenaIncorrectaDa401SinSesion(t *testing.T) {
	e := newEnv(thirtyDays)
	e.repo.seedUser(okEmail, okPassword, "ACTIVE")
	_, err := doLogin(e, okEmail, "otra-contrasena")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("error = %v", err)
	}
	if e.pw.burned != 0 || e.repo.createSessionCalls != 0 || e.repo.setLastLoginCalls != 0 {
		t.Fatalf("efectos no esperados: burn=%d sesion=%d ultimo_login=%d", e.pw.burned, e.repo.createSessionCalls, e.repo.setLastLoginCalls)
	}
}

func TestLG04_CuentaInactivaNoInicia(t *testing.T) {
	e := newEnv(thirtyDays)
	e.repo.seedUser(okEmail, okPassword, "SUSPENDED")
	_, err := doLogin(e, okEmail, okPassword)
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("error = %v", err)
	}
	if e.repo.createSessionCalls != 0 || e.repo.setLastLoginCalls != 0 {
		t.Fatal("una cuenta inactiva dejo rastro de sesion")
	}
}

func TestLG05_CorreoSeNormalizaAntesDeBuscar(t *testing.T) {
	e := newEnv(thirtyDays)
	e.repo.seedUser(okEmail, okPassword, "ACTIVE")
	if _, err := doLogin(e, "  ANA@Example.Test  ", okPassword); err != nil {
		t.Fatalf("el correo con mayusculas y espacios no inicio sesion: %v", err)
	}
}

func TestLG06_LosTresFallosDeCredencialesSonIdenticos(t *testing.T) {
	e := newEnv(thirtyDays)
	e.repo.seedUser(okEmail, okPassword, "ACTIVE")
	e.repo.seedUser("susp@example.test", okPassword, "SUSPENDED")
	_, errMissing := doLogin(e, "nadie@example.test", okPassword)
	_, errWrong := doLogin(e, okEmail, "mala")
	_, errInactive := doLogin(e, "susp@example.test", okPassword)
	for _, err := range []error{errMissing, errWrong, errInactive} {
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("error inesperado: %v", err)
		}
	}
	if errMissing.Error() != errWrong.Error() || errWrong.Error() != errInactive.Error() {
		t.Fatalf("los mensajes difieren: %q %q %q", errMissing, errWrong, errInactive)
	}
}

func TestLG07_FalloDeBaseDeDatosSePropagaSinTokens(t *testing.T) {
	e := newEnv(thirtyDays)
	boom := errors.New("base de datos caida")
	e.repo.errUserByEmail = boom
	res, err := doLogin(e, okEmail, okPassword)
	if !errors.Is(err, boom) || errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("error = %v", err)
	}
	if res.Tokens.AccessToken != "" || res.Tokens.RefreshToken != "" {
		t.Fatal("se entregaron tokens a pesar del fallo")
	}
}

func TestLG08_SiFallaCrearSesionNoSeEntregaNingunToken(t *testing.T) {
	e := newEnv(thirtyDays)
	e.repo.seedUser(okEmail, okPassword, "ACTIVE")
	boom := errors.New("no se pudo guardar la sesion")
	e.repo.errCreateSession = boom
	res, err := doLogin(e, okEmail, okPassword)
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v", err)
	}
	if res.Tokens.AccessToken != "" || res.Tokens.RefreshToken != "" {
		t.Fatal("se entregaron tokens sin sesion guardada")
	}
}

// ===================== REGISTRO (RG-01 a RG-12) =====================

func doRegister(e *env, email, password, name string) (authregister.Presenter, error) {
	return e.register.Execute(ctx, authregister.Requester{Email: email, Password: password, Name: name})
}

func expectInvalid(t *testing.T, e *env, err error) {
	t.Helper()
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("se esperaba argumento invalido, llego: %v", err)
	}
	if e.repo.createUserCalls != 0 {
		t.Fatal("se llamo al repositorio a pesar de la entrada invalida")
	}
}

func TestRG01_AltaValidaGuardaSoloElHash(t *testing.T) {
	e := newEnv(thirtyDays)
	pw := "Una-Clave-Larga-1"
	res, err := doRegister(e, "nuevo@example.test", pw, "Nuevo Usuario")
	if err != nil {
		t.Fatal(err)
	}
	if res.User.Email != "nuevo@example.test" || res.User.Status != "ACTIVE" {
		t.Fatalf("usuario inesperado: %#v", res.User)
	}
	if e.repo.createdHash == "" || e.repo.createdHash == pw {
		t.Fatalf("la contrasena no se guardo como hash: %q", e.repo.createdHash)
	}
}

func TestRG02_CorreoSeNormaliza(t *testing.T) {
	e := newEnv(thirtyDays)
	res, err := doRegister(e, "  Ana@Example.TEST ", "Una-Clave-Larga-1", "Ana")
	if err != nil {
		t.Fatal(err)
	}
	if res.User.Email != "ana@example.test" || e.repo.createdEmail != "ana@example.test" {
		t.Fatalf("correo no normalizado: %q / %q", res.User.Email, e.repo.createdEmail)
	}
}

func TestRG03_CorreoSinArrobaSeRechaza(t *testing.T) {
	e := newEnv(thirtyDays)
	_, err := doRegister(e, "sin-arroba", "Una-Clave-Larga-1", "Ana")
	expectInvalid(t, e, err)
}

func TestRG04_CorreoConNombreSeRechaza(t *testing.T) {
	e := newEnv(thirtyDays)
	_, err := doRegister(e, "Ana <ana@example.test>", "Una-Clave-Larga-1", "Ana")
	expectInvalid(t, e, err)
}

func TestRG05_NombreVacioOSoloEspaciosSeRechaza(t *testing.T) {
	e := newEnv(thirtyDays)
	_, err := doRegister(e, "ana@example.test", "Una-Clave-Larga-1", "   ")
	expectInvalid(t, e, err)
}

func TestRG06_NombreDe200CaracteresSeAcepta(t *testing.T) {
	e := newEnv(thirtyDays)
	if _, err := doRegister(e, "ana@example.test", "Una-Clave-Larga-1", repeat("a", 200)); err != nil {
		t.Fatalf("200 caracteres deben aceptarse: %v", err)
	}
}

func TestRG07_NombreDe201CaracteresSeRechaza(t *testing.T) {
	e := newEnv(thirtyDays)
	_, err := doRegister(e, "ana@example.test", "Una-Clave-Larga-1", repeat("a", 201))
	expectInvalid(t, e, err)
}

func TestRG08_ContrasenaDe11BytesSeRechaza(t *testing.T) {
	e := newEnv(thirtyDays)
	_, err := doRegister(e, "ana@example.test", repeat("a", 11), "Ana")
	expectInvalid(t, e, err)
}

func TestRG09_ContrasenaDe12BytesSeAcepta(t *testing.T) {
	e := newEnv(thirtyDays)
	if _, err := doRegister(e, "ana@example.test", repeat("a", 12), "Ana"); err != nil {
		t.Fatalf("12 bytes deben aceptarse: %v", err)
	}
}

func TestRG10_ContrasenaDe72BytesSeAcepta(t *testing.T) {
	e := newEnv(thirtyDays)
	if _, err := doRegister(e, "ana@example.test", repeat("a", 72), "Ana"); err != nil {
		t.Fatalf("72 bytes deben aceptarse: %v", err)
	}
}

func TestRG11_ContrasenaDe73BytesSeRechaza(t *testing.T) {
	e := newEnv(thirtyDays)
	_, err := doRegister(e, "ana@example.test", repeat("a", 73), "Ana")
	expectInvalid(t, e, err)
}

func TestRG12_CorreoYaRegistradoDaConflicto(t *testing.T) {
	e := newEnv(thirtyDays)
	if _, err := doRegister(e, "ana@example.test", "Una-Clave-Larga-1", "Ana"); err != nil {
		t.Fatal(err)
	}
	_, err := doRegister(e, "ANA@example.test", "Otra-Clave-Larga-2", "Ana Dos")
	if !errors.Is(err, domain.ErrEmailTaken) {
		t.Fatalf("error = %v, se esperaba correo repetido", err)
	}
}

// evita "imported and not used" si se cambian las pruebas
var _ = authtokens.HashRefreshToken
