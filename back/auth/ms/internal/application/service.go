package application

import (
	"context"
	"superapp/auth/internal/application/auth"
	"superapp/auth/internal/application/authorization"
	contracts "superapp/auth/internal/application/contracts"
	"superapp/auth/internal/application/permissions"
	"superapp/auth/internal/application/roles"
	"superapp/auth/internal/application/sessions"
	"superapp/auth/internal/application/users"
	"superapp/auth/internal/domain"
)

type authUseCases interface {
	Register(context.Context, string, string, string) (*domain.User, error)
	Login(context.Context, string, string, string, string, string) (*domain.User, *contracts.TokenPair, error)
	Refresh(context.Context, string) (*contracts.TokenPair, error)
	Logout(context.Context, string) error
	Authenticate(string) (*contracts.TokenClaims, error)
	JWKS() map[string]any
}
type userUseCases interface {
	User(context.Context, string) (*domain.User, error)
	UpdateUser(context.Context, string, string, string) error
	SetUserStatus(context.Context, string, string) error
}
type sessionUseCases interface {
	LogoutAll(context.Context, string) error
	Sessions(context.Context, string) ([]domain.Session, error)
	RevokeSession(context.Context, string, string) error
}
type roleUseCases interface {
	Roles(context.Context, *string) ([]domain.Role, error)
	CreateRole(context.Context, domain.Role) error
	RoleByID(context.Context, string, *string) (*domain.Role, error)
	UpdateRole(context.Context, domain.Role, *string) error
	DeleteRole(context.Context, string, *string) error
}
type permissionUseCases interface {
	Permissions(context.Context) ([]domain.Permission, error)
	CreatePermission(context.Context, domain.Permission) error
	PermissionByID(context.Context, string) (*domain.Permission, error)
	UpdatePermission(context.Context, domain.Permission) error
	DeletePermission(context.Context, string) error
}
type authorizationUseCases interface {
	HasPermission(context.Context, string, string, *string) (bool, error)
	EffectivePermissions(context.Context, string, *string) ([]string, error)
	EffectiveRoles(context.Context, string, *string) ([]domain.Role, error)
	GrantPermission(context.Context, string, string, *string) error
	RevokePermission(context.Context, string, string, *string) error
	AssignRole(context.Context, string, string, *string, string) error
	RevokeRole(context.Context, string, string, *string) error
}
type Service struct {
	authUseCases
	userUseCases
	sessionUseCases
	roleUseCases
	permissionUseCases
	authorizationUseCases
}

func New(repo Repository, passwords PasswordHasher, tokens TokenIssuer, ids IDGenerator, cfg Config) *Service {
	return &Service{
		authUseCases: auth.New(repo, repo, passwords, tokens, ids, cfg),
		userUseCases: users.New(repo), sessionUseCases: sessions.New(repo),
		roleUseCases: roles.New(repo, ids), permissionUseCases: permissions.New(repo, ids),
		authorizationUseCases: authorization.New(repo),
	}
}
