package auth

import (
	"time"

	"superapp/auth/internal/core/domain"
)

type UserPresenter struct {
	ID        string
	Email     string
	FullName  string
	Status    string
	CreatedAt time.Time
}

func NewUserPresenter(user *domain.User) UserPresenter {
	return UserPresenter{
		ID: user.ID, Email: user.Email, FullName: user.FullName,
		Status: user.Status, CreatedAt: user.CreatedAt,
	}
}
