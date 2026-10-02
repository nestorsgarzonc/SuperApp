package application

import (
	"context"
	"time"

	"superapp/auth/internal/domain"
)

type TokenClaims struct {
	Subject, Email, SessionID string
	ExpiresAt                 time.Time
}
type TokenIssuer interface {
	Issue(userID, email, sessionID string) (string, time.Time, error)
	Verify(token string) (*TokenClaims, error)
	JWKS() map[string]any
}
type PasswordHasher interface {
	Hash(string) (string, error)
	Verify(hash, password string) error
	Burn(password string)
}

type Repository interface {
	Ping(context.Context) error
	CreateUser(context.Context, string, string, string, string) (*domain.User, error)
	UserByEmail(context.Context, string) (*domain.User, string, error)
	UserByID(context.Context, string) (*domain.User, error)
	UpdateUser(context.Context, string, string, string) error
	SetLastLogin(context.Context, string) error
	CreateSession(context.Context, string, string, string, time.Time, string, string, string) error
	RotateRefresh(context.Context, string, string, string, time.Time, time.Time) (sessionID, userID, email string, err error)
	RevokeRefresh(context.Context, string) error
	RevokeAllSessions(context.Context, string) error
	RevokeSession(context.Context, string, string) error
	Sessions(context.Context, string) ([]domain.Session, error)
	Roles(context.Context, *string) ([]domain.Role, error)
	CreateRole(context.Context, domain.Role) error
	RoleByID(context.Context, string, *string) (*domain.Role, error)
	UpdateRole(context.Context, domain.Role, *string) error
	DeleteRole(context.Context, string, *string) error
	Permissions(context.Context) ([]domain.Permission, error)
	CreatePermission(context.Context, domain.Permission) error
	PermissionByID(context.Context, string) (*domain.Permission, error)
	UpdatePermission(context.Context, domain.Permission) error
	DeletePermission(context.Context, string) error
	GrantPermission(context.Context, string, string, *string) error
	RevokePermission(context.Context, string, string, *string) error
	AssignRole(context.Context, string, string, *string, string) error
	RevokeRole(context.Context, string, string, *string) error
	SetUserStatus(context.Context, string, string) error
	EffectiveRoles(context.Context, string, *string) ([]domain.Role, error)
	EffectivePermissions(context.Context, string, *string) ([]string, error)
	PermissionAllowed(context.Context, string, string, *string) (bool, error)
}

type IDGenerator interface{ New() string }
