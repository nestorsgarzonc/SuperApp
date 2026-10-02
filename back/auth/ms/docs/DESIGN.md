# Diseño inicial — Auth MS

Estado: diseño aplicado parcialmente en el servicio nuevo bajo `back/auth`. Go 1.27.1, PostgreSQL 17, `net/http`, `database/sql` con pgx, SQL versionado, bcrypt y JWT RS256 ya están seleccionados e implementados. Este servicio no toma código, dependencias ni modelo físico del monolito. Los contratos de producción con el API Gateway siguen pendientes.

## 1. Responsabilidades y límites

Auth es responsable de credenciales e identidad mínima, estado de usuario, sesiones, emisión/renovación/revocación de tokens y administración de roles y permisos. Su base de datos es exclusiva del servicio. Otros servicios no leen sus tablas: consultan APIs o validan tokens según el contrato acordado.

Auth puede guardar el identificador opaco del conjunto para delimitar una asignación. La existencia y estado del conjunto pertenecen al dominio de otro servicio; Auth no replica su catálogo ni es dueño de esos datos. El API Gateway aporta o valida el contexto de tenant conforme al contrato por definir.

Fuera de este diseño: auditoría, broker, consumidores de eventos, frontend, servicios de negocio y despliegue cloud completo.

## 2. Capas y dependencias

```text
HTTP/API → casos de uso → dominio
                 ↑           ↑
          adaptadores de infraestructura
                 ↓
       persistencia propia de Auth
```

- **Presentation:** validación sintáctica, DTOs, conversión de errores a HTTP y rutas; sin reglas de negocio ni SQL.
- **Application:** casos de uso de login, renovación, revocación, administración de identidad y RBAC; coordina puertos y transacciones.
- **Domain:** invariantes de usuario, sesión, rol y permiso; contratos internos sin dependencia del framework.
- **Infrastructure:** adaptadores SQL, hashing de credenciales, emisión/verificación de tokens, configuración y OIDC cuando se acuerde el protocolo.

## 3. Modelo conceptual y relaciones

```text
User 1 ── * Session 1 ── * RefreshToken (historial de rotación)
User * ── * Role       (UserRole; asignación global o a tenant)
Role * ── * Permission (RolePermission)
```

| Entidad | Datos conceptuales | Invariantes |
|---|---|---|
| User | id, email normalizado, password hash, estado, fechas | Email normalizado único; credencial nunca se devuelve; solo estado habilitado inicia sesión. |
| Session | id, user id, metadata de dispositivo opcional, expiración, revocación, fechas | Una sesión revocada no puede renovarse; cerrar sesión invalida su familia de refresh tokens. |
| RefreshToken | id, session id, hash, expiración, revocación, reemplazo, fechas | Token aleatorio de alta entropía; solo hash persistido; rotación de un solo uso; detectar replay y revocar la sesión afectada. |
| Role | id, nombre estable, descripción, alcance (sistema/tenant), estado | Identificador/nombre único dentro de su alcance; roles asignables solo en el alcance permitido. |
| Permission | id, código estable, descripción | Código único; semántica definida por Auth/API y no editable libremente por clientes. |
| UserRole | user id, role id, tenant id nullable, asignador opcional, fechas | El alcance debe coincidir con el rol; revocación elimina efecto inmediatamente. |
| RolePermission | role id, permission id | Asociación única; cambios afectan permisos efectivos en la siguiente evaluación/token según estrategia elegida. |

El identificador del tenant es una referencia opaca sin clave foránea entre servicios. No se almacenan datos del conjunto. Para tenant roles, `tenant_id` es obligatorio; para roles del sistema es nulo. Las restricciones de unicidad con nulos dependen del motor que se seleccione y se concretarán en la migración.

Las sesiones son independientes por dispositivo. Se almacenan hashes de refresh tokens rotados para detectar reutilización; no se persisten access tokens. La política de retención del historial y el límite de sesiones simultáneas requieren una decisión de producto.

## 4. Casos de uso

