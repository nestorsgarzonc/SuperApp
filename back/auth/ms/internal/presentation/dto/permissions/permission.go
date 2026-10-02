package permissions

import "superapp/auth/internal/core/domain"

type PermissionResponse struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Description string `json:"description"`
}

func FromDomain(permission *domain.Permission) PermissionResponse {
	return PermissionResponse{ID: permission.ID, Code: permission.Code, Description: permission.Description}
}

func ListFromDomain(values []domain.Permission) []PermissionResponse {
	result := make([]PermissionResponse, 0, len(values))
	for i := range values {
		result = append(result, FromDomain(&values[i]))
	}
	return result
}
