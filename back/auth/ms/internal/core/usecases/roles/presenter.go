package roles

type (
	Presenter struct {
		ID          string
		Code        string
		Name        string
		Description string
		Scope       string
		TenantID    *string
		Enabled     bool
	}

	ListPresenter struct {
		Roles []Presenter
	}

	GetRequester struct {
		RoleID   string
		TenantID *string
	}

	ListRequester struct {
		TenantID *string
	}

	UpdateRequester struct {
		RoleID      string
		TenantID    *string
		Name        *string
		Description *string
		Enabled     *bool
	}

	DeleteRequester struct {
		RoleID   string
		TenantID *string
	}

	DeletePresenter struct{}
)
