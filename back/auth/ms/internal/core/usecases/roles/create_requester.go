package roles

type Requester struct {
	Code        string
	Name        string
	Description string
	Scope       string
	TenantID    *string
}
