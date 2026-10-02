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

	"superapp/auth/internal/core/domain"
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
	setup := exec.CommandContext(ctx, "go", "run", "../../../cmd/setup")
	setup.Dir = workingDir
	setup.Env = append(os.Environ(), "AUTH_DATABASE_URL="+dsn, "AUTH_DB_DIR="+filepath.Clean(filepath.Join(workingDir, "../../../db")))
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

	if _, err = r.CreateUser(ctx, userID, userID+"@integration.test", "test-hash", "Integration user"); err != nil {
		t.Fatal(err)
	}
	if _, err = r.CreateUser(ctx, actorID, actorID+"@integration.test", "test-hash", "Integration actor"); err != nil {
		t.Fatal(err)
	}
	if err = r.CreateRole(ctx, domain.Role{ID: roleID, Code: roleCode, Name: "Integration role", Scope: "TENANT", TenantID: &tenantID, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err = r.CreatePermission(ctx, domain.Permission{ID: permissionID, Code: permissionCode, Description: "Integration permission"}); err != nil {
		t.Fatal(err)
	}
	if err = r.GrantPermission(ctx, roleID, permissionID, &tenantID); err != nil {
		t.Fatal(err)
	}
	if err = r.AssignRole(ctx, userID, roleID, &tenantID, actorID); err != nil {
		t.Fatal(err)
	}
	allowed, err := r.PermissionAllowed(ctx, userID, permissionCode, &tenantID)
	if err != nil || !allowed {
		t.Fatalf("tenant permission not effective: allowed=%v err=%v", allowed, err)
	}
	allowed, err = r.PermissionAllowed(ctx, userID, permissionCode, nil)
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
	if err = r.CreateSession(ctx, sessionID, userID, refreshHash, expires, "integration", "test-agent", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	rotatedID, _ := newUUID()
	gotSession, gotUser, _, err := r.RotateRefresh(ctx, refreshHash, rotatedHash, rotatedID, expires, time.Now())
	if err != nil || gotSession != sessionID || gotUser != userID {
		t.Fatalf("refresh rotation failed: session=%s user=%s err=%v", gotSession, gotUser, err)
	}
	if _, _, _, err = r.RotateRefresh(ctx, refreshHash, "1111111111111111111111111111111111111111111111111111111111111111", refreshID, expires, time.Now()); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("replayed refresh token accepted: %v", err)
	}
	if err = r.RevokeAllSessions(ctx, userID); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = r.RotateRefresh(ctx, rotatedHash, "2222222222222222222222222222222222222222222222222222222222222222", refreshID, expires, time.Now()); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("refresh token after logout-all accepted: %v", err)
	}
}
