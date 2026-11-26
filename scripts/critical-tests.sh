#!/usr/bin/env bash
# Delivery 1 — component-level verification. Runs one suite per tier and
# prints a verdict per test case:
#   1. presentation tier: front/src/test/critical (Vitest + MSW)
#   2. data tier:         back/control-de-accesos/db/tests (PostgreSQL in Docker)
# Exit code is non-zero if any suite fails. Needs Node 20+ and Docker.
set -uo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
status=0

echo "== [1/2] Presentation tier (front) =="
(cd "$ROOT/front" && npx vitest run src/test/critical --reporter=verbose) || status=1

echo
echo "== [2/2] Data tier (control-de-accesos schema) =="
"$ROOT/back/control-de-accesos/db/tests/run.sh" || status=1

echo
[ "$status" -eq 0 ] && echo "ALL SUITES PASSED" || echo "SOME SUITES FAILED"
exit "$status"
