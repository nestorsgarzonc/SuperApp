# control-de-accesos

Microservicio de **gestión de acceso** del residencial: códigos QR,
pre-autorizaciones y validación de visitantes en portería. Extraído del
monolito `residential-superapp`.

Ver [`architecture.md`](architecture.md) para cómo encaja en el resto de
SuperApp. Código en [`back/control-de-accesos/`](../back/control-de-accesos/).

## Estructura

```
back/control-de-accesos/
├── ms/     # microservicio Go (chi + database/sql)
├── db/     # migraciones y seed, propias de este feature (sin FKs a otros servicios)
├── test/   # smoke test + mock de notificaciones
└── docker-compose.yml
```

`ms/internal/directory`: réplica local de "qué usuario pertenece a qué
unidad/complejo" (en el monolito vivía en `tenancy`/`identity`). Sembrada en
`db/migrations/002_seed.sql`.

`ms/internal/notifier`: `POST` best-effort a `NOTIFICATIONS_URL` en cada
creación/validación/cancelación de grant. Vacío o con error → no-op, no
bloquea la operación. *(Esquema de eventos — nombres, `eventId`/`correlationId`,
retry+DLQ — pendiente de rediseño, ver sección "Pendiente".)*

`ms/internal/audit`: tabla `audit_log` append-only (triggers `BEFORE UPDATE/DELETE`
que rechazan la operación a nivel de base de datos, no solo por disciplina de
código). Se escribe un registro en cada creación, cancelación y validación
(exitosa o denegada) de un grant, y en cada intento cross-tenant.

`/api/v1/dev/token` (solo `ENV=development`): firma un JWT demo con el mismo
formato (HS256, mismo secreto) que emitiría `back/auth`. Acepta `?role=GUARD`
(o `ADMIN`) para probar endpoints restringidos por rol — por defecto emite
`RESIDENT`.

## Reglas de negocio

- **Solo `GUARD`/`ADMIN` pueden validar** (`POST /access-grants/validate`) — un
  `RESIDENT` recibe `403`.
- **Todo grant es de un solo uso**: se marca `USED` atómicamente en la primera
  validación exitosa. Validar el mismo QR/PRE_AUTH una segunda vez devuelve
  `valid:false` ("Access grant has already been used").
- **QR tiene tope de 12h desde su creación**, sin importar qué `valid_until`
  pida el residente (gana lo que ocurra primero). `PRE_AUTH` no tiene este tope.
- **Fechas**: `valid_until` no puede estar en el pasado, y si se envía
  `valid_from`, debe ser anterior a `valid_until`. Si no, `422`.
- **Longitudes máximas**: `visitor_name` 200, `visitor_id_number` 50,
  `details` 1000 caracteres. Si no, `422`.
- **conjuntoId siempre viene del JWT**, nunca de `?complex_id=` ni de
  `X-Complex-ID` — un valor en el query/header se ignora por completo.

## Levantarlo en local

Requiere Docker Desktop (no hace falta tener Go instalado).

