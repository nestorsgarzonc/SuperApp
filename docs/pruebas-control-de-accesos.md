# Guía de pruebas — control de accesos

Cómo levantar el entorno, correr las pruebas automáticas y recorrer a mano todos
los flujos. Todo se hace en modo desarrollo: el ms se compila con `-tags dev` (docker compose ya lo hace) y se arranca con
`ENV=development`; así existen los endpoints `/dev/*` y el panel de pruebas del front, que **no existen en
producción**. Si `ENV` no está definido el ms se comporta como producción (exige JWT y no tiene `/dev/*`).

## 1. Levantar el entorno

```bash
# Backend + base de datos (Docker Desktop encendido)
cd back/control-de-accesos
cp .env.example .env                 # solo la primera vez
docker compose down -v               # IMPORTANTE si ya tenías datos: migraciones y datos de
                                     # demostración solo se aplican con el volumen vacío
docker compose up --build            # ms en :8081, Postgres en :5433

# Front
cd ../../front
cp .env.example .env                 # solo la primera vez (ACCESS_URL=http://localhost:8081)
npm install
npm run dev                          # http://localhost:5173
```

Comprobar: `curl http://localhost:8081/healthz` → `ok`, y abrir
http://localhost:5173 → verás **"Control de acceso · pruebas"** con 7 personas.

**Reiniciar los datos** cuando quieras empezar de cero (los pases son de un solo uso, las
pruebas los consumen):

```bash
cd back/control-de-accesos && ./test/reset_data.sh
```

> Consejo: no mezcles tus pruebas manuales con las automáticas sobre la misma base.
> Las automáticas **reinician los datos**. Usa una base/puertos aparte para ellas
> (ver §3) o corre las automáticas cuando no estés probando a mano.

## 2. Personas de prueba

| Persona | Conjunto 1 · Los Pinos | Conjunto 2 · Alto del Bosque |
|---|---|---|
| Ana García | residente A-101 | — |
| Pedro Gómez | — | residente B-201 |
| Luis Mora | guardia | — |
| Marta Ruiz | — | guardia |
| Carlos Rojas | administrador | — |
| Sofía Díaz | residente A-102 | residente B-202 |
| Andrés Vega | residente A-102 | **guardia** |

Quien tiene varios conjuntos ve la pantalla **"Selecciona tu conjunto"** y luego puede
cambiar desde el selector del encabezado. Cada sesión opera en **un solo** conjunto.

## 3. Pruebas automáticas

Orden recomendado, desde la raíz del repo. Cada comando imprime `PASS/FAIL` por caso
y un total.

| # | Qué cubre | Comando | Casos |
|---|---|---|---|
| 1 | Reglas puras: breaker, máquina de estados entrada/salida, middleware de personal, validaciones | `cd back/control-de-accesos/ms && go test ./...` | unitarios |
| 2 | API base: 10 endpoints, errores, roles, un solo uso | `BASE_URL=http://localhost:8081 ./back/control-de-accesos/test/smoke_test.sh` | 41 |
| 3 | **Aislamiento entre conjuntos**: guardia 1 vs 2, persona en ambos, tokens manipulados, bitácora, sin filtrar existencia | `node back/control-de-accesos/test/isolation.mjs` | 70 |
| 4 | **Entrada/salida de residentes**: estados, concurrencia, presencia, por conjunto | `node back/control-de-accesos/test/residents.mjs` | 47 |
| 5 | **Circuit breaker del sistema QR** (ver nota) | `node back/control-de-accesos/test/qr-breaker.mjs` | 33 |
| 5b | **Endurecimiento**: un residente no ve pases de otras unidades, cuerpo máximo (413), formato único de error, lote atómico | `node back/control-de-accesos/test/hardening.mjs` | 23 |
| 6 | Interfaz (navegador real): flujo base, multi-conjunto, entrada/salida, breaker | `cd front && npm run e2e` | ~120 |

Reinicia los datos (`./test/reset_data.sh`) **antes de cada uno** de los pasos 2–6 (incluido 5b).

**Nota del breaker (5 y la parte de breaker de 6):** necesitan un ms con tiempos cortos.
Levanta el ms así (en `.env` o al invocar compose) y luego corre las pruebas:

