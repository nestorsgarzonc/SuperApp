package httpapi

import (
	"context"
	"superapp/auth/internal/core/ports/security"
	"superapp/auth/internal/core/usecases/auth/login"
	"superapp/auth/internal/core/usecases/auth/logout"
	"superapp/auth/internal/core/usecases/auth/refresh"
	"superapp/auth/internal/core/usecases/auth/register"
	"superapp/auth/internal/core/usecases/authorization"
	"superapp/auth/internal/core/usecases/permissions"
	"superapp/auth/internal/core/usecases/roles"
	appsessions "superapp/auth/internal/core/usecases/sessions"
	"superapp/auth/internal/core/usecases/users"
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
	GetUser(context.Context, users.GetRequester) (users.Presenter, error)
	UpdateUser(context.Context, users.UpdateRequester) (users.UpdatePresenter, error)
}

type SessionService interface {
	LogoutAll(context.Context, appsessions.LogoutAllRequester) (appsessions.LogoutAllPresenter, error)
	ListSessions(context.Context, appsessions.ListRequester) (appsessions.ListPresenter, error)
	RevokeSession(context.Context, appsessions.RevokeParams) (appsessions.RevokePresenter, error)
}

type RoleService interface {
	CreateRole(context.Context, roles.CreateRequester) (roles.Presenter, error)
	UpdateRole(context.Context, roles.UpdateRequester) (roles.Presenter, error)
	DeleteRole(context.Context, roles.DeleteRequester) (roles.DeletePresenter, error)
	GetRole(context.Context, roles.GetRequester) (roles.Presenter, error)
	ListRoles(context.Context, roles.ListRequester) (roles.ListPresenter, error)
}

type PermissionService interface {
	CreatePermission(context.Context, permissions.CreateRequester) (permissions.Presenter, error)
	UpdatePermission(context.Context, permissions.UpdateRequester) (permissions.Presenter, error)
	DeletePermission(context.Context, permissions.DeleteRequester) (permissions.DeletePresenter, error)
	GetPermission(context.Context, permissions.GetRequester) (permissions.Presenter, error)
	ListPermissions(context.Context, permissions.ListRequester) (permissions.ListPresenter, error)
}

type AuthorizationService interface {
	CheckPermission(context.Context, authorization.CheckPermissionRequester) (authorization.CheckPermissionPresenter, error)
	EffectivePermissions(context.Context, authorization.EffectivePermissionsRequester) (authorization.EffectivePermissionsPresenter, error)
	EffectiveRoles(context.Context, authorization.EffectiveRolesRequester) (authorization.EffectiveRolesPresenter, error)
	GrantPermission(context.Context, authorization.RolePermissionParams) (authorization.Presenter, error)
	RevokePermission(context.Context, authorization.RolePermissionParams) (authorization.Presenter, error)
	AssignRole(context.Context, authorization.AssignRoleParams) (authorization.Presenter, error)
	RevokeRole(context.Context, authorization.RevokeRoleParams) (authorization.Presenter, error)
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
		Logout: services.Logout,
		Tokens: services.Tokens, Users: services.Users, Sessions: services.Sessions,
		Roles: services.Roles, Permissions: services.Permissions,
		Authorization: services.Authorization, TenantSecret: secret,
	}
}
