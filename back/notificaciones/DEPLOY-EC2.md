# Desplegar notificaciones en EC2 (consola de AWS)

Resultado: un EC2 con Docker que corre el ms en el puerto 8083 y se conecta a
la RDS compartida, en su propio schema `notificaciones`.

```
 control-de-accesos ──POST /internal/notifications──┐
 api-gateway (futuro) ──/users/me/notifications────┤
                                                   ▼
                                  EC2  sg-notificaciones  :8083
                                                   │ 5432
                                                   ▼
                              RDS (base compartida, schema notificaciones)
```

## 0. Datos de la RDS

En **RDS → Databases → tu instancia** anota:

| Dónde | Dato |
|---|---|
| *Connectivity & security* | **Endpoint**, **VPC**, **security group** de la RDS |
| *Configuration* | **Engine version**: debe ser **PostgreSQL 18 o superior** (las migraciones usan `uuidv7()`) |
| *Configuration* | **DB name** (si está vacío, la base es `postgres`) |

Trabaja todo en la **misma región** que la RDS.

## 1. Security group del EC2

**EC2 → Security Groups → Create security group**

- Nombre: `sg-notificaciones`, VPC: **la misma de la RDS**.
- Inbound:
  - `SSH` (22) — Source: **My IP**.
  - `Custom TCP` 8083 — Source: el security group de control-de-accesos (o, mientras
    tanto, el CIDR de la VPC, p. ej. `172.31.0.0/16`). **Nunca `0.0.0.0/0`.**
- Outbound: dejar *All traffic* (necesita salir a la RDS y a internet para FCM).

## 2. Permitir el EC2 en la RDS

**EC2 → Security Groups → (security group de la RDS) → Edit inbound rules → Add rule**

- Type `PostgreSQL` (5432), Source: **`sg-notificaciones`** (el security group,
  no una IP: así sigue funcionando si la IP del EC2 cambia).

## 3. Lanzar el EC2

**EC2 → Instances → Launch instances**

| Campo | Valor |
|---|---|
| Name | `notificaciones` |
| AMI | Amazon Linux 2023 |
| Instance type | `t4g.small` (ARM, recomendado) o `t3.small` (x86). `t4g.micro`/`t3.micro` alcanzan: el ms usa < 100 MB |
| Key pair | crear o elegir uno (guardar el `.pem` en `~/.ssh/` con `chmod 400`) |
| Network settings → Edit | VPC de la RDS; una subnet **pública**; *Auto-assign public IP*: **Enable**; security group: `sg-notificaciones` |
| Storage | 8–16 GB gp3 |
| Advanced details → User data | el script de abajo |

```bash
#!/bin/bash
dnf install -y docker
systemctl enable --now docker
usermod -aG docker ec2-user
mkdir -p /usr/local/lib/docker/cli-plugins
curl -fsSL "https://github.com/docker/compose/releases/download/v2.39.4/docker-compose-linux-$(uname -m)" \
  -o /usr/local/lib/docker/cli-plugins/docker-compose
chmod +x /usr/local/lib/docker/cli-plugins/docker-compose
```

Al terminar, anota la **IP pública** (para SSH) y la **IP privada** (para que los
demás ms lo llamen). Espera ~2 min a que corra el user data.

## 4. Subir la imagen y los archivos (desde tu Mac)

Desde `back/notificaciones/`:

```bash
make ship HOST=ec2-user@<ip-publica> KEY=~/.ssh/<key>.pem PLATFORM=linux/arm64   # t4g
make ship HOST=ec2-user@<ip-publica> KEY=~/.ssh/<key>.pem PLATFORM=linux/amd64   # t3
```

Construye la imagen localmente, la carga en el EC2 y copia
`docker-compose.prod.yml`, `.env.prod.example` y `db/` a `~/notificaciones/`.

## 5. Preparar la base (una sola vez, en el EC2)

```bash
ssh -i ~/.ssh/<key>.pem ec2-user@<ip-publica>
cd ~/notificaciones

# Probar conectividad EC2 → RDS
timeout 3 bash -c '</dev/tcp/<endpoint-rds>/5432' && echo "RDS alcanzable"

# Crear rol y schema con el usuario maestro (pide la clave del maestro)
docker run --rm -it -v "$PWD/db:/db:ro" postgres:18-alpine \
  psql "host=<endpoint-rds> dbname=<db-name> user=<usuario-maestro> sslmode=require" \
  -v app_password='<clave-nueva-para-notificaciones>' -f /db/bootstrap.sql
```

La clave del maestro solo se usa aquí. El ms se conecta con el rol
`notificaciones`, que no puede leer los schemas de los demás ms.

## 6. Configurar y arrancar

