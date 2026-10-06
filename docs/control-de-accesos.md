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
├── db/     # propias de este feature (sin FKs a otros servicios)
│   ├── migrations/   # SOLO esquema: se aplican a cualquier base (RDS incluida)
│   └── seeds/        # datos de demostración, solo desarrollo (nunca en producción)
├── test/   # smoke, aislamiento, residentes, breaker, endurecimiento + reset de datos
└── docker-compose.yml
```

`ms/internal/directory`: réplica local de "quién pertenece a qué conjunto"
(en el monolito vivía en `tenancy`/`identity`). Un **residente** pertenece a un
conjunto por ser miembro de una unidad (`unit_members`); un **guardia/admin**
por estar en `complex_staff` con ese rol *para ese conjunto*. Una misma persona
puede tener cualquier combinación en conjuntos distintos (p. ej. residente en
el 1 y guardia en el 2). Los datos de demostración están en `db/seeds/001_demo.sql`
(2 conjuntos, 7 personas, ver más abajo).

`ms/internal/accesslog`: bitácora de portería (ver *Reglas de negocio*).

`ms/internal/notifier`: publica en el stream de Redis `notifications.requested` (contrato de
`notificaciones`) al crear/validar/cancelar un grant, dirigido a los residentes de la unidad.
Sin `REDIS_URL` o con error de Redis → no-op, no bloquea la operación.

`ms/internal/audit`: tabla `audit_log` append-only (triggers `BEFORE UPDATE/DELETE`
que rechazan la operación a nivel de base de datos, no solo por disciplina de
código). Se escribe un registro en cada creación, cancelación y validación
(exitosa o denegada) de un grant, y en cada intento cross-tenant.

`ms/internal/devtools`, sustituto de `back/auth` mientras no esté integrado — nada fijo en el
código, todo sale de la base. **Solo existe si el binario se compila con `-tags dev` *y* se arranca con
`ENV=development`** (docker compose ya compila con `GO_TAGS=dev`); un binario normal no tiene rutas `/dev/*`:

- `GET /api/v1/dev/users` → personas con sus membresías (conjunto, rol, unidad).
- `GET /api/v1/dev/token?user_id=&complex_id=[&role=]` → JWT (HS256) de esa
  persona **en ese conjunto**. El rol y la unidad los deriva el servidor de la
  pertenencia real: pedir un conjunto al que no pertenece (o un rol que no tiene
  allí) devuelve 403. `role` solo desambigua si tiene varios en el mismo conjunto.

### Personas de prueba (`db/seeds/001_demo.sql`)

| Persona | Conjunto 1 · Los Pinos | Conjunto 2 · Alto del Bosque |
|---|---|---|
| Ana García | residente A-101 | — |
| Pedro Gómez | — | residente B-201 |
| Luis Mora | guardia | — |
| Marta Ruiz | — | guardia |
| Carlos Rojas | administrador | — |
| Sofía Díaz | residente A-102 | residente B-202 |
| Andrés Vega | residente A-102 | **guardia** |

La cédula `COL555555` tiene una pre-autorización en *cada* conjunto, a propósito.

### Aislamiento entre conjuntos

- El conjunto sale del **token**; un `X-Complex-ID`, `?complex_id=` o `complex_id`
  del body distintos se ignoran.
- `/access-grants/validate` y `/access-logs` exigen que el rol del token coincida
  con una fila de `complex_staff` **de ese conjunto** (se consulta la base en cada
  petición; un token con `role=GUARD` de alguien que no es guardia allí → 403).
- Un QR o una cédula de otro conjunto devuelve `valid:false` y **no se consume**.

## Reglas de negocio

- **Bitácora de portería (`access-logs`)**: registra lo que *ocurrió* en la puerta
  (el grant dice quién *puede* entrar). `ENTRY`/`EXIT` son siempre `AUTHORIZED`;
  un `ATTEMPT` siempre `DENIED`. `occurred_at` es opcional (registro offline) pero
  no puede estar en el futuro ni tener más de 7 días. `client_id` lo genera el
  cliente: reenviar el mismo no duplica. `grant_id` solo se conserva si el grant es
  del mismo complejo. Cada registro nuevo deja una entrada en `audit_log`.

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
curl http://localhost:8081/api/v1/dev/users          # personas y conjuntos
```

El token se pide por persona + conjunto. Para `/access-grants/validate`
(paso 5 y 6) hace falta un guardia del conjunto:

```bash
COMPLEX_ID=00000000-0000-0000-0000-000000000001      # Los Pinos
UNIT_ID=10000000-0000-0000-0000-000000000101         # A-101
tok() { curl -s "http://localhost:8081/api/v1/dev/token?user_id=$1&complex_id=$COMPLEX_ID" | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['token'])"; }
TOKEN=$(tok 10000000-0000-0000-0000-000000000001)        # Ana García, residente
GUARD_TOKEN=$(tok 10000000-0000-0000-0000-000000000003)  # Luis Mora, guardia
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

41 checks sobre los 10 endpoints: happy path, JSON malformado, campos
faltantes, enums inválidos, fechas inválidas/longitud excedida, recurso
inexistente, UUID inválido, método no permitido, auth faltante, rol
incorrecto (`RESIDENT` contra `/validate` → 403), aislamiento de tenant
(`?complex_id=` ajeno se ignora) y consumo de un solo uso (mismo QR
validado dos veces). Necesita una base recién migrada/sembrada (usa IDs
del seed, cancela un grant y consume el QR semilla).

Aislamiento entre conjuntos (68 checks: dos conjuntos, guardias, residentes,
una persona en ambos conjuntos, tokens manipulados, bitácora). Reiniciar los
datos de prueba antes de cada corrida (consume pases de un solo uso):

```bash
JWT_SECRET=<el del ms> node test/isolation.mjs                      # BASE=http://localhost:8081/api/v1 por defecto
cd ms && go test ./...                       # validaciones de accesslog + middleware de personal
```

Para probar las notificaciones contra el microservicio real (`back/notificaciones`, Redis en `localhost:6380`):

```bash
cd ../notificaciones && docker compose up -d db redis && docker compose --profile tools run --rm migrate && docker compose up -d ms
REDIS_URL=redis://localhost:6380/0 go run ./cmd/api   # desde ms/
# la bandeja del residente: GET localhost:8083/users/me/notifications (headers X-User-ID y X-Complex-ID)
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
| POST | `/api/v1/access/qr-codes/batch` | autenticado + dueño de la unidad | Crear varios QR (tope 12h c/u). **Atómico**: todos o ninguno |
| GET | `/api/v1/access/qr-codes` | autenticado | Mis QR: **solo los de las unidades a las que pertenezco** (el código es una credencial) |
| POST | `/api/v1/access/pre-auth` | autenticado + dueño de la unidad | Crear pre-autorización |
| GET | `/api/v1/access/pre-auth` | autenticado | Mis pre-autorizaciones (solo de mis unidades) |
| GET | `/api/v1/access/summary` | **`GUARD`/`ADMIN`** | Conteos de pases del conjunto (total, activos, QR, pre-auth). Nunca códigos ni documentos |
| POST | `/api/v1/access-grants` | autenticado + dueño de la unidad | Crear grant genérico (QR o PRE_AUTH) |
| GET | `/api/v1/access-grants/{id}` | autenticado + dueño de la unidad | Obtener un grant |
| PATCH | `/api/v1/access-grants/{id}/cancel` | autenticado + dueño de la unidad | Cancelar un grant |
| POST | `/api/v1/access-grants/validate` | **`GUARD`/`ADMIN` únicamente** | Validar QR/PRE_AUTH (consume el grant) |
| GET | `/api/v1/units/{unitId}/access-grants` | autenticado + miembro de la unidad | Grants de una unidad |
| POST | `/api/v1/access-logs` | **`GUARD`/`ADMIN` únicamente** | Registrar ingreso (`ENTRY`), salida (`EXIT`) o intento denegado (`ATTEMPT`). Idempotente por `client_id` (201 nuevo / 200 repetido) |
| GET | `/api/v1/access-logs` | **`GUARD`/`ADMIN` únicamente** | Bitácora del complejo. Filtros: `from`, `to` (RFC3339), `movement`, `result`, `limit` (≤500) |
| POST | `/api/v1/resident-movements` | `RESIDENT` | Residente entra/sale del edificio (409 si ya está en ese estado) |
| GET | `/api/v1/resident-movements/me` | `RESIDENT` | Mi estado (dentro/fuera) y movimientos |
| GET | `/api/v1/resident-movements/presence` | **`GUARD`/`ADMIN`** | Residentes dentro ahora |
| GET | `/api/v1/access/qr-system` | autenticado | Disponibilidad del sistema QR (circuit breaker) |
| GET/PUT | `/api/v1/dev/qr-system` | ninguno | *(solo dev)* ver/forzar caída simulada del sistema QR |
| POST | `/api/v1/dev/qr-system/reset` | ninguno | *(solo dev)* cierra el circuito ya |
| GET | `/api/v1/dev/users` | ninguno | *(solo dev)* personas y membresías, leídas de la base |
| GET | `/api/v1/dev/token` | ninguno | *(solo dev)* `?user_id=&complex_id=[&role=]` → JWT de esa persona en ese conjunto |
| GET | `/healthz` | ninguno | Health check |

## Identidad detrás del API Gateway

Además del JWT HS256 (desarrollo y pruebas), el `ms` acepta la identidad que reenvía el gateway
(contrato en [`api-gateway.md`](api-gateway.md)). Se activa con `GATEWAY_SECRET`; con él configurado
`JWT_SECRET` pasa a ser opcional.

| Header | Uso |
|---|---|
| `X-Gateway-Secret` | debe ser igual a `GATEWAY_SECRET` (comparación en tiempo constante); si no → `401` |
| `X-User-ID` | usuario autenticado (UUID) |
| `X-Complex-ID` | conjunto seleccionado; se sigue validando contra `unit_members`/`complex_staff` |
| `X-Complex-Role` | rol que el gateway obtuvo de este servicio |

Sin `X-Gateway-Secret` esos headers se ignoran como cualquier input del cliente. El rol del header no es
suficiente para las rutas de personal: `RequireStaffMiddleware` lo vuelve a comprobar en la base.

| Método | Ruta | Quién | Respuesta |
|---|---|---|---|
| GET | `/api/v1/me/memberships` | el front, tras el login | `{user_id, name, memberships:[{complex_id, complex_name, role, unit_id?, unit_name?}]}` |
| GET | `/internal/users/{userId}/complexes/{complexId}/role` | el gateway (`X-Internal-Secret`) | `{role}` (el más alto si tiene varios) o `404` |

`/internal/*` solo se monta si `INTERNAL_SECRET` está definido. El `ms` de auth no devuelve conjuntos ni roles:
el front los obtiene de `/me/memberships` después de iniciar sesión.

## Eventos salientes (hacia `notificaciones`)

`ms/internal/notifier` publica en el stream de Redis `notifications.requested` (contrato en
[`docs/events.md`](events.md)), best-effort. Los destinatarios son los miembros de la unidad del
grant (`unit_members`), sin el residente que originó la acción:

| Momento | Categoría | Destinatarios | Se dispara en |
|---|---|---|---|
| Grant creado (QR, lote QR, pre-autorización, grant) | `ACCESS` | demás residentes de la unidad | `CreateQRCode`, `CreateQRBatch`, `CreatePreAuth`, `CreateAccessGrant` |
| Grant cancelado | `ACCESS` | demás residentes de la unidad | `CancelAccessGrant` |
| Visitante validado en portería | `VISITOR` | todos los residentes de la unidad | `ValidateAccess` (solo si el resultado es válido) |

Cada evento lleva `event_id` (UUID v7), `source: control-de-accesos`, `complex_id`, `user_ids`,
`title`, `body` y `data` (`event`, `grant_id`, `unit_id`, `method`). Sin `REDIS_URL` el cliente no hace nada.

## Entrada y salida de residentes

Los residentes marcan que entran o salen del edificio (conjunto). Se guarda en la misma
bitácora `access_logs` con `subject_type = RESIDENT` (migración `006`).

- `POST /api/v1/resident-movements` `{client_id, movement: ENTRY|EXIT}` (rol `RESIDENT`). **Nombre, unidad y
  hora los pone el servidor**; lo que mande el cliente se ignora. 201 nuevo, 200 si repite el `client_id`,
  **409** si contradice el estado (ya está dentro / ya está fuera). El estado es el último movimiento
  **en ese conjunto**; sin historial se permite cualquiera de los dos.
- Dos toques simultáneos no duplican: la transacción toma un `pg_advisory_xact_lock` por residente+conjunto.
- `GET /api/v1/resident-movements/me` → estado (`INSIDE|OUTSIDE|UNKNOWN`), desde cuándo y sus movimientos.
- `GET /api/v1/resident-movements/presence` (guardia/admin) → residentes dentro ahora, por conjunto.
- `GET /api/v1/access-logs?subject=RESIDENT|VISITOR` filtra la bitácora general.

## Circuit breaker del sistema QR

`ms/internal/qrsystem` aísla la integración que **emite y lee** códigos QR (dispositivo/servicio externo en
portería). Un dispositivo que no responde no debe dejar solicitudes bloqueadas: cada llamada tiene un
**timeout** y tras varios fallos seguidos el circuito **se abre** y se responde al instante con un error
controlado.

```
CLOSED ──(N fallos seguidos)──▶ OPEN ──(enfriamiento)──▶ HALF_OPEN ──(prueba ok)──▶ CLOSED
                                  ▲                          │
                                  └──────(prueba falla)──────┘
```

- **Dónde se aplica:** emitir un QR (`POST /access-grants` con `method=QR`, `/access/qr-codes`, lote) y leer un
  QR en portería (`/access-grants/validate` con `method=QR`). La **pre-autorización por documento no pasa por
  ahí** y sigue funcionando: es el *fallback*.
- **Error controlado:** `503` con `error.code = QR_001`, cabecera `Retry-After` y
  `details: {cause: TIMEOUT|CIRCUIT_OPEN|ERROR, fallback: "PRE_AUTH", retry_after_seconds}`.
- **Estado para el front:** `GET /api/v1/access/qr-system` → `{available, state, retry_after_seconds, fallback}`.
  Pasado el enfriamiento informa `HALF_OPEN` (disponible) para que el siguiente intento real sea el que
  cierra el circuito; si no, nadie volvería a probar.
- Una cancelación del cliente (cerró la pestaña) **no** cuenta como fallo del dispositivo.
- **Configuración:** `QR_BREAKER_FAILURES` (3), `QR_BREAKER_OPEN_SECONDS` (10), `QR_CALL_TIMEOUT_MS` (800),
  `QR_MAX_CONCURRENT` (32).
- **Límite de concurrencia (bulkhead):** como los fallos solo se cuentan cuando cada llamada termina, una ráfaga
  antes de abrirse el circuito dejaría una goroutine esperando el timeout por petición. Pasado `QR_MAX_CONCURRENT`
  llamadas en vuelo, el resto se rechaza al instante (`cause: OVERLOADED`), sin contarlo como fallo del dispositivo.
- Los callbacks de cambio de estado se ejecutan **fuera** del lock del breaker.
- **Simulación (solo `ENV=development`):** el cliente es un `Simulator`; en modo `DOWN` *no responde* (cuelga hasta
  el timeout), como un lector averiado. `GET/PUT /api/v1/dev/qr-system {"mode":"UP|DOWN"}` y
  `POST /api/v1/dev/qr-system/reset`. El front lo expone en el panel 🛠 de la esquina. Fuera de desarrollo se
  usa un cliente local (códigos aleatorios de 256 bits) con el mismo breaker.

## Motivos de rechazo al validar

`/access-grants/validate` devuelve `valid:false` con `reason` (el front lo traduce para el guardia):
`NOT_FOUND`, `NO_AUTHORIZATION` (cédula sin pase), `NOT_YET_VALID` y `EXPIRED` (con
`valid_from`/`valid_until`), `ALREADY_USED`, `CANCELLED`. La vigencia ya no se filtra en SQL: antes un pase
que aún no empezaba o ya vencía llegaba como "not found or expired" sin explicación.

**No se filtra información entre conjuntos:** un QR de *otro* conjunto responde exactamente igual que un código
inexistente (`NOT_FOUND`, mismo mensaje, sin datos del pase); el motivo real queda solo en `audit_log`. Lo mismo para
una cédula sin pre-autorización en este conjunto. Los motivos específicos (usado, vencido, revocado, no vigente) solo se
muestran cuando el pase **es de tu conjunto**.

## Seguridad y endurecimiento

Resultado de la revisión de código; cada punto tiene prueba (`test/hardening.mjs`, tests Go):

- **Un pase es una credencial.** El QR (y la cédula de una pre-autorización) permite entrar. Los listados
  `/access/qr-codes` y `/access/pre-auth` devuelven solo lo de *mis* unidades; antes devolvían todo el conjunto y un
  residente podía tomar el pase de un vecino. Portería/administración ven conteos (`/access/summary`).
- **Falla cerrada.** `ENV` por defecto es `production`: JWT obligatorio en todas las rutas y sin herramientas de
  desarrollo. Solo `ENV=development` explícito (con un binario `-tags dev`) las habilita.
- **Seeds fuera de las migraciones.** `db/migrations/` se aplica a cualquier base (incluida RDS) con `db/migrate.sh`;
  los datos de demostración (con UUID fijos y filas de guardia/admin) viven en `db/seeds/` y solo se cargan en el primer
  arranque de docker compose, con `WITH_SEEDS=1 db/migrate.sh` o con `test/reset_data.sh`.
- **Cuerpo máximo de 1 MiB** en todos los endpoints JSON → `413` (`SYS_003`), sin leer ni decodificar el exceso.
- **Un solo formato de error** `{"success":false,"error":{"code","message"}}`, también para 401/403 del middleware.
- **Lote de QR atómico:** se emiten todos los códigos y se insertan en una sola transacción.
- **No se revela qué existe en otros conjuntos:** un QR/cédula/id de otro conjunto responde igual que uno inexistente.
- **Índice** `unit_members(user_id)` (migración `007`): las comprobaciones de pertenencia corren en cada petición.

## Pendiente

- Las notificaciones salientes son best-effort: si Redis no responde el evento se pierde (se
  registra en el log). Para no perderlos haría falta un outbox transaccional; los nombres
  `AccessGranted`/`VisitorEntered`/… del diseño original no se usan: se publica el contrato
  `notifications.requested` que define `notificaciones` (ver `docs/events.md`).
