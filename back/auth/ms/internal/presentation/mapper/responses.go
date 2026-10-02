package mapper

import (
	"superapp/auth/internal/core/domain"
	"superapp/auth/internal/core/usecases/contracts"
	createpermission "superapp/auth/internal/core/usecases/permissions"
	createrole "superapp/auth/internal/core/usecases/roles"
	dtoAuth "superapp/auth/internal/presentation/dto/auth"
	dtoPermissions "superapp/auth/internal/presentation/dto/permissions"
	dtoRoles "superapp/auth/internal/presentation/dto/roles"
	dtoSessions "superapp/auth/internal/presentation/dto/sessions"
	dtoUsers "superapp/auth/internal/presentation/dto/users"
)

func UserResponse(user *domain.User) dtoUsers.UserResponse {
	return dtoUsers.UserResponse{ID: user.ID, Email: user.Email, FullName: user.FullName, Status: user.Status, CreatedAt: user.CreatedAt}
}

func RoleResponse(role *domain.Role) dtoRoles.RoleResponse {
	return dtoRoles.RoleResponse{ID: role.ID, Code: role.Code, Name: role.Name, Description: role.Description, Scope: role.Scope, TenantID: role.TenantID, Enabled: role.Enabled}
}

func RoleResponses(values []domain.Role) []dtoRoles.RoleResponse {
	result := make([]dtoRoles.RoleResponse, 0, len(values))
	for i := range values {
		result = append(result, RoleResponse(&values[i]))
	}
	return result
}

func PermissionResponse(permission *domain.Permission) dtoPermissions.PermissionResponse {
	return dtoPermissions.PermissionResponse{ID: permission.ID, Code: permission.Code, Description: permission.Description}
}

func PermissionResponses(values []domain.Permission) []dtoPermissions.PermissionResponse {
	result := make([]dtoPermissions.PermissionResponse, 0, len(values))
	for i := range values {
		result = append(result, PermissionResponse(&values[i]))
	}
	return result
}

func SessionResponses(values []domain.Session) []dtoSessions.SessionResponse {
	result := make([]dtoSessions.SessionResponse, 0, len(values))
	for _, value := range values {
		result = append(result, dtoSessions.SessionResponse{ID: value.ID, Device: value.Device, CreatedAt: value.CreatedAt, ExpiresAt: value.ExpiresAt})
	}
	return result
}

func TokenPairResponse(pair contracts.TokenPair) dtoAuth.TokenPairResponse {
	return dtoAuth.TokenPairResponse{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken, ExpiresIn: pair.ExpiresIn}
}

func CreatedRoleResponse(role createrole.Presenter) dtoRoles.RoleResponse {
	return dtoRoles.RoleResponse{ID: role.ID, Code: role.Code, Name: role.Name, Description: role.Description, Scope: role.Scope, TenantID: role.TenantID, Enabled: role.Enabled}
}

func CreatedPermissionResponse(permission createpermission.Presenter) dtoPermissions.PermissionResponse {
	return dtoPermissions.PermissionResponse{ID: permission.ID, Code: permission.Code, Description: permission.Description}
}
