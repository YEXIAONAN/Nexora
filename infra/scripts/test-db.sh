#!/usr/bin/env bash
# Owns only the Nexora disposable test container, port 5433 and cached credential.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
CONTAINER=nexora-devtest-pg
LABEL=phase04-disposable
PW_FILE=.cache/devtest-pg-password
owned() {
  [ "$(docker inspect -f '{{index .Config.Labels "nexora.purpose"}}' "$CONTAINER" 2>/dev/null)" = "$LABEL" ] || {
    echo 'test-db: refusing a container without the expected disposable label' >&2; exit 1;
  }
}
case "${1:-status}" in
 start)
  if docker inspect "$CONTAINER" >/dev/null 2>&1; then
    owned
    [ -f "$PW_FILE" ] || { echo 'test-db: missing private credential; restore it or remove only this owned test container' >&2; exit 1; }
    docker start "$CONTAINER" >/dev/null
  else
    mkdir -p .cache
    umask 077
    openssl rand -hex 24 > "$PW_FILE"
    TEST_DB_PASSWORD="$(cat "$PW_FILE")"
    POSTGRES_PASSWORD="${TEST_DB_PASSWORD}" docker run -d --name "$CONTAINER" \
      --label "nexora.purpose=$LABEL" \
      --tmpfs /var/lib/postgresql/data:rw,noexec,nosuid,size=1024m \
      -e POSTGRES_USER=nexora -e POSTGRES_PASSWORD -e POSTGRES_DB=nexora_test \
      -p 127.0.0.1:5433:5432 postgres:17-alpine >/dev/null
  fi
  docker port "$CONTAINER" 5432/tcp | grep -qx '127.0.0.1:5433' || { echo 'test-db: unexpected port binding' >&2; exit 1; }
  for _ in $(seq 1 30); do
    if docker exec "$CONTAINER" pg_isready -U nexora -d nexora_test >/dev/null 2>&1; then
      echo 'test-db: disposable nexora_test ready on 127.0.0.1:5433'; exit 0
    fi
    sleep 0.5
  done
  echo 'test-db: PostgreSQL readiness failed' >&2; exit 1
  ;;
 stop)
  owned
  docker stop "$CONTAINER" >/dev/null
  docker rm --volumes "$CONTAINER" >/dev/null
  echo 'test-db: removed only the owned disposable container'
  ;;
 status)
  owned
  docker ps -a --filter "name=^/${CONTAINER}$" --format '{{.Names}} {{.Status}} {{.Ports}}'
  ;;
 *) echo 'usage: bash infra/scripts/test-db.sh start|status|stop' >&2; exit 2 ;;
esac
