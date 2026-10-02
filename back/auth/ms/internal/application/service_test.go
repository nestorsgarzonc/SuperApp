package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"superapp/auth/internal/domain"
)

type testRepository struct {
	Repository
	user           *domain.User
	hash           string
	createdEmail   string
	createdHash    string
	refreshHash    string
	rotatedHash    string
	loginUpdated   bool
	sessionCreated bool
	burned         int
}

func (r *testRepository) CreateUser(_ context.Context, id, email, hash, name string) (*domain.User, error) {
	r.createdEmail = email
	r.createdHash = hash
	return &domain.User{ID: id, Email: email, FullName: name, Status: "ACTIVE"}, nil
}
func (r *testRepository) UserByEmail(_ context.Context, email string) (*domain.User, string, error) {
	if email == "missing@example.test" {
		return nil, "", nil
	}
	return r.user, r.hash, nil
}
func (r *testRepository) SetLastLogin(context.Context, string) error {
	r.loginUpdated = true
	return nil
}
func (r *testRepository) CreateSession(_ context.Context, _, _, hash string, _ time.Time, _, _, _ string) error {
	r.sessionCreated = true
	r.refreshHash = hash
	return nil
}
func (r *testRepository) RotateRefresh(_ context.Context, oldHash, newHash, newID string, expires, now time.Time) (string, string, string, error) {
	r.refreshHash = oldHash
	r.rotatedHash = newHash
	return "session", "user", "user@example.test", nil
}
func (r *testRepository) CreateRole(context.Context, domain.Role) error { return nil }

type testPasswords struct {
	hash   string
	burned int
}

func (p *testPasswords) Hash(string) (string, error) { return "bcrypt-hash", nil }
func (p *testPasswords) Verify(_, password string) error {
	if password == "correct-password" {
		return nil
	}
	return errors.New("bad password")
}
func (p *testPasswords) Burn(string) { p.burned++ }

type testTokens struct{}

func (testTokens) Issue(string, string, string) (string, time.Time, error) {
	return "signed-access-token", time.Now().Add(time.Minute), nil
}
func (testTokens) Verify(string) (*TokenClaims, error) { return nil, nil }
func (testTokens) JWKS() map[string]any                { return nil }

func testIDs() IDGenerator { return func() string { return "generated-id" } }

func TestRegisterNormalizesEmailAndStoresHash(t *testing.T) {
	repo := &testRepository{}
	s := New(repo, &testPasswords{}, testTokens{}, testIDs(), Config{})
	u, err := s.Register(context.Background(), "  User@Example.Test ", "a-strong-password", "User")
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "user@example.test" || repo.createdEmail != "user@example.test" || repo.createdHash != "bcrypt-hash" {
		t.Fatalf("email/hash not normalized or hashed: %#v %#v", u, repo)
	}
}
func TestRegisterRejectsWeakPassword(t *testing.T) {
	s := New(&testRepository{}, &testPasswords{}, testTokens{}, testIDs(), Config{})
	if _, err := s.Register(context.Background(), "x@y.test", "short", "User"); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}
func TestLoginGenericCredentialsAndCreatesSession(t *testing.T) {
	pw := &testPasswords{}
	repo := &testRepository{user: &domain.User{ID: "u1", Email: "user@example.test", Status: "ACTIVE"}, hash: "hash"}
	s := New(repo, pw, testTokens{}, testIDs(), Config{AccessTTL: time.Minute, RefreshTTL: time.Hour})
	if _, _, err := s.Login(context.Background(), "missing@example.test", "wrong", "", "", ""); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("missing account error: %v", err)
	}
	if pw.burned != 1 {
		t.Fatal("missing email did not take dummy hash path")
	}
	u, pair, err := s.Login(context.Background(), " USER@example.test ", "correct-password", "phone", "ua", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != "u1" || pair.AccessToken != "signed-access-token" || pair.RefreshToken == "" || !repo.loginUpdated || !repo.sessionCreated {
		t.Fatalf("login did not complete session flow: %#v %#v", u, pair)
	}
}
func TestInactiveAccountUsesGenericCredentialFailure(t *testing.T) {
	repo := &testRepository{user: &domain.User{ID: "u1", Email: "inactive@example.test", Status: "DISABLED"}, hash: "hash"}
	s := New(repo, &testPasswords{}, testTokens{}, testIDs(), Config{})
	if _, _, err := s.Login(context.Background(), "inactive@example.test", "correct-password", "", "", ""); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("expected generic credential error, got %v", err)
	}
}

func TestRefreshRotatesAndOnlyPersistsHash(t *testing.T) {
	repo := &testRepository{}
	s := New(repo, &testPasswords{}, testTokens{}, testIDs(), Config{RefreshTTL: time.Hour})
	pair, err := s.Refresh(context.Background(), "presented-refresh-token")
	if err != nil {
		t.Fatal(err)
	}
	if pair.RefreshToken == "" || repo.refreshHash == "presented-refresh-token" || len(repo.rotatedHash) != 64 || repo.rotatedHash == pair.RefreshToken {
		t.Fatalf("refresh token was not replaced with a hash-only persistence value: %#v %#v", pair, repo)
	}
}

