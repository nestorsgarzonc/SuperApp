# SuperApp

Aplicación para un conjunto residencial. Una sola pantalla (`front`)  
reúne varios módulos. En el backend cada módulo es autónomo: su  
microservicio (`ms`) y su base de datos (`db`). Ningún servicio lee la  
base de otro. El tráfico del navegador entra por el API Gateway, que  
enruta y valida antes de llegar a cada microservicio.

El mapa de componentes está en [architecture.md](./architecture.md).

## Módulos

| Módulo | Qué cubre | Estado |
| --- | --- | --- |
| `front` | La aplicación. Un módulo de interfaz por feature y código común en `shared`. | La pantalla de control de accesos ya se puede abrir. Auth y notificaciones están reservados. |
| `back/api-gateway` | Entrada única: enrutamiento, token y rate limit. No tiene base de datos. | Carpeta reservada. |
| `back/auth` | Identidad, credenciales y tokens. | Carpeta reservada. |
| `back/control-de-accesos` | Pases de visitantes y vehículos de visitantes. Microservicio en Go. | Implementado para prueba local. Ver la sección de abajo. |
| `back/notificaciones` | Avisos (email, push, SMS) y su bitácora. | Carpeta reservada. |
| `infra` | Infraestructura y despliegue. | Carpeta reservada. |

Las carpetas vacías se mantienen en git con `.gitkeep` hasta que tengan código.

```
SuperApp/
├── back/
│   ├── api-gateway/
│   ├── auth/
│   │   ├── ms/
│   │   └── db/
│   ├── control-de-accesos/
│   │   ├── ms/
│   │   └── db/
│   └── notificaciones/
│       ├── ms/
│       └── db/
├── front/
│   └── src/
│       ├── modules/
│       │   ├── auth/
│       │   ├── control-de-accesos/
│       │   └── notificaciones/
│       └── shared/
├── infra/
└── docs/
```

## Convenciones

*   `ms/` es el servicio desplegable del módulo.
*   `db/` guarda solo el esquema, las migraciones y las semillas de ese módulo. Una base por módulo.
*   `api-gateway` no tiene `db/`.
*   El frontend llama al gateway, no a un microservicio directo. La excepción de hoy está en control de accesos: el gateway y auth todavía no existen, así que esa pantalla de prueba habla con su microservicio.

## `node_modules`

`node_modules` no es código de SuperApp. Es la carpeta donde npm descarga  
las librerías declaradas en un `package.json`. No se edita, no se sube a  
git (está en `.gitignore`) y se regenera con `npm install`.

Hoy solo el frontend tiene `package.json`. El microservicio de control de  
accesos es Go: sus librerías quedan en `go.mod` y se descargan con  
`go mod download`, no con npm.

| Dónde | Paquete | Para qué está |
| --- | --- | --- |
| Frontend | `react`, `react-dom` | La interfaz. |
|   | `vite` | Servidor de desarrollo y empaquetado. Es dependencia de desarrollo. |
|   | `@vitejs/plugin-react` | Traduce el JSX (el HTML escrito dentro de JavaScript). Es dependencia de desarrollo. |

## Control de accesos

Microservicio en Go. Cubre visitantes a pie y vehículos de visitantes.
No incluye la identidad de los residentes (eso es `auth`), ni el envío
real de avisos (eso es `notificaciones`), ni el inventario de carros del
residente.

La pantalla de prueba habla con este servicio directo, en el puerto 4000,
porque el API Gateway todavía no existe. Manda `X-Complex-Id` y
`X-User-Id`. El servicio mira las tablas `ref_*` y decide si esa persona
es residente o portería. El selector **Probar como** reemplaza el login
hasta que exista `auth`.

Go 1.25 o superior tiene que estar en el `PATH`. Las dependencias están
en `ms/go.mod` (`pgx`). No hay `package.json` en este módulo.

### Cómo está armado

La forma es la de `feature/auth-microservice`: dominio, casos de uso,
infraestructura y HTTP. El proceso no aplica el esquema al arrancar.
Eso lo hace `cmd/setup`, una sola vez.

```
back/control-de-accesos/
├── docker-compose.yml          # Postgres 18, puerto 5433
├── db/
│   ├── migrations/
│   │   ├── 001_access_schema.up.sql
│   │   └── 001_access_schema.down.sql
│   └── seeds/
│       └── 001_demo.sql
└── ms/
    ├── cmd/access/             # servidor HTTP
    ├── cmd/setup/              # migraciones y semillas
    ├── go.mod
    └── internal/
        ├── domain/             # entidades, reglas y errores
        ├── application/        # casos de uso
        ├── infrastructure/
        │   ├── config/         # ACCESS_ADDR, ACCESS_DATABASE_URL
        │   └── postgres/       # SQL
        └── presentation/httpapi/

front/src/modules/control-de-accesos/
├── AccessControlModule.jsx
├── api.js
└── module.css
```

`front/index.html`, `front/package.json` y `front/vite.config.js` son de
la aplicación completa. Hoy `App.jsx` monta solo esta pantalla.

