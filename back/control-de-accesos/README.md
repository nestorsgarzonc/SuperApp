# control-de-accesos

Microservicio de **gestión de acceso** del residencial: códigos QR,
pre-autorizaciones y validación de visitantes en portería. Extraído del
monolito `residential-superapp` — se dejó **fuera** todo lo de citofonía /
llamadas VoIP (`intercom`, `call_logs`, LiveKit, notificaciones push), que
pertenece a otro feature.

Ver [`../../docs/architecture.md`](../../docs/architecture.md) para cómo
encaja en el resto de SuperApp.

## Estructura

```
control-de-accesos/
├── ms/     # microservicio Go (chi + database/sql)
├── db/     # migraciones y seed, propias de este feature (sin FKs a otros servicios)
└── docker-compose.yml
```

`ms/internal/directory` es una réplica local mínima de "qué usuario pertenece
a qué unidad/complejo" — en el monolito eso vivía en `tenancy`/`identity`;
aquí el propio microservicio guarda solo lo que necesita para sus chequeos de
acceso (en producción se sincronizaría vía eventos desde el servicio de
identidad; para desarrollo local se siembra directo en `db/migrations/002_seed.sql`).

Tampoco hay microservicio de `notificaciones` todavía. `ms/internal/notifier`
es el extremo saliente de la flecha "evento de acceso" del diagrama de
`architecture.md`: cuando se crea un QR, una pre-autorización, un grant
genérico, se valida un acceso o se cancela un grant, el servicio intenta un
`POST` *best-effort* a `NOTIFICATIONS_URL` con el evento. Si esa variable está
vacía (el valor por defecto) o el POST falla, no pasa nada — se loguea y la
operación de acceso sigue normal. No se implementó nada del lado de
`notificaciones`; esto es solo el gancho para cuando exista.

No hay microservicio de `auth` todavía, así que este servicio expone un
endpoint de solo-desarrollo (`/api/v1/dev/token`) que firma un JWT válido para
el usuario demo sembrado — mismo formato de token (HS256, mismo secreto) que
usaría el futuro `back/auth`. En producción ese endpoint no se monta
(solo cuando `ENV=development`) y los tokens los emitiría `back/auth`.

## Levantarlo en local

Requiere Docker Desktop (no hace falta tener Go instalado).

```bash
cd SuperApp/back/control-de-accesos
docker compose up --build
```

Esto levanta:
- `db` — Postgres en `localhost:5433`, con el esquema y datos demo ya cargados.
- `ms` — la API en `localhost:8081`.

Verificar que está vivo:

```bash
curl http://localhost:8081/healthz
```

### 1. Obtener un token (solo dev)

```bash
curl http://localhost:8081/api/v1/dev/token
```

Devuelve `token`, `complex_id` y `unit_id` del usuario demo (residente de la
unidad `A-101`). Guárdalo en una variable:

```bash
TOKEN=$(curl -s http://localhost:8081/api/v1/dev/token | python3 -c "import sys,json;print(json.load(sys.stdin)['token'])")
COMPLEX_ID=00000000-0000-0000-0000-000000000001
UNIT_ID=10000000-0000-0000-0000-000000000101
```

### 2. Listar los códigos QR sembrados

```bash
curl -s "http://localhost:8081/api/v1/access/qr-codes?complex_id=$COMPLEX_ID" \
  -H "Authorization: Bearer $TOKEN" | python3 -m json.tool
```

### 3. Crear un código QR nuevo

```bash
curl -s -X POST http://localhost:8081/api/v1/access/qr-codes \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Complex-ID: $COMPLEX_ID" \
  -H "Content-Type: application/json" \
  -d '{"unit_id":"'"$UNIT_ID"'","visitor_name":"Maria Lopez","valid_until":"2026-12-31T23:59:59Z"}' \
  | python3 -m json.tool
```

La respuesta incluye `qr_code_url` con la imagen del QR en base64
(`data:image/png;base64,...`).

### 4. Crear una pre-autorización

```bash
curl -s -X POST http://localhost:8081/api/v1/access/pre-auth \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Complex-ID: $COMPLEX_ID" \
  -H "Content-Type: application/json" \
  -d '{"unit_id":"'"$UNIT_ID"'","visitor_name":"Rappi Delivery 2","visitor_id_number":"COL999999","visitor_type":"DELIVERY","valid_from":"2026-01-01T00:00:00Z","valid_until":"2026-12-31T23:59:59Z"}' \
  | python3 -m json.tool
```

### 5. Validar un QR en portería (el guardia escanea)

El seed incluye un QR activo con hash fijo `demo-qr-hash-0001`:

```bash
curl -s -X POST http://localhost:8081/api/v1/access-grants/validate \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Complex-ID: $COMPLEX_ID" \
  -H "Content-Type: application/json" \
  -d '{"method":"QR","qr_code":"demo-qr-hash-0001"}' \
  | python3 -m json.tool
```

### 6. Validar una pre-autorización por cédula

