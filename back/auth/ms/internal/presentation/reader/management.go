package reader

import (
	"net/http"
	createpermission "superapp/auth/internal/core/usecases/permissions"
	createrole "superapp/auth/internal/core/usecases/roles"
	dtoPermissions "superapp/auth/internal/presentation/dto/permissions"
	dtoRoles "superapp/auth/internal/presentation/dto/roles"
	shared "superapp/auth/internal/presentation/shared"
)

func CreateRoleRequest(w http.ResponseWriter, r *http.Request) (createrole.Requester, bool) {
	var dto dtoRoles.CreateRequestDTO
	if !shared.Decode(w, r, &dto) {
		return createrole.Requester{}, false
	}
	return createrole.Requester{Code: dto.Code, Name: dto.Name, Description: dto.Description, Scope: dto.Scope}, true
}

func CreatePermissionRequest(w http.ResponseWriter, r *http.Request) (createpermission.Requester, bool) {
	var dto dtoPermissions.CreateRequestDTO
	if !shared.Decode(w, r, &dto) {
		return createpermission.Requester{}, false
	}
	return createpermission.Requester{Code: dto.Code, Description: dto.Description}, true
}
