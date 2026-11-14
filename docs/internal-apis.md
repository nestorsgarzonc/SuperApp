# APIs internas entre servicios

Endpoints que un servicio expone **solo a otros servicios**, bajo el prefijo
`/internal/`. Reglas comunes:

- Nunca se enrutan desde el `api-gateway` (el gateway responde `404` a `/internal/*`).
- Se autentican con el header `X-Internal-Secret` (`INTERNAL_SECRET`, compartido
  por los servicios de la red privada). Sin él → `401`.
- Respuesta con el mismo sobre que las APIs públicas: `{ "success": true, "data": ... }`.
- Deben ser rápidos (< 100 ms): se llaman en caliente desde otros servicios.

Estado: **implementado**.

## control-de-accesos

### `GET /internal/users/{userId}/complexes/{complexId}/role`

Rol de un usuario en un conjunto.

**Consumidor:** api-gateway, para inyectar `X-Complex-Role` (ver `api-gateway.md`).

```json
200 { "success": true, "data": { "role": "ADMIN" } }   // ADMIN | RESIDENT | GUARD | ...
404 el usuario no pertenece al conjunto
```

El gateway cachea la respuesta 60 s, así que un cambio de rol tarda hasta un
minuto en aplicarse.

## auth

### `GET /.well-known/jwks.json`

Llaves públicas (JWKS) con las que `auth` firma los JWT.

**Consumidor:** api-gateway, para validar tokens localmente.

- Rotación de llaves: publicar la llave nueva en el JWKS **antes** de empezar a
  firmar con ella, y mantener la anterior hasta que expiren sus tokens.
- Ruta pública del ms (no está bajo `/internal`), pero el gateway no la expone.
- Revocación: auth escribe `auth:revoked:sid:<sid>` en Redis (`REDIS_URL`) con
  TTL igual a la vida del access token + 1 min; el gateway rechaza los tokens de
  esas sesiones (claim `sid`). Ver `api-gateway.md#tokens`.

> El monolito firma con HMAC y un secreto compartido (`JWT_SECRET`). Con
> microservicios conviene pasar a una firma asimétrica (RS256/EdDSA): así el
> gateway solo necesita la llave pública y no puede emitir tokens.
