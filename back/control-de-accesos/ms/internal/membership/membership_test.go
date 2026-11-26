package membership

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"controldeaccesos/internal/directory"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type fakeDir struct {
	memberships []directory.Membership
	role        string
	found       bool
	err         error
}

func (f fakeDir) ListMemberships(context.Context, uuid.UUID) ([]directory.Membership, error) {
	return f.memberships, f.err
}
func (f fakeDir) GetUserComplexRole(context.Context, uuid.UUID, uuid.UUID) (string, bool, error) {
	return f.role, f.found, f.err
}
func (f fakeDir) GetUserDisplayName(context.Context, uuid.UUID) (string, error) { return "Ana", nil }

func router(dir Directory, internalSecret string) http.Handler {
	r := chi.NewRouter()
	h := NewHandler(dir)
	RegisterInternalRoutes(r, h, internalSecret)
	r.Route("/api/v1", func(r chi.Router) { RegisterRoutes(r, h) })
	return r
}

// Caso ME-01 · GET /me devuelve las membresías de quien llama.
// Entrada: Usuario autenticado con membresías simuladas
// Debe hacer / esperado: 200 con sus membresías
func TestME01_MeReturnsTheCallersMemberships(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("GATEWAY_SECRET", "gw")
	user, cx := uuid.New(), uuid.New()
	unitName := "A-101"
	dir := fakeDir{memberships: []directory.Membership{{ComplexID: cx, ComplexName: "Parques", Role: "RESIDENT", UnitName: &unitName}}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/memberships", nil)
	req.Header.Set("X-Gateway-Secret", "gw")
	req.Header.Set("X-User-ID", user.String())
	rec := httptest.NewRecorder()
	router(dir, "").ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		Data meResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.UserID != user || body.Data.Name != "Ana" || len(body.Data.Memberships) != 1 || body.Data.Memberships[0].Role != "RESIDENT" {
		t.Fatalf("unexpected body: %s", rec.Body)
	}
}

// Caso ME-02 · GET /me exige autenticación.
// Entrada: Petición sin identidad
// Debe hacer / esperado: 401
func TestME02_MeRequiresAuthentication(t *testing.T) {
	t.Setenv("ENV", "production")
	rec := httptest.NewRecorder()
	router(fakeDir{}, "").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/me/memberships", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
}

// Caso ME-03 · En desarrollo, sin identidad sigue siendo no autorizado.
// Entrada: ENV development y sin identidad
// Debe hacer / esperado: 401
func TestME03_MeInDevelopmentWithoutIdentityIsStillUnauthorized(t *testing.T) {
	t.Setenv("ENV", "development")
	rec := httptest.NewRecorder()
	router(fakeDir{}, "").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/me/memberships", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d", rec.Code)
	}
}

// Caso ME-04 · El rol interno se reconoce solo con el secreto interno correcto.
// Entrada: Peticiones con y sin secreto interno
// Debe hacer / esperado: Rol interno solo con el secreto correcto
func TestME04_InternalRole(t *testing.T) {
	user, cx := uuid.New(), uuid.New()
	path := "/internal/users/" + user.String() + "/complexes/" + cx.String() + "/role"

	cases := []struct {
		name   string
		secret string
		dir    fakeDir
		path   string
		want   int
	}{
		{"member", "s3cret", fakeDir{role: "GUARD", found: true}, path, 200},
		{"not a member", "s3cret", fakeDir{}, path, 404},
		{"wrong secret", "bad", fakeDir{role: "GUARD", found: true}, path, 401},
		{"no secret header", "", fakeDir{role: "GUARD", found: true}, path, 401},
		{"bad uuid", "s3cret", fakeDir{}, "/internal/users/x/complexes/" + cx.String() + "/role", 422},
		{"lookup failure", "s3cret", fakeDir{err: errors.New("db down")}, path, 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.secret != "" {
				req.Header.Set(HeaderInternalSecret, tc.secret)
			}
			rec := httptest.NewRecorder()
			router(tc.dir, "s3cret").ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("got %d (%s), want %d", rec.Code, rec.Body, tc.want)
			}
			if tc.want == 200 && !json.Valid(rec.Body.Bytes()) {
				t.Fatalf("invalid json: %s", rec.Body)
			}
		})
	}
}

// Caso ME-05 · Las rutas internas no existen si no hay secreto configurado.
// Entrada: Servidor sin secreto interno
// Debe hacer / esperado: 404 en las rutas internas
func TestME05_InternalRoutesAreNotMountedWithoutASecret(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/internal/users/"+uuid.NewString()+"/complexes/"+uuid.NewString()+"/role", nil)
	req.Header.Set(HeaderInternalSecret, "")
	rec := httptest.NewRecorder()
	router(fakeDir{role: "ADMIN", found: true}, "").ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("must fail closed (404), got %d", rec.Code)
	}
}
