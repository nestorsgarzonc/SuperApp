# Solicitudes de notificación entre servicios

Los `ms` no comparten bus de mensajes: un servicio que necesita notificar a
usuarios llama por HTTP interno a notificaciones, `POST /internal/notifications`
con `X-Internal-Secret` (reglas de `/internal/*` en
[`internal-apis.md`](internal-apis.md); respuestas y errores en
[`back/notificaciones/API.md`](../back/notificaciones/API.md#interno)).

## Convenciones

| Regla | Detalle |
|---|---|
| `event_id` | obligatorio, único por solicitud (UUID v7 recomendado). Es la llave de idempotencia |
| `source` | nombre del servicio que la envía, para auditoría |
| Idempotencia | un `event_id` repetido responde `202` y no duplica la bandeja: reintentar es seguro |
| Reintentos | el productor reintenta ante `5xx`, timeout o error de red, **con el mismo `event_id`** |
| Inválidos | `422`: no se reintentan; el productor registra el error |
| Compatibilidad | solo cambios aditivos. Un campo nuevo es opcional para notificaciones |

Llamar **después** de confirmar la transacción propia. La llamada es
best-effort: si notificaciones no responde, el flujo del productor (p. ej.
registrar un ingreso) no debe fallar. Si la notificación no se puede perder,
guardar la solicitud en un outbox propio y reintentar desde ahí.

## Cuerpo

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
- Una categoría desconocida se rechaza con `422`. Para añadir una categoría,
  agrégala primero en `back/notificaciones/ms/internal/features/preferences/models.go`.

### Enviar desde Go

```go
body, _ := json.Marshal(map[string]any{
	"event_id":   uuid.Must(uuid.NewV7()).String(),
	"source":     "control-de-accesos",
	"complex_id": complexID,
	"user_ids":   residentIDs,
	"category":   "VISITOR",
	"title":      "Visitante en portería",
	"body":       visitor.Name + " está en la entrada",
	"data":       map[string]any{"deep_link": "/visitors/" + visitor.ID.String()},
})
req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
	notificacionesURL+"/internal/notifications", bytes.NewReader(body))
req.Header.Set("Content-Type", "application/json")
req.Header.Set("X-Internal-Secret", internalSecret)
resp, err := httpClient.Do(req) // 202 = aceptada
```

Implementación de referencia (timeout, reintentos, best-effort):
`back/control-de-accesos/ms/internal/notifier`.

Desde la terminal:

```bash
curl -s -X POST localhost:8083/internal/notifications \
  -H "X-Internal-Secret: $INTERNAL_SECRET" \
  -d '{"event_id":"test-1","source":"manual","complex_id":null,"user_ids":["<uuid>"],"category":"ACCOUNT","title":"Hola","body":"Prueba"}'
```
