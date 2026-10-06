package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

func NewRefreshToken() (raw, hash string, err error) {
	value := make([]byte, 32)
	if _, err = rand.Read(value); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(value)
	return raw, HashRefreshToken(raw), nil
}

func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
