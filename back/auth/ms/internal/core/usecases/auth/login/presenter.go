package login

import (
	"superapp/auth/internal/core/domain"
	"superapp/auth/internal/core/usecases/contracts"
)

type Presenter struct {
	User   *domain.User
	Tokens contracts.TokenPair
}
