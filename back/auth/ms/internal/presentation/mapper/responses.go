package mapper

import (
	"superapp/auth/internal/core/domain"
	"superapp/auth/internal/core/usecases/authorization"
	"superapp/auth/internal/core/usecases/contracts"
	"superapp/auth/internal/core/usecases/permissions"
	"superapp/auth/internal/core/usecases/roles"
	"superapp/auth/internal/core/usecases/sessions"
	"superapp/auth/internal/core/usecases/users"
	dtoAuth "superapp/auth/internal/presentation/dto/auth"
	dtoPermissions "superapp/auth/internal/presentation/dto/permissions"
	dtoRoles "superapp/auth/internal/presentation/dto/roles"
	dtoSessions "superapp/auth/internal/presentation/dto/sessions"
	dtoUsers "superapp/auth/internal/presentation/dto/users"
)

func UserResponse(user *domain.User) dtoUsers.UserResponse {
	return dtoUsers.UserResponse{ID: user.ID, Email: user.Email, FullName: user.FullName, Status: user.Status, CreatedAt: user.CreatedAt}
}

func UserPresenterResponse(user users.Presenter) dtoUsers.UserResponse {
	return dtoUsers.UserResponse{ID: user.ID, Email: user.Email, FullName: user.FullName, Status: user.Status, CreatedAt: user.CreatedAt}
}

func RoleResponse(role roles.Presenter) dtoRoles.RoleResponse {
	return dtoRoles.RoleResponse{ID: role.ID, Code: role.Code, Name: role.Name, Description: role.Description, Scope: role.Scope, TenantID: role.TenantID, Enabled: role.Enabled}
}

func RoleResponses(values roles.ListPresenter) []dtoRoles.RoleResponse {
	result := make([]dtoRoles.RoleResponse, 0, len(values.Roles))
	for _, role := range values.Roles {
		result = append(result, RoleResponse(role))
	}
	return result
}

func AuthorizedRoleResponses(values authorization.EffectiveRolesPresenter) []dtoRoles.RoleResponse {
	result := make([]dtoRoles.RoleResponse, 0, len(values.Roles))
	for _, role := range values.Roles {
		result = append(result, dtoRoles.RoleResponse{
			ID: role.ID, Code: role.Code, Name: role.Name, Description: role.Description,
			Scope: role.Scope, TenantID: role.TenantID, Enabled: role.Enabled,
		})
	}
	return result
}

func PermissionResponse(permission permissions.Presenter) dtoPermissions.PermissionResponse {
	return dtoPermissions.PermissionResponse{ID: permission.ID, Code: permission.Code, Description: permission.Description}
}

func PermissionResponses(values permissions.ListPresenter) []dtoPermissions.PermissionResponse {
	result := make([]dtoPermissions.PermissionResponse, 0, len(values.Permissions))
	for _, permission := range values.Permissions {
		result = append(result, PermissionResponse(permission))
	}
	return result
}

func SessionResponses(values sessions.ListPresenter) []dtoSessions.SessionResponse {
	result := make([]dtoSessions.SessionResponse, 0, len(values.Sessions))
	for _, value := range values.Sessions {
		result = append(result, dtoSessions.SessionResponse{ID: value.ID, Device: value.Device, CreatedAt: value.CreatedAt, ExpiresAt: value.ExpiresAt})
	}
	return result
}

func EffectivePermissionsResponse(values authorization.EffectivePermissionsPresenter) []string {
	return values.Permissions
}

func TokenPairResponse(pair contracts.TokenPair) dtoAuth.TokenPairResponse {
	return dtoAuth.TokenPairResponse{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken, ExpiresIn: pair.ExpiresIn}
}
