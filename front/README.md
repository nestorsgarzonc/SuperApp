# SuperApp · Frontend de autenticación

Aplicación React SPA modular de SuperApp, comunicación y gestión residencial. El módulo de autenticación integrado respeta la estructura existente de `src/shared` y `src/modules/control-de-accesos`, y deja la navegación y las pantallas de control de accesos dentro del `AppShell` de `main` branch. Requiere Node.js 20 o posterior.

## Inicio rápido

```sh
npm install
npm run dev
```

El modo de demostración de autenticación usa MSW en el navegador. Copia `.env.example` como `.env.local` y conserva `VITE_USE_MOCK_API=true`. Credenciales de demo: `demo@superapp.co` / `Demo123!`. Cinco intentos fallidos llevan a la pantalla de cuenta bloqueada por 15 minutos. Para apuntar a un servicio, configura `VITE_API_BASE_URL` y desactiva el mock. El módulo de control de accesos sigue usando el API Gateway (`GATEWAY_URL`) o `ACCESS_URL` como proxy de desarrollo, igual que en `main`.

Comandos: `npm run build`, `npm test` y `npm run lint`.

## Citofonía (señalización WebSocket)

### Ejecutar solo Citofonía, sin backend local

Desde `front/` ejecuta:

```powershell
npm run dev:citofonia
```

Abre la URL que imprima Vite (normalmente `http://localhost:5173`). Verás
**Citofonía · Prueba**, sin pasar por `/dev-login` ni introducir contraseñas.
Abre un navegador/perfil como **Portería** y otro como **Apartamento 4**.
Espera a que ambos indiquen **Conectado**; desde Portería selecciona Apartamento
4 y pulsa **Llamar**. En el apartamento acepta o rechaza y prueba finalizar
desde cualquiera de los dos. No uses simultáneamente otros clientes `wscat`
con esas identidades si no conoces la política de entrega a conexiones múltiples.

Este comando carga `.env.intercom-demo`: sesiones temporales en memoria y el
directorio `PORTERIA` / `APT_4` de prueba, pero usa el WebSocket real de AWS.
No llama al backend de usuarios, accesos, autenticación ni notificaciones.
La navegación queda limitada a Citofonía; cerrar sesión vuelve al selector y
recargar la página elimina la sesión temporal. Necesitas Internet y el endpoint
AWS disponible; no es una simulación offline.

Si ya ejecutas `npm run dev`, detenlo primero con Ctrl+C y arranca el comando
de prueba. No hace falta editar `.env.production`. Para volver a la aplicación
normal, detén este proceso y ejecuta `npm run dev`.
El modo requiere tanto `import.meta.env.DEV` como `VITE_INTERCOM_DEMO=true`;
no habilita sesiones de prueba en builds de producción, incluso si se compilan
con `--mode intercom-demo`. Las variables específicas de `.env.intercom-demo.local`
o del entorno del proceso pueden sobrescribir el endpoint/directorio de prueba.

La ruta `/citofonia` se integra en la navegación lateral y móvil para residentes,
propietarios, guardias y administradores. Guardia/administrador operan como
portería; residente/propietario reciben llamadas del apartamento asociado a su
unidad. Reutiliza la sesión compartida, componentes `PageHead`, `Badge`, `Field`,
`ErrorBox`, tarjetas, botones y variables CSS existentes. No transmite ni simula
audio/video, ni solicita permisos de micrófono.

Configura estas variables en `.env.local` antes de `npm run dev`:

```dotenv
VITE_INTERCOM_WS_URL=wss://kmb9uxwe19.execute-api.us-east-2.amazonaws.com/production
VITE_INTERCOM_CLIENTS=[{"complexId":"<conjunto-id>","clientId":"PORTERIA","role":"porter","label":"Portería"},{"complexId":"<conjunto-id>","unitId":"<unidad-id>","clientId":"APT_4","role":"apartment","label":"Apartamento 4"}]
```

Sustituye los marcadores por los identificadores reales de `session.complexId` y
`session.unitId`. Añade una entrada por apartamento y una portería por conjunto.
`label` es solo texto visible; `clientId` es el identificador lógico reconocido
por Lambda/DynamoDB. El apartamento no se deduce del nombre del usuario ni se
fija en componentes visuales. `config.ts` es el adaptador temporal reemplazable
por un directorio autenticado: por ahora resuelve el cliente a partir del rol,
conjunto y unidad de la sesión. La portería elige un apartamento del directorio
de su conjunto; no escribe `connectionId` ni identidades arbitrarias.

El servicio añade `clientId` y `role` como query parameters con `URLSearchParams`.
Usa `wss://` y el stage correcto. Las variables `VITE_*` son públicas y quedan
incorporadas al build: no pongas secretos en ellas. Reinicia Vite al cambiarlas;
para producción configúralas antes de compilar. El proxy HTTP de Vite no se usa
para este WebSocket.

