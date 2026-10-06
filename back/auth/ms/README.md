# Auth MS implementation

Go 1.27.1 service using the standard HTTP server, `database/sql` with `pgx`, and the layered layout below.

- `cmd/main.go`, `cmd/config.go`: HTTP service entry point and process configuration.
- `cmd/setup/`: transactionally applies ordered SQL migrations and idempotent seeds.
- `cmd/bootstrap-admin/`: one-time first-admin creation guarded by a PostgreSQL advisory lock and empty-user check.
- `cmd/keygen/`: generates a local RSA keypair without overwriting existing files.
- `internal/api.go`: HTTP API composition.
- `internal/presentation/reader/` and `internal/presentation/writer/`: query and command routes/handlers.
- `internal/presentation/dto/`: HTTP representations grouped by feature.
- `internal/core/usecases/`: application use cases grouped by domain responsibility.
- `internal/core/domain/`: user, session, role, permission models and domain errors.
- `internal/core/ports/`: repository and security contracts owned by the core.
- `internal/infrastructure/repository/postgres/`: PostgreSQL repository adapters.
- `internal/infrastructure/persistence/postgres/`: PostgreSQL connection pool setup.
- `internal/infrastructure/security/`: password hashing and JWT/JWKS adapters.
- `Makefile`, `Dockerfile`, `.env.example`: local commands and service packaging.

Run `go test ./...`, `go vet ./...` or `make build` from this directory. The PostgreSQL integration test applies the real migrations and seeds and verifies tenant RBAC, refresh rotation/replay, and logout-all against a dedicated disposable database. Set `AUTH_TEST_DATABASE_URL` to that database and run `make integration`; without it the integration test is skipped. Do not point this variable at a database with data you need to keep.