func TestTenantRoleRequiresTenantScope(t *testing.T) {
	s := New(&testRepository{}, &testPasswords{}, testTokens{}, testIDs(), Config{})
	err := s.CreateRole(context.Background(), domain.Role{Code: "TENANT_MANAGER", Name: "Manager", Scope: "TENANT"})
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected tenant scope validation, got %v", err)
	}
}

func TestPermissionCodeMustBeNamespaced(t *testing.T) {
	s := New(&testRepository{}, &testPasswords{}, testTokens{}, testIDs(), Config{})
	err := s.CreatePermission(context.Background(), domain.Permission{Code: "ADMIN"})
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected namespaced permission validation, got %v", err)
	}
}

type revocationRepository struct {
	Repository
	refreshHash, userID string
	allCalled           bool
}

func (r *revocationRepository) RevokeRefresh(_ context.Context, hash string) error {
	r.refreshHash = hash
	return nil
}
func (r *revocationRepository) RevokeAllSessions(_ context.Context, userID string) error {
	r.userID = userID
	r.allCalled = true
	return nil
}

func TestLogoutRevokesRefreshTokenByHash(t *testing.T) {
	repo := &revocationRepository{}
	s := New(repo, &testPasswords{}, testTokens{}, testIDs(), Config{})
	if err := s.Logout(context.Background(), "opaque-refresh-token"); err != nil {
		t.Fatal(err)
	}
	if repo.refreshHash == "" || repo.refreshHash == "opaque-refresh-token" {
		t.Fatalf("logout did not pass a one-way token hash to repository: %q", repo.refreshHash)
	}
}

func TestLogoutAllRevokesSessionsForAuthenticatedUser(t *testing.T) {
	repo := &revocationRepository{}
	s := New(repo, &testPasswords{}, testTokens{}, testIDs(), Config{})
	if err := s.LogoutAll(context.Background(), "user-123"); err != nil {
		t.Fatal(err)
	}
	if !repo.allCalled || repo.userID != "user-123" {
		t.Fatalf("logout-all did not revoke sessions for caller: %+v", repo)
	}
}

type managementRepository struct {
	Repository
	user                 *domain.User
	updatedUserID        string
	statusUserID, status string
	role                 domain.Role
	updatedRoleID        string
	deletedRoleID        string
	permission           domain.Permission
	updatedPermissionID  string
	deletedPermissionID  string
	grantedRole          string
	revokedRole          string
	assignedUser         string
	assignedRole         string
	revokedUser          string
	revokedAssignedRole  string
	allowed              bool
	effectiveRoles       []domain.Role
	effectivePermissions []string
}

func (r *managementRepository) CreateUser(_ context.Context, id, email, hash, name string) (*domain.User, error) {
	r.user = &domain.User{ID: id, Email: email, FullName: name, Status: "ACTIVE"}
	return r.user, nil
}
func (r *managementRepository) UserByID(context.Context, string) (*domain.User, error) {
	return r.user, nil
}
func (r *managementRepository) UpdateUser(_ context.Context, id, email, name string) error {
	r.updatedUserID = id
	return nil
}
func (r *managementRepository) SetUserStatus(_ context.Context, id, status string) error {
	r.statusUserID, r.status = id, status
	return nil
}
func (r *managementRepository) CreateRole(_ context.Context, role domain.Role) error {
	r.role = role
	return nil
}
func (r *managementRepository) UpdateRole(_ context.Context, role domain.Role, _ *string) error {
	r.updatedRoleID = role.ID
	return nil
}
func (r *managementRepository) DeleteRole(_ context.Context, id string, _ *string) error {
	r.deletedRoleID = id
	return nil
}
func (r *managementRepository) CreatePermission(_ context.Context, permission domain.Permission) error {
	r.permission = permission
	return nil
}
func (r *managementRepository) UpdatePermission(_ context.Context, permission domain.Permission) error {
	r.updatedPermissionID = permission.ID
	return nil
}
func (r *managementRepository) DeletePermission(_ context.Context, id string) error {
	r.deletedPermissionID = id
	return nil
}
func (r *managementRepository) GrantPermission(_ context.Context, role, permission string, _ *string) error {
	r.grantedRole = role + ":" + permission
	return nil
}
func (r *managementRepository) RevokePermission(_ context.Context, role, permission string, _ *string) error {
	r.grantedRole = ""
	r.revokedRole = role + ":" + permission
	return nil
}
func (r *managementRepository) AssignRole(_ context.Context, user, role string, _ *string, _ string) error {
	r.assignedUser, r.assignedRole = user, role
	return nil
}
func (r *managementRepository) RevokeRole(_ context.Context, user, role string, _ *string) error {
	r.revokedUser, r.revokedAssignedRole = user, role
	return nil
}
func (r *managementRepository) EffectiveRoles(context.Context, string, *string) ([]domain.Role, error) {
	return r.effectiveRoles, nil
}
func (r *managementRepository) EffectivePermissions(context.Context, string, *string) ([]string, error) {
	return r.effectivePermissions, nil
}
func (r *managementRepository) PermissionAllowed(context.Context, string, string, *string) (bool, error) {
	return r.allowed, nil
}

