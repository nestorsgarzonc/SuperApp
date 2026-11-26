#!/usr/bin/env bash
# Unit-test runner for the Control de Accesos back (Go) with per-case traceability.
#
#   scripts/test-unit.sh [--cover] [--branches] [--mutation]
#
#   (no option)  go test -race -count=1 -v ./...  ->  "PASS VA-01 <description>" per case
#   --cover      statement coverage (go test -coverprofile + go tool cover -func),
#                filtered by the files of each component
#   --branches   condition coverage with gobco (needs scripts/quality-tools.sh)
#   --mutation   mutation testing with gremlins on internal/access and internal/qrsystem
#                (runs on a COPY of the module in a temp dir; needs quality-tools.sh)
#
# Environment:
#   TOOLS      where gobco/gremlins live (default $HOME/.cache/ca-quality-tools)
#   GOPROXY    if the network blocks proxy.golang.org, export GOPROXY=direct GOSUMDB=off
#
# Go version: back/control-de-accesos/ms/go.mod pins a Go release. If the local Go is
# older and cannot download the toolchain, a temporary go.mod copy (go.sum copied
# next to it) is passed with -modfile. The repo files are NEVER modified.
#
# Case IDs: Test<PREFIX><NN>_<Desc> -> <PREFIX>-<NN>   (TestVA01_X -> VA-01,
#           TestMWE1_X -> MW-E1, TestMW08b_X -> MW-08b) and table subtests
#           t.Run("VA-13a antes del limite", ...) -> VA-13a.
# Prefix -> component: VA/GS/MV/AH = access, AL = accesslog, CB = qrsystem,
#                      NT = notifier, MW = middleware,
#                      CF/ME/CO/ER = soporte (config, membership, cors, errors).
#
# Exit status: non-zero if any test FAILs or go test itself fails (build error...).
# SKIP verdicts (suites out of scope for this version) never fail.
set -uo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
MOD=$ROOT/back/control-de-accesos/ms
TOOLS=${TOOLS:-$HOME/.cache/ca-quality-tools}

DO_COVER=0
DO_BRANCHES=0
DO_MUTATION=0
for arg in "$@"; do
	case $arg in
	--cover) DO_COVER=1 ;;
	--branches) DO_BRANCHES=1 ;;
	--mutation) DO_MUTATION=1 ;;
	--findings)
		echo "--findings is not implemented: findings are only documented, not tested." >&2
		exit 2
		;;
	-h | --help)
		sed -n '2,/^set -uo/p' "${BASH_SOURCE[0]}" | sed '$d' | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*)
		echo "unknown option: $arg (use --cover --branches --mutation)" >&2
		exit 2
		;;
	esac
done

[ -f "$MOD/go.mod" ] || {
	echo "module not found: $MOD" >&2
	exit 2
}
command -v go >/dev/null 2>&1 || {
	echo "go not found in PATH" >&2
	exit 2
}

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

# ---------------------------------------------------------------------------
# 1. Go version
# ---------------------------------------------------------------------------
vlt() { # a < b ?  (version compare)
	[ "$1" != "$2" ] && [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | head -n1)" = "$1" ]
}
REQUIRED=$(awk '/^go /{print $2; exit}' "$MOD/go.mod")
LOCAL=$(cd "$TMP" && go env GOVERSION | sed 's/^go//')
MODFLAG=""        # non-empty only when the temporary modfile is in use
EXTRA_GOFLAGS=""  # flags added to GOFLAGS for every go invocation of this run

if vlt "$LOCAL" "$REQUIRED"; then
	# Can the toolchain be downloaded? (GOTOOLCHAIN=auto inside the module)
	NATIVE=$(cd "$MOD" && timeout 60 go env GOVERSION 2>/dev/null | sed 's/^go//')
	if [ -n "$NATIVE" ] && ! vlt "$NATIVE" "$REQUIRED"; then
		echo "Go local $LOCAL < $REQUIRED: toolchain $NATIVE disponible, se usa tal cual."
	else
		echo "Go local $LOCAL < $REQUIRED: usando modfile temporal (no modifica el repo)"
		# go mod edit works on the copy only; the toolchain line is dropped.
		cp "$MOD/go.mod" "$TMP/ca.mod"
		cp "$MOD/go.sum" "$TMP/ca.sum"
		(cd "$TMP" && go mod edit -go="$LOCAL" -toolchain=none "$TMP/ca.mod") || {
			echo "no se pudo crear el modfile temporal" >&2
			exit 2
		}
		MODFLAG="-modfile=$TMP/ca.mod"
		# -mod=mod only ever rewrites the temporary copy (ca.mod/ca.sum).
		EXTRA_GOFLAGS="-mod=mod $MODFLAG"
		export GOTOOLCHAIN=local
	fi
