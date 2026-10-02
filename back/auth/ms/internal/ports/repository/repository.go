package repository

type Repository interface {
	UserRepository
	SessionRepository
	RoleRepository
	PermissionRepository
	AuthorizationRepository
}
