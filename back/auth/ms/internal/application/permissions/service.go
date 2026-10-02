package permissions

import (
	"superapp/auth/internal/application/contracts"
	"superapp/auth/internal/ports/repository"
)

type Service struct {
	repository repository.PermissionRepository
	newID      contracts.IDGenerator
}

func New(repository repository.PermissionRepository, newID contracts.IDGenerator) *Service {
	return &Service{repository: repository, newID: newID}
}
