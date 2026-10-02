package roles

type Presenter struct {
	ID          string
	Code        string
	Name        string
	Description string
	Scope       string
	TenantID    *string
	Enabled     bool
}

type ListPresenter struct {
	Roles []Presenter
}

type GetRequester struct {
	RoleID   string
	TenantID *string
}

type ListRequester struct {
	TenantID *string
}

type UpdateRequester struct {
	RoleID      string
	TenantID    *string
	Name        *string
	Description *string
	Enabled     *bool
}

type DeleteRequester struct {
	RoleID   string
	TenantID *string
}

type DeletePresenter struct{}
