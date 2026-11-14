#!/bin/sh
# Entry point of Dockerfile.init. Requires AUTH_DATABASE_URL and KEYS_DIR.
set -eu
: "${AUTH_DATABASE_URL:?AUTH_DATABASE_URL is required}"
: "${KEYS_DIR:?KEYS_DIR is required}"

if [ ! -s "$KEYS_DIR/jwt-private.pem" ]; then
  keygen -out "$KEYS_DIR"
fi
# The auth container runs as its own non-root user with a different uid; this
# volume is only mounted (read-only) into auth, so the key is made readable.
# In production the key is a mounted secret owned by the service user instead.
chmod 0444 "$KEYS_DIR/jwt-private.pem"

setup

if [ "${WITH_DEV_SEEDS:-0}" = "1" ]; then
  for f in /db/dev-seeds/*.sql; do
    echo "applying dev seed $(basename "$f")"
    psql "$AUTH_DATABASE_URL" -v ON_ERROR_STOP=1 -q -f "$f"
  done
fi
echo "auth init done"
