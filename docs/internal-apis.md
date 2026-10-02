# APIs internas entre servicios

Endpoints que un servicio expone **solo a otros servicios**, bajo el prefijo
`/internal/`. Reglas comunes:

- Nunca se enrutan desde el `api-gateway` (el gateway responde `404` a `/internal/*`).
- Se autentican con el header `X-Internal-Secret` (`INTERNAL_SECRET`, compartido
  por los servicios de la red privada). Sin él → `401`.
- Respuesta con el mismo sobre que las APIs públicas: `{ "success": true, "data": ... }`.
- Deben ser rápidos (< 100 ms): se llaman en caliente desde otros servicios.

Estado: **por implementar**. Los consumidores están especificados.

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

### `GET /internal/.well-known/jwks.json`

Llaves públicas (JWKS) con las que `auth` firma los JWT.

**Consumidor:** api-gateway, para validar tokens localmente.

- Rotación de llaves: publicar la llave nueva en el JWKS **antes** de empezar a
  firmar con ella, y mantener la anterior hasta que expiren sus tokens.
- Revocación: `auth` escribe `auth:revoked:<jti>` en Redis con TTL igual al
  tiempo que le queda al token.

> El monolito firma con HMAC y un secreto compartido (`JWT_SECRET`). Con
> microservicios conviene pasar a una firma asimétrica (RS256/EdDSA): así el
> gateway solo necesita la llave pública y no puede emitir tokens.
