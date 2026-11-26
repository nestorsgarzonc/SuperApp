#!/usr/bin/env bash
# Delivery 1 — component-level verification. Runs one suite per tier and
# prints a verdict per test case:
#   1. presentation tier: front/src/test/critical (Vitest + MSW)
#   2. data tier:         back/control-de-accesos/db/tests (PostgreSQL in Docker)
#   3. logic tier:        back/control-de-accesos/ms unit tests (Go; needs Go 1.26,
#                         or set GOFLAGS/GOPROXY as described in scripts/test-unit.sh)
# Exit code is non-zero if any suite fails. Needs Node 20+ and Docker.
set -uo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
status=0

echo "== [1/3] Presentation tier (front) =="
(cd "$ROOT/front" && npx vitest run src/test/critical --reporter=verbose) || status=1

echo
echo "== [2/3] Data tier (control-de-accesos schema) =="
"$ROOT/back/control-de-accesos/db/tests/run.sh" || status=1

echo
echo "== [3/3] Logic tier (control-de-accesos back, Go unit tests) =="
"$ROOT/scripts/test-unit.sh" || status=1

echo
[ "$status" -eq 0 ] && echo "ALL SUITES PASSED" || echo "SOME SUITES FAILED"
exit "$status"
