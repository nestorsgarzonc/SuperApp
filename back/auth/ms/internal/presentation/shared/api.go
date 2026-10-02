package httpapi

import (
	"context"
	"superapp/auth/internal/core/domain"
	"superapp/auth/internal/core/ports/security"
	"superapp/auth/internal/core/usecases/auth/login"
	"superapp/auth/internal/core/usecases/auth/logout"
	"superapp/auth/internal/core/usecases/auth/refresh"
	"superapp/auth/internal/core/usecases/auth/register"
)

type LoginUseCase interface {
	Execute(context.Context, login.Requester) (login.Presenter, error)
}
type RegisterUseCase interface {
	Execute(context.Context, register.Requester) (register.Presenter, error)
}
type RefreshUseCase interface {
	Execute(context.Context, refresh.Requester) (refresh.Presenter, error)
}
type LogoutUseCase interface {
	Execute(context.Context, logout.Requester) (logout.Presenter, error)
}
type TokenReader interface {
	Verify(string) (*security.TokenClaims, error)
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
	CreateRole(context.Context, domain.Role) (*domain.Role, error)
	UpdateRole(context.Context, domain.Role, *string) error
	DeleteRole(context.Context, string, *string) error
}

type PermissionService interface {
	Permissions(context.Context) ([]domain.Permission, error)
	PermissionByID(context.Context, string) (*domain.Permission, error)
	CreatePermission(context.Context, domain.Permission) (*domain.Permission, error)
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
	Login         LoginUseCase
	Register      RegisterUseCase
	Refresh       RefreshUseCase
	Logout        LogoutUseCase
	Tokens        TokenReader
	Users         UserService
	Sessions      SessionService
	Roles         RoleService
	Permissions   PermissionService
	Authorization AuthorizationService
}

type API struct {
	Login         LoginUseCase
	Register      RegisterUseCase
	Refresh       RefreshUseCase
	Logout        LogoutUseCase
	Tokens        TokenReader
	Users         UserService
	Sessions      SessionService
	Roles         RoleService
	Permissions   PermissionService
	Authorization AuthorizationService
	TenantSecret  []byte
}
type Identity struct{ UserID, Email, SessionID string }

func New(services Services, secret []byte) *API {
	return &API{
		Login: services.Login, Register: services.Register, Refresh: services.Refresh,
		Logout: services.Logout, Tokens: services.Tokens, Users: services.Users, Sessions: services.Sessions,
		Roles: services.Roles, Permissions: services.Permissions,
		Authorization: services.Authorization, TenantSecret: secret,
	}
}
