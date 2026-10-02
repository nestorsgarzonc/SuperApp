package roles

import "superapp/auth/internal/core/domain"

type RoleResponse struct {
	ID          string  `json:"id"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Scope       string  `json:"scope"`
	TenantID    *string `json:"tenant_id,omitempty"`
	Enabled     bool    `json:"enabled"`
}

func FromDomain(role *domain.Role) RoleResponse {
	return RoleResponse{ID: role.ID, Code: role.Code, Name: role.Name, Description: role.Description, Scope: role.Scope, TenantID: role.TenantID, Enabled: role.Enabled}
}

func ListFromDomain(values []domain.Role) []RoleResponse {
	result := make([]RoleResponse, 0, len(values))
	for i := range values {
		result = append(result, FromDomain(&values[i]))
	}
	return result
}
