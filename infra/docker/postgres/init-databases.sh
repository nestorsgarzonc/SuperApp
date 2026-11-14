#!/bin/sh
# Runs once, on the first boot of an empty volume (docker-entrypoint-initdb.d).
# One Postgres server for the local stack, but each microservice gets its own
# database and its own role: no service can read another one's data, same
# isolation rule as production (database-per-service).
set -eu

create() { # <role/db name> <password>
  psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d postgres \
    -v name="$1" -v pass="$2" <<'SQL'
SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'name', :'pass')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'name')
\gexec
SELECT format('CREATE DATABASE %I OWNER %I', :'name', :'name')
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = :'name')
\gexec
SELECT format('REVOKE ALL ON DATABASE %I FROM PUBLIC', :'name')
\gexec
SQL
  echo "init: database and role '$1' ready"
}

create auth_db             "$AUTH_DB_PASSWORD"
create control_de_accesos  "$CONTROL_ACCESOS_DB_PASSWORD"
create notificaciones      "$NOTIFICACIONES_DB_PASSWORD"
