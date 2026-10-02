SHELL := /bin/sh

.PHONY: test integration vet build setup keygen run
test:
	go test ./...
integration:
	go test ./internal/infrastructure/postgres -run TestRepositoryPostgresIntegration -v
vet:
	go vet ./...
build:
	go build ./cmd/auth ./cmd/setup ./cmd/bootstrap-admin ./cmd/keygen
setup:
	go run ./cmd/setup
keygen:
	go run ./cmd/keygen -out ./keys
run:
	go run ./cmd/auth
