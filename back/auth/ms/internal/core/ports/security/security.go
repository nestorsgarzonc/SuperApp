package security

import "time"

type TokenClaims struct {
	Subject, Email, SessionID string
	ExpiresAt                 time.Time
}

type TokenSigner interface {
	Issue(string, string, string) (string, time.Time, error)
}

type TokenVerifier interface {
	Verify(string) (*TokenClaims, error)
}

type JWKSProvider interface {
	JWKS() map[string]any
}

type PasswordHasher interface {
	Hash(string) (string, error)
}

type PasswordVerifier interface {
	Verify(string, string) error
	Burn(string)
}

type IDGenerator func() string
