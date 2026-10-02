#!/usr/bin/env bash
# End-to-end smoke test against a running control-de-accesos instance.
# Usage: BASE_URL=http://localhost:8080 ./test/smoke_test.sh
#
# Assumes a freshly migrated+seeded database (it cancels seed grants and
# relies on exact seed IDs) — reset with db/migrate.sh against an empty
# database before running.
set -uo pipefail

BASE="${BASE_URL:-http://localhost:8080}"
COMPLEX_ID="00000000-0000-0000-0000-000000000001"
UNIT_ID="10000000-0000-0000-0000-000000000101"
GHOST_UNIT_ID="99999999-9999-9999-9999-999999999999"
GHOST_GRANT_ID="88888888-8888-8888-8888-888888888888"
SEED_QR_ID="20000000-0000-0000-0000-000000000001"
SEED_PREAUTH_ID="20000000-0000-0000-0000-000000000002"

RESP_BODY="$(mktemp)"
trap 'rm -f "$RESP_BODY"' EXIT

TOKEN=$(curl -s "$BASE/api/v1/dev/token" | python3 -c "import sys,json;print(json.load(sys.stdin)['token'])")

PASS=0
FAIL=0

check() {
  local desc="$1" expected="$2" actual="$3"
  if [[ "$actual" == "$expected" ]]; then
    PASS=$((PASS+1))
    printf "OK   [%s] %s\n" "$actual" "$desc"
  else
    FAIL=$((FAIL+1))
    printf "FAIL [esperado %s, dio %s] %s\n" "$expected" "$actual" "$desc"
  fi
}

req() {
  local method="$1" path="$2" body="${3:-}"
  if [[ -n "$body" ]]; then
    curl -s -o "$RESP_BODY" -w "%{http_code}" -X "$method" "$BASE$path" \
      -H "Authorization: Bearer $TOKEN" -H "X-Complex-ID: $COMPLEX_ID" -H "Content-Type: application/json" \
      --data "$body"
  else
    curl -s -o "$RESP_BODY" -w "%{http_code}" -X "$method" "$BASE$path" \
      -H "Authorization: Bearer $TOKEN"
  fi
}

echo "=== Infra ==="
check "GET /healthz" 200 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/healthz")"
check "GET /api/v1/dev/token" 200 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/dev/token")"

echo
echo "=== Auth guards ==="
check "GET qr-codes sin token -> 401" 401 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/access/qr-codes?complex_id=$COMPLEX_ID")"
check "GET qr-codes token basura -> 401" 401 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/access/qr-codes?complex_id=$COMPLEX_ID" -H "Authorization: Bearer basura")"
check "PUT /access-grants/validate (metodo no permitido) -> 405" 405 "$(curl -s -o /dev/null -w '%{http_code}' -X PUT "$BASE/api/v1/access-grants/validate" -H "Authorization: Bearer $TOKEN" -H "X-Complex-ID: $COMPLEX_ID")"

echo
echo "=== QR codes ==="
check "GET qr-codes (list)" 200 "$(req GET "/api/v1/access/qr-codes?complex_id=$COMPLEX_ID")"
check "POST qr-codes (valido)" 201 "$(req POST "/api/v1/access/qr-codes" "{\"unit_id\":\"$UNIT_ID\",\"visitor_name\":\"Test\",\"valid_until\":\"2026-12-31T23:59:59Z\"}")"
check "POST qr-codes (falta unit_id)" 422 "$(req POST "/api/v1/access/qr-codes" "{\"valid_until\":\"2026-12-31T23:59:59Z\"}")"
check "POST qr-codes (JSON malformado)" 400 "$(req POST "/api/v1/access/qr-codes" "{\"unit_id\":\"unit_id\":\"x\"}")"
check "POST qr-codes (unit inexistente)" 404 "$(req POST "/api/v1/access/qr-codes" "{\"unit_id\":\"$GHOST_UNIT_ID\",\"valid_until\":\"2026-12-31T23:59:59Z\"}")"
check "POST qr-codes/batch (valido, qty=3)" 201 "$(req POST "/api/v1/access/qr-codes/batch" "{\"unit_id\":\"$UNIT_ID\",\"quantity\":3,\"valid_from\":\"2026-01-01T00:00:00Z\",\"valid_until\":\"2026-12-31T23:59:59Z\"}")"
check "POST qr-codes/batch (quantity=0 invalido)" 422 "$(req POST "/api/v1/access/qr-codes/batch" "{\"unit_id\":\"$UNIT_ID\",\"quantity\":0,\"valid_from\":\"2026-01-01T00:00:00Z\",\"valid_until\":\"2026-12-31T23:59:59Z\"}")"

