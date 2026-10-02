package auth

import "superapp/auth/internal/core/usecases/contracts"

type TokenPairResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

func FromDomain(pair *contracts.TokenPair) TokenPairResponse {
	return TokenPairResponse{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken, ExpiresIn: pair.ExpiresIn}
}
