package authorization

import (
	"superapp/auth/internal/ports/repository"
)

type Service struct {
	repository repository.AuthorizationRepository
}

func New(repository repository.AuthorizationRepository) *Service {
	return &Service{repository: repository}
}
