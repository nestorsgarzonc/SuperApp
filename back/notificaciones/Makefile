DB_URL    := postgres://notificaciones:notificaciones@localhost:5433/notificaciones?sslmode=disable

.PHONY: up migrate down run test test-integration image ship

up: ## start db
	docker compose up -d --wait db

migrate: up ## apply db/migrations
	docker compose --profile tools run --rm migrate

down: ## stop and delete local containers
	docker compose down

run: migrate ## run the service against the local db
	cd ms && ENV=development DATABASE_URL='$(DB_URL)' go run ./cmd/server

test: ## unit tests (no infrastructure needed)
	cd ms && go test -race ./...

test-integration: migrate ## SQL against a real database
	cd ms && TEST_DATABASE_URL='$(DB_URL)' \
		go test -race -count=1 -tags integration ./internal/integration/...

IMAGE    ?=
PLATFORM ?= linux/amd64

image: ## build + push the production image: make image IMAGE=<registry>/notificaciones:<tag> [PLATFORM=linux/arm64]
	@test -n "$(IMAGE)" || (echo "IMAGE is required" && exit 1)
	docker buildx build --platform $(PLATFORM) -t $(IMAGE) --push ms

HOST ?=
KEY  ?=
SSH  := ssh $(if $(KEY),-i $(KEY))

ship: ## build here, load the image on the EC2 and copy the deploy files: make ship HOST=ec2-user@<ip> KEY=~/.ssh/<key>.pem [PLATFORM=linux/arm64]
	@test -n "$(HOST)" || (echo "HOST is required" && exit 1)
	docker buildx build --platform $(PLATFORM) -t notificaciones:latest --load ms
	docker save notificaciones:latest | gzip | $(SSH) $(HOST) 'gunzip | docker load'
	$(SSH) $(HOST) 'mkdir -p ~/notificaciones/secrets'
	rsync -av -e "$(SSH)" docker-compose.prod.yml .env.prod.example db $(HOST):~/notificaciones/