fi
gotool() { # go with the modfile/flags of this run (inside the module directory)
	(cd "$MOD" && GOFLAGS="${GOFLAGS:-} $EXTRA_GOFLAGS" go "$@")
}

# ---------------------------------------------------------------------------
# 2. Case descriptions: "// Caso VA-01 ... condición: <text>" above each Test func
# ---------------------------------------------------------------------------
find "$MOD/internal" -name '*_test.go' -print0 2>/dev/null | xargs -0 awk '
	/^\/\/ Caso / {
		cond = ""
		i = index($0, "condición:")
		if (i > 0) { cond = substr($0, i + length("condición:")); sub(/^ +/, "", cond); sub(/[ .]+$/, "", cond) }
		pending = 1; next
	}
	/^func Test[A-Za-z0-9_]*\(/ {
		name = $2; sub(/\(.*/, "", name)
		if (pending && cond != "") print name "\t" cond
		pending = 0; next
	}
	/^\/\// { next }
	{ pending = 0 }
' >"$TMP/desc.tsv" 2>/dev/null || : >"$TMP/desc.tsv"

# ---------------------------------------------------------------------------
# 3. Control de Accesos suite
# ---------------------------------------------------------------------------
echo
echo "=== Control de Accesos (go test -race -count=1 -v ./...) ==="
COVERARGS=()
if [ "$DO_COVER" = 1 ]; then COVERARGS=(-coverprofile="$TMP/cover.out"); fi
gotool test -race -count=1 -v ${COVERARGS[@]+"${COVERARGS[@]}"} ./... 2>&1 | tee "$TMP/ca.log" >/dev/null
GOTEST_RC=${PIPESTATUS[0]}

awk -v descfile="$TMP/desc.tsv" '
BEGIN {
	while ((getline line < descfile) > 0) { split(line, kv, "\t"); desc[kv[1]] = kv[2] }
	comp["VA"] = "access"; comp["GS"] = "access"; comp["MV"] = "access"; comp["AH"] = "access"
	comp["AL"] = "accesslog"; comp["CB"] = "qrsystem"; comp["NT"] = "notifier"; comp["MW"] = "middleware"
	comp["CF"] = "soporte"; comp["ME"] = "soporte"; comp["CO"] = "soporte"; comp["ER"] = "soporte"
	order[1] = "access"; order[2] = "accesslog"; order[3] = "qrsystem"; order[4] = "notifier"; order[5] = "middleware"; order[6] = "soporte"
}
function humanize(s,   out, i, c, p) {
	out = ""
	for (i = 1; i <= length(s); i++) {
		c = substr(s, i, 1)
		if (i > 1 && c ~ /[A-Z]/ && p ~ /[a-z0-9]/) out = out " "
		out = out c; p = c
	}
	return out
}
function emit(status, id, text,   pre, comp_name) {
	pre = substr(id, 1, 2)
	comp_name = comp[pre]
	total[comp_name]++; count[comp_name, status]++
	g_total++; g[status]++
	printf "%s %s %s\n", status, id, text
}
/^[ \t]*--- (PASS|FAIL|SKIP): / {
	status = $2; sub(/:$/, "", status)
	name = $3
	sub(/#[0-9]+$/, "", name)
	n = split(name, seg, "/")
	if (n == 1) {
		# top-level test
		if (match(name, /^Test(VA|GS|MV|AH|AL|CB|NT|MW|CF|ME|CO|ER)[ES]?[0-9]+[a-z]?(_|$)/)) {
			rest = substr(name, 5)
			pre = substr(rest, 1, 2); rest = substr(rest, 3)
			e = ""
			if (substr(rest, 1, 1) ~ /[ES]/) { e = substr(rest, 1, 1); rest = substr(rest, 2) }
			match(rest, /^[0-9]+[a-z]?/)
			num = substr(rest, 1, RLENGTH)
			id = pre "-" e num
			text = (name in desc) ? desc[name] : humanize(substr(rest, RLENGTH + 2))
			emit(status, id, text)
		} else {
			noid++; noid_status[status]++
			printf "%s (sin-ID) %s\n", status, name
		}
	} else {
		# subtest: only those whose own name starts with an ID are cases
		sub_name = seg[n]
		tok = sub_name; sub(/_.*/, "", tok)
		if (tok ~ /^(VA|GS|MV|AH|AL|CB|NT|MW|CF|ME|CO|ER)-[ES]?[0-9]+[a-z]*(-[0-9]+)?$/) {
			text = substr(sub_name, length(tok) + 2); gsub(/_/, " ", text)
			emit(status, tok, text)
		}
	}
}
END {
	print ""
	printf "%-12s %6s %6s %6s %6s\n", "Componente", "Total", "PASS", "FAIL", "SKIP"
	for (i = 1; i <= 6; i++) {
		c = order[i]
		printf "%-12s %6d %6d %6d %6d\n", c, total[c], count[c, "PASS"], count[c, "FAIL"], count[c, "SKIP"]
	}
	printf "%-12s %6d %6d %6d %6d\n", "TOTAL", g_total, g["PASS"], g["FAIL"], g["SKIP"]
	printf "\nTests sin ID (trazabilidad pendiente, no fallan): %d (PASS %d, FAIL %d, SKIP %d)\n", noid, noid_status["PASS"], noid_status["FAIL"], noid_status["SKIP"]
	# machine-readable tail for the shell: failures with and without ID
	printf "%d\n", g["FAIL"] + noid_status["FAIL"] > "/dev/stderr"
}
' "$TMP/ca.log" 2>"$TMP/fails.txt"
FAILS=$(tail -n1 "$TMP/fails.txt" 2>/dev/null)
FAILS=${FAILS:-0}

# Packages that failed as a whole (build errors, panics, timeouts) show up as "FAIL <pkg>".
PKGFAIL=$(grep -cE '^FAIL[[:space:]]+[^[:space:]]+' "$TMP/ca.log" || true)
if [ "$GOTEST_RC" -ne 0 ]; then
	echo
	echo "go test terminó con código $GOTEST_RC; paquetes con FAIL:"
	grep -E '^FAIL[[:space:]]+[^[:space:]]+|build failed|setup failed' "$TMP/ca.log" | sed 's/^/  /'
	if grep -qE 'dial tcp|lookup .*proxy|no such host|Get "https' "$TMP/ca.log"; then
		echo "  sugerencia: sin acceso a proxy.golang.org, exporta GOPROXY=direct GOSUMDB=off y repite."
	fi
fi

# ---------------------------------------------------------------------------
# 4. Suites out of scope (placeholders, never fail)
# ---------------------------------------------------------------------------
# run_suite_placeholder <name> [command...]
#   Without a command: prints the NO EVALUADA block with a SKIP verdict (rc 0).
#   With a command: runs it and reports PASS/FAIL (rc 0/1) -- this is the hook to
#   connect a real suite later (e.g. run_suite_placeholder auth ./scripts/test-auth.sh).
run_suite_placeholder() {
	local name=$1
	shift
	echo
	if [ "$#" -eq 0 ]; then
		echo "SUITE $name: NO EVALUADA (fuera de alcance de esta versión)"
		echo "SKIP $name"
		return 0
	fi
	echo "SUITE $name: ejecutando $*"
	if "$@"; then
		echo "PASS $name"
		return 0
	fi
	echo "FAIL $name"
	return 1
}

EXTRA_FAIL=0
for suite in auth notificaciones api-gateway "capa de datos" front; do
	run_suite_placeholder "$suite" || EXTRA_FAIL=1
done

# ---------------------------------------------------------------------------
# 5. Optional reports
# ---------------------------------------------------------------------------
# Files of each component (statement coverage is reported over these only: the
# packages also hold SQL repositories that are out of scope for unit tests).
component_files() {
	case $1 in
	access) echo "internal/access/service.go internal/access/models.go internal/access/handler.go" ;;
	accesslog) echo "internal/accesslog/models.go internal/accesslog/resident.go internal/accesslog/handler.go internal/accesslog/resident_handler.go" ;;
	qrsystem) echo "internal/qrsystem/breaker.go internal/qrsystem/system.go" ;;
	notifier) echo "internal/notifier/notifier.go" ;;
	middleware) echo "internal/platform/middleware/auth.go" ;;
	esac
}

