package roles

import (
	"superapp/auth/internal/application/contracts"
	"superapp/auth/internal/ports/repository"
)

type Service struct {
	repository repository.RoleRepository
	newID      contracts.IDGenerator
}

func New(repository repository.RoleRepository, newID contracts.IDGenerator) *Service {
	return &Service{repository: repository, newID: newID}
}