### Flujo y ciclo de vida

- En una sesión autenticada se abre una sola conexión, independiente de la ruta.
  El diálogo de llamada entrante aparece también fuera de Citofonía, con foco
  modal nativo; Escape rechaza la llamada. Al aceptar, un aviso permite volver
  a Citofonía y finalizar desde otras pantallas.
- Todos los envíos llevan exactamente `action: "seendMessage"`, `type`, `target`
  lógico y `message`. El typo se mantiene porque es la ruta actualmente probada
  en AWS. La constante `INTERCOM_ACTION` en `protocol.ts` solo debe cambiarse
  coordinadamente con API Gateway y Lambda. No se envía `callId`, `connectionId`,
  `from` ni información adicional de la sesión.
- Se soportan `CALL`, `CALL_ACCEPTED`, `CALL_REJECTED` y `HANGUP`. Los mensajes
  de llamada recibidos necesitan `from` y `target`, pero NO `callId`.
  Solo afectan a la llamada activa si coinciden interlocutor y destino;
  únicamente portería puede originar llamadas hacia apartamentos.
  También se reconocen respuestas tipadas `DELIVERED` (entrega, no aceptación)
  y `ERROR` si el backend las emite. Los errores JSON, de red y del backend
  se muestran explícitamente.
- No se permiten llamadas simultáneas. Mientras el apartamento tiene una llamada
  activa, un `CALL` repetido de portería se ignora sin reiniciar su estado.
  Rechazar/finalizar permite otra llamada. Cada interacción conserva un UUID
  exclusivamente local, que no correlaciona mensajes del backend. Sin un
  `callId` de extremo a extremo no es posible distinguir una señal atrasada de
  otra llamada del mismo interlocutor ni deduplicar llamadas ya finalizadas.
  No se impone un timeout de respuesta en esta etapa.
- Se reconecta con espera exponencial de 1 a 30 segundos. No hay cola ni replay:
  perder la conexión interrumpe la llamada y reconectar no la reanuda. No existe
  aún un contrato backend para consultar/reconciliar llamadas tras reconexión.
- Al iniciar logout se intenta enviar `HANGUP` si hay una llamada y conexión,
  se cierra el socket y se cancelan reintentos antes de esperar al logout HTTP.
  También se limpia al expirar la sesión, desmontar o cambiar de conjunto,
  unidad, usuario o rol. No se depende de un envío durante el cierre de pestaña.

### Prueba manual con el prototipo

1. Conserva en API Gateway la ruta `seendMessage` y la selección
   `$request.body.action`. El endpoint real ya figura en `.env.example` y
   `.env.production`; configura el directorio con los IDs de la sesión.
2. Abre dos sesiones/perfiles separados: un guardia y un residente de la unidad
   configurada. Puedes usar `/dev-login` con el backend de accesos en desarrollo;
   el login mock de autenticación no crea un perfil de guardia ni aporta
   `unitId`, así que no basta por sí solo para probar la identidad del apartamento.
3. Comprueba `Conectado` en ambos. Desde portería selecciona el apartamento y
   pulsa `Llamar`. Debe aparecer `Esperando respuesta...`, sin confundir
   `DELIVERED` con aceptación.
4. Desde el residente, también estando en otra ruta, acepta o rechaza.
   Verifica el cambio en portería. Finaliza desde cualquiera de los dos perfiles
   y vuelve a llamar; no debe requerirse ningún `callId` en los mensajes.
5. Prueba apartamento desconectado (`ERROR`), pérdida de red, cambio de conjunto
   y logout. Las pruebas automatizadas están en `src/modules/citofonia/`:
   `npx vitest run src/modules/citofonia`.

### Límites del backend de prototipo

`clientId` y `role` en una query no prueban identidad ni pertenencia; los guards
de UI tampoco autorizan una conexión. El backend debe autenticar y validar esos
valores antes de usar este flujo con usuarios reales. Los identificadores
deben ser únicos para todo el endpoint, incluidos conjuntos distintos; no uses
`APT_4` o `PORTERIA` repetidos en varios conjuntos del mismo endpoint.
El backend registra conexiones en `CitofoniaConnections` y resuelve clientes con
`clientId-index`; el frontend solo abre/cierra el socket. `$disconnect` y la
eliminación del registro son responsabilidad de AWS. La política de entrega
cuando hay varios dispositivos/pestañas con el mismo `clientId` depende de
Lambda; no se presupone broadcast ni selección de un dispositivo. Esta
integración mantiene una conexión por sesión montada, no coordinación entre
pestañas. La vista debe estar abierta y autenticada para recibir señalización;
no sustituye un mecanismo de llamadas en segundo plano.
No se modifican API Gateway, Lambda ni DynamoDB desde este frontend.

