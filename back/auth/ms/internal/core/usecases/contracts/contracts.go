package contracts

import "time"

type Config struct{ AccessTTL, RefreshTTL time.Duration }
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}
