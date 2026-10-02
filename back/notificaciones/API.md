# notificaciones — API HTTP

Referencia de los endpoints del servicio. Las notificaciones **no se crean por
HTTP**: llegan como eventos `notifications.requested` (ver
[`docs/events.md`](../../docs/events.md)). Por HTTP solo se consulta la bandeja,
se configuran preferencias y se registran dispositivos.

## Convenciones

**Ruta.** El cliente llama al gateway con el prefijo `/api/v1`; el gateway lo
quita y reenvía al servicio. `GET /api/v1/me/devices` llega como
`GET /me/devices`. Las rutas de abajo son las del servicio.

**Identidad.** El cliente no envía el usuario: el gateway valida el JWT e
inyecta los headers (contrato en [`docs/api-gateway.md`](../../docs/api-gateway.md)).

| Header | Lo envía | Uso |
|---|---|---|
| `Authorization: Bearer <jwt>` | cliente → gateway | autenticación |
| `X-Complex-ID` | cliente → gateway (validado) → servicio | conjunto seleccionado; obligatorio en la bandeja |
| `X-User-ID`, `X-Gateway-Secret` | gateway → servicio | usuario autenticado; prueba de que pasó por el gateway |

**Sobre de respuesta.**

```json
{ "success": true, "data": { ... } }
{ "success": false, "error": { "code": "VAL_001", "message": "Validation failed",
  "details": { "fields": { "push_token": ["A valid push token is required"] } } } }
```

`204 No Content` no tiene cuerpo.

**Errores comunes a todos los endpoints.**

| HTTP | `code` | Caso |
|---|---|---|
| 401 | `AUTH_001` | falta `X-Gateway-Secret` o `X-User-ID` (petición que no pasó por el gateway) |
| 422 | `VAL_001` | cuerpo JSON inválido o > 1 MiB, `X-Complex-ID` no es UUID, o falla una validación (detalle en `details.fields`) |
| 500 | `SYS_001` | error interno; la causa se registra en logs, nunca se devuelve |

---

## Bandeja

Requieren `X-Complex-ID`. Devuelven las notificaciones del conjunto
seleccionado **más** las notificaciones de cuenta (`complex_id` nulo), que
aparecen en todos los conjuntos del usuario. Sin `X-Complex-ID` → `422`.

### `GET /users/me/notifications`

Lista la bandeja, la más reciente primero.

| Query | Default | |
|---|---|---|
| `unread` | `false` | `true` = solo no leídas |
| `limit` | `50` | 1–200; un valor fuera de rango usa 50 |

`200`

```json
{
  "success": true,
  "data": [
    {
      "id": "0192f1c4-6b7e-7c3a-9d1e-5f2a8b3c4d5e",
      "complex_id": "0192f1c4-0000-7000-8000-000000000001",
      "user_id": "0192f1c4-0000-7000-8000-000000000002",
      "title": "Visitante en portería",
      "body": "Juan Pérez está en la entrada",
      "data": { "deep_link": "/visitors/0192…" },
      "type": "VISITOR",
      "is_read": false,
      "delivery_status": "PUSH_SENT",
      "delivered_at": "2026-10-02T17:20:11Z",
      "created_at": "2026-10-02T17:20:10Z"
    }
  ]
}
```

