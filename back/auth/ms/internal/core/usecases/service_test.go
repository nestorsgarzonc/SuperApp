package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"superapp/auth/internal/core/domain"
	"superapp/auth/internal/core/ports/repository"
	"superapp/auth/internal/core/ports/security"
	authlogin "superapp/auth/internal/core/usecases/auth/login"
	authlogout "superapp/auth/internal/core/usecases/auth/logout"
	authrefresh "superapp/auth/internal/core/usecases/auth/refresh"
	authregister "superapp/auth/internal/core/usecases/auth/register"
	authorizationusecase "superapp/auth/internal/core/usecases/authorization"
	"superapp/auth/internal/core/usecases/contracts"
	permissionusecase "superapp/auth/internal/core/usecases/permissions"
	roleusecase "superapp/auth/internal/core/usecases/roles"
	sessions "superapp/auth/internal/core/usecases/sessions"
	users "superapp/auth/internal/core/usecases/users"
)

type testRepository struct {
	repository.UserRepository
	repository.SessionRepository
	repository.RoleRepository
	repository.PermissionRepository
	repository.AuthorizationRepository
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

type testRepositories interface {
	repository.UserRepository
	repository.SessionRepository
	repository.RoleRepository
	repository.PermissionRepository
	repository.AuthorizationRepository
}

type testApplication struct {
	*Service
	login    *authlogin.UseCase
	register *authregister.UseCase
	refresh  *authrefresh.UseCase
	logout   *authlogout.UseCase
}

func newTestApplication(repo testRepositories, passwords security.PasswordHasher, tokens security.TokenIssuer, ids security.IDGenerator, cfg contracts.Config) *testApplication {
	return &testApplication{
		Service: New(RepositoryDependencies{
			Users: repo, Sessions: repo, Roles: repo, Permissions: repo, Authorization: repo,
		}, ids),
		login:    authlogin.New(repo, repo, passwords, tokens, ids, cfg),
		register: authregister.New(repo, passwords, ids),
		refresh:  authrefresh.New(repo, tokens, ids, cfg),
		logout:   authlogout.New(repo),
	}
}

func (s *testApplication) Register(ctx context.Context, email, password, name string) (*domain.User, error) {
	result, err := s.register.Execute(ctx, authregister.Requester{Email: email, Password: password, Name: name})
	return result.User, err
}

func (s *testApplication) Login(ctx context.Context, email, password, device, userAgent, ip string) (*domain.User, *contracts.TokenPair, error) {
	result, err := s.login.Execute(ctx, authlogin.Requester{Email: email, Password: password, Device: device, UserAgent: userAgent, IP: ip})
	return result.User, &result.Tokens, err
}

func (s *testApplication) Refresh(ctx context.Context, raw string) (*contracts.TokenPair, error) {
	result, err := s.refresh.Execute(ctx, authrefresh.Requester{RefreshToken: raw})
	return &result.Tokens, err
}

func (s *testApplication) Logout(ctx context.Context, raw string) error {
	_, err := s.logout.Execute(ctx, authlogout.Requester{RefreshToken: raw})
	return err
}

func (r *testRepository) CreateUser(_ context.Context, params repository.CreateUserParams) (*domain.User, error) {
	r.createdEmail = params.Email
	r.createdHash = params.PasswordHash
	return &domain.User{ID: params.ID, Email: params.Email, FullName: params.FullName, Status: "ACTIVE"}, nil
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
func (r *testRepository) CreateSession(_ context.Context, params repository.CreateSessionParams) error {
	r.sessionCreated = true
	r.refreshHash = params.RefreshHash
	return nil
}
func (r *testRepository) RotateRefresh(_ context.Context, params repository.RotateRefreshParams) (repository.RotateRefreshResult, error) {
	r.refreshHash = params.OldHash
	r.rotatedHash = params.NewHash
	return repository.RotateRefreshResult{SessionID: "session", UserID: "user", Email: "user@example.test"}, nil
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
func (testTokens) Verify(string) (*security.TokenClaims, error) { return nil, nil }
func (testTokens) JWKS() map[string]any                         { return nil }

func testIDs() security.IDGenerator { return func() string { return "generated-id" } }

func TestRegisterNormalizesEmailAndStoresHash(t *testing.T) {
	repo := &testRepository{}
	s := newTestApplication(repo, &testPasswords{}, testTokens{}, testIDs(), contracts.Config{})
	u, err := s.Register(context.Background(), "  User@Example.Test ", "a-strong-password", "User")
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "user@example.test" || repo.createdEmail != "user@example.test" || repo.createdHash != "bcrypt-hash" {
		t.Fatalf("email/hash not normalized or hashed: %#v %#v", u, repo)
	}
}
func TestRegisterRejectsWeakPassword(t *testing.T) {
	s := newTestApplication(&testRepository{}, &testPasswords{}, testTokens{}, testIDs(), contracts.Config{})
	if _, err := s.Register(context.Background(), "x@y.test", "short", "User"); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}
func TestLoginGenericCredentialsAndCreatesSession(t *testing.T) {
	pw := &testPasswords{}
	repo := &testRepository{user: &domain.User{ID: "u1", Email: "user@example.test", Status: "ACTIVE"}, hash: "hash"}
	s := newTestApplication(repo, pw, testTokens{}, testIDs(), contracts.Config{AccessTTL: time.Minute, RefreshTTL: time.Hour})
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
	s := newTestApplication(repo, &testPasswords{}, testTokens{}, testIDs(), contracts.Config{})
	if _, _, err := s.Login(context.Background(), "inactive@example.test", "correct-password", "", "", ""); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("expected generic credential error, got %v", err)
	}
}

func TestRefreshRotatesAndOnlyPersistsHash(t *testing.T) {
	repo := &testRepository{}
	s := newTestApplication(repo, &testPasswords{}, testTokens{}, testIDs(), contracts.Config{RefreshTTL: time.Hour})
	pair, err := s.Refresh(context.Background(), "presented-refresh-token")
	if err != nil {
		t.Fatal(err)
	}
	if pair.RefreshToken == "" || repo.refreshHash == "presented-refresh-token" || len(repo.rotatedHash) != 64 || repo.rotatedHash == pair.RefreshToken {
		t.Fatalf("refresh token was not replaced with a hash-only persistence value: %#v %#v", pair, repo)
	}
}

func TestTenantRoleRequiresTenantScope(t *testing.T) {
	s := newTestApplication(&testRepository{}, &testPasswords{}, testTokens{}, testIDs(), contracts.Config{})
	_, err := s.CreateRole(context.Background(), roleusecase.CreateRequester{Code: "TENANT_MANAGER", Name: "Manager", Scope: "TENANT"})
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected tenant scope validation, got %v", err)
	}
}

func TestPermissionCodeMustBeNamespaced(t *testing.T) {
	s := newTestApplication(&testRepository{}, &testPasswords{}, testTokens{}, testIDs(), contracts.Config{})
	_, err := s.CreatePermission(context.Background(), permissionusecase.CreateRequester{Code: "ADMIN"})
	if !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("expected namespaced permission validation, got %v", err)
	}
}

