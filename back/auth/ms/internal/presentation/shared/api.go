package httpapi

import (
	"context"
	"superapp/auth/internal/core/domain"
	"superapp/auth/internal/core/ports/security"
	"superapp/auth/internal/core/usecases/contracts"
)

type AuthService interface {
	Register(context.Context, string, string, string) (*domain.User, error)
	Login(context.Context, string, string, string, string, string) (*domain.User, *contracts.TokenPair, error)
	Refresh(context.Context, string) (*contracts.TokenPair, error)
	Logout(context.Context, string) error
	Authenticate(string) (*security.TokenClaims, error)
	JWKS() map[string]any
}

type UserService interface {
	User(context.Context, string) (*domain.User, error)
	UpdateUser(context.Context, string, *string, *string, *string) error
}

type SessionService interface {
	LogoutAll(context.Context, string) error
	Sessions(context.Context, string) ([]domain.Session, error)
	RevokeSession(context.Context, string, string) error
}

type RoleService interface {
	Roles(context.Context, *string) ([]domain.Role, error)
	RoleByID(context.Context, string, *string) (*domain.Role, error)
	CreateRole(context.Context, domain.Role) error
	UpdateRole(context.Context, domain.Role, *string) error
	DeleteRole(context.Context, string, *string) error
}

type PermissionService interface {
	Permissions(context.Context) ([]domain.Permission, error)
	PermissionByID(context.Context, string) (*domain.Permission, error)
	CreatePermission(context.Context, domain.Permission) error
	UpdatePermission(context.Context, domain.Permission) error
	DeletePermission(context.Context, string) error
}

type AuthorizationService interface {
	HasPermission(context.Context, string, string, *string) (bool, error)
	EffectivePermissions(context.Context, string, *string) ([]string, error)
	EffectiveRoles(context.Context, string, *string) ([]domain.Role, error)
	GrantPermission(context.Context, string, string, *string) error
	RevokePermission(context.Context, string, string, *string) error
	AssignRole(context.Context, string, string, *string, string) error
	RevokeRole(context.Context, string, string, *string) error
}

type Services struct {
	Auth          AuthService
	Users         UserService
	Sessions      SessionService
	Roles         RoleService
	Permissions   PermissionService
	Authorization AuthorizationService
}

type API struct {
	Authentication AuthService
	Users          UserService
	Sessions       SessionService
	Roles          RoleService
	Permissions    PermissionService
	Authorization  AuthorizationService
	TenantSecret   []byte
}
type Identity struct{ UserID, Email, SessionID string }

func New(services Services, secret []byte) *API {
	return &API{
		Authentication: services.Auth, Users: services.Users, Sessions: services.Sessions,
		Roles: services.Roles, Permissions: services.Permissions,
		Authorization: services.Authorization, TenantSecret: secret,
	}
}
