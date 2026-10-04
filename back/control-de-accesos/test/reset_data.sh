#!/usr/bin/env bash
# Deja los datos de PRUEBA como recién sembrados: borra pases, bitácora y auditoría
# y vuelve a cargar personas, conjuntos y pases de ejemplo (db/seeds/001_demo.sql).
#
# Las pruebas consumen pases de un solo uso, así que conviene correrlo antes de cada
# batería. NO usar contra datos reales.
#
# Con Docker (por defecto, desde back/control-de-accesos):   ./test/reset_data.sh
# Contra otro Postgres:  PSQL_CMD="psql -h HOST -p PUERTO -U USUARIO -d BASE -q" ./test/reset_data.sh
set -euo pipefail
cd "$(dirname "$0")/.."

PSQL_CMD="${PSQL_CMD:-docker compose exec -T db psql -U control_de_accesos -d control_de_accesos -q}"
export PGCLIENTENCODING=UTF8   # los seeds llevan acentos

$PSQL_CMD -v ON_ERROR_STOP=1 <<'SQL'
TRUNCATE access_logs, access_grants;
ALTER TABLE audit_log DISABLE TRIGGER USER;   -- audit_log es append-only por triggers
DELETE FROM audit_log;
ALTER TABLE audit_log ENABLE TRIGGER USER;
SQL

$PSQL_CMD -v ON_ERROR_STOP=1 < db/seeds/001_demo.sql > /dev/null
echo "Datos de prueba reiniciados."
