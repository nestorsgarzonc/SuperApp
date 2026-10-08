package login

import (
	"superapp/auth/internal/core/usecases/auth"
	"superapp/auth/internal/core/usecases/contracts"
)

type Presenter struct {
	User   auth.UserPresenter
	Tokens contracts.TokenPair
}
