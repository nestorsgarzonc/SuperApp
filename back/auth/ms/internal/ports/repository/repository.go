package repository

type Repository interface {
	HealthRepository
	UserRepository
	SessionRepository
	RoleRepository
	PermissionRepository
	AuthorizationRepository
}
