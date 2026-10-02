package authorization

type (
	CheckPermissionRequester struct {
		UserID         string
		PermissionCode string
		TenantID       *string
	}

	CheckPermissionPresenter struct {
		Allowed bool
	}

	RolePermissionParams struct {
		RoleID       string
		PermissionID string
		TenantID     *string
	}

	AssignRoleParams struct {
		UserID   string
		RoleID   string
		TenantID *string
		ActorID  string
	}

	RevokeRoleParams struct {
		UserID   string
		RoleID   string
		TenantID *string
	}

	EffectiveRolesRequester struct {
		UserID   string
		TenantID *string
	}

	EffectiveRolesPresenter struct {
		Roles []RolePresenter
	}

	RolePresenter struct {
		ID          string
		Code        string
		Name        string
		Description string
		Scope       string
		TenantID    *string
		Enabled     bool
	}

	EffectivePermissionsRequester struct {
		UserID   string
		TenantID *string
	}

	EffectivePermissionsPresenter struct {
		Permissions []string
	}

	Presenter struct{}
)
