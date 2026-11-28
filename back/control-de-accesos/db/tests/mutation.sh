#!/usr/bin/env bash
# Manual mutation testing of the control-de-accesos schema (Delivery 1, C1).
# For each mutant in mutants.txt: copy db/migrations, apply that one change,
# run the data-tier suite (run.sh) on a fresh database and report whether some
# case FAILs (killed) or none does (survived). Production files are never touched.
#
#   back/control-de-accesos/db/tests/mutation.sh            # all mutants
#   back/control-de-accesos/db/tests/mutation.sh M4 M13      # some of them
#
# Uses Docker like run.sh, or an existing server with PG_ADMIN_URL (see run.sh).
set -uo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
DB_DIR=$(dirname "$HERE")
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

killed=0; survived=0; invalid=0
printf '%-5s %-9s %s\n' MUTANT VERDICT "KILLED BY"
while IFS='|' read -r id file original mutated; do
	id=$(echo "$id" | xargs)
	case "$id" in ''|\#*) continue ;; esac
	if [ "$#" -gt 0 ] && ! printf '%s\n' "$@" | grep -qx "$id"; then continue; fi
	rm -rf "$WORK/m"; cp -r "$DB_DIR/migrations" "$WORK/m"
	if ! python3 - "$WORK/m/$(echo "$file" | xargs)" "$original" "$mutated" <<'PY'
import sys
path, old, new = sys.argv[1], sys.argv[2].strip().replace('\\n', '\n'), sys.argv[3].strip().replace('\\n', '\n')
new = '' if new == '<empty>' else new
src = open(path).read()
if src.count(old) != 1:
    sys.exit(f'original text found {src.count(old)} times in {path}')
open(path, 'w').write(src.replace(old, new))
PY
	then printf '%-5s %-9s %s\n' "$id" INVALID "mutation could not be applied"; invalid=$((invalid+1)); continue; fi
	out=$(MIGRATIONS_DIR="$WORK/m" "$HERE/run.sh" 2>&1)
	if echo "$out" | grep -q 'ERROR' && ! echo "$out" | grep -qE '^\s*(PASS|FAIL|SKIP)\b'; then
		printf '%-5s %-9s %s\n' "$id" KILLED "migration no longer applies"; killed=$((killed+1)); continue
	fi
	fails=$(echo "$out" | awk -F'|' '$1 ~ /^ *FAIL/ {gsub(/^ +| +$/, "", $2); print $2}' | cut -d' ' -f1 | sort -u | paste -sd, -)
	if [ -n "$fails" ]; then
		printf '%-5s %-9s %s\n' "$id" KILLED "$fails"; killed=$((killed+1))
	else
		printf '%-5s %-9s %s\n' "$id" SURVIVED "-"; survived=$((survived+1))
	fi
done < "$HERE/mutants.txt"

total=$((killed+survived))
echo
echo "killed $killed / $total, survived $survived, invalid $invalid"
[ "$total" -gt 0 ] && echo "mutation score: $(( killed * 100 / total ))%"
[ "$survived" -eq 0 ] && [ "$invalid" -eq 0 ]
