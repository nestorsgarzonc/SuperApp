# Control de accesos — visitantes y vehículos

Este documento explica cómo está armado el módulo de control de accesos
(pases de visitantes y vehículos de visitantes) y cómo levantarlo en local
para probarlo. El mapa general del repositorio está en
[architecture.md](./architecture.md).

## Qué puedes probar hoy

Dos flujos, en la misma pantalla, cambiando de persona con el selector
**Probar como**:

- **Personas.** Un residente autoriza a un visitante y recibe un token
  (el equivalente al QR). Portería registra el ingreso y la salida con ese token.
- **Vehículos.** Un residente preautoriza una placa. Portería consulta la
  placa y registra entrada o salida. Si la placa no está autorizada, el
  ingreso igual queda en la bitácora, marcado como no preautorizado.

La identidad todavía no pasa por el módulo de auth. La pantalla envía dos
encabezados (`X-Complex-Id` y `X-User-Id`) y el microservicio decide si esa
persona es residente o guardia. El API Gateway tampoco está construido: en
este entorno de prueba el frontend habla directo con el microservicio.

## Cómo levantarlo

Hace falta Node.js (en esta máquina, v22) y Docker. Tres procesos:

### 1. Base de datos

Desde `back/control-de-accesos`:

```bash
docker compose up -d
```

PostgreSQL 18 queda en el puerto **5433** (el 5432 se deja libre por si
hay otro Postgres). Usuario, clave y base: `accesos` / `accesos` /
`control_accesos`. El contenedor se llama `control_accesos_db`.

La primera vez que el volumen está vacío, Docker ejecuta
`db/001_schema.sql` y `db/002_seed.sql`. Si el volumen ya existe y las
tablas no se crearon, al arrancar el microservicio las aplica él mismo
(`applySchema` en `ms/src/db.js`) solo si falta la tabla `visitor_passes`.

### 2. Microservicio

Desde `back/control-de-accesos/ms`:

```bash
npm install
npm start
```

Escucha en **http://localhost:4000**. `npm install` solo hace falta la
primera vez, o cuando cambie `package.json`.

Comprobación:

```bash
curl http://localhost:4000/health
```

Respuesta esperada: `{"ok":true,"module":"control-de-accesos"}`.

### 3. Frontend

Desde `front`:

```bash
npm install
npm run dev
```

Abre **http://localhost:5173**. Vite reenvía `/visitor-passes`, `/vehicles`,
`/integration` y `/health` al puerto 4000, así que el navegador no llama
al microservicio por otro origen.

## Recorrido en la pantalla

El seed carga el conjunto **Demo Residential Complex** con estas personas:

| Persona | Rol en la pantalla | Qué trae de ejemplo |
|---|---|---|
| John Resident | Residente, unidad A-101 | Pase activo de **Maria Visitor**. Token: `demo-token-maria-visitor` |
| Jane Resident | Residente, unidad A-102 | Placa **ABC-123** (se guarda como `ABC123`), visitante Rappi Driver |
| Guard Demo | Portería | Consulta tokens y placas, registra entradas y salidas |

La pantalla abre con el primer residente de la lista (Jane). Para ver el
pase de María hay que cambiar a John.

**Personas**

1. Como John Resident, pestaña Personas: aparece Maria Visitor y su token.
2. Crea otro visitante (nombre y documento). El aviso muestra el token nuevo.
3. Cambia a Guard Demo. El campo ya trae `demo-token-maria-visitor`.
4. **Registrar ingreso** y después **Registrar salida**. La lista de portería
   muestra el movimiento y el anfitrión recibe un aviso (se ve al volver
   al residente, en "Avisos del anfitrión").

**Vehículos**

1. Como Jane Resident, pestaña Vehículos: aparece la placa ABC123.
2. Cambia a Guard Demo, busca `ABC-123` y pulsa **Entrada** y luego **Salida**.
3. Para una placa nueva: como residente, autoriza otra placa; como portería,
   búsca la misma y registra la entrada.

## Cómo está hecho

Cada feature del backend es autónomo: su microservicio (`ms`) y su base
(`db`). Este módulo no lee tablas de auth ni de notificaciones. Lo que
necesita de otros módulos (complejo, unidades, usuarios) vive en tablas
`ref_*`, una copia local con los mismos UUID del demo.

