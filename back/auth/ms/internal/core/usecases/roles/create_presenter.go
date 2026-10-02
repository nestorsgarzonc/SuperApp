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
