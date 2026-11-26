# Pruebas críticas — verificación a nivel de componente

Resumen de las pruebas de los flujos críticos (autenticación, control de acceso y
generación/validación de pases QR) para la entrega 1 de Calidad de Software.
Cada prueba lleva en su nombre el ID del caso.

## Cómo ejecutarlas

```sh
scripts/critical-tests.sh                      # todas las suites, una por tier (requiere Node 20+ y Docker)
cd front && npm run test:critical              # solo tier de presentación
back/control-de-accesos/db/tests/run.sh        # solo tier de datos
```

| Tier | Suite | Casos | Herramientas |
|---|---|---|---|
| Presentación | `front/src/test/critical/auth.critical.test.tsx` | 21 | Vitest, Testing Library, MSW |
| Presentación | `front/src/test/critical/access-control.critical.test.tsx` | 12 | Vitest, Testing Library, MSW |
| Presentación | `front/src/test/critical/qr.critical.test.tsx` | 31 | Vitest, Testing Library, MSW |
| Datos | `back/control-de-accesos/db/tests/critical_data.test.sql` | 33 | PostgreSQL 18 (contenedor desechable), plpgsql |
| **Total** | | **97** | |

Técnicas: **EP** partición de equivalencia · **BVA** valores límite · **TD** tabla de decisión ·
**ST** transición de estados · **EXC** comportamiento excepcional.

## Componentes

| ID | Tier | Componente | Ubicación |
|---|---|---|---|
| C1 | Presentación | `loginSchema` / `passwordSchema` | `front/src/features/auth/schemas/authSchemas.ts` |
| C2 | Presentación | `LoginPage` | `front/src/features/auth/pages/LoginPage.tsx` |
| C3 | Presentación | `login()`, `AuthProvider.signIn`, `api()` / refresh | `features/auth/api/authApi.ts`, `features/auth/components/AuthContext.tsx`, `shared/api/client.ts`, `lib/http.ts` |
| C4 | Presentación | `RequireRole`, `ProtectedRoute`, `permissionsForRole` | `modules/control-de-accesos/routes.tsx`, `features/auth/routes/guards.tsx`, `features/auth/demoDirectory.ts` |
| C5 | Presentación | `grantView` / `rejectText` | `front/src/modules/control-de-accesos/types.ts` |
| C6 | Presentación | `NewAuthorizationPage` + `useQrDataUrl` | `modules/control-de-accesos/pages/resident.tsx`, `useQrDataUrl.ts` |
| C7 | Presentación | `ValidatePassPage` | `front/src/modules/control-de-accesos/pages/guard.tsx` |
| C8 | Datos | Esquema de control-de-accesos (CHECK, triggers, UNIQUE, UPDATE de un solo uso) | `back/control-de-accesos/db/migrations/*.sql` |

## Autenticación (`auth.critical.test.tsx`)

| ID | Comp. | Técnica | Propósito |
|---|---|---|---|
| TC-AUTH-01a–d | C1 | EP / BVA | El login acepta un correo válido (también con espacios alrededor) y rechaza un correo mal formado o una contraseña vacía. |
| TC-AUTH-02a–c | C1 | BVA | La contraseña nueva exige mínimo 8 caracteres: 7 se rechaza; 8 y 9 se aceptan. |
| TC-AUTH-03a–d | C1 | TD | Cada regla de la política (mayúscula, número, símbolo, confirmación) incumplida sola produce exactamente su mensaje. |
| TC-AUTH-04 | C2 | EXC | Un 401 muestra un único mensaje genérico (no revela si el correo existe) y no guarda token. |
| TC-AUTH-05 | C2 | EXC | Un 423 (cuenta bloqueada) lleva a `/account-blocked`. |
| TC-AUTH-06 | C2 | EXC | Un fallo de red muestra el mensaje de conexión con «Intentar nuevamente». |
| TC-AUTH-07 | C2 | EP / EXC | Un correo inválido se rechaza en el cliente sin llamar a la API. |
| TC-AUTH-08 | C3 | EXC | `login()` normaliza el correo y, si `/me/memberships` falla, borra ambos tokens. |
| TC-AUTH-09a–c | C3 | ST | Tras el login: 0 membresías → `/unauthorized`; 1 → `/dashboard` con conjunto activo; varias → `/select-complex`. |
| TC-AUTH-10 | C3 | ST / EXC | Un 401 con refresh fallido cierra la sesión y emite `superapp:session-expired`. |
| TC-AUTH-11 | C3 | EXC | Dos 401 concurrentes comparten una sola llamada de refresh (el refresh token es de un solo uso). |

## Control de acceso (`access-control.critical.test.tsx`)

| ID | Comp. | Técnica | Propósito |
|---|---|---|---|
| TC-AC-01 | C4 | TD (R1) | Sin sesión, una página de portería redirige a `/unauthorized`. |
| TC-AC-02 | C4 | TD (R2) | Un residente no entra a una página de portería aunque tenga el permiso. |
| TC-AC-03 | C4 | TD (R3) | Un guarda sin el permiso requerido es rechazado. |
| TC-AC-04 | C4 | TD (R4) | Un guarda con rol y permiso ve la página. |
| TC-AC-05 | C4 | TD (R4) | Un administrador con rol y permiso ve la página. |
| TC-AC-06 | C4 | EXC | Un anónimo en una ruta protegida va a `/login` recordando la ruta de origen. |
| TC-AC-07a–c | C4 | TD | `ProtectedRoute`: falta el permiso → `/unauthorized`; rol fuera de la lista → `/unauthorized`; ambos correctos → contenido. |
| TC-AC-08 | C4 | EP | Separación de funciones: el residente crea pases y no valida; el guarda valida y no crea; el administrador hace ambas cosas. |
| TC-AC-09 | C4 | EP | Toda petición con sesión envía `Authorization: Bearer` y `X-Complex-ID`; con `auth:false` no envía ninguna. |
| TC-AC-10 | C4 | EXC | Un 403 llega como `ApiError` con su código y no cierra la sesión. |

