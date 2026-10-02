DB_URL    := postgres://notificaciones:notificaciones@localhost:5433/notificaciones?sslmode=disable
REDIS_URL := redis://localhost:6380/0

.PHONY: up migrate down run test test-integration

up: ## start db + redis
	docker compose up -d --wait db redis

migrate: up ## apply db/migrations
	docker compose --profile tools run --rm migrate

down: ## stop and delete local containers
	docker compose down

run: migrate ## run the service against the local db + redis
	cd ms && ENV=development DATABASE_URL='$(DB_URL)' REDIS_URL='$(REDIS_URL)' go run ./cmd/server

test: ## unit tests (no infrastructure needed)
	cd ms && go test -race ./...

test-integration: migrate ## SQL + Redis consumer against real infrastructure
	cd ms && TEST_DATABASE_URL='$(DB_URL)' TEST_REDIS_URL='$(REDIS_URL)' \
		go test -race -count=1 -tags integration ./internal/integration/...
