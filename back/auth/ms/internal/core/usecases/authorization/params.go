package authorization

type CheckPermissionRequester struct {
	UserID         string
	PermissionCode string
	TenantID       *string
}

type CheckPermissionPresenter struct {
	Allowed bool
}

type RolePermissionParams struct {
	RoleID       string
	PermissionID string
	TenantID     *string
}

type AssignRoleParams struct {
	UserID   string
	RoleID   string
	TenantID *string
	ActorID  string
}

type RevokeRoleParams struct {
	UserID   string
	RoleID   string
	TenantID *string
}

type EffectiveRolesRequester struct {
	UserID   string
	TenantID *string
}

type EffectiveRolesPresenter struct {
	Roles []RolePresenter
}

type RolePresenter struct {
	ID          string
	Code        string
	Name        string
	Description string
	Scope       string
	TenantID    *string
	Enabled     bool
}

type EffectivePermissionsRequester struct {
	UserID   string
	TenantID *string
}

type EffectivePermissionsPresenter struct {
	Permissions []string
}

type Presenter struct{}