## Pases QR (`qr.critical.test.tsx`)

| ID | Comp. | Técnica | Propósito |
|---|---|---|---|
| TC-QR-01a–f | C5 | TD | Estado visible del pase: CANCELLED, USED, EXPIRED, aún no iniciado, vigente y vencido. |
| TC-QR-02a–e | C5 | BVA | Fronteras a ±1 ms de `valid_until` y `valid_from` (vencido solo si es estrictamente anterior a ahora). |
| TC-QR-03 (×7) | C5 | EP | Un mensaje exacto por cada motivo de rechazo; `null` sin motivo. `NOT_FOUND` no revela que el código exista en otro conjunto. |
| TC-QR-04 | C6 | EP | `useQrDataUrl` genera un PNG distinto por código y `null` para un código vacío. |
| TC-QR-05 | C6 | EP / EXC | Un nombre en blanco se rechaza y no se envía nada. |
| TC-QR-06a–c | C6 | BVA | Hora fin anterior al inicio, igual al inicio (límite) o ya pasada se rechazan. |
| TC-QR-07 | C6 | EXC | Una pre-autorización exige el documento del visitante. |
| TC-QR-08 | C6 | BVA / EP | Un pase de 1 minuto se envía con la ventana ISO y el nombre recortado, y abre el pase creado. |
| TC-QR-09 | C6 | ST / EXC | Si el breaker se abre al generar (`QR_001`), pasa a «Por documento» y lo explica. |
| TC-QR-10 | C6 | ST | Con el sistema QR caído desde el inicio, la opción QR aparece deshabilitada. |
| TC-QR-11 | C7 | EP | La portería envía el código recortado con el conjunto del guarda y muestra «✓ Pase válido». |
| TC-QR-12 | C7 | ST | Un segundo escaneo de un pase de un solo uso muestra `ALREADY_USED`. |
| TC-QR-13 | C7 | EXC | Un código vacío se rechaza sin llamar a la API. |
| TC-QR-14 | C7 | ST / EXC | Con el lector QR caído (`QR_001`) cambia a validación por documento. |

## Tier de datos (`critical_data.test.sql`)

| ID | Técnica | Propósito |
|---|---|---|
| TC-DB-01a–d | TD | `method × qr_code_hash`: un pase QR exige código; PRE_AUTH no; un método desconocido se rechaza. |
| TC-DB-02a–d | EP / EXC | `status` y `visitor_type` solo admiten sus enumeraciones (sensibles a mayúsculas); `valid_until` es obligatorio. |
| TC-DB-03 | EP | Un pase nuevo queda ACTIVE, de un solo uso y vigente desde su creación. |
| TC-DB-04a–f | ST | El UPDATE condicional de `TryMarkGrantUsed` consume el pase una sola vez; no consume un pase usado, cancelado o inexistente. |
| TC-DB-05a–e | EXC | `audit_log` es append-only: se inserta, pero UPDATE y DELETE fallan y la fila queda intacta. |
| TC-DB-06a–c | EP / EXC | La bitácora es idempotente por `(complex_id, client_id)`: un reintento offline no duplica el registro. |
| TC-DB-07a–e | TD | Regla de sujeto: un residente requiere usuario, solo ENTRY/EXIT y siempre AUTHORIZED. |
| TC-DB-08a–c | EP / EXC | El personal solo puede ser GUARD o ADMIN, sin duplicados. |
| TC-DB-09 | EP | Borrar una unidad elimina sus membresías (no quedan accesos huérfanos). |

## Aislamiento

- **Red:** handlers de MSW declarados en cada prueba; cualquier petición no declarada hace fallar la prueba.
- **Sesión:** los contextos de autenticación se reemplazan con `vi.mock` en control de acceso y QR.
- **Reloj:** solo se falsifica `Date` (fijado al 2026-11-21 10:00 hora local); `grantView` recibe `now` como parámetro.
- **Datos:** PostgreSQL desechable con solo las migraciones; cada caso corre en su propia subtransacción y todo termina en `ROLLBACK`.

## Mutación manual

| Mutante | Prueba que lo detecta |
|---|---|
| `grantView`: `< now` → `<= now` | TC-QR-02b |
| `RequireRole`: se quita la verificación del permiso | TC-AC-03 |
| `NewAuthorizationPage`: `end <= start` → `end < start` | TC-QR-06b |
| Se elimina el CHECK `qr_required_if_method_qr` | TC-DB-01b |
| El trigger de `audit_log` deja pasar los cambios | TC-DB-05b, 05c, 05d |

5 de 5 mutantes detectados.

## Hallazgos

- La política de contraseñas del front (8 o más caracteres con mayúscula, número y símbolo) no coincide con la de auth (12 a 72 bytes).
- `activateComplex()` es un stub de demostración: no confirma el conjunto con el servidor.
- Un pase no puede cruzar la medianoche (inicio y fin comparten la fecha).
- `Field` no asocia la etiqueta con su input (impide `getByLabelText` y afecta la accesibilidad).
- `qr_code_hash` solo tiene un índice, no UNIQUE: la base no impide dos pases con el mismo código.
- No se encontraron defectos funcionales: los 97 casos pasan.
