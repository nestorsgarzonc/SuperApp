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
internal/revocation/  sesiones cerradas en auth (auth:revoked:sid:<sid>)
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
go test -race ./...                              # todo, sin dependencias externas
bash scripts/test-unit-gateway.sh                # desde la raíz: un PASS/FAIL por caso, con su ID
bash scripts/test-unit-gateway.sh --cover        # + cobertura por paquete
bash scripts/test-unit-gateway.sh --mutation     # + mutación (gremlins, sobre una copia)
bash scripts/test-unit-gateway.sh --run PL,AN05  # solo esas partes o casos
```

Pruebas unitarias por paquete; cada caso lleva su ID al inicio del nombre
(`TestPL05_…` → PL-05, subcasos `PL-05a…`):

| Prefijo | Paquete | Qué cubre |
|---|---|---|
| AN | `authn` | claims RS256, ataques a JWT, tokens de desarrollo, JWKS (filtrado, rotación, caché) |
| MB | `membership` | respuestas de control-de-accesos, caché positiva/negativa, concurrencia |
| RL | `ratelimit` | bordes del límite y de la ventana, en memoria y en Redis |
| RV | `revocation` | sesiones revocadas en Redis |
| RT, CR | `gateway` | paths seguros, tabla de rutas, IP del cliente, firma del tenant, CORS |
| PL | `gateway` | el pipeline completo con dobles: orden de los controles, cabeceras, límites, fallas |
| AC | `gateway` | las pruebas de aceptación de `docs/api-gateway.md`, con el verificador y las membresías reales |
| AE, CF | `apierr`, `config` | sobre de error y arranque seguro |

Ningún caso necesita red real, base de datos ni Redis: los servicios son
servidores `httptest` falsos y Redis es [miniredis](https://github.com/alicebob/miniredis).
Detalle de casos, dobles, cobertura y hallazgos en
[`docs/pruebas-api-gateway.md`](../../docs/pruebas-api-gateway.md). De punta a
punta: `scripts/smoke.sh` en la raíz.

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
| `REDIS_URL` | | `redis://redis:6379/0`: rate limit compartido y sesiones revocadas por auth. Vacío = límites por instancia y sin revocación inmediata |
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