Una petición entra por `httpapi`, que resuelve al actor y exige el rol.
`application` valida la ventana de tiempo, la unidad y que el residente
pertenezca a esa unidad. `postgres` escribe en la base de este módulo.
Un vecino no puede crear, leer, revocar ni extender lo de otra unidad.

| Capa | Qué decide |
| --- | --- |
| `httpapi` | Ruta, JSON y rol (residente o portería). |
| `application` | Nombre, vigencia, placa y pertenencia a la unidad. |
| `postgres` | Inserta, actualiza y lee. No decide el negocio. |
| `domain` | Estados del pase, errores y normalización de la placa. |

### Datos

La base es solo de este módulo. PostgreSQL 18, puerto **5433**,
usuario `accesos`, clave `accesos`, base `control_accesos`.

| Tabla | Dueño del dato |
| --- | --- |
| `ref_complexes`, `ref_sections`, `ref_units`, `ref_users`, `ref_complex_staff`, `ref_unit_members` | Copia local. Los mismos UUID del demo. No son tablas de `auth`. |
| `visitor_passes` | Pase del visitante a pie. |
| `visitor_vehicle_preauths` | Placa que el residente espera. |
| `visitor_vehicle_access_log` | Entrada o salida anotada por portería. |
| `integration_notifications` | Aviso al anfitrión. Hueco hasta que exista `notificaciones`. |

El pase nace `active`. Portería lo pasa a `used` al ingresar y a
`checked_out` al salir. El residente puede dejarlo en `revoked` mientras
siga activo. Si se vence, pasa a `expired`. La vigencia máxima es 30 días.
La placa se guarda en mayúsculas y sin guiones: `ABC-123` y `ABC123` son
la misma.

### Rutas

El servicio escucha en **http://localhost:4000**. Vite reenvía estas
rutas desde el puerto 5173.

| Método | Ruta | Quién |
| --- | --- | --- |
| GET | `/health`, `/ready` | Nadie. `/ready` comprueba Postgres. |
| GET | `/integration/context` | Nadie. Lista el conjunto y las personas de prueba. |
| GET | `/integration/notifications` | El actor de los headers. |
| POST | `/visitor-passes` | Residente. Crea un pase y devuelve el token. |
| GET | `/visitor-passes/units/{unitId}` | Residente de esa unidad. |
| GET | `/visitor-passes/{id}` | Residente de la unidad del pase. |
| POST | `/visitor-passes/{id}/revoke` | Residente. Solo si el pase está activo. |
| POST | `/visitor-passes/{id}/extend` | Residente. Solo si sigue activo y no venció. |
| GET | `/visitor-passes/checkpoint-activity` | Portería. Conteos y movimientos, sin el token. |
| POST | `/visitor-passes/validate` | Portería. Marca el ingreso. |
| POST | `/visitor-passes/check-out` | Portería. Marca la salida. |
| POST, GET | `/vehicles/visitor-preauth` | Residente. Crea o lista sus placas. |
| DELETE | `/vehicles/visitor-preauth/{id}` | Residente. Revoca la suya. |
| GET | `/vehicles/lookup?plate=` | Portería. Busca una preautorización vigente. |
| POST, GET | `/vehicles/access-log` | Portería. Anota o lista la bitácora. |

Un error sale así: `{ "error": { "code", "message" } }`. La pantalla
muestra `message`.

### Levantar

Hace falta Docker y Go 1.25 o superior en el `PATH`. El frontend sigue
en Node.js.

Base de datos, desde `back/control-de-accesos`:

```powershell
docker compose up -d
```

Esquema y datos, desde `back/control-de-accesos/ms`. Docker solo deja
Postgres vacío. `setup` aplica `db/migrations/*.up.sql` y después
`db/seeds`.

```powershell
$env:ACCESS_DATABASE_URL = 'postgres://accesos:accesos@localhost:5433/control_accesos?sslmode=disable'
go run ./cmd/setup
go run ./cmd/access
```

`setup` hace falta la primera vez, o cuando cambie una migración. El
proceso HTTP no lo repite.

Frontend, desde `front`:

```powershell
npm install
npm run dev
```

Abre **http://localhost:5173**.

### Datos de ejemplo

El conjunto es **Demo Residential Complex**.

| Persona | Rol | Qué trae |
| --- | --- | --- |
| John Resident | Residente, A-101 | Pase de Maria Visitor. Token: `demo-token-maria-visitor` |
| Jane Resident | Residente, A-102 | Placa `ABC-123`, visitante Rappi Driver |
| Guard Demo | Portería | Registra ingresos y salidas. No crea autorizaciones. |

La pantalla abre en Jane. Para ver el pase de María hay que cambiar a John.

**Personas.** Como John, el pase aparece en la pestaña Personas; también
se puede crear otro y copiar el token. Como Guard Demo, ese token registra
el ingreso y después la salida. Al volver al residente se ve el aviso.

**Vehículos.** Como Jane, la placa aparece en Vehículos. Como Guard Demo,
buscar `ABC-123` y registrar entrada y salida. Una placa sin
preautorización igual queda en la bitácora.