```bash
QR_CALL_TIMEOUT_MS=300 QR_BREAKER_OPEN_SECONDS=8 QR_BREAKER_FAILURES=3 docker compose up --build
QR_BREAKER_OPEN_SECONDS=8 node test/qr-breaker.mjs
```

**Interfaz (6):** usa Edge o Chrome ya instalado (`E2E_CHANNEL=chrome` si no tienes Edge).
`APP_URL` cambia la URL del front. Deja capturas en `front/e2e/shots/`.

### Entorno aparte para las automáticas (recomendado)

Para no pisar tus datos manuales, levanta una segunda base y un segundo ms/front:

```bash
# segunda base (con el mismo Postgres de compose) y migraciones
docker compose exec -T db psql -U control_de_accesos -d postgres -c "CREATE DATABASE cda_test"
for f in db/migrations/*.sql db/seeds/*.sql; do docker compose exec -T db psql -U control_de_accesos -d cda_test -q < "$f"; done
# ms de pruebas en :8082 apuntando a cda_test (ver docker-compose.yml para las variables DB_*)
# front de pruebas en :5174:  ACCESS_URL=http://localhost:8082 npm run dev -- --port 5174 --strictPort
# y las pruebas:  BASE=http://localhost:8082/api/v1 node test/isolation.mjs   |   APP_URL=http://localhost:5174 npm run e2e
# reset de esa base:  PSQL_CMD="docker compose exec -T db psql -U control_de_accesos -d cda_test -q" ./test/reset_data.sh
```

## 4. Flujo manual (paso a paso)

Abre http://localhost:5173. Para ver dos personas a la vez usa dos ventanas/perfiles del
navegador (la sesión se guarda en `localStorage`, así que dos pestañas del mismo
navegador **comparten** sesión; usa una ventana privada para la segunda persona).

### A. Visita con QR (residente → portería)
1. **Ana García** → "Autorizar visitante". Por defecto el pase empieza **ahora** y dura 4 h.
   Escribe un nombre y "Generar pase". Verás el QR, el código y el estado **Activa**.
2. En la lista "Mis autorizaciones" aparece una sola vez, en **Activas**.
3. Ventana privada → **Luis Mora** → "Validar pase" → pega el código → **✓ Pase válido**
   (nombre, unidad A-101, vigencia). "Registrar ingreso" lo guarda en la bitácora.
4. Valida el mismo código otra vez → **✕ "Este pase ya fue utilizado"**.
5. De vuelta en Ana: el pase pasó a la pestaña **Usadas** (ya no está en Activas ni en Vencidas).

**Casos de rechazo (el guardia ve el motivo exacto):**
- Crea un pase que empiece en 2 horas → Ana ve el aviso azul "aún no está vigente" y estado
  **Pendiente** (pestaña Próximas); Luis lo valida → "El pase aún no está vigente. Empieza hoy a las …".
- Pega un código inventado → "El código no existe".
- Revoca un pase (botón **Revocar** en su pantalla) y valídalo → "El residente revocó este pase".
- Valida con **Marta Ruiz** (guardia del otro conjunto) → "No se pudo validar el pase": es **idéntico** a un código inexistente, a propósito, para no revelar que existe en otro conjunto.

### B. Pre-autorización por documento
1. Ana → "Autorizar visitante" → **"Por documento"** → nombre + documento (p. ej. `COL424242`).
2. Luis → "Validar pase" → pestaña **Documento (pre-autorización)** → `COL424242` → **✓ válido**.
3. Marta con el mismo documento → no válido. Otra vez con Luis → ya usado.
4. Semillas listas para probar: `COL123456` (Rappi, conjunto 1), `COL777777` (conjunto 2), y
   `COL555555`, que existe en **ambos** conjuntos (cada guardia consume solo la suya).

### C. Entrada y salida de residentes al edificio
1. **Ana** → pestaña **Edificio** → "Ingresar al edificio". Estado **Dentro**, "desde hoy a las …".
2. Intenta ingresar otra vez → el botón está deshabilitado (el servidor además responde 409).
3. "Salir del edificio" → **Fuera**. El historial "Mis movimientos" muestra ambos.
4. **Luis** (inicio de portería) → tarjeta **"Residentes dentro del edificio"**: aparece Ana con su
   unidad y desde cuándo; cuando Ana sale, desaparece (botón ⟳ para actualizar).
