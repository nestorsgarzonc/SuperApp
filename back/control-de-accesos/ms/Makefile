SHELL := /bin/sh

.PHONY: test vet build setup run
test:
	go test ./...
vet:
	go vet ./...
build:
	go build ./cmd/access ./cmd/setup
setup:
	go run ./cmd/setup
run:
	go run ./cmd/access