echo
echo "=== Pre-auth ==="
check "GET pre-auth (list)" 200 "$(req GET "/api/v1/access/pre-auth?complex_id=$COMPLEX_ID")"
check "POST pre-auth (valido)" 201 "$(req POST "/api/v1/access/pre-auth" "{\"unit_id\":\"$UNIT_ID\",\"visitor_name\":\"Rappi\",\"visitor_id_number\":\"COL1\",\"visitor_type\":\"DELIVERY\",\"valid_from\":\"2026-01-01T00:00:00Z\",\"valid_until\":\"2026-12-31T23:59:59Z\"}")"
check "POST pre-auth (visitor_type invalido)" 422 "$(req POST "/api/v1/access/pre-auth" "{\"unit_id\":\"$UNIT_ID\",\"visitor_name\":\"X\",\"visitor_id_number\":\"COL2\",\"visitor_type\":\"NOPE\",\"valid_from\":\"2026-01-01T00:00:00Z\",\"valid_until\":\"2026-12-31T23:59:59Z\"}")"
check "POST pre-auth (unit inexistente)" 404 "$(req POST "/api/v1/access/pre-auth" "{\"unit_id\":\"$GHOST_UNIT_ID\",\"visitor_name\":\"X\",\"visitor_id_number\":\"COL3\",\"visitor_type\":\"DELIVERY\",\"valid_from\":\"2026-01-01T00:00:00Z\",\"valid_until\":\"2026-12-31T23:59:59Z\"}")"

echo
echo "=== Access grants genericos ==="
check "POST access-grants (valido)" 201 "$(req POST "/api/v1/access-grants" "{\"unit_id\":\"$UNIT_ID\",\"method\":\"QR\",\"visitor_type\":\"VISITOR\",\"visitor_name\":\"Gen\",\"valid_until\":\"2026-12-31T23:59:59Z\"}")"
check "POST access-grants (method invalido)" 422 "$(req POST "/api/v1/access-grants" "{\"unit_id\":\"$UNIT_ID\",\"method\":\"FAX\",\"visitor_type\":\"VISITOR\",\"visitor_name\":\"Gen\",\"valid_until\":\"2026-12-31T23:59:59Z\"}")"
check "GET access-grants/{id seed valido}" 200 "$(req GET "/api/v1/access-grants/$SEED_QR_ID")"
check "GET access-grants/{uuid invalido}" 400 "$(req GET "/api/v1/access-grants/no-es-un-uuid")"
check "GET qr-codes (complex_id invalido en query)" 400 "$(req GET "/api/v1/access/qr-codes?complex_id=no-es-uuid")"
check "GET access-grants/{uuid valido pero no existe}" 404 "$(req GET "/api/v1/access-grants/$GHOST_GRANT_ID")"

echo
echo "=== Cancelar ==="
check "PATCH access-grants/{seed pre-auth}/cancel (valido)" 200 "$(req PATCH "/api/v1/access-grants/$SEED_PREAUTH_ID/cancel")"
check "PATCH access-grants/{mismo id}/cancel otra vez (ya cancelado)" 404 "$(req PATCH "/api/v1/access-grants/$SEED_PREAUTH_ID/cancel")"
check "PATCH access-grants/{id inexistente}/cancel" 404 "$(req PATCH "/api/v1/access-grants/$GHOST_GRANT_ID/cancel")"

echo
echo "=== Validate ==="
check "POST validate (QR seed, valido)" 200 "$(req POST "/api/v1/access-grants/validate" '{"method":"QR","qr_code":"demo-qr-hash-0001"}')"
check "POST validate (QR sin qr_code)" 422 "$(req POST "/api/v1/access-grants/validate" '{"method":"QR"}')"
check "POST validate (PRE_AUTH ya cancelado -> valid:false pero 200)" 200 "$(req POST "/api/v1/access-grants/validate" '{"method":"PRE_AUTH","visitor_id_number":"COL123456"}')"
check "POST validate (metodo no soportado)" 422 "$(req POST "/api/v1/access-grants/validate" '{"method":"FAX"}')"
check "POST validate (JSON malformado)" 400 "$(req POST "/api/v1/access-grants/validate" '{"method":"QR","qr_code":}')"

echo
echo "=== Unit-scoped ==="
check "GET units/{id}/access-grants" 200 "$(req GET "/api/v1/units/$UNIT_ID/access-grants")"
check "GET units/{uuid invalido}/access-grants" 400 "$(req GET "/api/v1/units/no-es-uuid/access-grants")"

echo
echo "=================================="
echo "PASS=$PASS FAIL=$FAIL"
echo "=================================="
exit $FAIL
