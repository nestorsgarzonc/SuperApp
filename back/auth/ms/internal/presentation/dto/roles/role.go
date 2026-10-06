package roles

type RoleResponse struct {
	ID          string  `json:"id"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Scope       string  `json:"scope"`
	TenantID    *string `json:"tenant_id,omitempty"`
	Enabled     bool    `json:"enabled"`
}
