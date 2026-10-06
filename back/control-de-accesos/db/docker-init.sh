#!/bin/sh
# Solo para el contenedor local: corre en el primer arranque (docker-entrypoint-initdb.d).
# Aplica db/migrations en orden y luego los datos de demo (db/seeds, solo dev).
# Se monta aparte para no crear archivos de montaje dentro de db/migrations del host.
set -eu
export PGCLIENTENCODING=UTF8
for f in /sql/migrations/*.sql /sql/seeds/*.sql; do
  echo "init: $f"
  psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f "$f"
done
