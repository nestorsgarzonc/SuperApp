package application

import (
	"context"
	"superapp/auth/internal/core/ports/repository"
	"superapp/auth/internal/core/ports/security"
	"superapp/auth/internal/core/usecases/authorization"
	"superapp/auth/internal/core/usecases/permissions"
	"superapp/auth/internal/core/usecases/roles"
	"superapp/auth/internal/core/usecases/sessions"
	"superapp/auth/internal/core/usecases/users"
)

type userUseCases interface {
	GetUser(context.Context, users.GetRequester) (users.Presenter, error)
	UpdateUser(context.Context, users.UpdateRequester) (users.UpdatePresenter, error)
}
type sessionUseCases interface {
	LogoutAll(context.Context, sessions.LogoutAllRequester) (sessions.LogoutAllPresenter, error)
	ListSessions(context.Context, sessions.ListRequester) (sessions.ListPresenter, error)
	RevokeSession(context.Context, sessions.RevokeParams) (sessions.RevokePresenter, error)
}
type roleUseCases interface {
	CreateRole(context.Context, roles.CreateRequester) (roles.Presenter, error)
	UpdateRole(context.Context, roles.UpdateRequester) (roles.Presenter, error)
	DeleteRole(context.Context, roles.DeleteRequester) (roles.DeletePresenter, error)
	GetRole(context.Context, roles.GetRequester) (roles.Presenter, error)
	ListRoles(context.Context, roles.ListRequester) (roles.ListPresenter, error)
}
type permissionUseCases interface {
	CreatePermission(context.Context, permissions.CreateRequester) (permissions.Presenter, error)
	UpdatePermission(context.Context, permissions.UpdateRequester) (permissions.Presenter, error)
	DeletePermission(context.Context, permissions.DeleteRequester) (permissions.DeletePresenter, error)
	GetPermission(context.Context, permissions.GetRequester) (permissions.Presenter, error)
	ListPermissions(context.Context, permissions.ListRequester) (permissions.ListPresenter, error)
}
type authorizationUseCases interface {
	CheckPermission(context.Context, authorization.CheckPermissionRequester) (authorization.CheckPermissionPresenter, error)
	EffectivePermissions(context.Context, authorization.EffectivePermissionsRequester) (authorization.EffectivePermissionsPresenter, error)
	EffectiveRoles(context.Context, authorization.EffectiveRolesRequester) (authorization.EffectiveRolesPresenter, error)
	GrantPermission(context.Context, authorization.RolePermissionParams) (authorization.Presenter, error)
	RevokePermission(context.Context, authorization.RolePermissionParams) (authorization.Presenter, error)
	AssignRole(context.Context, authorization.AssignRoleParams) (authorization.Presenter, error)
	RevokeRole(context.Context, authorization.RevokeRoleParams) (authorization.Presenter, error)
}
type Service struct {
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

func New(repositories RepositoryDependencies, ids security.IDGenerator) *Service {
	return &Service{
		userUseCases: users.New(repositories.Users), sessionUseCases: sessions.New(repositories.Sessions),
		roleUseCases: roles.New(repositories.Roles, ids), permissionUseCases: permissions.New(repositories.Permissions, ids),
		authorizationUseCases: authorization.New(repositories.Authorization),
	}
}