type revocationRepository struct {
	testRepository
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
	s := newTestApplication(repo, &testPasswords{}, testTokens{}, testIDs(), contracts.Config{})
	if err := s.Logout(context.Background(), "opaque-refresh-token"); err != nil {
		t.Fatal(err)
	}
	if repo.refreshHash == "" || repo.refreshHash == "opaque-refresh-token" {
		t.Fatalf("logout did not pass a one-way token hash to repository: %q", repo.refreshHash)
	}
}

func TestLogoutAllRevokesSessionsForAuthenticatedUser(t *testing.T) {
	repo := &revocationRepository{}
	s := newTestApplication(repo, &testPasswords{}, testTokens{}, testIDs(), contracts.Config{})
	if _, err := s.LogoutAll(context.Background(), sessions.LogoutAllRequester{UserID: "user-123"}); err != nil {
		t.Fatal(err)
	}
	if !repo.allCalled || repo.userID != "user-123" {
		t.Fatalf("logout-all did not revoke sessions for caller: %+v", repo)
	}
}

type managementRepository struct {
	testRepository
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
	assignedActor        string
	assignedTenant       *string
	revokedUser          string
	revokedAssignedRole  string
	allowed              bool
	effectiveRoles       []domain.Role
	effectivePermissions []string
}

func (r *managementRepository) CreateUser(_ context.Context, params repository.CreateUserParams) (*domain.User, error) {
	r.user = &domain.User{ID: params.ID, Email: params.Email, FullName: params.FullName, Status: "ACTIVE"}
	return r.user, nil
}
func (r *managementRepository) UserByID(context.Context, string) (*domain.User, error) {
	return r.user, nil
}
func (r *managementRepository) RoleByID(context.Context, string, *string) (*domain.Role, error) {
	return &r.role, nil
}
func (r *managementRepository) PermissionByID(context.Context, string) (*domain.Permission, error) {
	return &r.permission, nil
}
func (r *managementRepository) UpdateUser(_ context.Context, params repository.UpdateUserParams) error {
	r.updatedUserID = params.ID
	if params.Status != nil {
		r.statusUserID, r.status = params.ID, *params.Status
	}
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
func (r *managementRepository) GrantPermission(_ context.Context, params repository.RolePermissionParams) error {
	r.grantedRole = params.RoleID + ":" + params.PermissionID
	return nil
}
func (r *managementRepository) RevokePermission(_ context.Context, params repository.RolePermissionParams) error {
	r.grantedRole = ""
	r.revokedRole = params.RoleID + ":" + params.PermissionID
	return nil
}
func (r *managementRepository) AssignRole(_ context.Context, params repository.AssignRoleParams) error {
	r.assignedUser, r.assignedRole = params.UserID, params.RoleID
	r.assignedActor, r.assignedTenant = params.ActorID, params.TenantID
	return nil
}
func (r *managementRepository) RevokeRole(_ context.Context, params repository.RevokeRoleParams) error {
	r.revokedUser, r.revokedAssignedRole = params.UserID, params.RoleID
	return nil
}
func (r *managementRepository) EffectiveRoles(context.Context, string, *string) ([]domain.Role, error) {
	return r.effectiveRoles, nil
}
func (r *managementRepository) EffectivePermissions(context.Context, string, *string) ([]string, error) {
	return r.effectivePermissions, nil
}
func (r *managementRepository) PermissionAllowed(context.Context, repository.PermissionAllowedParams) (bool, error) {
	return r.allowed, nil
}

func TestUserUpdateStatusAndActivationValidation(t *testing.T) {
	repo := &managementRepository{user: &domain.User{ID: "user-1"}}
	s := newTestApplication(repo, &testPasswords{}, testTokens{}, testIDs(), contracts.Config{})
	email, name, status := " NEW@EXAMPLE.TEST ", " New Name ", "LOCKED"
	if _, err := s.UpdateUser(context.Background(), users.UpdateRequester{ID: "user-1", Email: &email, FullName: &name, Status: &status}); err != nil {
		t.Fatal(err)
	}
	if repo.updatedUserID != "user-1" || repo.statusUserID != "user-1" || repo.status != status {
		t.Fatalf("update did not reach repository: %q", repo.updatedUserID)
	}
	repo.updatedUserID = ""
	invalidStatus := "SUPERUSER"
	if _, err := s.UpdateUser(context.Background(), users.UpdateRequester{ID: "user-1", Status: &invalidStatus}); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("invalid status accepted: %v", err)
	}
	if repo.updatedUserID != "" {
		t.Fatalf("invalid status reached repository: %#v", repo)
	}
}

