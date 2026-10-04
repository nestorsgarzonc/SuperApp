# notificaciones

Entrega de notificaciones: consume eventos del bus, guarda la bandeja, envía
push (FCM) y respeta las preferencias por categoría del usuario. No decide qué
notificar ni a quién: eso lo resuelve el servicio que publica el evento. Portado desde el monolito `residential-superapp`
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
│       │   ├── preferences/     # mute, horas de silencio, zona horaria; categorías críticas
│       │   └── devices/         # tokens de push
│       ├── events/              # consumidor Redis Streams
│       ├── integration/         # tests contra Postgres + Redis reales (-tags integration)
│       └── platform/            # apperr, gatewayauth, push (FCM)
├── docker-compose.yml    # db + redis locales
└── Makefile
```

Documentos relacionados: [`docs/api-gateway.md`](../../docs/api-gateway.md),
[`docs/events.md`](../../docs/events.md), [`docs/internal-apis.md`](../../docs/internal-apis.md).

## Correr local

```bash
make run               # levanta db + redis, migra y corre el servicio en :8083
make test              # tests unitarios (sin infraestructura)
make test-integration  # SQL + consumidor Redis contra contenedores reales
make down
```

| Variable | Default | Notas |
|---|---|---|
| `ENV` | `development` | fuera de `development` exige `GATEWAY_SECRET` |
| `PORT` | `8083` | |
| `DATABASE_URL` | — | requerido. PostgreSQL 18+ (`uuidv7()`) |
| `REDIS_URL` | `redis://localhost:6379/0` | |
| `EVENTS_STREAM` / `EVENTS_GROUP` | `notifications.requested` / `notificaciones` | |
| `DEFAULT_TIMEZONE` | `America/Bogota` | horas de silencio de usuarios sin zona horaria propia |
| `GATEWAY_SECRET` | — | debe coincidir con `X-Gateway-Secret` que envía el gateway |
| `FIREBASE_CREDENTIALS_FILE` / `FIREBASE_PROJECT_ID` | — | vacío = sin push, solo bandeja |

## Autenticación

El servicio no valida tokens: confía en los headers que inyecta el api-gateway
(`X-Gateway-Secret`, `X-User-ID`, `X-Complex-ID`, `X-Complex-Role`). El
contrato completo está en [`docs/api-gateway.md`](../../docs/api-gateway.md).

## API

Referencia completa (cuerpos, respuestas y errores): [`API.md`](API.md).

| Método y ruta | Acceso | Monolito |
|---|---|---|
| `GET /users/me/notifications?unread=&limit=` | residente | igual; ahora incluye notificaciones de cuenta |
| `GET /users/me/notifications/unread-count` | residente | igual |
| `PATCH /notifications/{id}/read` | residente | igual |
| `POST /notifications/mark-all-read` | residente | igual |
| `GET` / `PUT /me/notification-preferences` | usuario | igual + campo `timezone` |
| `POST /me/devices`, `GET /me/devices` | usuario | antes en identity (`/devices`) |
| `POST /me/devices/unregister` `{push_token}` | usuario | nuevo: para logout |
| `DELETE /me/devices/{id}` | usuario | antes en identity |
| `GET /healthz` | interno | revisa db y redis |

Las respuestas usan el mismo sobre que el monolito: `{success, data}` /
`{success: false, error: {code, message, details}}`.

`PUT /me/notification-preferences` acepta `timezone` (nombre IANA), `preferences`
o ambos:

```json
{ "timezone": "America/Bogota",
  "preferences": [{ "category": "ANNOUNCEMENT", "enabled": true, "quiet_start": 1320, "quiet_end": 360 }] }
```

## Entrega

1. Se filtran destinatarios por preferencias: 2 consultas por envío, sin importar
   cuántos destinatarios haya. Las categorías críticas no se filtran.
2. Se guardan todas las filas de la bandeja con un solo `INSERT` (más
   `processed_events`), en una transacción.
3. El evento se confirma (XACK) aquí. El push corre en segundo plano, con
   un contexto propio de hasta 2 min.
4. Máximo 20 envíos FCM en paralelo para las categorías no críticas. Las
   críticas (`SOS`, `CALL`, ...) tienen su propio cupo (200), así que nunca
   esperan detrás de un envío masivo.
5. Cada notificación queda `PUSH_SENT`, `PUSH_PARTIAL`, `PUSH_FAILED` o
   `INBOX_ONLY` (el usuario no tiene dispositivos). Los tokens que FCM reporta
   como no registrados se borran.
6. Al apagar el servicio se esperan los pushes en curso.

## Eventos

El formato de `notifications.requested`, las categorías y la operación de la DLQ
están en [`docs/events.md`](../../docs/events.md). Un evento que falla por un
error transitorio se reintenta hasta 5 veces y luego va a la DLQ.

## Diferencias con el monolito

- **Tokens de push** viven aquí (`user_devices`), no en identity. Las apps
  deben registrar el token contra `POST /me/devices`.
- **Horas de silencio** se evalúan en la zona horaria del usuario (o
  `DEFAULT_TIMEZONE`). El monolito usaba la hora del servidor, que en un
  contenedor es UTC y corría la ventana 5 horas.
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
- **Categorías** `PQRS`, `AMENITY` y `MARKETPLACE` se eliminaron: ningún
  subdominio de la arquitectura las produce.

## Pendiente

- api-gateway y productores de eventos (especificados en `docs/`).
- Script de migración de datos desde el monolito (notificaciones, preferencias,
  tokens).
- Observabilidad: OpenTelemetry, Sentry y métricas como en el monolito.
- Limpieza periódica de `processed_events` y notificaciones antiguas.
- Prueba del push con credenciales reales de Firebase.
