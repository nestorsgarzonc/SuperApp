#!/usr/bin/env bash
# End-to-end smoke test of the docker compose stack, through the front's nginx
# (the same path the browser uses). Needs curl and jq; demo data loaded.
#
#   docker compose up -d --build --wait && scripts/smoke.sh
#
# BASE overrides the front URL (default http://localhost:3000).
set -uo pipefail

ROOT=${BASE:-http://localhost:3000}
B=$ROOT/api/v1
C1=00000000-0000-0000-0000-000000000001
C2=00000000-0000-0000-0000-000000000002
UNIT_A101=10000000-0000-0000-0000-000000000101
UNIT_A102=10000000-0000-0000-0000-000000000102
PASSWORD='Demo-Pass-2026!'
BODY=$(mktemp)
trap 'rm -f "$BODY"' EXIT

pass=0
fail=0
check() { # name expected actual
	if [ "$2" = "$3" ]; then
		echo "PASS $1"
		pass=$((pass + 1))
	else
		echo "FAIL $1: want '$2', got '$3'"
		fail=$((fail + 1))
	fi
}
status() { curl -s -o "$BODY" -w '%{http_code}' "$@"; }
login() { curl -s -X POST "$B/auth/login" -H 'Content-Type: application/json' -d "{\"email\":\"$1\",\"password\":\"$PASSWORD\",\"device_name\":\"smoke\"}"; }
access() { echo "$1" | jq -r .tokens.access_token; }
refresh() { echo "$1" | jq -r .tokens.refresh_token; }
auth() { echo "Authorization: Bearer $1"; }
in2h() { date -u -d '+2 hours' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v+2H +%Y-%m-%dT%H:%M:%SZ; }

echo "== auth"
ANA_LOGIN=$(login ana.garcia@demo.test)
ANA=$(access "$ANA_LOGIN")
GUARD=$(access "$(login luis.mora@demo.test)")
check "resident login returns an RS256 token" true "$([ "${#ANA}" -gt 100 ] && echo true || echo false)"
check "auth /users/me" 200 "$(status "$B/users/me" -H "$(auth "$ANA")")"
check "auth /users/me with signed tenant context" 200 "$(status "$B/users/me" -H "$(auth "$ANA")" -H "X-Complex-ID: $C1")"

echo "== memberships (what the front builds the session from)"
check "GET /me/memberships" 200 "$(status "$B/me/memberships" -H "$(auth "$ANA")")"
check "Ana is RESIDENT of A-101 in Los Pinos" "$C1 RESIDENT A-101" "$(jq -r '.data.memberships[0] | "\(.complex_id) \(.role) \(.unit_name)"' "$BODY")"

echo "== gateway rules"
check "own complex" 200 "$(status "$B/access/qr-codes" -H "$(auth "$ANA")" -H "X-Complex-ID: $C1")"
check "foreign complex → 403" 403 "$(status "$B/access/qr-codes" -H "$(auth "$ANA")" -H "X-Complex-ID: $C2")"
check "spoofed X-Complex-Role ignored" 403 "$(status "$B/access/summary" -H "$(auth "$ANA")" -H "X-Complex-ID: $C1" -H 'X-Complex-Role: ADMIN')"
check "guard role from memberships" 200 "$(status "$B/access/summary" -H "$(auth "$GUARD")" -H "X-Complex-ID: $C1")"
check "no token → 401" 401 "$(status "$B/access/qr-codes" -H "X-Complex-ID: $C1")"
check "malformed complex → 422" 422 "$(status "$B/access/qr-codes" -H "$(auth "$ANA")" -H "X-Complex-ID: nope")"
check "/internal via notifications prefix → 404" 404 "$(status -X POST "$B/notifications/internal/notifications" -H "$(auth "$ANA")" -d '{}')"
check "encoded traversal → 404" 404 "$(status -X POST "$B/notifications/x/%2e%2e/internal/notifications" -H "$(auth "$ANA")")"
check "unknown route → 404" 404 "$(status "$B/nope" -H "$(auth "$ANA")")"

echo "== cross-service: control-de-accesos → notificaciones → inbox"
SOFIA=$(access "$(login sofia.diaz@demo.test)")
ANDRES=$(access "$(login andres.vega@demo.test)")
VISITOR="Smoke $(date +%s)"
# Body built outside "$(...)": bash 3.2 (macOS) keeps the backslashes of \"
# inside a command substitution nested in double quotes, mangling the JSON.
QR_BODY=$(jq -nc --arg u "$UNIT_A102" --arg v "$VISITOR" --arg t "$(in2h)" '{unit_id:$u,visitor_name:$v,valid_until:$t}')
check "resident creates a QR pass" true "$(curl -s -X POST "$B/access/qr-codes" -H "$(auth "$SOFIA")" -H "X-Complex-ID: $C1" -H 'Content-Type: application/json' \
	-d "$QR_BODY" | jq -r .success)"
got=false
for _ in $(seq 1 20); do
	if curl -s "$B/notifications/users/me/notifications" -H "$(auth "$ANDRES")" -H "X-Complex-ID: $C1" | jq -e --arg v "$VISITOR" '.data[]? | select(.body | contains($v))' >/dev/null; then
		got=true
		break
	fi
	sleep 0.5
done
check "the other resident of the unit is notified" true "$got"
check "notif unread-count" 200 "$(status "$B/notifications/users/me/notifications/unread-count" -H "$(auth "$ANDRES")" -H "X-Complex-ID: $C1")"
check "notif preferences" 200 "$(status "$B/notifications/me/notification-preferences" -H "$(auth "$ANDRES")")"
check "resident QR for own unit" 200 "$(status "$B/units/$UNIT_A101/access-grants" -H "$(auth "$ANA")" -H "X-Complex-ID: $C1")"

echo "== dev login"
check "/dev/users" 200 "$(status "$B/dev/users")"
DEV=$(curl -s "$B/dev/token?user_id=10000000-0000-0000-0000-000000000003&complex_id=$C1&role=GUARD" | jq -r .data.token)
check "HS256 dev token accepted" 200 "$(status "$B/access-logs?limit=5" -H "$(auth "$DEV")" -H "X-Complex-ID: $C1")"

echo "== session revocation (logout invalidates the access token at once)"
NEW=$(curl -s -X POST "$B/auth/refresh" -H 'Content-Type: application/json' -d "{\"refresh_token\":\"$(refresh "$ANA_LOGIN")\"}")
check "refresh rotates the token" true "$(echo "$NEW" | jq -r '(.tokens.access_token // .access_token) != null')"
ANA2=$(echo "$NEW" | jq -r '.tokens.access_token // .access_token')
check "rotated token works" 200 "$(status "$B/me/memberships" -H "$(auth "$ANA2")")"
check "logout" 204 "$(status -X POST "$B/auth/logout" -H 'Content-Type: application/json' -d "{\"refresh_token\":\"$(echo "$NEW" | jq -r '.tokens.refresh_token // .refresh_token')\"}")"
check "access token after logout → 401" 401 "$(status "$B/me/memberships" -H "$(auth "$ANA2")")"
check "guard's session is unaffected" 200 "$(status "$B/access/summary" -H "$(auth "$GUARD")" -H "X-Complex-ID: $C1")"
PHONE=$(access "$(login carlos.rojas@demo.test)")
LAPTOP=$(access "$(login carlos.rojas@demo.test)")
check "logout-all" 204 "$(status -X POST "$B/auth/logout-all" -H "$(auth "$LAPTOP")" -H 'Content-Type: application/json' -d '{}')"
check "other device after logout-all → 401" 401 "$(status "$B/me/memberships" -H "$(auth "$PHONE")")"

if command -v docker >/dev/null 2>&1 && docker compose ps --status running control-de-accesos 2>/dev/null | grep -q control-de-accesos; then
	echo "== notificaciones without the gateway (private network)"
	# Status of a request made from inside the private network (busybox wget).
	direct() { docker compose exec -T control-de-accesos sh -c "wget -q -S -O /dev/null $1 2>&1 | grep -o 'HTTP/[0-9.]* [0-9]*' | tail -1 | cut -d' ' -f2"; }
	FORGED="x.$(printf '{"sub":"10000000-0000-0000-0000-000000000001"}' | base64 | tr -d '=\n' | tr '/+' '_-').sig"
	check "notificaciones: forged unsigned token → 401" 401 "$(direct "--header 'Authorization: Bearer $FORGED' http://notificaciones:8083/me/devices")"
	check "notificaciones: bare X-User-ID without gateway secret → 401" 401 "$(direct "--header 'X-User-ID: 10000000-0000-0000-0000-000000000001' http://notificaciones:8083/me/devices")"
	check "control-de-accesos: bare X-User-ID without gateway secret → 401" 401 "$(direct "--header 'X-User-ID: 10000000-0000-0000-0000-000000000001' http://127.0.0.1:8080/api/v1/me/memberships")"
fi

echo "== front"
check "index" 200 "$(status "$ROOT/")"
check "SPA fallback /dev-login" 200 "$(status "$ROOT/dev-login")"

echo "---- $pass passed, $fail failed"
[ "$fail" -eq 0 ]
