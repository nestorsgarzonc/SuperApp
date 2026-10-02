package contracts

import "time"

type Config struct{ AccessTTL, RefreshTTL time.Duration }
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}
type TokenClaims struct {
	Subject, Email, SessionID string
	ExpiresAt                 time.Time
}
type TokenIssuer interface {
	Issue(string, string, string) (string, time.Time, error)
	Verify(string) (*TokenClaims, error)
	JWKS() map[string]any
}
type PasswordHasher interface {
	Hash(string) (string, error)
	Verify(string, string) error
	Burn(string)
}
type IDGenerator interface{ New() string }
