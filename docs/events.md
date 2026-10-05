# Eventos entre servicios (Redis Streams)

La comunicación asíncrona entre `ms` usa **Redis Streams** con consumer groups.
Un servicio publica; el servicio dueño del stream consume.

## Convenciones

| Regla | Detalle |
|---|---|
| Nombre del stream | `<dominio>.<hecho-o-comando>`, p. ej. `notifications.requested` |
| Dueño | el servicio que **consume** define el esquema y crea el consumer group |
| Mensaje | un solo campo `payload` con JSON |
| `event_id` | obligatorio, único por evento (UUID v7 recomendado). Es la llave de idempotencia |
| `source` | nombre del servicio que publica, para auditoría |
| Entrega | at-least-once: un consumidor **debe** ser idempotente sobre `event_id` |
| Reintentos | errores transitorios se reintentan (máx. 5 entregas) |
| Inválidos | van a `<stream>.dlq` con el `error` y el `original_id`; no se reintentan |
| Compatibilidad | solo cambios aditivos. Un campo nuevo es opcional para el consumidor; romper el esquema exige un stream nuevo (`...v2`) |

Publicar **después** de confirmar la transacción propia. Si la publicación
falla, reintentar desde el productor (o usar un outbox si el evento no se puede
perder). Un `event_id` repetido es seguro: el consumidor lo descarta.

## `notifications.requested`

**Dueño:** notificaciones. **Productores:** cualquier servicio que necesite
notificar a usuarios.

```json
{
  "event_id": "0192f1c4-6b7e-7c3a-9d1e-5f2a8b3c4d5e",
  "source": "control-de-accesos",
  "complex_id": "0192…",
  "user_ids": ["0192…"],
  "category": "VISITOR",
  "title": "Visitante en portería",
  "body": "Juan Pérez está en la entrada",
  "data": { "deep_link": "/visitors/0192…", "visitor_id": "0192…" }
}
```

| Campo | Tipo | |
|---|---|---|
| `event_id` | string | requerido |
| `source` | string | recomendado |
| `complex_id` | uuid \| null | `null` = notificación de cuenta: aparece en la bandeja de todos los conjuntos del usuario |
| `user_ids` | uuid[] | requerido, 1–5000 (se eliminan duplicados). El productor resuelve los destinatarios |
| `category` | string | requerido, ver tabla |
| `title` | string | requerido, máx. 200 caracteres (se trunca) |
| `body` | string | requerido, máx. 2000 caracteres (se trunca) |
| `data` | objeto plano | opcional; se guarda en la bandeja y viaja en el payload del push, donde FCM solo acepta strings (cada valor se convierte a texto; no anidar objetos). Usar `deep_link` para la navegación en la app |

### Categorías

Cada categoría pertenece a un **grupo**. El grupo es lo que la app muestra
(etiqueta y filtros del centro de notificaciones) y lo que el usuario puede
silenciar en sus preferencias.

| Grupo | Categorías | El usuario puede silenciarlo | Productor |
|---|---|---|---|
| `ACCESS_CONTROL` (Control de acceso) | `SOS`*, `SECURITY`*, `EMERGENCY`*, `ACCESS`, `VISITOR`, `PACKAGE` | no (obligatorio) | control de acceso |
| `INTERCOM` (Citofonía) | `CALL`* | no (obligatorio) | citofonía |
| `COMMUNITY` (Comunicados) | `ANNOUNCEMENT`, `COMMUNITY`, `EVENT`, `MAINTENANCE` | sí | comunidad |
| `RESERVATIONS` (Reservas) | `RESERVATION` | sí | reservas / amenidades |
| `ASSEMBLIES` (Asambleas) | `ASSEMBLY` | sí | asambleas |
| `VOTES` (Votaciones) | `VOTE` | sí | asambleas |
| `SYSTEM` (Sistema) | `ACCOUNT`, `PAYMENT`, `GENERAL` | no | auth, gestión financiera |

- \* **Críticas:** push con prioridad alta y con su propio cupo de envío, así
  nunca esperan detrás de un envío masivo.
- Un evento de un grupo silenciado no llega a la bandeja ni al push de ese
  usuario.
- Una categoría desconocida manda el evento a la DLQ. Para añadir una categoría,
  agrégala primero en `back/notificaciones/ms/internal/features/preferences/models.go`.

### Publicar desde Go

```go
payload, _ := json.Marshal(map[string]any{
	"event_id":   uuid.Must(uuid.NewV7()).String(),
	"source":     "control-de-accesos",
	"complex_id": complexID,
	"user_ids":   residentIDs,
	"category":   "VISITOR",
	"title":      "Visitante en portería",
	"body":       visitor.Name + " está en la entrada",
	"data":       map[string]any{"deep_link": "/visitors/" + visitor.ID.String()},
})
err := rdb.XAdd(ctx, &redis.XAddArgs{
	Stream: "notifications.requested",
	MaxLen: 100_000, Approx: true, // acota el tamaño del stream
	Values: map[string]any{"payload": string(payload)},
}).Err()
```

Desde la terminal:

```bash
redis-cli XADD notifications.requested '*' payload '{"event_id":"test-1","source":"manual","complex_id":null,"user_ids":["<uuid>"],"category":"ACCOUNT","title":"Hola","body":"Prueba"}'
```

## Operación

```bash
# Mensajes sin confirmar (en reintento) por consumidor
redis-cli XPENDING notifications.requested notificaciones

# Revisar la DLQ
redis-cli XRANGE notifications.requested.dlq - + COUNT 20

# Reprocesar un mensaje de la DLQ después de corregir la causa:
# copiar su payload a un XADD nuevo y borrarlo de la DLQ
redis-cli XADD notifications.requested '*' payload '<payload>'
redis-cli XDEL notifications.requested.dlq <id>
```

Un mensaje que se reprocesa con el mismo `event_id` que ya se guardó se
descarta (idempotencia). Solo se reprocesan eventos que nunca se persistieron.
