# Guía de pruebas — API gateway

Pruebas unitarias del gateway (`back/api-gateway`): qué verifican, cómo se
aíslan, cómo se ejecutan y qué resultados dan. Ninguna necesita red real, base
de datos ni Redis: los microservicios son servidores `httptest` falsos y Redis
es [miniredis](https://github.com/alicebob/miniredis). El contrato que se
verifica es [`api-gateway.md`](api-gateway.md).

## 1. Ejecutar

Desde la raíz del repositorio (Go 1.24+):

| Comando | Qué hace |
|---|---|
| `bash scripts/test-unit-gateway.sh` | Todas las pruebas con `-race`; una línea por caso con su ID y una tabla por parte |
| `bash scripts/test-unit-gateway.sh --cover` | Agrega la cobertura de sentencias por paquete |
| `bash scripts/test-unit-gateway.sh --mutation` | Mutación con gremlins v0.6.0 sobre una copia del módulo (el repo no se toca) |
| `bash scripts/test-unit-gateway.sh --run PL,AN05` | Solo esas partes o casos |
| `cd back/api-gateway && go test -race ./...` | Lo mismo sin el informe por caso |

La salida termina con `VEREDICTO: PASS` o `VEREDICTO: FAIL`.

## 2. Casos

Cada prueba lleva su ID al inicio del nombre (`TestPL05_…` → PL-05) y un
comentario con la técnica, la entrada y el resultado esperado. Las filas de una
tabla se llaman `PL-05a`, `PL-05b`… y cuentan como casos. **369 casos.**

| IDs | Parte | Qué se verifica | Técnica | Casos |
|---|---|---|---|---|
| AN-01 a AN-04 | Autenticación | Token RS256 válido; cada claim inválido (vencido, otro emisor o audiencia, sin `sid`, `sub` no UUID…); otra clave, `kid` ausente o desconocido, payload alterado, `alg: none`, confusión HS256/RSA; tokens de desarrollo | Tabla de decisión, valores límite (margen de 5 s), adivinación de errores | 41 |
| AN-05 a AN-10 | Claves de auth (JWKS) | Claves inválidas descartadas (2048 bits, exponente 3…2³¹−1); JWKS caído o vacío; carga perezosa; rotación sin que un `kid` inventado golpee a auth; caché vencida con auth caído | Partición, valores límite, transición de estados | 25 |
| RV-01 a RV-04 | Sesiones cerradas | Clave exacta que escribe auth, vencimiento por TTL, Redis caído | Partición, transición de estados | 5 |
| MB-01 a MB-05 | Conjunto y rol | Secreto interno, rol normalizado, 404 = no miembro; rol vacío, JSON inválido, 401, 5xx o servicio caído fallan cerrado | Tabla de decisión, adivinación de errores | 14 |
| MB-06 a MB-14 | Conjunto y rol | Caché positiva y negativa con sus bordes, errores sin caché, caché por pareja, 20 consultas simultáneas = 1 llamada, llamador cancelado, límite de 100 000 entradas, URL inválida | Transición de estados, valores límite | 11 |
| RL-01 a RL-10 | Límites | Borde n/n+1, límite 1, claves independientes, último nanosegundo de la ventana, Retry-After, vencimiento en Redis, Redis caído, limpieza, concurrencia (memoria y Redis) | Valores límite, transición de estados | 16 |
| RT-01, RT-02 | Enrutamiento | Paths con `..`, `//`, `\`, NUL o codificados; cualquier segmento `internal` | Partición, adivinación de errores | 27 |
| RT-03 a RT-06 | Enrutamiento | Path → servicio por segmentos, prefijo de notificaciones, rutas públicas de auth y de `/dev` | Tabla de decisión, valores límite | 42 |
| RT-07, RT-08 | Enrutamiento | IP y protocolo del cliente con y sin proxy de confianza; firma del tenant contra un vector de openssl | Tabla de decisión, partición | 12 |
| CR-01, CR-02 | CORS | Origen permitido, ajeno, comodín, preflight y petición simple; nunca credenciales | Tabla de decisión | 13 |
| PL-01, PL-02 | Pipeline | Cada cabecera falsificada se descarta; token ausente o mal presentado = 401 sin consultar nada más | Partición, tabla de decisión | 23 |
| PL-03 a PL-05 | Pipeline | Revocación, límite por IP o por usuario, conjunto ausente / mal formado / normalizado / ajeno / membresías caídas | Tabla de decisión | 21 |
| PL-06 a PL-08 | Pipeline | Secreto propio de cada servicio, tenant firmado solo hacia auth, rutas públicas sin identidad | Tabla de decisión | 8 |
| PL-09 a PL-18 | Pipeline | Cuerpo 100/101 bytes, timeout (504), servicio caído (503), cabeceras de respuesta, X-Request-ID (1/128/129), `/healthz`, orden de los controles, log de acceso, streaming, cliente que cancela, errores del servicio tal cual | Valores límite, adivinación de errores | 25 |
| AC-01 a AC-17 | Aceptación | Pruebas de aceptación de [`api-gateway.md`](api-gateway.md) con el verificador y las membresías reales contra servicios falsos | Tabla de decisión | 17 |
| AE-01, AE-02 | Errores | Sobre de cada código; Retry-After redondeado, mínimo 1 | Partición, valores límite | 15 |
| CF-01 a CF-09 | Configuración | Defaults, URLs y secretos obligatorios, HMAC de 31/32 bytes, números 0/−1/texto/1, opciones de desarrollo solo con `ENV=development` exacto | Partición, valores límite | 54 |

## 3. Dobles de prueba

| Dependencia | Doble | Tipo | Cómo |
|---|---|---|---|
| JWKS de auth | `fakeJWKS` | Stub y spy | `httptest` que publica las claves o el status de cada prueba y cuenta descargas |
| Membresías (control-de-accesos) | `fakeCA` | Stub y spy | `httptest` que responde 200/404/4xx/5xx o JSON inválido, guarda la petición y puede retener la respuesta |
| Redis | miniredis | Fake | Redis en memoria; su reloj se adelanta para probar vencimientos |
| Verificador de tokens | `stubVerifier` | Stub y spy | Acepta un token fijo y cuenta llamadas |
| Roles | `spyRoles` | Stub y spy | Rol, "no miembro" o error; guarda cada consulta |
| Revocación | `spyRevocations`, `fakeRevocations` | Stub y spy | Revocada, vigente o error; cuenta consultas |
| Limitador | `stubLimiter` | Stub y spy | Permite, niega con Retry-After o falla; guarda clave y límite |
| Microservicios | `upstream` | Fake y spy | `httptest` que responde como un servicio y guarda lo recibido |
| Reloj | `fixedClock`, `clock`, `now` fijo | Stub | Función de hora inyectada en limitador, caché de membresías y gateway |
| Logger | `logSpy` / `io.Discard` | Spy / Dummy | `slog.Handler` que guarda los registros / descarta |

## 4. Adecuación

| Paquete | Cobertura | Mutantes muertos / vivos |
|---|---|---|
| `gateway` | 100 % | 44 / 3 |
| `authn` | 100 % | 35 / 2 |
| `membership` | 100 % | 15 / 1 |
| `ratelimit` | 100 % | 16 / 0 |
| `config` | 100 % | 38 / 0 |
| `revocation` | 100 % | 3 / 0 |
| `apierr` | 100 % | 2 / 1 |
| **Total** | **100 % (474/474)** | **153 / 7 (95,6 %)** |

Los 7 mutantes vivos se dividen así:
- **2 equivalentes:** `secs < 1` frente a `<= 1` en Retry-After, y el mínimo de 1 s de la caché negativa. El resultado no cambia.
- **2 en el instante exacto** en que vence la caché del JWKS (10 min) y en el intervalo de refresco (30 s). Usan `time.Since`, así que hace falta un reloj inyectable para fijarlos.
- **3 en los tiempos de conexión** del proxy (dial, keep-alive y conexiones inactivas). Son ajustes sin efecto observable en una prueba unitaria.

**No se prueba en este nivel:**
- El arranque (`cmd/gateway`): se cubre con `scripts/smoke.sh`.
- El límite compartido entre varias réplicas con un Redis real: es una prueba de integración.

## 5. Hallazgos

Solo están documentados; no se modificó código.

| ID | Esperado | Observado |
|---|---|---|
| G1 | El esquema `Bearer` no distingue mayúsculas (RFC 7235) | `Authorization: bearer <token>` responde 401 sin verificar el token |
| G2 | Los tokens inválidos cuentan para algún límite (por IP) | No consumen ningún límite: 50 tokens inválidos seguidos dan 50 respuestas 401 |
| G3 | Una petición abandonada por el cliente se registra como abortada (p. ej. 499) | El log de acceso la muestra con status 200 |
| G4 | `/healthz` indica si hay claves de auth cargadas | Responde `ok` sin claves; `Verifier.Ready()` no se usa |
| G5 | Respuestas de los servicios "sin modificar, salvo CORS" ([`api-gateway.md`](api-gateway.md)) | También se reemplaza su `X-Request-ID` por el del gateway |
