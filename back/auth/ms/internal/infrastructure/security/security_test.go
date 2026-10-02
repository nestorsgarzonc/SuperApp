package security

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPasswordHashAndVerify(t *testing.T) {
	p := Passwords{}
	hash, err := p.Hash("a-long-password-for-auth")
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Verify(hash, "a-long-password-for-auth"); err != nil {
		t.Fatalf("valid password rejected: %v", err)
	}
	if err = p.Verify(hash, "wrong-password"); err == nil {
		t.Fatal("wrong password accepted")
	}
	if len(hash) < 50 {
		t.Fatalf("unexpected password hash format: %q", hash)
	}
}

func TestRS256TokensAndJWKS(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "private.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	tokens, err := LoadTokens(path, "https://auth.example.test", "superapp-api", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := tokens.Issue("user-1", "user@example.test", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := tokens.Verify(raw)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "user-1" || claims.SessionID != "session-1" {
		t.Fatalf("unexpected claims: %#v", claims)
	}
	if got := tokens.JWKS()["keys"].([]jwk); len(got) != 1 || got[0].Kid == "" {
		t.Fatalf("invalid JWKS: %#v", tokens.JWKS())
	}
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	otherPath := filepath.Join(t.TempDir(), "other.pem")
	if err = os.WriteFile(otherPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(otherKey)}), 0600); err != nil {
		t.Fatal(err)
	}
	other, err := LoadTokens(otherPath, "https://auth.example.test", "superapp-api", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = other.Verify(raw); err == nil {
		t.Fatal("token signed by another key was accepted")
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "private.pem")
	_ = os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600)
	tokens, err := LoadTokens(path, "issuer", "audience", -time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := tokens.Issue("u", "e", "s")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tokens.Verify(raw); err == nil {
		t.Fatal("expired token accepted")
	}
}