```
back/control-de-accesos/
├── docker-compose.yml      # Postgres solo de este módulo, puerto 5433
├── db/
│   ├── 001_schema.sql      # tablas ref_*, pases, preautorizaciones, bitácora
│   └── 002_seed.sql        # Demo Residential Complex + María + placa ABC123
└── ms/
    ├── package.json
    └── src/
        ├── index.js        # rutas HTTP
        ├── passes.js       # pases de visitante
        ├── vehicles.js     # placas de visitantes
        ├── db.js           # conexión y aplicación del esquema
        └── errors.js       # errores de negocio con código HTTP

front/
├── index.html              # página que carga Vite
├── package.json
├── vite.config.js          # proxy hacia el puerto 4000
└── src/
    ├── main.jsx
    ├── App.jsx             # monta solo este módulo
    └── modules/control-de-accesos/
        ├── AccessControlModule.jsx
        ├── api.js          # llamadas al microservicio
        └── module.css
```

`index.html`, `package.json` y `vite.config.js` van en `front/`, no dentro
de `src/`. Vite trata la carpeta del `index.html` como raíz del sitio, y
el script de esa página apunta a `/src/main.jsx`.

### Pases de visitante

Tabla `visitor_passes`. El residente de la unidad crea el pase (máximo 30
días), lo extiende o lo revoca. El token es aleatorio; el del seed es fijo
para poder probar portería sin crearlo a mano. Portería solo puede validar
ingreso (`active` → `used`) y registrar salida (`used` → `checked_out`).
Al ingresar o salir se inserta una fila en `integration_notifications`
para el anfitrión. Esa tabla es el hueco donde, más adelante, escribe el
módulo de notificaciones; hoy la lee la misma pantalla.

Un residente solo ve y modifica pases de las unidades donde está en
`ref_unit_members`.

### Vehículos de visitantes

Tabla `visitor_vehicle_preauths` y bitácora `visitor_vehicle_access_log`.
La placa se normaliza a mayúsculas y sin espacios ni guiones, por eso
`ABC-123` y `ABC123` son la misma. No incluye los vehículos del residente:
ese inventario pertenece a otro módulo.

### Lo que no hace falta para esta prueba

Siguen vacíos, a propósito, y no bloquean este recorrido:

- `back/api-gateway`
- `back/auth`
- `back/notificaciones`
- `infra`

Cuando existan, el navegador dejará de hablar directo con el puerto 4000
y los encabezados de prueba los reemplazará un token de auth.

## Para qué sirve `node_modules`

`node_modules` no es código del proyecto. Es la carpeta donde npm descarga
las librerías que declara `package.json`. No se edita, no se sube a git
(está en `.gitignore`) y se puede borrar: `npm install` la vuelve a crear
a partir de `package.json` y `package-lock.json`.

Hay dos, una por aplicación:

- `back/control-de-accesos/ms/node_modules` — librerías del API.
- `front/node_modules` — librerías de la pantalla.

Cada librería trae las suyas, por eso la carpeta tiene cientos de
subcarpetas aunque en `package.json` solo figuren unas pocas. Esas
dependencias indirectas (por ejemplo las que usa Vite para empaquetar)
no se listan en el `package.json` del proyecto.

### Microservicio (`back/control-de-accesos/ms/package.json`)

| Paquete | Para qué está |
|---|---|
| `express` | Servidor HTTP y las rutas (`/visitor-passes`, `/vehicles`, …). |
| `cors` | Permite que el navegador llame al API si el origen no es el mismo puerto. |
| `pg` | Cliente de PostgreSQL. El pool de conexiones está en `ms/src/db.js`. |

### Frontend (`front/package.json`)

| Paquete | Para qué está |
|---|---|
| `react` y `react-dom` | La interfaz: pantallas, formularios y la lista de pases y placas. |
| `vite` | Servidor de desarrollo (puerto 5173) y el empaquetado de `npm run build`. |
| `@vitejs/plugin-react` | Traduce el JSX de los componentes (el HTML dentro de JavaScript) para que el navegador pueda ejecutarlo. |

`vite` y `@vitejs/plugin-react` son `devDependencies`: hacen falta para
desarrollar y construir, no para el API.
