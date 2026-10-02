package application

import (
	"context"
	"superapp/auth/internal/core/domain"
	"superapp/auth/internal/core/ports/repository"
	"superapp/auth/internal/core/ports/security"
	"superapp/auth/internal/core/usecases/auth"
	"superapp/auth/internal/core/usecases/authorization"
	contracts "superapp/auth/internal/core/usecases/contracts"
	"superapp/auth/internal/core/usecases/permissions"
	"superapp/auth/internal/core/usecases/roles"
	"superapp/auth/internal/core/usecases/sessions"
	"superapp/auth/internal/core/usecases/users"
)

type authUseCases interface {
	Register(context.Context, string, string, string) (*domain.User, error)
	Login(context.Context, string, string, string, string, string) (*domain.User, *contracts.TokenPair, error)
	Refresh(context.Context, string) (*contracts.TokenPair, error)
	Logout(context.Context, string) error
	Authenticate(string) (*security.TokenClaims, error)
	JWKS() map[string]any
}
type userUseCases interface {
	User(context.Context, string) (*domain.User, error)
	UpdateUser(context.Context, string, *string, *string, *string) error
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

type RepositoryDependencies struct {
	Users         repository.UserRepository
	Sessions      repository.SessionRepository
	Roles         repository.RoleRepository
	Permissions   repository.PermissionRepository
	Authorization repository.AuthorizationRepository
}

func New(repositories RepositoryDependencies, passwords security.PasswordHasher, tokens security.TokenIssuer, ids security.IDGenerator, cfg contracts.Config) *Service {
	return &Service{
		authUseCases: auth.New(repositories.Users, repositories.Sessions, passwords, tokens, ids, cfg),
		userUseCases: users.New(repositories.Users), sessionUseCases: sessions.New(repositories.Sessions),
		roleUseCases: roles.New(repositories.Roles, ids), permissionUseCases: permissions.New(repositories.Permissions, ids),
		authorizationUseCases: authorization.New(repositories.Authorization),
	}
}