if [ "$DO_COVER" = 1 ]; then
	echo
	echo "=== Cobertura de sentencias por componente ==="
	if [ -s "$TMP/cover.out" ]; then
		for comp in access accesslog qrsystem notifier middleware; do
			files=$(component_files "$comp")
			echo
			echo "-- $comp --"
			pattern=$(echo "$files" | tr ' ' '\n' | sed 's|^|/|; s|$|:|' | paste -sd'|')
			awk -v pat="$pattern" '
				BEGIN { n = split(pat, p, "|") }
				NR > 1 {
					for (i = 1; i <= n; i++) if (index($1, p[i]) > 0) {
						tot += $2; if ($3 > 0) cov += $2; break
					}
				}
				END { if (tot > 0) printf "sentencias de los archivos del componente: %.1f%% (%d/%d)\n", 100 * cov / tot, cov, tot; else print "sin datos de cobertura" }
			' "$TMP/cover.out"
			gotool tool cover -func="$TMP/cover.out" 2>/dev/null |
				grep -E "$(echo "$files" | tr ' ' '\n' | sed 's|$|:|' | paste -sd'|')" || true
		done
	else
		echo "no se generó el perfil de cobertura (¿falló la compilación?)"
	fi
fi

# A scratch copy of the module (with the temporary go.mod/go.sum in place) shared
# by --branches and --mutation, so the repo is never touched by instrumentation.
make_copy() {
	[ -d "$TMP/copy" ] && return 0
	mkdir -p "$TMP/copy"
	cp -a "$MOD/." "$TMP/copy/"
	if [ -n "$MODFLAG" ]; then
		cp "$TMP/ca.mod" "$TMP/copy/go.mod"
		cp "$TMP/ca.sum" "$TMP/copy/go.sum"
	fi
}

