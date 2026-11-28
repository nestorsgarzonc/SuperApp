package c4tests

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	api "superapp/auth/internal"
	"superapp/auth/internal/core/domain"
	"superapp/auth/internal/core/ports/security"
	"superapp/auth/internal/core/usecases/authorization"
	shared "superapp/auth/internal/presentation/shared"
)

const (
	callerID = "00000000-0000-4000-8000-000000000001"
	targetID = "00000000-0000-4000-8000-000000000002"
	tenantA  = "00000000-0000-4000-8000-0000000000a1"
	tenantB  = "00000000-0000-4000-8000-0000000000b2"
)

func check(svc *authorization.Service, user, code string, tenant *string) (bool, error) {
	res, err := svc.CheckPermission(ctx, authorization.CheckPermissionRequester{UserID: user, PermissionCode: code, TenantID: tenant})
	return res.Allowed, err
}

// ===================== AUTORIZACION (AZ-01 a AZ-07) =====================

func TestAZ01_SinPermisosSeDeniegaPorDefecto(t *testing.T) {
	repo := newMemRepo()
	ok, err := check(authorization.New(repo), callerID, "auth.roles.read", nil)
	if err != nil || ok {
		t.Fatalf("allowed=%v err=%v", ok, err)
	}
}

func TestAZ02_PermisoConcedidoSePermite(t *testing.T) {
	repo := newMemRepo()
	repo.grant(callerID, "auth.roles.read", nil)
	ok, err := check(authorization.New(repo), callerID, "auth.roles.read", nil)
	if err != nil || !ok {
		t.Fatalf("allowed=%v err=%v", ok, err)
	}
}

func TestAZ03_CodigoDePermisoInvalidoNoConsultaLaBase(t *testing.T) {
	repo := newMemRepo()
	for _, code := range []string{"", "Invalid Code", "sinpunto", "Auth.Roles.Read"} {
		_, err := check(authorization.New(repo), callerID, code, nil)
		if !errors.Is(err, domain.ErrInvalidArgument) {
			t.Fatalf("codigo %q: error = %v", code, err)
		}
	}
	if repo.permCalls != 0 {
		t.Fatal("se consulto la base con un codigo invalido")
	}
}

// verificador de tokens (stub): "good" identifica a callerID.
type stubVerifier struct{}

func (stubVerifier) Verify(raw string) (*security.TokenClaims, error) {
	if raw == "good" {
		return &security.TokenClaims{Subject: callerID, SessionID: "s1"}, nil
	}
	return nil, errors.New("token invalido")
}

func checkPermissionHTTP(repo *memRepo, body string) *httptest.ResponseRecorder {
	handler := api.New(shared.Services{TokenVerifier: stubVerifier{}, Authorization: authorization.New(repo)}, nil)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/check-permission", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer good")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestAZ04_ConsultarOtroUsuarioSinPermisoDa403(t *testing.T) {
	repo := newMemRepo()
	repo.grant(targetID, "auth.roles.read", nil)
	w := checkPermissionHTTP(repo, `{"user_id":"`+targetID+`","permission":"auth.roles.read"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, cuerpo=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"allowed"`) {
		t.Fatal("se filtro informacion del otro usuario")
	}
}

func TestAZ05_ConsultarOtroUsuarioConAuthUsersReadFunciona(t *testing.T) {
	repo := newMemRepo()
	repo.grant(callerID, "auth.users.read", nil)
	repo.grant(targetID, "auth.roles.read", nil)
	w := checkPermissionHTTP(repo, `{"user_id":"`+targetID+`","permission":"auth.roles.read"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"allowed":true`) {
		t.Fatalf("status = %d, cuerpo=%s", w.Code, w.Body.String())
	}
}

