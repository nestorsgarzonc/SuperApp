# SuperApp

Un front (`front/`) y un microservicio por feature (`back/<feature>/ms` + su
`db/`), todo detrás del API Gateway (`back/api-gateway`). Arquitectura:
[`docs/architecture.md`](docs/architecture.md) · contrato del gateway:
[`docs/api-gateway.md`](docs/api-gateway.md).

## Levantar todo con un comando

Requisito: Docker con Compose v2.

```sh
docker compose up --build
```

La primera vez tarda unos minutos (compila las imágenes). Luego abre
**http://localhost:3000**.

| Usuario (contraseña `Demo-Pass-2026!`) | Rol |
|---|---|
| `ana.garcia@demo.test` | residente · Los Pinos |
| `luis.mora@demo.test` | portería · Los Pinos |
| `carlos.rojas@demo.test` | administración · Los Pinos |
| `sofia.diaz@demo.test` | residente en dos conjuntos |

También está `/dev-login` para entrar como cualquier persona/rol sin contraseña.

### Qué se levanta

```
navegador ─► front (nginx :3000) ─/api─► api-gateway (:8080) ─┬─► auth
                                              │                ├─► control-de-accesos ─► notificaciones
                                              └─ redis          └─► notificaciones
                       postgres (:5440): auth_db · control_de_accesos · notificaciones
```

| Servicio | Qué hace |
|---|---|
| `postgres` | un servidor, una base y un rol por `ms` |
| `redis` | rate limiting del gateway y sesiones revocadas (logout inmediato) |
| `auth-init`, `control-de-accesos-migrate`, `notificaciones-migrate` | tareas de una vez: llave RS256, migraciones y datos demo (idempotentes) |
| `auth`, `control-de-accesos`, `notificaciones` | microservicios, solo en la red interna |
| `api-gateway` | único punto de entrada a la API |
| `front` | build de producción de Vite servido por nginx, que reenvía `/api` al gateway |

Solo `front`, `api-gateway` y `postgres` publican puertos, y solo en `127.0.0.1`.

### Verificarlo

```sh
scripts/smoke.sh   # 30+ comprobaciones de punta a punta contra el stack (curl + jq)
```

Antes de un merge: los tests de cada servicio (`go test -race ./...`), los del
front (`npm run lint && npm test`) y este smoke test contra el stack.

### Comandos útiles

```sh
docker compose logs -f api-gateway        # ver el tráfico
docker compose up --build api-gateway     # recompilar un servicio
docker compose down                       # parar (conserva los datos)
docker compose down -v                    # parar y borrar datos y llaves
```

**Front con hot reload** contra el stack: `cd front && GATEWAY_URL=http://localhost:8080 npm run dev`
(http://localhost:5173).

**Secretos:** todos tienen un valor por defecto **solo para local**. Para
cambiarlos, copia `.env.example` a `.env`. Producción usa los
`docker-compose.prod.yml` de cada feature.

### Fuera del stack local

- **Citofonía** usa un WebSocket de AWS (`VITE_INTERCOM_WS_URL` en `front/.env.production`).
- **Push (FCM)** está apagado sin credenciales de Firebase: las notificaciones
  llegan a la bandeja de la app (ver `back/notificaciones/README.md`).
