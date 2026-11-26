#!/usr/bin/env bash
# Builds the quality tools used by scripts/test-unit.sh (--branches, --mutation):
#   gremlins v0.6.0  mutation testing
#   gobco            condition (branch) coverage
#
# Everything is built OUTSIDE the repo, in ${TOOLS:-$HOME/.cache/ca-quality-tools};
# no binary is ever versioned and no repo file is touched.
#
#   scripts/quality-tools.sh
#
# Environment:
#   TOOLS      destination directory (default $HOME/.cache/ca-quality-tools)
#   PREBUILT   directory with already built binaries to reuse (default /tmp/muttools);
#              set FORCE=1 to ignore it and build from source.
#   GOPROXY    when proxy.golang.org / golang.org are blocked, export
#              GOPROXY=direct GOSUMDB=off to fetch straight from GitHub.
#
# gremlins needs golang.org/x/* modules; networks that block golang.org can still
# reach their GitHub mirrors, hence the replace block below (plan section 7.2).
set -uo pipefail

TOOLS=${TOOLS:-$HOME/.cache/ca-quality-tools}
PREBUILT=${PREBUILT:-/tmp/muttools}
GREMLINS_VERSION=v0.6.0
mkdir -p "$TOOLS"

command -v go >/dev/null 2>&1 || {
	echo "quality-tools: go not found in PATH" >&2
	exit 1
}
# gremlins' own go.mod asks for a recent Go; the build module below declares the
# local version, and GOTOOLCHAIN=local avoids any download attempt.
GOLOCAL=$(go env GOVERSION | sed 's/^go//' | cut -d. -f1,2)
export GOTOOLCHAIN=${GOTOOLCHAIN:-local}

BUILD=$(mktemp -d)
trap 'rm -rf "$BUILD"' EXIT

reuse() { # name
	if [ -z "${FORCE:-}" ] && [ ! -x "$TOOLS/$1" ] && [ -x "$PREBUILT/$1" ]; then
		cp "$PREBUILT/$1" "$TOOLS/$1" && echo "quality-tools: $1 reused from $PREBUILT"
	fi
}

build_gremlins() {
	cat >"$BUILD/go.mod" <<MOD
module tmpbuild

go $GOLOCAL

require github.com/go-gremlins/gremlins $GREMLINS_VERSION

replace (
	golang.org/x/tools => github.com/golang/tools v0.39.0
	golang.org/x/sys => github.com/golang/sys v0.38.0
	golang.org/x/text => github.com/golang/text v0.31.0
	golang.org/x/mod => github.com/golang/mod v0.30.0
	golang.org/x/sync => github.com/golang/sync v0.18.0
	golang.org/x/net => github.com/golang/net v0.47.0
	go.yaml.in/yaml/v3 => github.com/yaml/go-yaml/v3 v3.0.4
)
MOD
	printf '//go:build tools\n\npackage tools\n\nimport _ "github.com/go-gremlins/gremlins/cmd/gremlins"\n' >"$BUILD/tools.go"
	(cd "$BUILD" && GOFLAGS=-mod=mod go build -o "$TOOLS/gremlins" github.com/go-gremlins/gremlins/cmd/gremlins)
}

build_gobco() {
	# Outside any module so the repo's go.mod is not consulted.
	(cd "$BUILD" && GOFLAGS= GOBIN="$TOOLS" go install github.com/rillig/gobco@latest)
}

status=0
reuse gremlins
reuse gobco

if [ -x "$TOOLS/gremlins" ]; then
	echo "quality-tools: gremlins already present"
else
	echo "quality-tools: building gremlins $GREMLINS_VERSION ..."
	build_gremlins || {
		echo "quality-tools: gremlins build FAILED (the manual mutation fallback in the plan still applies)" >&2
		status=1
	}
fi

if [ -x "$TOOLS/gobco" ]; then
	echo "quality-tools: gobco already present"
else
	echo "quality-tools: building gobco ..."
	build_gobco || {
		echo "quality-tools: gobco build FAILED (--branches will only warn)" >&2
		status=1
	}
fi

for t in gremlins gobco; do
	if [ -x "$TOOLS/$t" ]; then echo "quality-tools: OK  $TOOLS/$t"; else echo "quality-tools: MISSING $t"; fi
done
exit $status