5. **Carlos Rojas** (administrador) → Historial: filas "Ingreso al edificio" / "Salida del edificio",
   Tipo **Residente**. Marta (otro conjunto) **no** ve a Ana.

### D. Una persona en dos conjuntos
- **Sofía Díaz**: elige "Los Pinos" → entra al edificio (unidad A-102). Cambia al conjunto 2 con el
  selector → su estado ahí es **independiente** (sin movimientos) y su unidad es B-202.
- **Andrés Vega**: en "Alto del Bosque" entra como **Portería**; en "Los Pinos" como **residente**
  (sin acciones de portería aunque sea guardia en el otro).

### E. Circuit breaker del sistema QR (simulación)
El panel **🛠 QR: operativo · circuito cerrado** (abajo a la derecha; pulsa para desplegar):
1. "Simular caída del sistema QR" → el sistema deja de responder (cuelga).
2. Como residente, intenta "Generar pase" por QR: tarda ~1 s (timeout) y el front cambia a
   **"Por documento"** explicando el motivo. Repite 2 veces más.
3. Tras 3 fallos el panel marca el circuito **ABIERTO**: aparece el aviso ámbar *"El sistema de QR no
   está funcionando… autoriza por documento"* en el inicio y en "Nueva autorización", y la opción
   QR se deshabilita. Ahora cualquier intento por QR falla **al instante**.
4. El guardia ve el aviso en su inicio y en "Validar pase" (con botón **Validar por documento**).
5. La pre-autorización por documento **sigue funcionando** de punta a punta.
6. "Restablecer sistema QR" → tras el enfriamiento (10 s por defecto) el aviso desaparece y el
   siguiente QR que se intente cierra el circuito. ("Cerrar circuito ya" lo fuerza al instante.)

### F. Sin conexión (portería)
Con la sesión de **Luis**, en las herramientas del navegador (F12 → Network → "Offline"):
registra un ingreso → queda **pendiente** ("Operaciones guardadas localmente"). Vuelve a "Online" →
se sincroniza solo y el historial lo muestra sin la marca "Sin sincronizar".

## 5. Mirar la base de datos

```bash
docker compose exec db psql -U control_de_accesos -d control_de_accesos
```
```sql
select visitor_name, status, method, valid_from, valid_until from access_grants order by created_at desc limit 10;
select occurred_at, subject_type, movement, result, visitor_name, unit_label from access_logs order by occurred_at desc limit 10;
select created_at, action, result, reason from audit_log order by created_at desc limit 15;   -- seguridad (append-only)
select * from complex_staff;   -- quién es guardia/admin de qué conjunto
```

## 6. Problemas frecuentes

| Síntoma | Causa y arreglo |
|---|---|
| "Pase no válido: aún no está vigente" | El pase empieza más tarde de lo que crees. Mira la hora de inicio; por defecto es *ahora*. |
| "Pase ya fue utilizado" | Es de un solo uso. Genera otro, o `./test/reset_data.sh`. |
| La lista de personas no aparece / `Failed to fetch` | El ms no está arriba o no está en `ENV=development`. `curl localhost:8081/healthz`. |
| Pase o código "desapareció" | Alguien corrió `reset_data.sh` o las pruebas automáticas sobre la misma base. |
| Faltan tablas (`complexes`, `access_logs`…) o personas | Volumen de Postgres viejo: `docker compose down -v` y `up --build`. |
| `/dev/users` da 404 | El binario no se compiló con `-tags dev` (usa `docker compose up --build`; si corres `go run` añade `-tags dev`) o falta `ENV=development`. |
| Un residente no ve el QR de otra unidad | Es a propósito: cada uno solo ve los pases de **sus** unidades. |
| Con QR caído nada cambia en la UI | Pulsa el panel 🛠 y revisa el estado; el circuito necesita 3 fallos para abrirse. |
| El e2e no abre el navegador | `E2E_CHANNEL=chrome` (o instala Edge). |
