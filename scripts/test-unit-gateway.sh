#!/usr/bin/env bash
# Unit tests of the API gateway (C5) with per-case traceability.
#
#   scripts/test-unit-gateway.sh [--cover] [--mutation]
#
#   (no option)  go test -race -count=1 -v ./...  ->  one line per case with its ID and verdict,
#                then a table per part
#   --cover      statement coverage per part (-coverpkg=./internal/..., so code exercised from
#                another package counts)
#   --mutation   mutation testing with gremlins v0.6.0 on every internal package, run on a COPY
#                of the module in a temp dir (the repo is never modified). Uses $GREMLINS or
#                gremlins from PATH; if missing it is built into $TOOLS.
#
# Case IDs: Test<PREFIX><NN>_<Desc> -> <PREFIX>-<NN>  (TestPL05_X -> PL-05), and table
# subtests named "PL-05c ..." count as cases of their own.
# Prefix -> part: AN authn · RV revocation · MB membership · RL ratelimit · RT routes ·
#                 CR cors · PL pipeline · AC acceptance (gateway) · AE apierr · CF config
#
# Exit status: non-zero if any test fails or go test itself fails.
set -uo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
MOD=$ROOT/back/api-gateway
TOOLS=${TOOLS:-$HOME/.cache/gw-quality-tools}

DO_COVER=0
DO_MUTATION=0
for arg in "$@"; do
	case $arg in
	--cover) DO_COVER=1 ;;
	--mutation) DO_MUTATION=1 ;;
	-h | --help)
		sed -n '2,/^set -uo/p' "${BASH_SOURCE[0]}" | sed '$d' | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*)
		echo "unknown option: $arg (use --cover --mutation)" >&2
		exit 2
		;;
	esac
done

command -v go >/dev/null 2>&1 || {
	echo "go not found in PATH" >&2
	exit 2
}
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

# ---------------------------------------------------------------------------
# 1. Tests, one line per case
# ---------------------------------------------------------------------------
echo "=== API gateway (go test -race -count=1 -v ./...) ==="
COVERARGS=()
if [ "$DO_COVER" = 1 ]; then COVERARGS=(-coverpkg=./internal/... -coverprofile="$TMP/cover.out"); fi
(cd "$MOD" && go test -race -count=1 -v ${COVERARGS[@]+"${COVERARGS[@]}"} ./...) >"$TMP/test.log" 2>&1
RC=$?

