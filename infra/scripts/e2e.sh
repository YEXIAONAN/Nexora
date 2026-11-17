#!/usr/bin/env bash
# Disposable browser E2E stack for the Phase 02/03/04 real API frontend workflows.
#
# Safety boundary: this script only ever talks to the disposable devtest
# PostgreSQL container (nexora-devtest-pg, loopback 127.0.0.1:5433) and only
# creates/drops the dedicated nexora_e2e database. It never touches the
# running development stack (5432/8080/3000), other projects, or volumes.
# The ROOT password is randomly generated per run and never printed.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

CONTAINER=nexora-devtest-pg
PG_HOST=127.0.0.1
PG_PORT=5433
E2E_DB=nexora_e2e
API_ADDR=127.0.0.1:8081
WEB_PORT="${E2E_WEB_PORT:-5173}"
CACHE_DIR="$ROOT_DIR/.cache"
BIN_DIR="$CACHE_DIR/e2e-bin"
PW_FILE="$CACHE_DIR/devtest-pg-password"

die() {
  echo "e2e: $*" >&2
  exit 1
}

[ -f "$PW_FILE" ] || die "missing $PW_FILE; start the disposable devtest PostgreSQL first"
[ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER" 2>/dev/null)" = "true" ] ||
  die "container $CONTAINER is not running"
docker port "$CONTAINER" 5432/tcp 2>/dev/null | grep -q "127.0.0.1:$PG_PORT" ||
  die "$CONTAINER is not bound to 127.0.0.1:$PG_PORT"

PGPASSWORD="$(cat "$PW_FILE")"
# psql runs inside the disposable container (unix socket, no host dependency).
psql_exec() { docker exec "$CONTAINER" psql -U nexora -v ON_ERROR_STOP=1 "$@"; }

mkdir -p "$BIN_DIR"
echo "e2e: building API binaries"
(
  cd apps/api
  go build -o "$BIN_DIR/nexora-migrate" ./cmd/migrate
  go build -o "$BIN_DIR/nexora-bootstrap" ./cmd/bootstrap
  go build -o "$BIN_DIR/nexora-api" ./cmd/server
)

echo "e2e: recreating disposable database $E2E_DB on $PG_HOST:$PG_PORT"
psql_exec -d postgres -c "DROP DATABASE IF EXISTS $E2E_DB;" >/dev/null
psql_exec -d postgres -c "CREATE DATABASE $E2E_DB;" >/dev/null

DB_URL="postgres://nexora:${PGPASSWORD}@${PG_HOST}:${PG_PORT}/${E2E_DB}?sslmode=disable"
ROOT_PASSWORD="$(openssl rand -hex 16)"
export E2E_ROOT_PASSWORD="$ROOT_PASSWORD"
export NEXORA_DATABASE_URL="$DB_URL"

ATTACHMENT_TMP="$(mktemp -d "$CACHE_DIR/e2e-attachments.XXXXXX")"
export NEXORA_ATTACHMENT_DIR="$ATTACHMENT_TMP"
export NEXORA_MIGRATE_ON_START=false
API_PID=""
WEB_PID=""
cleanup() {
  if [ -n "$API_PID" ]; then kill "$API_PID" 2>/dev/null || true; fi
  if [ -n "$WEB_PID" ]; then kill "$WEB_PID" 2>/dev/null || true; fi
  wait 2>/dev/null || true
  python3 - "$ATTACHMENT_TMP" <<'PY_CLEANUP'
import pathlib, shutil, sys
p=pathlib.Path(sys.argv[1])
if p.parent.name == '.cache' and p.name.startswith('e2e-attachments.'):
    shutil.rmtree(p)
PY_CLEANUP
  psql_exec -d postgres -c "DROP DATABASE IF EXISTS $E2E_DB;" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "e2e: applying migrations"
"$BIN_DIR/nexora-migrate" -command up

echo "e2e: bootstrapping ROOT (random password, never printed)"
printf '%s\n' "$ROOT_PASSWORD" | "$BIN_DIR/nexora-bootstrap" -username root

echo "e2e: starting API on $API_ADDR"
NEXORA_ENV=test NEXORA_HTTP_ADDR="$API_ADDR" NEXORA_LOG_LEVEL=warn \
  "$BIN_DIR/nexora-api" >"$CACHE_DIR/e2e-api.log" 2>&1 &
API_PID=$!

for _ in $(seq 1 60); do
  curl -fsS "http://$API_ADDR/readyz" >/dev/null 2>&1 && break
  sleep 0.5
done
curl -fsS "http://$API_ADDR/readyz" >/dev/null 2>&1 ||
  die "API did not become ready; see .cache/e2e-api.log"

echo "e2e: starting vite on 127.0.0.1:$WEB_PORT"
pushd apps/web >/dev/null
VITE_DEV_API_TARGET="http://$API_ADDR" node node_modules/vite/bin/vite.js \
  --host 127.0.0.1 --port "$WEB_PORT" --strictPort >"$CACHE_DIR/e2e-web.log" 2>&1 &
WEB_PID=$!
popd >/dev/null

for _ in $(seq 1 60); do
  curl -fsS "http://127.0.0.1:$WEB_PORT/" >/dev/null 2>&1 && break
  sleep 0.5
done
curl -fsS "http://127.0.0.1:$WEB_PORT/" >/dev/null 2>&1 ||
  die "vite did not become ready; see .cache/e2e-web.log"

echo "e2e: running Playwright against the real stack"
(
  cd apps/web
  E2E_BASE_URL="http://127.0.0.1:$WEB_PORT" pnpm exec playwright test
)
echo "e2e: passed"