package authorization

type HasPermissionParams struct {
	UserID         string
	PermissionCode string
	TenantID       *string
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
