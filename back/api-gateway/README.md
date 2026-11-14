# api-gateway

Punto de entrada único entre el front y los microservicios. Contrato (rutas,
headers, errores): [`docs/api-gateway.md`](../../docs/api-gateway.md).

Go 1.24+, `net/http` + `httputil.ReverseProxy`, sin base de datos. Redis solo
para el rate limiting compartido.

```
cmd/gateway/          main (+ -healthcheck para el HEALTHCHECK del contenedor)
internal/config/      variables de entorno; rechaza combinaciones inseguras
internal/gateway/     tabla de rutas, pipeline de la petición, CORS
internal/authn/       verificación RS256 (JWKS) y HS256 de desarrollo
internal/membership/  rol por conjunto (control-de-accesos /internal), con caché
internal/ratelimit/   ventana fija en Redis o en memoria
internal/apierr/      errores con el sobre {success,error:{code,message}}
```

## Ejecutar

Todo el stack: `docker compose up --build` en la raíz del repo. Solo el gateway
contra servicios ya levantados:

```sh
ENV=development AUTH_URL=http://localhost:8081 AUTH_JWT_ISSUER=http://localhost:8081 \
CONTROL_ACCESOS_URL=http://localhost:8082 CONTROL_ACCESOS_SECRET=... \
NOTIFICACIONES_URL=http://localhost:8083 NOTIFICACIONES_SECRET=... \
INTERNAL_SECRET=... go run ./cmd/gateway
```

## Pruebas

```sh
go test -race ./...
```

Cubren las pruebas de aceptación de `docs/api-gateway.md` contra servicios
falsos (`httptest`): suplantación de headers, rol real, conjunto ajeno, tokens
inválidos, `/internal` y path traversal, fail-closed, rate limit, CORS, body.

## Configuración

| Variable | Obligatoria | Descripción |
|---|---|---|
| `ENV` | | `production` (por defecto) o `development` |
| `PORT` | | `8080` |
| `AUTH_URL` | sí | p. ej. `http://auth:8081` |
| `AUTH_JWKS_URL` | | por defecto `AUTH_URL/.well-known/jwks.json` |
| `AUTH_JWT_ISSUER` | sí | igual a `JWT_ISSUER` de auth |
| `AUTH_JWT_AUDIENCE` | | `superapp-api` |
| `AUTH_TENANT_HMAC_SECRET` | | igual a `GATEWAY_TENANT_HMAC_SECRET` de auth (≥ 32 bytes). Vacío = no se envía tenant a auth |
| `CONTROL_ACCESOS_URL` / `CONTROL_ACCESOS_SECRET` | sí | destino y `GATEWAY_SECRET` de control-de-accesos |
| `NOTIFICACIONES_URL` / `NOTIFICACIONES_SECRET` | sí | destino y `GATEWAY_SECRET` de notificaciones |
| `INTERNAL_SECRET` | sí | `INTERNAL_SECRET` de control-de-accesos (consulta de rol) |
| `REDIS_URL` | | `redis://redis:6379/0`. Vacío = límites por instancia |
| `CORS_ALLOWED_ORIGINS` | | lista separada por comas. `*` solo en desarrollo |
| `TRUST_FORWARDED_FOR` | | `true` detrás de nginx/ALB (IP = última entrada de `X-Forwarded-For`) |
| `RATE_LIMIT_PUBLIC_PER_MIN` / `RATE_LIMIT_USER_PER_MIN` | | `10` / `300` |
| `MAX_BODY_BYTES` | | `1048576` |
| `UPSTREAM_TIMEOUT_SECONDS` | | `30` |
| `ROLE_CACHE_TTL_SECONDS` | | `60` (las respuestas negativas, 1/6 de eso) |
| `DEV_HS256_SECRET` | | solo `ENV=development`: acepta tokens de `/dev/token` (= `JWT_SECRET` de control-de-accesos) |
| `EXPOSE_DEV_ROUTES` | | solo `ENV=development`: enruta `/api/v1/dev/*` |

Fuera de `ENV=development`, `DEV_HS256_SECRET`, `EXPOSE_DEV_ROUTES` y
`CORS_ALLOWED_ORIGINS=*` hacen que el gateway **no arranque**.
