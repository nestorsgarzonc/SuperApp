package sessions

import (
	"superapp/auth/internal/core/ports/repository"
)

type Service struct{ repository repository.SessionRepository }

func New(repository repository.SessionRepository) *Service { return &Service{repository: repository} }
