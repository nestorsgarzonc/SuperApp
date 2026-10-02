# front

SPA única (React + Vite + TypeScript) con un módulo por feature. Hoy solo
`src/modules/control-de-accesos`. Diseños de referencia en `docs/figma/`.

```bash
cd front
npm install
npm run dev        # http://localhost:5173, /api -> GATEWAY_URL (def. localhost:8081)
npm run build      # typecheck + build
```

Requiere el ms de control-de-accesos arriba (`back/control-de-accesos`,
`docker compose up --build`) con `ENV=development` y compilado con `-tags dev` (docker compose ya lo hace); login de pruebas vía `/dev/token`.

## Pantallas por rol

| Rol | Pantallas |
|---|---|
| `RESIDENT` | Inicio, Autorizaciones (tabs), Nueva autorización, Pase de acceso (QR, compartir/descargar/revocar) |
| `GUARD` | Inicio de portería (estado online/offline), Validar pase, Registrar visitante, Registrar ingreso/salida, Historial |
| `ADMIN` | Panel (resumen de accesos), Historial |

## Entradas/salidas

`modules/control-de-accesos/accessLog.ts` envía a `POST /access-logs` con cola
offline en `localStorage` (idempotente por `client_id`) y lee `GET /access-logs`.
Cada registro en cola queda atado al **conjunto y a la persona** que lo creó y
solo se envía con esa sesión.

## Pruebas manuales / entrada de desarrollo

`modules/auth` **no se toca** (es del módulo de autenticación). Mientras no
exista, `modules/control-de-accesos/dev/` ofrece una entrada de pruebas:
personas y conjuntos salen de `GET /dev/users` del ms (solo `ENV=development`),
se elige conjunto si la persona tiene varios ("Selecciona tu conjunto") y el
chip del encabezado permite cambiarlo. Al integrar auth se borra esa carpeta y
`main.tsx` pasa a usar el login real; el resto del front solo depende de
`useAuth()` / `Session`.

En dev, `/api` se redirige a `GATEWAY_URL` o, mientras no exista, a `ACCESS_URL`
(`.env.example`).

## Pruebas

Guía completa (entorno, baterías y flujo manual): [`docs/pruebas-control-de-accesos.md`](../docs/pruebas-control-de-accesos.md).
El panel 🛠 (solo dev, esquina inferior) simula la caída del sistema QR.

## E2E

```bash
npm run e2e   # requiere ms (ENV=development) + `npm run dev` + Edge/Chrome (E2E_CHANNEL)
```

`flow` (residente → portería → admin, offline), `multi-complex` (selección de conjunto, pre-autorización,
aislamiento, Andrés residente/guardia), `building` (entrada/salida de residentes y presencia) y
`qr-breaker` (caída del sistema QR y fallback; requiere el ms con `QR_CALL_TIMEOUT_MS=300 QR_BREAKER_OPEN_SECONDS=8`). Reiniciar los datos de
prueba antes de correr.

## Pendiente

- **Login real**: lo aporta el módulo `auth`; hoy solo la entrada de pruebas.
- **Gateway**: el JWT de auth es RS256 y el ms de accesos valida HS256 con
  `role`/`complex_id`; el gateway debe traducirlo (y `complex_staff` debe
  sincronizarse desde quien gestione roles).
- Escaneo de cámara en "Validar pase": por ahora solo código manual o documento.
- Citofonía vive en su propio módulo (no en control de accesos).