1. Registrar usuario (si el alta pública forma parte del flujo aprobado) y autenticar credenciales.
2. Emitir access token y crear sesión/refresh token.
3. Renovar sesión transaccionalmente: validar token y estado del usuario/sesión, invalidar el token presentado, registrar el siguiente token y emitir access token.
4. Cerrar una sesión o todas las sesiones del usuario.
5. Consultar identidad propia, sesiones, roles y permisos efectivos.
6. Administrar estado de usuario, roles, permisos y asignaciones con autorización administrativa.
7. Evaluar un permiso para usuario y tenant explícitos, verificando que la asignación esté vigente y sea aplicable al tenant.

Las operaciones de autorización usan denegación por defecto. Un rol global y uno asignado a tenant no se mezclan implícitamente; los permisos efectivos se resuelven para el contexto solicitado.

## 5. API propuesta (contrato por versionar)

Base implementada: `/api/v1`. Los DTOs viven en `internal/presentation/dto/`; las rutas de lectura y escritura están separadas en `internal/presentation/reader/` y `internal/presentation/writer/`.

| Operación | Ruta propuesta | Acceso |
|---|---|---|
| Registro | `POST /auth/register` | Público solo si el flujo de altas lo aprueba. |
| Login | `POST /auth/login` | Público; credenciales inválidas con respuesta genérica. |
| Renovar | `POST /auth/refresh` | Presentar refresh token; rotación de un solo uso. |
| Logout | `POST /auth/logout` | Sesión autenticada o refresh token, según contrato acordado. |
| Logout global | `POST /auth/logout-all` | Usuario autenticado; revoca todas sus sesiones. |
| Identidad propia | `GET /users/me` | Usuario autenticado. |
| Sesiones propias | `GET /sessions`, `DELETE /sessions/{sessionId}` | Solo propietario; revocación de una sesión. |
| Roles efectivos | `GET /users/me/roles` | Usuario autenticado; acepta contexto tenant validado. |
| Permisos efectivos | `GET /users/me/permissions` | Usuario autenticado; tenant explícito y validado cuando aplique. |
| Evaluación de permiso | `POST /permissions/evaluate` | Consumidores internos autorizados o usuario autenticado bajo política acordada. |
| Administración de usuarios/roles/permisos | `/admin/users`, `/admin/roles`, `/admin/permissions` y asignaciones | Permiso administrativo explícito; alcance system/tenant protegido. |

No se fija todavía si operaciones administrativas se servirán en el mismo API, si habrá registro público, ni el esquema de tenant en headers/claims. El Gateway y Auth deben compartir una fuente verificable para ese contexto; un valor enviado por el cliente sin validación no autoriza acceso.

## 6. Seguridad: requisitos de diseño

- Hash de contraseñas con algoritmo de derivación adaptativo reconocido y parámetros configurables; nunca cifrado reversible ni logs con credenciales.
- Tokens aleatorios de refresh con alta entropía; almacenar solo hashes. Rotar en cada uso y detectar reutilización de tokens consumidos.
- TLS en tránsito. Access tokens breves; duración y almacenamiento del lado cliente se concretan por plataforma.
- Preferir claves asimétricas y publicar únicamente claves públicas para validadores del Gateway (por ejemplo JWKS) si Auth es emisor OIDC. Algoritmo, rotación de claves y claims se acuerdan antes de implementar.
- Claims mínimos: sujeto, emisor, audiencia, expiración, emisión e identificador de token; tenant/roles solo si se decide cómo mantenerlos frescos y verificables. No incluir información sensible.
- Validar emisor, audiencia, firma, expiración y algoritmo permitido. Nunca aceptar el algoritmo anunciado por el token sin lista permitida.
- Respuestas de login no revelan si la cuenta existe. Limitar intentos de login, registro, refresh y administración; valores por endpoint y almacenamiento distribuido quedan por decidir.
- Autorización de roles/permisos en servidor y dentro de cada tenant; no confiar en rol o tenant remitidos por el cliente.
- Cambiar contraseña, deshabilitar usuario o revocar roles debe invalidar refresh tokens; la latencia de invalidación de access tokens requiere decidir entre expiración breve, introspección o mecanismo de revocación.
- Secrets solo por configuración segura del entorno/secret manager; nunca en repositorio, seeds, imágenes o logs.