func TestUserUpdateStatusAndActivationValidation(t *testing.T) {
	repo := &managementRepository{user: &domain.User{ID: "user-1"}}
	s := New(repo, &testPasswords{}, testTokens{}, testIDs(), Config{})
	if err := s.UpdateUser(context.Background(), "user-1", " NEW@EXAMPLE.TEST ", " New Name "); err != nil {
		t.Fatal(err)
	}
	if repo.updatedUserID != "user-1" {
		t.Fatalf("update did not reach repository: %q", repo.updatedUserID)
	}
	for _, status := range []string{"ACTIVE", "DISABLED", "LOCKED"} {
		if err := s.SetUserStatus(context.Background(), "user-1", status); err != nil {
			t.Fatalf("status %s rejected: %v", status, err)
		}
	}
	if repo.statusUserID != "user-1" || repo.status != "LOCKED" {
		t.Fatalf("last user status not saved: %#v", repo)
	}
	if err := s.SetUserStatus(context.Background(), "user-1", "SUPERUSER"); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("invalid status accepted: %v", err)
	}
}

func TestRolePermissionManagementAndAuthorizationDelegation(t *testing.T) {
	tenant := "tenant-1"
	repo := &managementRepository{
		effectiveRoles:       []domain.Role{{ID: "role-1", Code: "RESIDENT"}, {ID: "role-2", Code: "OWNER"}},
		effectivePermissions: []string{"homes.read", "payments.read"},
		allowed:              true,
	}
	s := New(repo, &testPasswords{}, testTokens{}, testIDs(), Config{})
	if err := s.CreateRole(context.Background(), domain.Role{Code: "TENANT_ADMIN", Name: "Tenant Admin", Scope: "TENANT", TenantID: &tenant}); err != nil {
		t.Fatal(err)
	}
	if repo.role.ID == "" || repo.role.TenantID == nil || *repo.role.TenantID != tenant {
		t.Fatalf("role scope/id not set: %#v", repo.role)
	}
	if err := s.UpdateRole(context.Background(), domain.Role{ID: "role-1", Name: "Resident Updated"}, &tenant); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteRole(context.Background(), "role-1", &tenant); err != nil || repo.deletedRoleID != "role-1" {
		t.Fatalf("role delete not delegated: id=%s err=%v", repo.deletedRoleID, err)
	}
	if err := s.CreatePermission(context.Background(), domain.Permission{Code: "homes.read", Description: "Read homes"}); err != nil {
		t.Fatal(err)
	}
	if repo.permission.ID == "" {
		t.Fatal("permission id was not generated")
	}
	if err := s.UpdatePermission(context.Background(), domain.Permission{ID: "permission-1", Code: "homes.write"}); err != nil || repo.updatedPermissionID != "permission-1" {
		t.Fatalf("permission update not delegated: id=%s err=%v", repo.updatedPermissionID, err)
	}
	if err := s.DeletePermission(context.Background(), "permission-1"); err != nil || repo.deletedPermissionID != "permission-1" {
		t.Fatalf("permission delete not delegated: id=%s err=%v", repo.deletedPermissionID, err)
	}
	if err := s.GrantPermission(context.Background(), "role-1", "permission-1", &tenant); err != nil || repo.grantedRole != "role-1:permission-1" {
		t.Fatalf("permission grant not delegated: %s, %v", repo.grantedRole, err)
	}
	if err := s.RevokePermission(context.Background(), "role-1", "permission-1", &tenant); err != nil || repo.revokedRole != "role-1:permission-1" {
		t.Fatalf("permission revoke not delegated: %s, %v", repo.revokedRole, err)
	}
	if err := s.AssignRole(context.Background(), "user-1", "role-1", &tenant, "actor-1"); err != nil || repo.assignedUser != "user-1" || repo.assignedRole != "role-1" {
		t.Fatalf("role assignment not delegated: %#v err=%v", repo, err)
	}
	if err := s.RevokeRole(context.Background(), "user-1", "role-1", &tenant); err != nil || repo.revokedUser != "user-1" || repo.revokedAssignedRole != "role-1" {
		t.Fatalf("role removal not delegated: %#v err=%v", repo, err)
	}
	roles, err := s.EffectiveRoles(context.Background(), "user-1", &tenant)
	if err != nil || len(roles) != 2 {
		t.Fatalf("multiple effective roles lost: %v %v", roles, err)
	}
	permissions, err := s.EffectivePermissions(context.Background(), "user-1", &tenant)
	if err != nil || len(permissions) != 2 {
		t.Fatalf("permissions from multiple roles lost: %v %v", permissions, err)
	}
	allowed, err := s.HasPermission(context.Background(), "user-1", "homes.read", &tenant)
	if err != nil || !allowed {
		t.Fatalf("permission check failed: allowed=%v err=%v", allowed, err)
	}
}
