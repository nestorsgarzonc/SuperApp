# notificaciones

Entrega de notificaciones: recibe solicitudes de otros servicios por HTTP
interno (`POST /internal/notifications`), guarda la bandeja, envía
push (FCM) y respeta las preferencias por grupo del usuario. No decide qué
notificar ni a quién: eso lo resuelve el servicio que la solicita. Portado desde el monolito `residential-superapp`
(`features/communication` — parte de notificaciones —, `features/notificationprefs`
y `platform/firebase`).

```
notificaciones/
├── db/migrations/        # esquema propio (golang-migrate)
├── ms/                   # servicio Go: Handler → Service → Repository
│   ├── cmd/server/
│   └── internal/
│       ├── features/
│       │   ├── notifications/   # bandeja y Dispatch de eventos
│       │   ├── preferences/     # categorías → grupos; grupos silenciables y obligatorios
│       │   └── devices/         # tokens de push
│       ├── integration/         # tests contra Postgres real (-tags integration)
│       └── platform/            # apperr, gatewayauth, internalauth, push (FCM)
├── docker-compose.yml    # db local
└── Makefile
```

Documentos relacionados: [`docs/api-gateway.md`](../../docs/api-gateway.md),
[`docs/events.md`](../../docs/events.md), [`docs/internal-apis.md`](../../docs/internal-apis.md).

## Correr local

```bash
make run               # levanta la db, migra y corre el servicio en :8083
make test              # tests unitarios (sin infraestructura)
make test-integration  # SQL contra un Postgres real
make down
```

| Variable | Default | Notas |
|---|---|---|
| `ENV` | `development` | fuera de `development` exige `GATEWAY_SECRET` e `INTERNAL_SECRET` |
| `PORT` | `8083` | |
| `DATABASE_URL` | — | requerido. PostgreSQL 18+ (`uuidv7()`) |
| `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` | `10` / `5` | la RDS es compartida: en producción 5 / 2 |
| `GATEWAY_SECRET` | — | debe coincidir con `X-Gateway-Secret` que envía el gateway |
| `INTERNAL_SECRET` | — | debe coincidir con `X-Internal-Secret` que envían los demás ms |
| `FIREBASE_CREDENTIALS_FILE` / `FIREBASE_PROJECT_ID` | — | vacío = sin push, solo bandeja |

## Autenticación

El servicio no valida tokens: confía en los headers que inyecta el api-gateway
(`X-Gateway-Secret`, `X-User-ID`, `X-Complex-ID`, `X-Complex-Role`). El
contrato completo está en [`docs/api-gateway.md`](../../docs/api-gateway.md).

## API

Referencia completa (cuerpos, respuestas y errores): [`API.md`](API.md).

| Método y ruta | Acceso | Monolito |
|---|---|---|
| `GET /users/me/notifications?unread=&group=&limit=` | residente | igual + filtro por grupo; incluye notificaciones de cuenta |
| `GET /users/me/notifications/unread-count` | residente | igual |
| `PATCH /notifications/{id}/read` | residente | igual |
| `POST /notifications/mark-all-read` | residente | igual |
| `GET` / `PUT /me/notification-preferences` | usuario | por grupo (antes por categoría con horas de silencio) |
| `POST /me/devices`, `GET /me/devices` | usuario | antes en identity (`/devices`) |
| `POST /me/devices/unregister` `{push_token}` | usuario | nuevo: para logout |
| `DELETE /me/devices/{id}` | usuario | antes en identity |
| `POST /internal/notifications` | otros ms (`X-Internal-Secret`) | antes evento de Redis `notifications.requested` |
| `GET /healthz` | interno | revisa la db |

Las respuestas usan el mismo sobre que el monolito: `{success, data}` /
`{success: false, error: {code, message, details}}`.

Las pantallas que cubre (Figma, sección 07 — Notificaciones):

| Pantalla | Endpoints |
|---|---|
| Centro: lista, filtros, "No leídas (N)", "Marcar todas como leídas" | `GET /users/me/notifications?group=`, `unread-count`, `mark-all-read`, `PATCH .../read` |
| Campana del header y "Últimas notificaciones" del dashboard | `unread-count`, `GET /users/me/notifications?limit=3` |
| Preferencias: 6 interruptores, 2 bloqueados | `GET` / `PUT /me/notification-preferences` |
| Ejemplos push | `POST /internal/notifications` + `POST /me/devices` |

## Entrega

1. Se filtran destinatarios por preferencias: 1 consulta por evento, sin importar
   cuántos destinatarios haya. Los grupos obligatorios no consultan nada.
2. Se guardan todas las filas de la bandeja con un solo `INSERT` (más
   `processed_events`), en una transacción.
3. Aquí se responde `202` al productor. El push corre en segundo plano, con
   un contexto propio de hasta 2 min.
4. Máximo 20 envíos FCM en paralelo para las categorías no críticas. Las
   críticas (`SOS`, `CALL`, ...) tienen su propio cupo (200), así que nunca
   esperan detrás de un envío masivo.
5. Cada notificación queda `PUSH_SENT`, `PUSH_PARTIAL`, `PUSH_FAILED` o
   `INBOX_ONLY` (el usuario no tiene dispositivos). Los tokens que FCM reporta
   como no registrados se borran.
6. Al apagar el servicio se esperan los pushes en curso.

## Solicitudes de otros servicios

El cuerpo de `POST /internal/notifications` y las categorías están en
[`API.md`](API.md#interno) y [`docs/events.md`](../../docs/events.md). Es
idempotente por `event_id`: ante un `5xx` o un timeout el productor puede
reintentar con el mismo `event_id` sin duplicar la bandeja.

## Diferencias con el monolito

- **Tokens de push** viven aquí (`user_devices`), no en identity. Las apps
  deben registrar el token contra `POST /me/devices`.
- **Preferencias por grupo**, como en el diseño: sin horas de silencio ni zona
  horaria. Control de acceso y Citofonía no se pueden silenciar.
- **Notificaciones de cuenta** (`complex_id` nulo) aparecen en la bandeja de
  cualquier conjunto. En el monolito se guardaban pero no se mostraban.
- **Sin envío manual del admin.** `POST /notifications/send`, `/sent` y `/stats`
  no existen: los comunicados son de Comunidad, que publica un evento
  `ANNOUNCEMENT`. Así este servicio no depende de nadie de forma síncrona.
- **Push asíncrono.** El evento se confirma cuando la bandeja está guardada,
  sin esperar a FCM.
- **DND global** (`users.dnd_until`) no se aplica: estaba en la tabla de
  usuarios de auth. Si se necesita, modelarlo como preferencia aquí.
- **Tabla `notifications`** sin particionar por mes. Reevaluar cuando el volumen
  lo justifique.
- **Categorías** `PQRS` y `MARKETPLACE` se eliminaron. `AMENITY` pasó a
  `RESERVATION` y se agregó `VOTE`, ambas presentes en el diseño.

## Pendiente

- api-gateway y productores de eventos (especificados en `docs/`).
- Script de migración de datos desde el monolito (notificaciones, preferencias,
  tokens).
- Observabilidad: OpenTelemetry, Sentry y métricas como en el monolito.
- Limpieza periódica de `processed_events` y notificaciones antiguas.
- Prueba del push con credenciales reales de Firebase.

## Despliegue en EC2 (RDS compartida)

Todos los ms usan la misma base de datos de RDS; notificaciones vive en el
schema `notificaciones` con su propio rol (`db/bootstrap.sql`), que no puede
leer otros schemas. Paso a paso con la consola de AWS:
[`DEPLOY-EC2.md`](DEPLOY-EC2.md).
