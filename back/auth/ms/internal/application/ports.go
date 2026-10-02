package application

import (
	contracts "superapp/auth/internal/application/contracts"
	ports "superapp/auth/internal/ports/repository"
)

type Config = contracts.Config
type TokenPair = contracts.TokenPair
type TokenClaims = contracts.TokenClaims
type TokenIssuer = contracts.TokenIssuer
type PasswordHasher = contracts.PasswordHasher
type IDGenerator = contracts.IDGenerator
type Repository = ports.Repository
