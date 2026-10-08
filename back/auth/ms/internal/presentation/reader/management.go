package reader

import (
	"net/http"
	"superapp/auth/internal/core/usecases/authorization"
	createpermission "superapp/auth/internal/core/usecases/permissions"
	createrole "superapp/auth/internal/core/usecases/roles"
	appusers "superapp/auth/internal/core/usecases/users"
	dtoPermissions "superapp/auth/internal/presentation/dto/permissions"
	dtoRoles "superapp/auth/internal/presentation/dto/roles"
	dtoUsers "superapp/auth/internal/presentation/dto/users"
	shared "superapp/auth/internal/presentation/shared"
)

func CreateRoleRequest(w http.ResponseWriter, r *http.Request) (createrole.CreateRequester, bool) {
	var dto dtoRoles.CreateRequestDTO
	if !shared.Decode(w, r, &dto) {
		return createrole.CreateRequester{}, false
	}
	return createrole.CreateRequester{Code: dto.Code, Name: dto.Name, Description: dto.Description, Scope: dto.Scope}, true
}

func CreatePermissionRequest(w http.ResponseWriter, r *http.Request) (createpermission.CreateRequester, bool) {
	var dto dtoPermissions.CreateRequestDTO
	if !shared.Decode(w, r, &dto) {
		return createpermission.CreateRequester{}, false
	}
	return createpermission.CreateRequester{Code: dto.Code, Description: dto.Description}, true
}

func UpdateRoleRequest(w http.ResponseWriter, r *http.Request, roleID string, tenantID *string) (createrole.UpdateRequester, bool) {
	var dto dtoRoles.UpdateRequestDTO
	if !shared.Decode(w, r, &dto) {
		return createrole.UpdateRequester{}, false
	}
	return createrole.UpdateRequester{
		RoleID: roleID, TenantID: tenantID, Name: dto.Name,
		Description: dto.Description, Enabled: dto.Enabled,
	}, true
}

func UpdatePermissionRequest(w http.ResponseWriter, r *http.Request, permissionID string) (createpermission.UpdateRequester, bool) {
	var dto dtoPermissions.UpdateRequestDTO
	if !shared.Decode(w, r, &dto) {
		return createpermission.UpdateRequester{}, false
	}
	return createpermission.UpdateRequester{PermissionID: permissionID, Code: dto.Code, Description: dto.Description}, true
}

func CreateRolePermissionRequest(w http.ResponseWriter, r *http.Request, roleID string, tenantID *string) (authorization.RolePermissionParams, bool) {
	var dto dtoRoles.GrantPermissionRequestDTO
	if !shared.Decode(w, r, &dto) {
		return authorization.RolePermissionParams{}, false
	}
	return authorization.RolePermissionParams{RoleID: roleID, PermissionID: dto.PermissionID, TenantID: tenantID}, true
}

func AssignRoleRequest(w http.ResponseWriter, r *http.Request, userID string, tenantID *string, actorID string) (authorization.AssignRoleParams, bool) {
	var dto dtoUsers.AssignRoleRequestDTO
	if !shared.Decode(w, r, &dto) {
		return authorization.AssignRoleParams{}, false
	}
	return authorization.AssignRoleParams{UserID: userID, RoleID: dto.RoleID, TenantID: tenantID, ActorID: actorID}, true
}

func UpdateUserRequest(w http.ResponseWriter, r *http.Request, userID string) (appusers.UpdateRequester, bool) {
	var dto dtoUsers.UpdateRequestDTO
	if !shared.Decode(w, r, &dto) {
		return appusers.UpdateRequester{}, false
	}
	return appusers.UpdateRequester{ID: userID, Email: dto.Email, FullName: dto.FullName, Status: dto.Status}, true
}
