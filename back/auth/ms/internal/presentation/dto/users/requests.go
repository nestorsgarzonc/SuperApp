package users

type UpdateRequestDTO struct {
	Email    *string `json:"email"`
	FullName *string `json:"full_name"`
	Status   *string `json:"status"`
}

type AssignRoleRequestDTO struct {
	RoleID string `json:"role_id"`
}
