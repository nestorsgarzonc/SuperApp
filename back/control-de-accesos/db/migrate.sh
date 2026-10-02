#!/usr/bin/env bash
# Applies db/migrations/*.sql in order against whatever Postgres the DB_* env
# vars point to — the local Docker container (docker-entrypoint-initdb.d does
# this automatically on first boot) or a real AWS RDS instance (which has no
# such hook, so this script is how you'd bootstrap/upgrade its schema).
#
# All migrations are idempotent (CREATE TABLE IF NOT EXISTS, ON CONFLICT DO
# NOTHING), so re-running this against an already-migrated database is safe.
#
# Required env vars: DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME
# Optional: DB_SSLMODE (default require — RDS should not be reached in
# sslmode=disable), DB_SSLROOTCERT (CA bundle path, for sslmode=verify-full)
set -euo pipefail

: "${DB_HOST:?Set DB_HOST (local: localhost, RDS: the instance endpoint)}"
: "${DB_PORT:=5432}"
: "${DB_USER:?Set DB_USER}"
: "${DB_PASSWORD:?Set DB_PASSWORD}"
: "${DB_NAME:?Set DB_NAME}"
: "${DB_SSLMODE:=require}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

CONNINFO="host=$DB_HOST port=$DB_PORT user=$DB_USER dbname=$DB_NAME sslmode=$DB_SSLMODE"
if [[ -n "${DB_SSLROOTCERT:-}" ]]; then
  CONNINFO="$CONNINFO sslrootcert=$DB_SSLROOTCERT"
fi

for f in "$SCRIPT_DIR"/migrations/*.sql; do
  echo "Applying $(basename "$f")..."
  PGPASSWORD="$DB_PASSWORD" psql "$CONNINFO" -v ON_ERROR_STOP=1 -f "$f"
done

echo "Done."