awk '
BEGIN {
	part["AN"] = "Autenticación (authn)"; part["RV"] = "Revocación (revocation)"
	part["MB"] = "Membresías (membership)"; part["RL"] = "Límite de peticiones (ratelimit)"
	part["RT"] = "Enrutamiento (gateway)"; part["CR"] = "CORS (gateway)"
	part["PL"] = "Pipeline (gateway)"; part["AC"] = "Aceptación (gateway)"
	part["AE"] = "Errores (apierr)"; part["CF"] = "Configuración (config)"
	split("AN RV MB RL RT CR PL AC AE CF", order, " ")
}
/^ *--- (PASS|FAIL|SKIP): / {
	verdict = $2; sub(/:$/, "", verdict)
	name = $3
	n++; names[n] = name; res[name] = verdict
}
END {
	for (i = 1; i <= n; i++) { split(names[i], segs, "/"); parent = names[i]; sub(/\/[^\/]*$/, "", parent); if (parent != names[i]) hasChild[parent] = 1 }
	for (i = 1; i <= n; i++) {
		name = names[i]
		if (hasChild[name]) continue
		top = name; sub(/\/.*/, "", top)
		if (match(top, /^Test[A-Z][A-Z][0-9][0-9]/) == 0) continue
		pre = substr(top, 5, 2)
		id = pre "-" substr(top, 7, 2)
		label = top; sub(/^Test[A-Z]+[0-9]+_/, "", label)
		if (name != top) {
			label = name; sub(/^[^\/]*\//, "", label); gsub(/_/, " ", label)
			if (match(label, /^[A-Z][A-Z]-[0-9][0-9][a-z]? /)) { id = substr(label, 1, RLENGTH - 1); label = substr(label, RLENGTH + 1) }
		}
		printf "%-5s %-7s %s\n", res[name], id, label
		total[pre]++; if (res[name] == "PASS") pass[pre]++; else if (res[name] == "FAIL") fail[pre]++
	}
	print ""
	printf "%-36s %7s %6s %6s\n", "Parte", "Pruebas", "PASS", "FAIL"
	for (k = 1; k <= 10; k++) {
		p = order[k]; if (!total[p]) continue
		printf "%-36s %7d %6d %6d\n", part[p], total[p], pass[p], fail[p]
		T += total[p]; P += pass[p]; F += fail[p]
	}
	printf "%-36s %7d %6d %6d\n", "Total", T, P, F
}' "$TMP/test.log"

if [ "$RC" -ne 0 ]; then
	echo
	grep -E '^(FAIL|---|\s+\S+_test\.go:[0-9]+:)|panic|cannot|undefined' "$TMP/test.log" | head -40
	echo
	echo "VEREDICTO: FAIL"
	exit "$RC"
fi

# ---------------------------------------------------------------------------
# 2. Coverage per part
# ---------------------------------------------------------------------------
if [ "$DO_COVER" = 1 ]; then
	echo
	echo "=== Cobertura de sentencias por paquete ==="
	# cover.out: file:start.col,end.col numStatements count. With -coverpkg every test binary
	# reports every block, so a block counts as covered if any binary ran it.
	awk -F'[: ]' 'NR > 1 {
		key = $1 ":" $2
		file[key] = $1; n[key] = $3
		if ($4 > 0) hit[key] = 1
	}
	END {
		for (key in n) {
			pkg = file[key]; sub(/\/[^\/]*$/, "", pkg); sub(/.*\/internal\//, "", pkg)
			stmts[pkg] += n[key]; if (key in hit) cov[pkg] += n[key]
		}
		for (p in stmts) { printf "%-12s %6.1f %%  (%d/%d)\n", p, 100 * cov[p] / stmts[p], cov[p], stmts[p]; S += stmts[p]; C += cov[p] }
		printf "%-12s %6.1f %%  (%d/%d)\n", "~total", 100 * C / S, C, S
	}' "$TMP/cover.out" | sort | sed 's/^~total/total /'
fi

# ---------------------------------------------------------------------------
# 3. Mutation testing
# ---------------------------------------------------------------------------
if [ "$DO_MUTATION" = 1 ]; then
	GREMLINS=${GREMLINS:-$(command -v gremlins || true)}
	if [ -z "$GREMLINS" ]; then
		GREMLINS=$TOOLS/gremlins
		if [ ! -x "$GREMLINS" ]; then
			echo "instalando gremlins v0.6.0 en $TOOLS ..."
			mkdir -p "$TOOLS"
			(cd "$TMP" && GOBIN="$TOOLS" go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0) || exit 2
		fi
	fi
	echo
	echo "=== Mutación (gremlins, sobre una copia del módulo) ==="
	cp -r "$MOD" "$TMP/mod"
	printf "%-12s %7s %6s %9s %10s\n" "Paquete" "Muertos" "Vivos" "Timeout" "Eficacia"
	for pkg in apierr authn config gateway membership ratelimit revocation; do
		(cd "$TMP/mod" && "$GREMLINS" unleash --timeout-coefficient 20 --workers 1 "./internal/$pkg") >"$TMP/mut-$pkg.log" 2>&1
		awk -v pkg="$pkg" '
			/^Killed:/ { gsub(/,/, ""); k = $2; l = $4 }
			/^Timed out:/ { gsub(/,/, ""); to = $3 }
			/^Test efficacy:/ { e = $3 }
			END { printf "%-12s %7d %6d %9d %10s\n", pkg, k, l, to, e }' "$TMP/mut-$pkg.log"
		grep -E 'LIVED|NOT COVERED' "$TMP/mut-$pkg.log" | sed 's/^ */    /'
	done
fi

echo
echo "VEREDICTO: PASS"
