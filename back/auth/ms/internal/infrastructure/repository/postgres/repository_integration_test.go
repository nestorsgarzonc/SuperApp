package postgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"superapp/auth/internal/core/domain"
	coreRepository "superapp/auth/internal/core/ports/repository"
)

// TestRepositoryPostgresIntegration is opt-in because it runs the real setup
// command against a dedicated disposable database. Set AUTH_TEST_DATABASE_URL.
func TestRepositoryPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set AUTH_TEST_DATABASE_URL to a dedicated disposable PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	setup := exec.CommandContext(ctx, "go", "run", "../../../../cmd/setup")
	setup.Dir = workingDir
	setup.Env = append(os.Environ(), "AUTH_DATABASE_URL="+dsn, "AUTH_DB_DIR="+filepath.Clean(filepath.Join(workingDir, "../../../../../db")))
	if output, err := setup.CombinedOutput(); err != nil {
		t.Fatalf("apply real Auth migrations/seeds: %v\n%s", err, output)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	r := New(db)
	userID, _ := newUUID()
	actorID, _ := newUUID()
	roleID, _ := newUUID()
	permissionID, _ := newUUID()
	sessionID, _ := newUUID()
	refreshID, _ := newUUID()
	tenantID, _ := newUUID()
	roleCode := "IT_ROLE_" + roleID[:8]
	permissionCode := "integration." + permissionID[:8]
	cleanup := func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM auth_users WHERE id IN ($1,$2)`, userID, actorID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM auth_roles WHERE id=$1`, roleID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM auth_permissions WHERE id=$1`, permissionID)
	}
	t.Cleanup(cleanup)

	if _, err = r.CreateUser(ctx, coreRepository.CreateUserParams{ID: userID, Email: userID + "@integration.test", PasswordHash: "test-hash", FullName: "Integration user"}); err != nil {
		t.Fatal(err)
	}
	if _, err = r.CreateUser(ctx, coreRepository.CreateUserParams{ID: actorID, Email: actorID + "@integration.test", PasswordHash: "test-hash", FullName: "Integration actor"}); err != nil {
		t.Fatal(err)
	}
	if err = r.CreateRole(ctx, domain.Role{ID: roleID, Code: roleCode, Name: "Integration role", Scope: "TENANT", TenantID: &tenantID, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err = r.CreatePermission(ctx, domain.Permission{ID: permissionID, Code: permissionCode, Description: "Integration permission"}); err != nil {
		t.Fatal(err)
	}
	if err = r.GrantPermission(ctx, coreRepository.RolePermissionParams{RoleID: roleID, PermissionID: permissionID, TenantID: &tenantID}); err != nil {
		t.Fatal(err)
	}
	if err = r.AssignRole(ctx, coreRepository.AssignRoleParams{UserID: userID, RoleID: roleID, TenantID: &tenantID, ActorID: actorID}); err != nil {
		t.Fatal(err)
	}
	allowed, err := r.PermissionAllowed(ctx, coreRepository.PermissionAllowedParams{UserID: userID, PermissionCode: permissionCode, TenantID: &tenantID})
	if err != nil || !allowed {
		t.Fatalf("tenant permission not effective: allowed=%v err=%v", allowed, err)
	}
	allowed, err = r.PermissionAllowed(ctx, coreRepository.PermissionAllowedParams{UserID: userID, PermissionCode: permissionCode})
	if err != nil || allowed {
		t.Fatalf("tenant permission leaked into system scope: allowed=%v err=%v", allowed, err)
	}
	roles, err := r.EffectiveRoles(ctx, userID, &tenantID)
	if err != nil || len(roles) != 1 || roles[0].ID != roleID {
		t.Fatalf("tenant role relationship missing: roles=%#v err=%v", roles, err)
	}
	permissions, err := r.EffectivePermissions(ctx, userID, &tenantID)
	if err != nil || len(permissions) != 1 || permissions[0] != permissionCode {
		t.Fatalf("role permission relationship missing: permissions=%#v err=%v", permissions, err)
	}

	refreshHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	rotatedHash := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	expires := time.Now().Add(time.Hour)
	if err = r.CreateSession(ctx, coreRepository.CreateSessionParams{
		SessionID: sessionID, UserID: userID, RefreshHash: refreshHash, ExpiresAt: expires,
		Device: "integration", UserAgent: "test-agent", IP: "127.0.0.1",
	}); err != nil {
		t.Fatal(err)
	}
	rotatedID, _ := newUUID()
	rotated, err := r.RotateRefresh(ctx, coreRepository.RotateRefreshParams{
		OldHash: refreshHash, NewHash: rotatedHash, NewTokenID: rotatedID, ExpiresAt: expires, Now: time.Now(),
	})
	if err != nil || rotated.SessionID != sessionID || rotated.UserID != userID {
		t.Fatalf("refresh rotation failed: session=%s user=%s err=%v", rotated.SessionID, rotated.UserID, err)
	}
	if _, err = r.RotateRefresh(ctx, coreRepository.RotateRefreshParams{
		OldHash: refreshHash, NewHash: "1111111111111111111111111111111111111111111111111111111111111111", NewTokenID: refreshID, ExpiresAt: expires, Now: time.Now(),
	}); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("replayed refresh token accepted: %v", err)
	}
	if err = r.RevokeAllSessions(ctx, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = r.RotateRefresh(ctx, coreRepository.RotateRefreshParams{
		OldHash: rotatedHash, NewHash: "2222222222222222222222222222222222222222222222222222222222222222", NewTokenID: refreshID, ExpiresAt: expires, Now: time.Now(),
	}); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("refresh token after logout-all accepted: %v", err)
	}
}
