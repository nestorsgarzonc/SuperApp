package roles

type CreateRequester struct {
	Code        string
	Name        string
	Description string
	Scope       string
	TenantID    *string
}
