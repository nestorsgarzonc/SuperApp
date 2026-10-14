DB_URL    := postgres://notificaciones:notificaciones@localhost:5433/notificaciones?sslmode=disable

# Push local si existe secrets/firebase.json (gitignored); si no, solo bandeja.
FIREBASE_ENV := $(if $(wildcard secrets/firebase.json),FIREBASE_CREDENTIALS_FILE=$(CURDIR)/secrets/firebase.json FIREBASE_PROJECT_ID=$(shell python3 -c "import json;print(json.load(open('secrets/firebase.json'))['project_id'])"))

.PHONY: up migrate down run test test-integration image ship

up: ## start db
	docker compose up -d --wait db

migrate: up ## apply db/migrations
	docker compose --profile tools run --rm migrate

down: ## stop and delete local containers
	docker compose down

run: migrate ## run the service against the local db
	cd ms && ENV=development DATABASE_URL='$(DB_URL)' $(FIREBASE_ENV) go run ./cmd/server

test: ## unit tests (no infrastructure needed)
	cd ms && go test -race ./...

test-integration: migrate ## SQL against a real database
	cd ms && TEST_DATABASE_URL='$(DB_URL)' \
		go test -race -count=1 -tags integration ./internal/integration/...

IMAGE     ?=
PLATFORM  ?= linux/amd64
PLATFORMS ?= linux/amd64,linux/arm64
TAG       ?= $(shell git rev-parse --short HEAD)$(shell git diff --quiet HEAD -- ms || echo -dirty)

image: ## build (amd64 + arm64) and push: make image IMAGE=<usuario>/notificaciones  → :<commit> y :latest
	@test -n "$(IMAGE)" || (echo "IMAGE is required (p. ej. nestorsgarzonc/superapp-notificaciones)" && exit 1)
	docker buildx build --platform $(PLATFORMS) -t $(IMAGE):$(TAG) -t $(IMAGE):latest --push ms

HOST ?=
KEY  ?=
SSH  := ssh $(if $(KEY),-i $(KEY))

ship: ## build here, load the image on the EC2 and copy the deploy files: make ship HOST=ec2-user@<ip> KEY=~/.ssh/<key>.pem [PLATFORM=linux/arm64]
	@test -n "$(HOST)" || (echo "HOST is required" && exit 1)
	docker buildx build --platform $(PLATFORM) -t notificaciones:latest --load ms
	docker save notificaciones:latest | gzip | $(SSH) $(HOST) 'gunzip | docker load'
	$(SSH) $(HOST) 'mkdir -p ~/notificaciones/secrets'
	rsync -av -e "$(SSH)" docker-compose.prod.yml .env.prod.example db $(HOST):~/notificaciones/
