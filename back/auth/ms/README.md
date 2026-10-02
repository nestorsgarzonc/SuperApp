# Auth MS implementation

Go 1.27.1 service using the standard HTTP server, `database/sql` with `pgx`, and the layered layout below.

- `cmd/auth/`: HTTP service entry point.
- `cmd/setup/`: transactionally applies ordered SQL migrations and idempotent seeds.
- `cmd/bootstrap-admin/`: one-time first-admin creation guarded by a PostgreSQL advisory lock and empty-user check.
- `cmd/keygen/`: generates a local RSA keypair without overwriting existing files.
- `internal/presentation/http/`: HTTP routing, request validation, auth context, permission gates.
- `internal/application/`: orchestration and repository/security/ID ports.
- `internal/domain/`: identity, role, permission, session shapes and domain errors.
- `internal/infrastructure/storage`: PostgreSQL repository and transactional refresh-token rotation.
- `internal/infrastructure/security/`: bcrypt and RS256/JWKS.
- `Makefile`, `Dockerfile`, `.env.example`: local commands and service packaging.

Run `go test ./...`, `go vet ./...` or `make build` from this directory. The PostgreSQL integration test applies the real migrations and seeds and verifies tenant RBAC, refresh rotation/replay, and logout-all against a dedicated disposable database. Set `AUTH_TEST_DATABASE_URL` to that database and run `make integration`; without it the integration test is skipped. Do not point this variable at a database with data you need to keep.