## Rutas

`/splash`, `/login`, `/forgot-password`, `/forgot-password/sent`, `/reset-password`, `/reset-password/success`, `/select-complex`, `/session-expired`, `/unauthorized`, `/account-blocked`. Tras autenticar, el usuario entra al `AppShell` y a las rutas existentes de control de accesos. La selección y el cambio de comunidad envían el identificador al BFF para validación; solo se asigna como conjunto activo después de la respuesta autenticada del servicio.

## Tokens y decisiones de diseño

Se empleó CSS variables de forma consistente en `src/styles/tokens.css`, con una escala base de 4 px. Los valores de color, radios y espaciado parten de la lista incluida en el requerimiento pegado. Añadí tonos soft de estados y sombras sutiles no especificadas literalmente en el material recibido.

El conector de Figma (lectura de variables, contexto y capturas) no está habilitado en esta sesión y no se pudo abrir el archivo privado para inspeccionar los nodos solicitados. Por eso estos tokens todavía no están verificados contra Figma y las pantallas son una interpretación basada en la descripción recibida. Falta validar medidas, textos, iconos, estados y componentes frente a los nodos `2:2` a `2:147` y `3:26`, junto con Button, Input, Badge y Card del sistema de diseño.

El logo del diseño original es un marcador. `Logo` busca una imagen en `VITE_LOGO_URL` (por defecto `/src/assets/brand/logo.svg`) y muestra una marca tipográfica de respaldo. No se proporcionó la imagen oficial.

## Integración y seguridad

El access token solo se guarda en memoria. `src/lib/http.ts` adjunta el bearer token, reintenta una solicitud una vez luego de un refresh silencioso concurrente y emite un evento de sesión expirada si el refresh falla. El refresh se solicita con `credentials: include` para que un BFF pueda gestionar una cookie `HttpOnly; Secure; SameSite`; nunca se lee desde JavaScript.

El Auth MS actual (`back/auth/ms`) devuelve access y refresh token en el JSON de login, y el refresh requiere que el refresh token vaya en el cuerpo. Tampoco implementa OIDC/OAuth completo, recuperación de contraseña, activación de conjunto, datos de conjuntos ni el bloqueo de 5 intentos por 15 minutos. Además, permite configurar refresh tokens por más de 8 horas. No conectes directamente este frontend a esa respuesta de producción: hace falta un BFF/proveedor OIDC que convierta el refresh a cookie `HttpOnly`, aplique los límites de sesión e intentos, proporcione rutas de recuperación e identidad/membresías y emita un access token limitado al conjunto validado. La sesión compartida existente que consumen los módulos conserva el access token solo en memoria; ya no escribe tokens en `localStorage`.

El BFF debe devolver permisos del conjunto activo. El shell y las rutas del módulo existente aplican roles y los códigos `access.home.read`, `access.authorizations.read/create`, `access.building.read`, `access.pass.validate`, `access.visitor.register`, `access.movement.register` y `access.history.read`. El microservicio Auth actual solo incluye permisos de administración de Auth; todavía no existe catálogo RBAC residencial que emita estos códigos, por lo que hay que alinearlos con el backend/Gateway antes del despliegue.

La UI responde con el mismo mensaje para cualquier HTTP 401 de login y la solicitud de recuperación siempre presenta la misma confirmación para cualquier correo. El backend/BFF debe mantener esa igualdad de respuesta. El mock de desarrollo incluye comunidades ficticias; nunca es fuente de autorización. El tenant activo debe ser validado por el servicio en cada operación.

El flujo solicita únicamente correo y contraseña, no escribe datos personales ni credenciales en consola y presenta el aviso de tratamiento de datos conforme a la Ley 1581 de 2012. La versión final debe enlazar el aviso jurídico aprobado por el responsable del tratamiento.

## Pruebas y límites

Vitest, Testing Library y MSW cubren validación de esquemas, mensaje genérico de credenciales, guards por permiso, refresh silencioso y selección multi-tenant. Las suites E2E que venían con `main` dependen del login de desarrollo y de JWT en `localStorage`; el módulo nuevo elimina ese flujo inseguro y esas pruebas requieren adaptarse al contrato del BFF antes de ejecutarlas. No se configuró `jest-axe` ni se completó la comparación visual con Figma.

`npm audit` reporta vulnerabilidades moderadas conocidas en las versiones de React Router 6 y Vitest que se eligieron para respetar el stack React Router v6 solicitado; npm propone migrar a React Router 7 y Vitest 5, lo cual cambia versiones mayores. Revisar esa actualización antes de publicar.
