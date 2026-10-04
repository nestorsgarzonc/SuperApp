# SuperApp · Frontend de autenticación

Aplicación React SPA modular de SuperApp, comunicación y gestión residencial. El módulo de autenticación integrado respeta la estructura existente de `src/shared` y `src/modules/control-de-accesos`, y deja la navegación y las pantallas de control de accesos dentro del `AppShell` de `main` branch. Requiere Node.js 20 o posterior.

## Inicio rápido

```sh
npm install
npm run dev
```

El modo de demostración de autenticación usa MSW en el navegador. Copia `.env.example` como `.env.local` y conserva `VITE_USE_MOCK_API=true`. Credenciales de demo: `demo@superapp.co` / `Demo123!`. Cinco intentos fallidos llevan a la pantalla de cuenta bloqueada por 15 minutos. Para apuntar a un servicio, configura `VITE_API_BASE_URL` y desactiva el mock. El módulo de control de accesos sigue usando el API Gateway (`GATEWAY_URL`) o `ACCESS_URL` como proxy de desarrollo, igual que en `main`.

Comandos: `npm run build`, `npm test` y `npm run lint`.

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