if [ "$DO_BRANCHES" = 1 ]; then
	echo
	echo "=== Cobertura de condiciones (gobco) ==="
	if [ -x "$TOOLS/gobco" ]; then
		make_copy
		for pkg in access accesslog qrsystem notifier platform/middleware; do
			echo
			echo "-- internal/$pkg --"
			(cd "$TMP/copy" && GOFLAGS="${GOFLAGS:-} $EXTRA_GOFLAGS" "$TOOLS/gobco" "./internal/$pkg" 2>&1) | grep -vE '^(ok|---|===|PASS)' | sed 's/^/  /'
		done
	else
		echo "gobco no está disponible en $TOOLS: ejecuta scripts/quality-tools.sh (se omite --branches)."
	fi
fi

if [ "$DO_MUTATION" = 1 ]; then
	echo
	echo "=== Mutación (gremlins, sobre una copia del módulo) ==="
	if [ -x "$TOOLS/gremlins" ]; then
		make_copy
		for pkg in access qrsystem; do
			echo
			echo "-- internal/$pkg --"
			(cd "$TMP/copy" && GOFLAGS="${GOFLAGS:-} $EXTRA_GOFLAGS" "$TOOLS/gremlins" unleash --workers 4 "./internal/$pkg" 2>&1) | sed 's/^/  /'
		done
	else
		echo "gremlins no está disponible en $TOOLS: ejecuta scripts/quality-tools.sh (se omite --mutation)."
	fi
fi

# ---------------------------------------------------------------------------
# 6. Verdict
# ---------------------------------------------------------------------------
echo
if [ "$GOTEST_RC" -ne 0 ] || [ "$FAILS" -gt 0 ] || [ "$EXTRA_FAIL" -ne 0 ] || [ "$PKGFAIL" -gt 0 ]; then
	echo "VEREDICTO: FAIL (go test rc=$GOTEST_RC, tests en FAIL=$FAILS, paquetes en FAIL=$PKGFAIL)"
	exit 1
fi
echo "VEREDICTO: PASS"
exit 0