```bash
curl -s -X POST http://localhost:8081/api/v1/access-grants/validate \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Complex-ID: $COMPLEX_ID" \
  -H "Content-Type: application/json" \
  -d '{"method":"PRE_AUTH","visitor_id_number":"COL123456"}' \
  | python3 -m json.tool
```

### 7. Cancelar un grant

```bash
curl -s -X PATCH "http://localhost:8081/api/v1/access-grants/20000000-0000-0000-0000-000000000002/cancel" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Complex-ID: $COMPLEX_ID" | python3 -m json.tool
```

### 8. Grants de una unidad (ruta unit-scoped)

```bash
curl -s "http://localhost:8081/api/v1/units/$UNIT_ID/access-grants" \
  -H "Authorization: Bearer $TOKEN" | python3 -m json.tool
```

## Apagar / limpiar

```bash
docker compose down        # detiene los contenedores
docker compose down -v     # además borra los datos (vuelve a sembrar desde cero)
```

## Producción / AWS RDS

La idea es que este microservicio, cuando se despliegue, guarde sus datos en
**RDS (Postgres)** en vez del Postgres de `docker compose`. No hay nada de
Terraform/IaC aquí — eso es un paso aparte — pero la app ya está lista para
ese cambio con solo variables de entorno, sin tocar código:

- `DB_HOST` / `DB_PORT` → endpoint de la instancia RDS.
- `DB_SSLMODE=require` (mínimo) o `verify-full` + `DB_SSLROOTCERT=/ruta/al/global-bundle.pem`
  (el bundle de CA de RDS) — nunca `disable` contra RDS.
- `DB_PASSWORD` / `JWT_SECRET` deben venir de Secrets Manager / SSM Parameter
  Store en el momento del despliegue, no hardcodeados como en el
  `docker-compose.yml` local.
- `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` / `DB_CONN_MAX_LIFETIME_MINUTES` —
  ajustar según el `max_connections` de la clase de instancia RDS elegida.
  El valor de `CONN_MAX_LIFETIME` por defecto (5 min) existe para que el pool
  recicle conexiones periódicamente y así sobreviva un failover de RDS.

Localmente, `docker-entrypoint-initdb.d` aplica `db/migrations/*.sql`
automáticamente la primera vez que arranca el volumen — RDS no tiene ese
hook. Para aplicar el esquema ahí (o para repetirlo tras un cambio), corre:

```bash
DB_HOST=<endpoint-rds> DB_PORT=5432 DB_USER=... DB_PASSWORD=... DB_NAME=... \
DB_SSLMODE=require ./db/migrate.sh
```

El script usa `psql` y aplica cada archivo de `db/migrations/` en orden; es
idempotente (`CREATE TABLE IF NOT EXISTS`, `ON CONFLICT DO NOTHING`), así que
correrlo de nuevo no rompe nada.

## Endpoints

| Método | Ruta | Descripción |
|---|---|---|
| POST | `/api/v1/access/qr-codes` | Crear QR |
| POST | `/api/v1/access/qr-codes/batch` | Crear varios QR |
| GET | `/api/v1/access/qr-codes` | Listar QR del complejo |
| POST | `/api/v1/access/pre-auth` | Crear pre-autorización |
| GET | `/api/v1/access/pre-auth` | Listar pre-autorizaciones |
| POST | `/api/v1/access-grants` | Crear grant genérico (QR o PRE_AUTH) |
| GET | `/api/v1/access-grants/{id}` | Obtener un grant |
| PATCH | `/api/v1/access-grants/{id}/cancel` | Cancelar un grant |
| POST | `/api/v1/access-grants/validate` | Validar QR/PRE_AUTH (uso del guardia) |
| GET | `/api/v1/units/{unitId}/access-grants` | Grants de una unidad |
| GET | `/api/v1/dev/token` | *(solo dev)* token demo |
| GET | `/healthz` | Health check |

## Eventos salientes (hacia `notificaciones`, cuando exista)

`ms/internal/notifier` dispara un `POST` a `NOTIFICATIONS_URL` (JSON, best-effort) en estos momentos:

| Evento | Se dispara en |
|---|---|
| `access.qr_created` | `CreateQRCode`, `CreateQRBatch` |
| `access.pre_auth_created` | `CreatePreAuth` |
| `access.grant_created` | `CreateAccessGrant` |
| `access.grant_validated` | `ValidateAccessGrant` (cuando el resultado es válido) |
| `access.grant_cancelled` | `CancelAccessGrant` |

Payload: `{type, complex_id, unit_id, grant_id, visitor_name, method}`.

## Lo que se dejó fuera a propósito

Todo lo de **llamadas** (intercom/citofonía) del feature `access` original:
`StartCall`, `AnswerCall`, `AcceptIncomingCall`, `DeclineIncomingCall`,
`EndCall`, `FallbackGSM`, logs e stats de llamadas, LiveKit y notificaciones
push — eso es otro feature y no se tocó en este backend.