func TestAZ06_ElConjuntoIndicadoLlegaTalCualAlRepositorio(t *testing.T) {
	repo := newMemRepo()
	repo.grant(callerID, "auth.roles.read", strPtr(tenantA))
	svc := authorization.New(repo)
	okA, _ := check(svc, callerID, "auth.roles.read", strPtr(tenantA))
	if repo.lastPerm.TenantID == nil || *repo.lastPerm.TenantID != tenantA {
		t.Fatalf("conjunto enviado al repositorio: %v", repo.lastPerm.TenantID)
	}
	okB, _ := check(svc, callerID, "auth.roles.read", strPtr(tenantB))
	if !okA || okB {
		t.Fatalf("conjunto A=%v (esperado true), conjunto B=%v (esperado false)", okA, okB)
	}
}

func TestAZ07_SinConjuntoNoSeInventaUno(t *testing.T) {
	repo := newMemRepo()
	repo.grant(callerID, "auth.roles.read", strPtr(tenantA))
	ok, _ := check(authorization.New(repo), callerID, "auth.roles.read", nil)
	if repo.lastPerm.TenantID != nil {
		t.Fatalf("se invento un conjunto: %v", *repo.lastPerm.TenantID)
	}
	if ok {
		t.Fatal("un permiso de conjunto se concedio sin indicar el conjunto")
	}
}

func strPtr(s string) *string { return &s }

// ===================== CONTEXTO DE CONJUNTO (CT-01 a CT-07) =====================

var gatewaySecret = []byte("0123456789abcdef0123456789abcdef")

func tenantRequest(secret []byte, user, tenant string, secondsAgo int, tamper bool) *http.Request {
	ts := strconv.FormatInt(time.Now().Unix()-int64(secondsAgo), 10)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(user + "\n" + tenant + "\n" + ts))
	sig := hex.EncodeToString(mac.Sum(nil))
	if tamper {
		sig = "00" + sig[2:]
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Tenant-ID", tenant)
	r.Header.Set("X-Gateway-Timestamp", ts)
	r.Header.Set("X-Gateway-Signature", sig)
	return r
}

func TestCT01_FirmaValidaDevuelveElConjunto(t *testing.T) {
	a := shared.New(shared.Services{}, gatewaySecret)
	got, err := a.Tenant(tenantRequest(gatewaySecret, callerID, tenantA, 0, false), callerID)
	if err != nil || got == nil || *got != tenantA {
		t.Fatalf("tenant=%v err=%v", got, err)
	}
}

func TestCT02_FirmaAlteradaSeRechaza(t *testing.T) {
	a := shared.New(shared.Services{}, gatewaySecret)
	_, err := a.Tenant(tenantRequest(gatewaySecret, callerID, tenantA, 0, true), callerID)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("error = %v", err)
	}
}

func TestCT03_FirmaDeHace59SegundosSeAcepta(t *testing.T) {
	a := shared.New(shared.Services{}, gatewaySecret)
	got, err := a.Tenant(tenantRequest(gatewaySecret, callerID, tenantA, 59, false), callerID)
	if err != nil || got == nil {
		t.Fatalf("tenant=%v err=%v", got, err)
	}
}

func TestCT04_FirmaDeHace61SegundosSeRechaza(t *testing.T) {
	a := shared.New(shared.Services{}, gatewaySecret)
	_, err := a.Tenant(tenantRequest(gatewaySecret, callerID, tenantA, 61, false), callerID)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("error = %v", err)
	}
}

func TestCT05_FirmaConFechaFuturaSeRechaza(t *testing.T) {
	a := shared.New(shared.Services{}, gatewaySecret)
	_, err := a.Tenant(tenantRequest(gatewaySecret, callerID, tenantA, -61, false), callerID)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("error = %v", err)
	}
}

func TestCT06_SinCabeceraNoSeInventaConjunto(t *testing.T) {
	a := shared.New(shared.Services{}, gatewaySecret)
	got, err := a.Tenant(httptest.NewRequest(http.MethodGet, "/", nil), callerID)
	if err != nil || got != nil {
		t.Fatalf("tenant=%v err=%v", got, err)
	}
}

func TestCT07_SecretoDeMenosDe32BytesNuncaAutoriza(t *testing.T) {
	short := []byte("secreto-corto")
	a := shared.New(shared.Services{}, short)
	_, err := a.Tenant(tenantRequest(short, callerID, tenantA, 0, false), callerID)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("error = %v", err)
	}
}