## 7. Integración de tokens y Gateway

La primera implementación hace a Auth emisor de JWT RS256 con `iss`, `sub`, `aud`, `iat`, `nbf`, `exp` y `sid`, y publica su clave pública vigente en `/.well-known/jwks.json`. Los permisos no se incrustan en el JWT: Auth consulta PostgreSQL al evaluar autorización, evitando permisos obsoletos en tokens de hasta 15 minutos. Refresh tokens aleatorios de 256 bits se guardan como SHA-256, rotan en cada uso y se invalidan en logout/replay.

El contexto tenant requiere que el Gateway autentique al usuario y compruebe su pertenencia al conjunto. Después envía `X-Tenant-ID`, timestamp Unix en `X-Gateway-Timestamp` y HMAC-SHA256 en hex por `X-Gateway-Signature` sobre la cadena UTF-8 `userId + LF + tenantId + LF + timestamp`. Auth valida el HMAC y la ventana de ±60 segundos. El secreto compartido debe tener al menos 32 bytes y no debe exponerse a clientes.

Para asignar roles a otra identidad dentro de un tenant, el Gateway debe verificar también la membresía del usuario objetivo y enviar `X-Gateway-Membership-Timestamp` junto con `X-Gateway-Membership-Signature`, HMAC-SHA256 sobre `actorUserId + LF + targetUserId + LF + tenantId + LF + timestamp`. Auth rechaza la asignación si falta o no valida esta prueba. Esta afirmación firmada es un contrato requerido del Gateway: Auth no consulta la base de datos de otro servicio ni infiere membresías.

Los access tokens emitidos antes de logout o logout-all siguen siendo criptográficamente válidos hasta `exp`; la verificación local actual no consulta estado de sesión. El TTL breve limita, pero no elimina, esa ventana de revocación.

Esto no convierte aún al servicio en un proveedor OIDC/OAuth 2.0 completo. No están implementados authorization code + PKCE, clientes OAuth, consentimientos ni discovery OIDC; JWKS solo permite verificar JWT. Debe acordarse si Auth completará OIDC o si se conectará a un Identity Provider antes de integrarse como tal en el Gateway.

## 8. Migraciones, seeds y pruebas

- Migraciones versionadas y ejecutadas por `ms/cmd/setup`; cada archivo y seed se aplica en transacción con advisory lock y queda registrado en PostgreSQL.
- Seeds idempotentes solo para permisos base aprobados; no crear usuarios admin ni contraseñas por defecto.
- Pruebas unitarias de invariantes y casos de uso; pruebas de persistencia/transacción para rotación y revocación; pruebas HTTP para respuestas y autorización; casos negativos de cross-tenant, cuenta inactiva y replay de refresh token.
- El motor y los comandos ya están seleccionados; las pruebas de persistencia con PostgreSQL quedan pendientes porque este entorno no tiene `docker` ni `psql`.

## 9. Pendientes de integración y producto

1. Contrato OAuth/OIDC y validación de tokens con el API Gateway; esta implementación ofrece login propio y JWKS, no un servidor OIDC completo.
2. Acordar el secreto/firma del tenant context con el Gateway y mantener Auth en red privada. El Gateway debe validar membresía usuario–conjunto antes de firmar contexto.
3. Registro es administrativo; alta pública/invitación, verificación de email y recuperación de contraseña no se implementan en este alcance inicial.
4. Rotación de claves RSA/JWKS en producción, almacenamiento en secret manager y procedimiento de rollover.
5. Rate limiting y protección de credential stuffing corresponden al Gateway en esta fase; no exponer Auth directamente a Internet.
6. Acceso directo con PostgreSQL no está cubierto por pruebas de integración en este entorno (no hay PostgreSQL ni Docker disponibles).

Las decisiones seleccionadas de stack/modelo se documentan además en `README.md`; este diseño deja visibles los límites restantes en lugar de afirmar compatibilidad OIDC que aún no existe.