```bash
cp .env.prod.example .env.prod && chmod 600 .env.prod
openssl rand -hex 32   # una vez para GATEWAY_SECRET y otra para INTERNAL_SECRET
nano .env.prod
```

| Variable | Valor |
|---|---|
| `MS_IMAGE` | `notificaciones:latest` |
| `DATABASE_URL` | `postgres://notificaciones:<clave-nueva>@<endpoint-rds>:5432/<db-name>?sslmode=require&search_path=notificaciones` |
| `GATEWAY_SECRET` | generado |
| `INTERNAL_SECRET` | generado; **el mismo** en control-de-accesos |

Si la clave tiene `@ : / ? # &` o espacios, va URL-encoded en `DATABASE_URL`.

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod --profile tools run --rm migrate
docker compose -f docker-compose.prod.yml --env-file .env.prod up -d
curl -s localhost:8083/healthz                  # {"data":{"status":"ok"},"success":true}
docker logs -f notificaciones_ms
```

El contenedor tiene `restart: unless-stopped`: vuelve solo si el EC2 se reinicia.

## 7. Conectar control-de-accesos

En el `.env.prod` de control-de-accesos:

```
NOTIFICACIONES_URL=http://<ip-privada-del-ec2>:8083
INTERNAL_SECRET=<el mismo valor>
```

Y en `sg-notificaciones`, el puerto 8083 debe aceptar el security group de
control-de-accesos.

## 8. Push con Firebase (opcional)

Sin esto el ms funciona igual, solo con la bandeja (log: `push disabled, inbox only`).

**En Firebase console** (un solo proyecto para back y front):

1. *Project settings → Service accounts → Generate new private key*: descarga
   un JSON. Es un secreto: no va al repo.
2. *Project settings → General → Your apps → Add app → Web*: copia
   `apiKey`, `authDomain`, `projectId`, `messagingSenderId`, `appId`.
3. *Project settings → Cloud Messaging → Web Push certificates → Generate key pair*:
   copia la clave pública (VAPID).

**Backend** (desde tu Mac, luego en el EC2):

```bash
scp -i ~/.ssh/<key>.pem <descarga>.json ec2-user@<ip-publica>:~/notificaciones/secrets/firebase.json
ssh -i ~/.ssh/<key>.pem ec2-user@<ip-publica> 'chmod 644 ~/notificaciones/secrets/firebase.json'
```

En `.env.prod`:

```
FIREBASE_CREDENTIALS_FILE=/secrets/firebase.json
FIREBASE_PROJECT_ID=<projectId>
WEB_APP_URL=https://<dominio-del-front>
```

`docker compose -f docker-compose.prod.yml --env-file .env.prod up -d --force-recreate`
y comprobar que el log ya no dice `push disabled`. (644 porque el contenedor
corre sin root y debe poder leerlo; el EC2 solo lo usa `ec2-user`.)

**Front**: las `VITE_FIREBASE_*` de `front/.env.example` con los datos de los
pasos 2 y 3, antes de `npm run build`. El push web exige que el front se sirva
por **HTTPS** (o `localhost` en desarrollo). El usuario lo activa en
*Notificaciones → Preferencias → Notificaciones en este navegador*.

## Actualizar a una versión nueva

```bash
make ship HOST=ec2-user@<ip-publica> KEY=~/.ssh/<key>.pem PLATFORM=linux/arm64
ssh -i ~/.ssh/<key>.pem ec2-user@<ip-publica> 'cd ~/notificaciones && \
  docker compose -f docker-compose.prod.yml --env-file .env.prod --profile tools run --rm migrate && \
  docker compose -f docker-compose.prod.yml --env-file .env.prod up -d --force-recreate'
```

## Problemas comunes

| Síntoma | Causa |
|---|---|
| `RDS alcanzable` no aparece / timeout | regla 5432 de la RDS sin `sg-notificaciones`, o EC2 en otra VPC |
| `password authentication failed` | clave mal escrita o sin URL-encode en `DATABASE_URL` |
| `no schema has been selected to create in` | falta `search_path=notificaciones` o no se corrió `bootstrap.sql` |
| `function uuidv7() does not exist` | la RDS es PostgreSQL < 18 |
| `exec format error` | la imagen se construyó para otra arquitectura (`PLATFORM`) |
| `INTERNAL_SECRET is required` / `GATEWAY_SECRET is required` | falta en `.env.prod` |
| `failed to initialize firebase app` | ruta de `FIREBASE_CREDENTIALS_FILE` mal escrita o el JSON sin permiso 644 |
| Push en estado `PUSH_FAILED` | `FIREBASE_PROJECT_ID` no es el del JSON, o la API *Firebase Cloud Messaging* está apagada en Google Cloud |
