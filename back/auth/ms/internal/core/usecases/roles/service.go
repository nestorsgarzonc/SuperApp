package roles

import (
	"superapp/auth/internal/core/ports/repository"
	"superapp/auth/internal/core/ports/security"
)

type Service struct {
	repository repository.RoleRepository
	newID      security.IDGenerator
}

func New(repository repository.RoleRepository, newID security.IDGenerator) *Service {
	return &Service{repository: repository, newID: newID}
}
