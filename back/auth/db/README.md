# Base de datos propia de Auth

PostgreSQL 17 es exclusivo de Auth (`database-per-service`); no se importan esquemas ni migraciones del monolito.

- `migrations/`: SQL versionado con par `.up.sql`/`.down.sql`.
- `seeds/`: permisos base y rol técnico de bootstrap; no contiene usuarios, contraseñas ni tokens.
- `dev-seeds/`: usuarios y roles demo (mismas personas que control-de-accesos), solo desarrollo; no los aplica `cmd/setup`. Cárgalos con `psql "$AUTH_DATABASE_URL" -f db/dev-seeds/001_demo.sql` (contraseña `Demo-Pass-2026!`).
- `docker-compose.yml` en el directorio padre: instancia PostgreSQL local separada.

Ejecuta `go run ./cmd/setup` desde `back/auth/ms/` con `AUTH_DATABASE_URL` configurada. El comando toma un advisory lock, registra migraciones/seeds aplicados y ejecuta cada archivo en una transacción. El UUID de tenant es una referencia opaca sin FK a otros servicios. El alcance tenant de los roles se valida tanto en la persistencia como en la autorización.
