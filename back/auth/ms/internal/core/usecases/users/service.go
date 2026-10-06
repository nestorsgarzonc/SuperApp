package users

import (
	"superapp/auth/internal/core/ports/repository"
)

type Service struct{ repository repository.UserRepository }

func New(repository repository.UserRepository) *Service { return &Service{repository: repository} }
