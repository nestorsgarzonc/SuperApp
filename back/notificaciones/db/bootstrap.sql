-- notificaciones: preparación única de la base compartida (RDS).
--
-- Todos los ms usan la misma base de datos; cada uno vive en su propio schema
-- con su propio rol. Este script crea el rol `notificaciones` y el schema
-- `notificaciones`; el rol solo puede crear y usar objetos en su schema.
--
-- Se ejecuta UNA vez, con el usuario maestro de RDS (no con el rol del ms):
--   psql "host=<rds> dbname=<db> user=<master> sslmode=require" \
--     -v app_password='<clave>' -f db/bootstrap.sql
-- Es idempotente: volver a ejecutarlo no cambia nada (ni la clave).

\set ON_ERROR_STOP on

SELECT format('CREATE ROLE notificaciones LOGIN PASSWORD %L', :'app_password')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'notificaciones')
\gexec

SELECT format('GRANT CONNECT ON DATABASE %I TO notificaciones', current_database())
\gexec

-- El schema es del maestro; el rol recibe USAGE + CREATE, así las tablas que
-- crean las migraciones quedan a nombre de `notificaciones`.
CREATE SCHEMA IF NOT EXISTS notificaciones;
GRANT USAGE, CREATE ON SCHEMA notificaciones TO notificaciones;

-- Respaldo de DATABASE_URL (?search_path=notificaciones): las consultas sin
-- schema resuelven siempre a `notificaciones`, nunca a `public`.
ALTER ROLE notificaciones SET search_path = notificaciones;
