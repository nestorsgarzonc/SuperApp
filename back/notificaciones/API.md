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
| `unread` | `false` | `true` = solo no leídas (chip "No leídas") |
| `group` | todos | uno o varios [grupos](#grupos) separados por coma. Grupo desconocido → `422` |
| `limit` | `50` | 1–200; un valor fuera de rango usa 50. El dashboard usa `3` |

Chips del centro de notificaciones (Figma):

| Chip | Query |
|---|---|
| Todas | — |
| No leídas (N) | `unread=true` (N viene de `unread-count`) |
| Control de acceso | `group=ACCESS_CONTROL,INTERCOM` |
| Comunidad | `group=COMMUNITY` |
| Reservas | `group=RESERVATIONS` |
| Asambleas | `group=ASSEMBLIES,VOTES` |
| Sistema | `group=SYSTEM` |

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
      "group": "ACCESS_CONTROL",
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
| `type` | categoría del evento |
| `group` | grupo de la categoría: define la etiqueta del item ("Control de acceso", "Citofonía", ...). Ver [grupos](#grupos) |
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
todos sus conjuntos. Son por [grupo](#grupos), una por interruptor de la
pantalla de preferencias.

### `GET /me/notification-preferences`

Devuelve los 6 grupos en el orden de la pantalla. Un grupo sin configurar
aparece habilitado. `locked: true` = obligatorio por seguridad: se muestra con
candado y no se puede apagar.

`200`

```json
{
  "success": true,
  "data": {
    "preferences": [
      { "group": "ACCESS_CONTROL", "enabled": true,  "locked": true },
      { "group": "INTERCOM",       "enabled": true,  "locked": true },
      { "group": "RESERVATIONS",   "enabled": true,  "locked": false },
      { "group": "COMMUNITY",      "enabled": true,  "locked": false },
      { "group": "ASSEMBLIES",     "enabled": true,  "locked": false },
      { "group": "VOTES",          "enabled": false, "locked": false }
    ]
  }
}
```

### `PUT /me/notification-preferences`

Cambia uno o varios grupos. Solo se modifican los grupos enviados. Responde con
el estado completo, igual que el `GET`, para que la app pinte los interruptores
sin otra llamada.

```json
{ "preferences": [ { "group": "VOTES", "enabled": false } ] }
```

| Campo | |
|---|---|
| `preferences[].group` | `RESERVATIONS`, `COMMUNITY`, `ASSEMBLIES` o `VOTES` |
| `preferences[].enabled` | requerido |

| Respuesta | Caso |
|---|---|
| `200` | guardado; cuerpo igual al `GET` |
| `422` `VAL_001` | lista vacía, falta `enabled`, grupo desconocido, o grupo obligatorio (`ACCESS_CONTROL`, `INTERCOM`, `SYSTEM`) |

Un grupo silenciado deja de recibir **bandeja y push** de sus categorías desde
ese momento. Lo ya recibido no cambia.

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

## Grupos

| Grupo | Etiqueta en la app | Categorías | Silenciable |
|---|---|---|---|
| `ACCESS_CONTROL` | Control de acceso | `SOS`, `SECURITY`, `EMERGENCY`, `ACCESS`, `VISITOR`, `PACKAGE` | no |
| `INTERCOM` | Citofonía | `CALL` | no |
| `COMMUNITY` | Comunicados / Comunidad | `ANNOUNCEMENT`, `COMMUNITY`, `EVENT`, `MAINTENANCE` | sí |
| `RESERVATIONS` | Reservas | `RESERVATION` | sí |
| `ASSEMBLIES` | Asambleas | `ASSEMBLY` | sí |
| `VOTES` | Votaciones | `VOTE` | sí |
| `SYSTEM` | Sistema | `ACCOUNT`, `PAYMENT`, `GENERAL` | no (no aparece en preferencias) |

`SOS`, `SECURITY`, `EMERGENCY` y `CALL` además se envían por push con prioridad
alta. Qué servicio publica cada categoría está en
[`docs/events.md`](../../docs/events.md#categorías).

## Probar en local

```bash
make run
U=0192f1c4-0000-7000-8000-000000000002
C=0192f1c4-0000-7000-8000-000000000001

curl -s localhost:8083/users/me/notifications -H "X-User-ID: $U" -H "X-Complex-ID: $C"
curl -s -X PUT localhost:8083/me/notification-preferences -H "X-User-ID: $U" \
  -d '{"preferences":[{"group":"VOTES","enabled":false}]}'
```

Con `ENV=development` y sin `GATEWAY_SECRET` no se exige `X-Gateway-Secret`.
Para tener algo en la bandeja, publicar un evento como en
[`docs/events.md`](../../docs/events.md#publicar-desde-go).
