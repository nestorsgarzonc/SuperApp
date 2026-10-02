package users

import (
	"time"

	"superapp/auth/internal/core/domain"
)

type UserResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func FromDomain(user *domain.User) UserResponse {
	return UserResponse{ID: user.ID, Email: user.Email, FullName: user.FullName, Status: user.Status, CreatedAt: user.CreatedAt}
}