func TestRolePermissionManagementAndAuthorizationDelegation(t *testing.T) {
	tenant := "tenant-1"
	repo := &managementRepository{
		effectiveRoles:       []domain.Role{{ID: "role-1", Code: "RESIDENT"}, {ID: "role-2", Code: "OWNER"}},
		effectivePermissions: []string{"homes.read", "payments.read"},
		allowed:              true,
	}
	s := newTestApplication(repo, &testPasswords{}, testTokens{}, testIDs(), contracts.Config{})
	createdRole, err := s.CreateRole(context.Background(), roleusecase.CreateRequester{Code: "TENANT_ADMIN", Name: "Tenant Admin", Scope: "TENANT", TenantID: &tenant})
	if err != nil {
		t.Fatal(err)
	}
	if createdRole.ID == "" || !createdRole.Enabled || repo.role.ID != createdRole.ID || repo.role.TenantID == nil || *repo.role.TenantID != tenant {
		t.Fatalf("created role did not return generated values: result=%#v persisted=%#v", createdRole, repo.role)
	}
	updatedName := "Resident Updated"
	if _, err := s.UpdateRole(context.Background(), roleusecase.UpdateRequester{RoleID: "role-1", Name: &updatedName, TenantID: &tenant}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteRole(context.Background(), roleusecase.DeleteRequester{RoleID: "role-1", TenantID: &tenant}); err != nil || repo.deletedRoleID != "role-1" {
		t.Fatalf("role delete not delegated: id=%s err=%v", repo.deletedRoleID, err)
	}
	createdPermission, err := s.CreatePermission(context.Background(), permissionusecase.CreateRequester{Code: "homes.read", Description: "Read homes"})
	if err != nil {
		t.Fatal(err)
	}
	if createdPermission.ID == "" || repo.permission.ID != createdPermission.ID {
		t.Fatalf("created permission did not return generated ID: result=%#v persisted=%#v", createdPermission, repo.permission)
	}
	updatedCode := "homes.write"
	if _, err := s.UpdatePermission(context.Background(), permissionusecase.UpdateRequester{PermissionID: createdPermission.ID, Code: &updatedCode}); err != nil || repo.updatedPermissionID != createdPermission.ID {
		t.Fatalf("permission update not delegated: id=%s err=%v", repo.updatedPermissionID, err)
	}
	if _, err := s.DeletePermission(context.Background(), permissionusecase.DeleteRequester{PermissionID: createdPermission.ID}); err != nil || repo.deletedPermissionID != createdPermission.ID {
		t.Fatalf("permission delete not delegated: id=%s err=%v", repo.deletedPermissionID, err)
	}
	if _, err := s.GrantPermission(context.Background(), authorizationusecase.RolePermissionParams{RoleID: "role-1", PermissionID: "permission-1", TenantID: &tenant}); err != nil || repo.grantedRole != "role-1:permission-1" {
		t.Fatalf("permission grant not delegated: %s, %v", repo.grantedRole, err)
	}
	if _, err := s.RevokePermission(context.Background(), authorizationusecase.RolePermissionParams{RoleID: "role-1", PermissionID: "permission-1", TenantID: &tenant}); err != nil || repo.revokedRole != "role-1:permission-1" {
		t.Fatalf("permission revoke not delegated: %s, %v", repo.revokedRole, err)
	}
	if _, err := s.AssignRole(context.Background(), authorizationusecase.AssignRoleParams{UserID: "user-1", RoleID: "role-1", TenantID: &tenant, ActorID: "actor-1"}); err != nil || repo.assignedUser != "user-1" || repo.assignedRole != "role-1" {
		t.Fatalf("role assignment not delegated: %#v err=%v", repo, err)
	}
	if repo.assignedActor != "actor-1" || repo.assignedTenant == nil || *repo.assignedTenant != tenant {
		t.Fatalf("actor or tenant context lost during role assignment: %#v", repo)
	}
	if _, err := s.RevokeRole(context.Background(), authorizationusecase.RevokeRoleParams{UserID: "user-1", RoleID: "role-1", TenantID: &tenant}); err != nil || repo.revokedUser != "user-1" || repo.revokedAssignedRole != "role-1" {
		t.Fatalf("role removal not delegated: %#v err=%v", repo, err)
	}
	roles, err := s.EffectiveRoles(context.Background(), authorizationusecase.EffectiveRolesRequester{UserID: "user-1", TenantID: &tenant})
	if err != nil || len(roles.Roles) != 2 {
		t.Fatalf("multiple effective roles lost: %v %v", roles, err)
	}
	permissions, err := s.EffectivePermissions(context.Background(), authorizationusecase.EffectivePermissionsRequester{UserID: "user-1", TenantID: &tenant})
	if err != nil || len(permissions.Permissions) != 2 {
		t.Fatalf("permissions from multiple roles lost: %v %v", permissions, err)
	}
	allowed, err := s.CheckPermission(context.Background(), authorizationusecase.CheckPermissionRequester{UserID: "user-1", PermissionCode: "homes.read", TenantID: &tenant})
	if err != nil || !allowed.Allowed {
		t.Fatalf("permission check failed: allowed=%v err=%v", allowed.Allowed, err)
	}
}
