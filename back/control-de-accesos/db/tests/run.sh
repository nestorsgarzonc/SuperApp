#!/usr/bin/env bash
# Data-tier suite: starts a throwaway PostgreSQL (same image as docker-compose),
# applies db/migrations in order, runs critical_data.test.sql and removes the
# container. Prints PASS/FAIL/SKIP per test case; exit code 1 if any case fails.
#
#   back/control-de-accesos/db/tests/run.sh
#
# MIGRATIONS_DIR  apply another copy of the migrations (used by mutation.sh).
# PG_ADMIN_URL    use an existing PostgreSQL instead of Docker: a fresh database
#                 is created there and dropped at the end
#                 (e.g. postgres://postgres@localhost:5432/postgres).
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
DB_DIR=$(dirname "$HERE")
MIG_DIR=$(cd "${MIGRATIONS_DIR:-$DB_DIR/migrations}" && pwd)
IMAGE=${PG_IMAGE:-postgres:18.6-alpine}
NAME=superapp-db-tests-$$
DB=accesos_test_$$

if [ -n "${PG_ADMIN_URL:-}" ]; then
	command psql "$PG_ADMIN_URL" -qc "CREATE DATABASE $DB" >/dev/null
	cleanup() { command psql "$PG_ADMIN_URL" -qc "DROP DATABASE IF EXISTS $DB" >/dev/null 2>&1 || true; }
	trap cleanup EXIT
	URL=$(python3 -c 'import sys,urllib.parse as u; p=u.urlsplit(sys.argv[1]); print(u.urlunsplit(p._replace(path="/"+sys.argv[2])))' "$PG_ADMIN_URL" "$DB")
	run_file() { PGOPTIONS="-c client_min_messages=warning" command psql "$URL" -v ON_ERROR_STOP=1 -q -f "$1"; }
	for f in $(ls "$MIG_DIR" | sort); do run_file "$MIG_DIR/$f" >/dev/null; done
	run_file "$HERE/critical_data.test.sql"
	exit
fi

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run -d --name "$NAME" -e POSTGRES_PASSWORD=test -e POSTGRES_DB=accesos_test \
	-v "$MIG_DIR:/sql/migrations:ro" -v "$HERE:/sql/tests:ro" "$IMAGE" >/dev/null

for _ in $(seq 1 60); do
	docker exec "$NAME" pg_isready -U postgres -d accesos_test -q 2>/dev/null && break
	sleep 1
done
# pg_isready can answer during the init restart; wait for a real query.
until docker exec "$NAME" psql -U postgres -d accesos_test -tAc 'SELECT 1' >/dev/null 2>&1; do sleep 1; done

psql() { docker exec "$NAME" psql -U postgres -d accesos_test -v ON_ERROR_STOP=1 -q "$@"; }

for f in $(ls "$MIG_DIR" | sort); do
	PGOPTIONS="-c client_min_messages=warning" docker exec -e PGOPTIONS "$NAME" psql -U postgres -d accesos_test -v ON_ERROR_STOP=1 -q -f "/sql/migrations/$f" >/dev/null
done

psql -f /sql/tests/critical_data.test.sql
