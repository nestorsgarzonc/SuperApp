package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	dir := flag.String("out", "./keys", "directory to receive a new signing keypair")
	bits := flag.Int("bits", 3072, "RSA key size (minimum 2048)")
	flag.Parse()
	if *bits < 2048 {
		fatal("RSA key size must be at least 2048")
	}
	if err := os.MkdirAll(*dir, 0700); err != nil {
		fatal(err.Error())
	}
	priv := filepath.Join(*dir, "jwt-private.pem")
	pub := filepath.Join(*dir, "jwt-public.pem")
	if _, err := os.Stat(priv); err == nil {
		fatal("private key already exists; refusing to overwrite")
	}
	if _, err := os.Stat(pub); err == nil {
		fatal("public key already exists; refusing to overwrite")
	}
	key, err := rsa.GenerateKey(rand.Reader, *bits)
	if err != nil {
		fatal(err.Error())
	}
	if err = os.WriteFile(priv, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil {
		fatal(err.Error())
	}
	pubBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		fatal(err.Error())
	}
	if err = os.WriteFile(pub, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes}), 0644); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("Created signing keypair in %s\n", *dir)
}
func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