| Campo | |
|---|---|
| `complex_id` | ausente en notificaciones de cuenta |
| `type` | categoría del evento (ver [categorías](#categorías)) |
| `data` | objeto libre del productor; usar `deep_link` para navegar |
| `read_at` | presente solo si `is_read` |
| `delivery_status` | `INBOX_ONLY` (sin dispositivos o push pendiente), `PUSH_SENT`, `PUSH_PARTIAL`, `PUSH_FAILED` |
| `delivered_at` | presente si el push llegó al menos a un dispositivo |

### `GET /users/me/notifications/unread-count`

`200` → `{ "success": true, "data": { "count": 3 } }`

### `PATCH /notifications/{id}/read`

Marca una notificación como leída. Idempotente: marcarla otra vez conserva el
`read_at` original.

| Respuesta | Caso |
|---|---|
| `204` | marcada |
| `404` `RES_001` | no existe, es de otro usuario o de otro conjunto |
| `422` `VAL_001` | `id` no es UUID |

### `POST /notifications/mark-all-read`

Sin cuerpo. Marca como leídas todas las no leídas de la bandeja actual.

`200` → `{ "success": true, "data": { "marked_count": 5 } }`

---

## Preferencias

No requieren `X-Complex-ID`: las preferencias son del usuario y aplican en
todos sus conjuntos.

### `GET /me/notification-preferences`

Devuelve el estado **efectivo** de cada categoría configurable: una categoría
sin configurar aparece habilitada y sin horas de silencio.

`200`

```json
{
  "success": true,
  "data": {
    "timezone": "America/Bogota",
    "preferences": [
      { "category": "GENERAL", "enabled": true, "quiet_start": 1320, "quiet_end": 360 },
      { "category": "PACKAGE", "enabled": false, "quiet_start": null, "quiet_end": null }
    ]
  }
}
```

`timezone` es el `DEFAULT_TIMEZONE` del servicio si el usuario nunca guardó uno.

### `PUT /me/notification-preferences`

Actualiza la zona horaria, una o varias categorías, o ambas. Solo se modifican
las categorías enviadas.

```json
{
  "timezone": "America/Bogota",
  "preferences": [
    { "category": "GENERAL", "enabled": true, "quiet_start": 1320, "quiet_end": 360 },
    { "category": "PACKAGE", "enabled": false }
  ]
}
```

| Campo | |
|---|---|
| `timezone` | opcional; nombre IANA (`America/Bogota`, `Europe/Madrid`) |
| `preferences[].category` | requerido; una categoría configurable |
| `preferences[].enabled` | **enviarlo siempre**: si se omite cuenta como `false` (silenciada) |
| `preferences[].quiet_start`, `quiet_end` | minuto del día `[0, 1440)`; ambos o ninguno. Si `start > end` la ventana cruza la medianoche (`1320`–`360` = 22:00–06:00). Se evalúan en la `timezone` del usuario |

| Respuesta | Caso |
|---|---|
| `204` | guardado |
| `422` `VAL_001` | cuerpo vacío (ni `timezone` ni `preferences`), zona horaria inválida, categoría crítica o desconocida, horas de silencio incompletas o fuera de rango |

Las categorías críticas (`SOS`, `SECURITY`, `EMERGENCY`, `CALL`) no se pueden
silenciar ni programar: se rechazan con `422`.

---

## Dispositivos

Tokens de push (FCM) del usuario. No requieren `X-Complex-ID`.

### `POST /me/devices`

Registra un dispositivo. La app debe llamarlo al iniciar sesión y cada vez que
FCM entregue un token nuevo. Si el token ya existe (aunque sea de otro usuario)
se reasigna a este usuario y se actualiza `last_seen_at`.

```json
{ "platform": "ANDROID", "push_token": "fcm-token…", "device_name": "Pixel 8", "app_version": "1.4.0" }
```

| Campo | |
|---|---|
| `platform` | requerido: `IOS`, `ANDROID` o `WEB` (sin distinguir mayúsculas) |
| `push_token` | requerido, máx. 4096 caracteres |
| `device_name`, `app_version` | opcionales |

`201`

```json
{
  "success": true,
  "data": {
    "id": "0192f1c4-…",
    "user_id": "0192f1c4-…",
    "platform": "ANDROID",
    "device_name": "Pixel 8",
    "app_version": "1.4.0",
    "last_seen_at": "2026-10-02T17:20:10Z",
    "created_at": "2026-09-30T08:00:00Z"
  }
}
```

El `push_token` nunca se devuelve.

### `GET /me/devices`

Lista los dispositivos del usuario, el más reciente primero.
`200` → `data` es un arreglo con la forma de arriba.

### `POST /me/devices/unregister`

Borra el dispositivo por token. Usarlo al **cerrar sesión**, cuando la app
conoce su token pero no el `id`.

```json
{ "push_token": "fcm-token…" }
```

| Respuesta | Caso |
|---|---|
| `204` | borrado |
| `404` `RES_001` | el token no está registrado para este usuario |
| `422` `VAL_001` | falta `push_token` |

### `DELETE /me/devices/{id}`

Borra un dispositivo por `id` (p. ej. desde una pantalla de "mis dispositivos").

| Respuesta | Caso |
|---|---|
| `204` | borrado |
| `404` `RES_001` | no existe o es de otro usuario |
| `422` `VAL_001` | `id` no es UUID |

---

## Operación

### `GET /healthz`

Sin autenticación; lo usa el balanceador como health check. Comprueba Postgres y Redis.

| Respuesta | |
|---|---|
| `200` | `{ "success": true, "data": { "status": "ok" } }` |
| `503` `SYS_005` | `database unavailable` o `redis unavailable` |

---

## Categorías

| Categoría | Crítica | Configurable por el usuario |
|---|---|---|
| `SOS`, `SECURITY`, `EMERGENCY`, `CALL` | sí | no — siempre se entregan, con prioridad alta |
| `ACCESS`, `VISITOR`, `PACKAGE` | no | sí |
| `ANNOUNCEMENT`, `COMMUNITY`, `EVENT`, `MAINTENANCE`, `GENERAL` | no | sí |
| `PAYMENT` | no | sí |
| `ASSEMBLY` | no | sí |
| `ACCOUNT` | no | sí |

Qué servicio publica cada una está en [`docs/events.md`](../../docs/events.md#categorías).

## Probar en local

```bash
make run
U=0192f1c4-0000-7000-8000-000000000002
C=0192f1c4-0000-7000-8000-000000000001

curl -s localhost:8083/users/me/notifications -H "X-User-ID: $U" -H "X-Complex-ID: $C"
curl -s -X PUT localhost:8083/me/notification-preferences -H "X-User-ID: $U" \
  -d '{"timezone":"America/Bogota","preferences":[{"category":"GENERAL","enabled":false}]}'
```

Con `ENV=development` y sin `GATEWAY_SECRET` no se exige `X-Gateway-Secret`.
Para tener algo en la bandeja, publicar un evento como en
[`docs/events.md`](../../docs/events.md#publicar-desde-go).
