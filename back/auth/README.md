# Auth MS — SuperApp

Microservicio nuevo de identidad y autorización en `back/auth/`, separado del monolito. Su código y base de datos son propios; no depende de las tablas ni del código de `backend/`.

## Alcance implementado

- Inicio de sesión con email/contraseña y creación de sesiones.
- Access tokens JWT RS256, refresh tokens aleatorios con hash almacenado y rotación de un solo uso.
- Logout de sesión y logout global; detección de reutilización de refresh tokens revocando la sesión.
- Gestión protegida de usuarios, estado de cuenta, roles y permisos.
- Roles globales y roles tenant vinculados a un conjunto por UUID opaco; asignaciones usuario–rol y rol–permiso.
- Consulta/evaluación de permisos efectivos desde PostgreSQL (sin claims RBAC stale en el access token).
- API HTTP en Go, esquema PostgreSQL, migraciones/seed idempotentes, keygen y bootstrap de primer administrador.

Fuera de alcance: frontend, auditoría, broker, notificaciones, servicios de negocio residencial y despliegue cloud completo.

## Stack

- Go 1.27.1, `net/http`, `database/sql` y driver `pgx`.
- PostgreSQL 17.
- bcrypt (costo 12) para credenciales; RSA RS256 para access tokens.
- Access token predeterminado de 15 minutos; refresh token de 30 días, configurable.
- SQL versionado y runner local transaccional en `ms/cmd/setup`; sin ORM.

## Estructura

- `ms/internal/presentation/httpapi/`: rutas, DTOs HTTP, autenticación y autorización.
- `ms/internal/application/`: casos de uso y puertos.
- `ms/internal/domain/`: entidades y errores.
- `ms/internal/infrastructure/`: PostgreSQL, seguridad, configuración e IDs.
- `ms/cmd/`: servidor, setup de DB, bootstrap de admin y generación de claves.
- `db/migrations/`, `db/seeds/`: esquema y catálogos de Auth.
- `docker-compose.yml`: PostgreSQL local exclusivo de Auth.
- `DESIGN.md`: modelo, decisiones y límites de integración.

## Inicio local

Desde `back/auth/`, crea `.secrets/auth_db_password` como archivo local con una contraseña aleatoria y arranca PostgreSQL con `docker compose up -d auth-db`. Compose lo monta como Docker Secret; el archivo está excluido de Git. Luego, desde `back/auth/ms/`:

```powershell
$env:AUTH_DATABASE_URL = 'postgres://auth_user:<password>@localhost:5434/auth_db?sslmode=disable'
go run ./cmd/setup
go run ./cmd/keygen -out ./keys
```

Para crear el primer usuario administrador (solo funciona con `auth_users` vacío), inyecta `AUTH_BOOTSTRAP_EMAIL` y `AUTH_BOOTSTRAP_PASSWORD` desde un gestor local de secretos y ejecuta `go run ./cmd/bootstrap-admin`. El proceso borra la variable de contraseña de su entorno al iniciar, crea el usuario con bcrypt y le asigna el rol base. Nunca guardes la clave privada ni credenciales en Git.

Completa las variables de `ms/.env.example` en el entorno de ejecución; el servicio no carga archivos `.env` automáticamente. Después ejecuta `go run ./cmd/auth` desde `back/auth/ms/`. Para despliegue en contenedor, el contexto de build es `back/auth/ms`.

El contenedor no incluye claves privadas ni ejecuta migraciones al arrancar. Monta la clave RSA como secreto de solo lectura, apunta `JWT_PRIVATE_KEY_FILE` a esa ruta y ejecuta el comando `setup` como paso separado del despliegue.

## Endpoints

Prefijo: `/api/v1`. Las operaciones administrativas requieren permisos persistidos.

| Método | Ruta | Función |
|---|---|---|
| POST | `/auth/login` | Validar credenciales y crear sesión |
| POST | `/auth/refresh` | Rotar refresh token y emitir access token |
| POST | `/auth/logout` | Revocar la sesión asociada al refresh token |
| POST | `/auth/logout-all` | Revocar todas las sesiones del usuario autenticado |
| GET | `/users/me` | Identidad propia |
| POST | `/users` | Crear usuario (permiso global `auth.users.manage`) |
| GET, PATCH, PUT | `/users/{id}` | Consultar/actualizar usuario y estado |
| GET, POST | `/roles` | Listar/crear rol global o tenant |
| GET, PATCH, PUT, DELETE | `/roles/{id}` | Consultar, actualizar o eliminar rol |
| POST, DELETE | `/roles/{id}/permissions[/{permissionId}]` | Asignar/quitar permiso de rol |
| GET, POST | `/permissions` | Listar/crear permisos |
| GET, PATCH, PUT, DELETE | `/permissions/{id}` | Consultar, actualizar o eliminar permiso |
| POST, DELETE | `/users/{userId}/roles[/{roleId}]` | Asignar/quitar rol (permiso global) |
| GET | `/users/{userId}/roles` | Consultar roles efectivos |
| GET | `/users/{userId}/permissions` | Consultar permisos efectivos |
| POST | `/auth/check-permission` | Evaluar permiso para identidad propia o usuario autorizado |
| GET | `/sessions`, DELETE `/sessions/{id}` | Listar/revocar sesión propia |
| GET | `/.well-known/jwks.json` | Clave pública vigente del emisor JWT |
| GET | `/health` | Liveness |

Para recursos tenant, el Gateway debe validar pertenencia del usuario al conjunto y firmar el contexto enviado a Auth con `X-Tenant-ID`, `X-Gateway-Timestamp` (Unix seconds) y `X-Gateway-Signature` (hex HMAC-SHA256 de `userId + LF + tenantId + LF + timestamp`). Auth verifica firma y ventana de ±60 s con `GATEWAY_TENANT_HMAC_SECRET`. El secreto debe compartirse solo entre Auth y Gateway. Sin contexto firmado, los recursos tenant se deniegan.

La creación/edición de usuarios y la asignación/revocación de roles a usuarios requieren permisos de alcance sistema. Auth no copia el padrón de miembros de cada conjunto; por eso un administrador tenant no puede añadir arbitrariamente IDs de usuarios a su tenant. El Gateway valida pertenencia antes de firmar contexto y los permisos tenant limitan la administración de roles a ese conjunto.

## Límites actuales

Este baseline emite JWT y ofrece JWKS para validadores, pero **no implementa un proveedor OIDC/OAuth 2.0 completo**: no hay authorization code + PKCE, registro de clientes OAuth, consentimientos ni discovery OIDC. La forma de integración con API Gateway debe acordarse antes de presentar Auth como Identity Provider OIDC. Las requests directas a Auth deben quedar en red privada detrás de un Gateway que aplique rate limiting; no se agregó un limitador distribuido propio.

Los access tokens no se invalidan instantáneamente al cerrar sesión o desactivar una cuenta: vencen en hasta 15 minutos. Las operaciones RBAC consultan PostgreSQL en cada autorización; el Gateway debe usar la clave JWKS para validar autenticidad y llamar Auth cuando requiera una decisión RBAC actualizada.