```bash
cd back/control-de-accesos
cp .env.example .env   # primera vez: JWT_SECRET y DB_PASSWORD, nunca committeado
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
unidad `A-101`). Guárdalo en una variable. Para `/access-grants/validate`
(paso 5 y 6) hace falta además un token con rol `GUARD`:

```bash
TOKEN=$(curl -s http://localhost:8081/api/v1/dev/token | python3 -c "import sys,json;print(json.load(sys.stdin)['token'])")
GUARD_TOKEN=$(curl -s "http://localhost:8081/api/v1/dev/token?role=GUARD" | python3 -c "import sys,json;print(json.load(sys.stdin)['token'])")
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

Requiere un token con rol `GUARD`/`ADMIN` (un `RESIDENT` recibe `403`). El
seed incluye un QR activo con hash fijo `demo-qr-hash-0001` — **se consume**
en la primera validación exitosa (un solo uso); validarlo de nuevo después
devuelve `valid:false`:

```bash
curl -s -X POST http://localhost:8081/api/v1/access-grants/validate \
  -H "Authorization: Bearer $GUARD_TOKEN" \
  -H "X-Complex-ID: $COMPLEX_ID" \
  -H "Content-Type: application/json" \
  -d '{"method":"QR","qr_code":"demo-qr-hash-0001"}' \
  | python3 -m json.tool
```

### 6. Validar una pre-autorización por cédula

```bash
curl -s -X POST http://localhost:8081/api/v1/access-grants/validate \
  -H "Authorization: Bearer $GUARD_TOKEN" \
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

## Pruebas

```bash
cd back/control-de-accesos
BASE_URL=http://localhost:8080 ./test/smoke_test.sh
```

39 checks sobre los 10 endpoints: happy path, JSON malformado, campos
faltantes, enums inválidos, fechas inválidas/longitud excedida, recurso
inexistente, UUID inválido, método no permitido, auth faltante, rol
incorrecto (`RESIDENT` contra `/validate` → 403), aislamiento de tenant
(`?complex_id=` ajeno se ignora) y consumo de un solo uso (mismo QR
validado dos veces). Necesita una base recién migrada/sembrada (usa IDs
del seed, cancela un grant y consume el QR semilla).

Para ver los eventos que dispara `internal/notifier` (sin montar el
microservicio de notificaciones real):

```bash
python3 test/mock_notifications.py 9000
# en otra terminal, con el server apuntando ahí:
NOTIFICATIONS_URL=http://localhost:9000/events go run ./cmd/api
```

## AWS RDS

- `DB_HOST` / `DB_PORT` → endpoint de la instancia RDS.
- `DB_SSLMODE=require` (o `verify-full` + `DB_SSLROOTCERT=/ruta/al/global-bundle.pem`). Nunca `disable`.
- `DB_PASSWORD` / `JWT_SECRET` → Secrets Manager / SSM Parameter Store.
- `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` / `DB_CONN_MAX_LIFETIME_MINUTES` → según el `max_connections` de la instancia.

Aplicar el esquema (sin el hook de `docker-entrypoint-initdb.d`):

```bash
DB_HOST=<endpoint-rds> DB_PORT=5432 DB_USER=... DB_PASSWORD=... DB_NAME=... \
DB_SSLMODE=require ./db/migrate.sh
```

## Endpoints

| Método | Ruta | Rol | Descripción |
|---|---|---|---|
| POST | `/api/v1/access/qr-codes` | autenticado + dueño de la unidad | Crear QR (tope 12h) |
| POST | `/api/v1/access/qr-codes/batch` | autenticado + dueño de la unidad | Crear varios QR (tope 12h c/u) |
| GET | `/api/v1/access/qr-codes` | autenticado | Listar QR del complejo (del token) |
| POST | `/api/v1/access/pre-auth` | autenticado + dueño de la unidad | Crear pre-autorización |
| GET | `/api/v1/access/pre-auth` | autenticado | Listar pre-autorizaciones |
| POST | `/api/v1/access-grants` | autenticado + dueño de la unidad | Crear grant genérico (QR o PRE_AUTH) |
| GET | `/api/v1/access-grants/{id}` | autenticado + dueño de la unidad | Obtener un grant |
| PATCH | `/api/v1/access-grants/{id}/cancel` | autenticado + dueño de la unidad | Cancelar un grant |
| POST | `/api/v1/access-grants/validate` | **`GUARD`/`ADMIN` únicamente** | Validar QR/PRE_AUTH (consume el grant) |
| GET | `/api/v1/units/{unitId}/access-grants` | autenticado + miembro de la unidad | Grants de una unidad |
| GET | `/api/v1/dev/token` | ninguno | *(solo dev)* token demo, `?role=` opcional |
| GET | `/healthz` | ninguno | Health check |

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

## Pendiente

- Nombres/forma del evento no son `AccessGranted`/`AccessDenied`/`VisitorEntered`/`VisitorExited`
  ni llevan `eventId`/`correlationId`/`schemaVersion`; tampoco hay retry/DLQ —
  rediseño de `internal/notifier` pendiente para una sesión aparte, ya que
  cambia el contrato que `notificaciones` tendría que consumir.
