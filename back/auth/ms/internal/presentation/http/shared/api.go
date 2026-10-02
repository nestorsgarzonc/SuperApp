package httpapi

import (
	"context"
	"superapp/auth/internal/application"
	"superapp/auth/internal/domain"
)

type Service interface {
	Register(context.Context, string, string, string) (*domain.User, error)
	Login(context.Context, string, string, string, string, string) (*domain.User, *application.TokenPair, error)
	Refresh(context.Context, string) (*application.TokenPair, error)
	Logout(context.Context, string) error
	LogoutAll(context.Context, string) error
	Authenticate(string) (*application.TokenClaims, error)
	User(context.Context, string) (*domain.User, error)
	UpdateUser(context.Context, string, string, string) error
	Sessions(context.Context, string) ([]domain.Session, error)
	RevokeSession(context.Context, string, string) error
	HasPermission(context.Context, string, string, *string) (bool, error)
	EffectivePermissions(context.Context, string, *string) ([]string, error)
	EffectiveRoles(context.Context, string, *string) ([]domain.Role, error)
	Roles(context.Context, *string) ([]domain.Role, error)
	RoleByID(context.Context, string, *string) (*domain.Role, error)
	CreateRole(context.Context, domain.Role) error
	UpdateRole(context.Context, domain.Role, *string) error
	DeleteRole(context.Context, string, *string) error
	Permissions(context.Context) ([]domain.Permission, error)
	PermissionByID(context.Context, string) (*domain.Permission, error)
	CreatePermission(context.Context, domain.Permission) error
	UpdatePermission(context.Context, domain.Permission) error
	DeletePermission(context.Context, string) error
	GrantPermission(context.Context, string, string, *string) error
	RevokePermission(context.Context, string, string, *string) error
	AssignRole(context.Context, string, string, *string, string) error
	RevokeRole(context.Context, string, string, *string) error
	SetUserStatus(context.Context, string, string) error
}

type API struct {
	Service      Service
	TenantSecret []byte
}
type Identity struct{ UserID, Email, SessionID string }

func New(service Service, secret []byte) *API { return &API{Service: service, TenantSecret: secret} }
