package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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

type recordingSink struct{ got [][]string }

func (s *recordingSink) SessionsRevoked(_ context.Context, ids []string) {
	s.got = append(s.got, append([]string(nil), ids...))
}

// TestRevocationSinkIntegration checks every revocation path reports exactly
// the sessions it revoked (the api-gateway rejects their access tokens).
func TestRevocationSinkIntegration(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set AUTH_TEST_DATABASE_URL to a dedicated disposable PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	sink := &recordingSink{}
	r := New(db).WithRevocationSink(sink)

	userID, _ := newUUID()
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM auth_users WHERE id=$1`, userID) })
	if _, err = r.CreateUser(ctx, coreRepository.CreateUserParams{ID: userID, Email: userID + "@integration.test", PasswordHash: "h", FullName: "Revocation user"}); err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(time.Hour)
	newSession := func(hash string) string {
		sid, _ := newUUID()
		if err := r.CreateSession(ctx, coreRepository.CreateSessionParams{SessionID: sid, UserID: userID, RefreshHash: hash, ExpiresAt: expires}); err != nil {
			t.Fatal(err)
		}
		return sid
	}
	// Unique per run: refresh token hashes are globally unique.
	hash := func(c byte) string {
		sum := sha256.Sum256([]byte(userID + string(c)))
		return hex.EncodeToString(sum[:])
	}
	expect := func(step string, want ...string) {
		t.Helper()
		if len(sink.got) != 1 || len(sink.got[0]) != len(want) {
			t.Fatalf("%s: published %v, want %v", step, sink.got, want)
		}
		seen := map[string]bool{}
		for _, id := range sink.got[0] {
			seen[id] = true
		}
		for _, id := range want {
			if !seen[id] {
				t.Fatalf("%s: published %v, want %v", step, sink.got, want)
			}
		}
		sink.got = nil
	}

	s1 := newSession(hash('1'))
	if err = r.RevokeRefresh(ctx, hash('1')); err != nil {
		t.Fatal(err)
	}
	expect("logout", s1)

	s2 := newSession(hash('2'))
	if err = r.RevokeSession(ctx, coreRepository.RevokeSessionParams{UserID: userID, SessionID: s2}); err != nil {
		t.Fatal(err)
	}
	expect("revoke one session", s2)

	s3, s4 := newSession(hash('3')), newSession(hash('4'))
	if err = r.RevokeAllSessions(ctx, userID); err != nil {
		t.Fatal(err)
	}
	expect("logout-all (only still-active sessions)", s3, s4)

	s5 := newSession(hash('5'))
	next, _ := newUUID()
	if _, err = r.RotateRefresh(ctx, coreRepository.RotateRefreshParams{OldHash: hash('5'), NewHash: hash('6'), NewTokenID: next, ExpiresAt: expires, Now: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if len(sink.got) != 0 {
		t.Fatalf("a normal refresh revoked something: %v", sink.got)
	}
	replay, _ := newUUID()
	if _, err = r.RotateRefresh(ctx, coreRepository.RotateRefreshParams{OldHash: hash('5'), NewHash: hash('7'), NewTokenID: replay, ExpiresAt: expires, Now: time.Now()}); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("replay accepted: %v", err)
	}
	expect("refresh token reuse", s5)

	s8 := newSession(hash('8'))
	disabled := "DISABLED"
	if err = r.UpdateUser(ctx, coreRepository.UpdateUserParams{ID: userID, Status: &disabled}); err != nil {
		t.Fatal(err)
	}
	expect("account disabled", s8)
}
