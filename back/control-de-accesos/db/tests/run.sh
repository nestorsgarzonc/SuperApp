#!/usr/bin/env bash
# Data-tier suite: starts a throwaway PostgreSQL (same image as docker-compose),
# applies db/migrations in order, runs critical_data.test.sql and removes the
# container. Prints PASS/FAIL per test case; exit code 1 if any case fails.
#
#   back/control-de-accesos/db/tests/run.sh
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
DB_DIR=$(dirname "$HERE")
IMAGE=${PG_IMAGE:-postgres:18.6-alpine}
NAME=superapp-db-tests-$$

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run -d --name "$NAME" -e POSTGRES_PASSWORD=test -e POSTGRES_DB=accesos_test \
	-v "$DB_DIR/migrations:/sql/migrations:ro" -v "$HERE:/sql/tests:ro" "$IMAGE" >/dev/null

for _ in $(seq 1 60); do
	docker exec "$NAME" pg_isready -U postgres -d accesos_test -q 2>/dev/null && break
	sleep 1
done
# pg_isready can answer during the init restart; wait for a real query.
until docker exec "$NAME" psql -U postgres -d accesos_test -tAc 'SELECT 1' >/dev/null 2>&1; do sleep 1; done

psql() { docker exec "$NAME" psql -U postgres -d accesos_test -v ON_ERROR_STOP=1 -q "$@"; }

for f in $(ls "$DB_DIR/migrations" | sort); do
	PGOPTIONS="-c client_min_messages=warning" docker exec -e PGOPTIONS "$NAME" psql -U postgres -d accesos_test -v ON_ERROR_STOP=1 -q -f "/sql/migrations/$f" >/dev/null
done

psql -f /sql/tests/critical_data.test.sql
