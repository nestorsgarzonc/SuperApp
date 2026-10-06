package auth

import "superapp/auth/internal/presentation/dto/users"

type LoginResponseDTO struct {
	User   users.UserResponse `json:"user"`
	Tokens TokenPairResponse  `json:"tokens"`
}

type RegisterResponseDTO = users.UserResponse
type RefreshResponseDTO = TokenPairResponse
