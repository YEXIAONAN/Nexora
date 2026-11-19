#!/usr/bin/env bash
# M6 split backup/restore drill: PostgreSQL dump + private attachment directory.
#
# Safety boundary: only the labeled disposable container (loopback 5433) and
# database names prefixed nexora_m6_ are used. The development database, main
# stack and production volumes are never touched.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

CONTAINER=nexora-devtest-pg
LABEL=phase04-disposable
PG_PORT=5433
DB_USER=nexora
SRC_DB=nexora_m6_source
RESTORE_DB=nexora_m6_restore
SRC_PORT=18081
RESTORE_PORT=18082
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
WORK="$ROOT_DIR/.cache/m6-split/$STAMP"
SRC_ATTACH="$WORK/source-attachments"
RESTORE_ATTACH="$WORK/restore-attachments"
BACKUP_DIR="$WORK/backup"
BIN_DIR="$ROOT_DIR/.cache/m6-bin"
STATE="$WORK/state.json"
SUMMARY="$WORK/summary.json"
PW_FILE="$ROOT_DIR/.cache/devtest-pg-password"
ROOT_PASSWORD="$(openssl rand -hex 16)"
SRC_PID=""
RESTORE_PID=""
DB_TOUCHED=0

die() { echo "m6-split: $*" >&2; exit 1; }
psql_db() { docker exec "$CONTAINER" psql -U "$DB_USER" -d "$1" -v ON_ERROR_STOP=1 -tAc "$2"; }
drop_db() { psql_db postgres "DROP DATABASE IF EXISTS $1" >/dev/null; }
create_db() { psql_db postgres "CREATE DATABASE $1" >/dev/null; }
wait_ready() {
  local port="$1"
  for _ in $(seq 1 60); do
    curl -fsS "http://127.0.0.1:$port/readyz" >/dev/null 2>&1 && return 0
    sleep 0.5
  done
  return 1
}
file_size() {
  python3 - "$1" <<'PY'
import os, sys
print(os.path.getsize(sys.argv[1]))
PY
}
file_mode() {
  python3 - "$1" <<'PY'
import os, sys
print(format(os.stat(sys.argv[1]).st_mode & 0o777, "04o"))
PY
}
file_sha256() {
  python3 - "$1" <<'PY'
import hashlib, sys
digest = hashlib.sha256()
with open(sys.argv[1], "rb") as handle:
    for chunk in iter(lambda: handle.read(1 << 20), b""):
        digest.update(chunk)
print(digest.hexdigest())
PY
}
cleanup() {
  [ -n "$SRC_PID" ] && kill "$SRC_PID" 2>/dev/null || true
  [ -n "$RESTORE_PID" ] && kill "$RESTORE_PID" 2>/dev/null || true
  wait 2>/dev/null || true
  # Never touch PostgreSQL until the disposable container, loopback port and
  # credentials have all been validated (and this script created the databases).
  if [ "$DB_TOUCHED" = 1 ]; then
    drop_db "$SRC_DB" >/dev/null 2>&1 || true
    drop_db "$RESTORE_DB" >/dev/null 2>&1 || true
  fi
}

[ "$(docker inspect -f '{{index .Config.Labels "nexora.purpose"}}' "$CONTAINER" 2>/dev/null)" = "$LABEL" ] ||
  die "refusing a container without the disposable label"
[ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER" 2>/dev/null)" = "true" ] ||
  die "$CONTAINER is not running; run infra/scripts/test-db.sh start first"
docker port "$CONTAINER" 5432/tcp 2>/dev/null | grep -qx "127.0.0.1:$PG_PORT" ||
  die "$CONTAINER is not bound to 127.0.0.1:$PG_PORT"
[ -f "$PW_FILE" ] || die "missing $PW_FILE"
PGPASSWORD="$(cat "$PW_FILE")"
# The exit trap is registered only after every destructive-target check passed.
trap cleanup EXIT
mkdir -p "$WORK" "$SRC_ATTACH" "$RESTORE_ATTACH" "$BACKUP_DIR" "$BIN_DIR"
chmod 700 "$WORK" "$SRC_ATTACH" "$RESTORE_ATTACH" "$BACKUP_DIR"

echo "m6-split: building API binaries"
(cd apps/api && go build -o "$BIN_DIR/nexora-migrate" ./cmd/migrate && go build -o "$BIN_DIR/nexora-bootstrap" ./cmd/bootstrap && go build -o "$BIN_DIR/nexora-api" ./cmd/server)

echo "m6-split: recreating isolated databases"
DB_TOUCHED=1
drop_db "$SRC_DB"
drop_db "$RESTORE_DB"
create_db "$SRC_DB"
SRC_URL="postgres://${DB_USER}:${PGPASSWORD}@127.0.0.1:${PG_PORT}/${SRC_DB}?sslmode=disable"
RESTORE_URL="postgres://${DB_USER}:${PGPASSWORD}@127.0.0.1:${PG_PORT}/${RESTORE_DB}?sslmode=disable"
NEXORA_DATABASE_URL="$SRC_URL" "$BIN_DIR/nexora-migrate" -command up
printf '%s\n' "$ROOT_PASSWORD" | NEXORA_DATABASE_URL="$SRC_URL" "$BIN_DIR/nexora-bootstrap" -username root

echo "m6-split: starting source API on 127.0.0.1:$SRC_PORT"
NEXORA_ENV=test NEXORA_HTTP_ADDR="127.0.0.1:$SRC_PORT" NEXORA_LOG_LEVEL=warn \
  NEXORA_DATABASE_URL="$SRC_URL" NEXORA_ATTACHMENT_DIR="$SRC_ATTACH" NEXORA_MIGRATE_ON_START=false \
  "$BIN_DIR/nexora-api" >"$WORK/source-api.log" 2>&1 &
SRC_PID=$!
wait_ready "$SRC_PORT" || die "source API did not become ready; see $WORK/source-api.log"

echo "m6-split: creating real business evidence over HTTP"
python3 infra/scripts/m6_flow.py seed \
  --base-url "http://127.0.0.1:$SRC_PORT" \
  --password "$ROOT_PASSWORD" \
  --state-file "$STATE"

echo "m6-split: quiescing source writes"
kill "$SRC_PID"
wait "$SRC_PID" 2>/dev/null || true
SRC_PID=""

echo "m6-split: dumping database and copying matching attachment directory"
docker exec "$CONTAINER" pg_dump -U "$DB_USER" -d "$SRC_DB" --format=custom --no-owner --no-privileges >"$BACKUP_DIR/db.dump"
docker exec -i "$CONTAINER" pg_restore --list <"$BACKUP_DIR/db.dump" >/dev/null
cp -a "$SRC_ATTACH/." "$BACKUP_DIR/attachments/"
chmod -R go-rwx "$BACKUP_DIR"

python3 - "$BACKUP_DIR/attachments" >"$BACKUP_DIR/attachments.sha256" <<'PY'
import hashlib, pathlib, sys
root = pathlib.Path(sys.argv[1])
for path in sorted(p for p in root.rglob('*') if p.is_file()):
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    print(f"{digest}  {path.relative_to(root)}")
PY
DB_SHA="$(file_sha256 "$BACKUP_DIR/db.dump")"
FILE_COUNT="$(wc -l <"$BACKUP_DIR/attachments.sha256" | tr -d ' ')"
TOTAL_BYTES="$(python3 - "$BACKUP_DIR/attachments" <<'PY'
import os, sys
total = 0
for root, _dirs, files in os.walk(sys.argv[1]):
    for name in files:
        total += os.path.getsize(os.path.join(root, name))
print(total)
PY
)"

echo "m6-split: restoring into $RESTORE_DB and a second attachment directory"
RESTORE_START="$(python3 -c 'import time; print(time.time())')"
create_db "$RESTORE_DB"
docker exec -i "$CONTAINER" pg_restore -U "$DB_USER" -d "$RESTORE_DB" --no-owner --no-privileges --exit-on-error <"$BACKUP_DIR/db.dump"
cp -a "$BACKUP_DIR/attachments/." "$RESTORE_ATTACH/"
chmod 700 "$RESTORE_ATTACH"

COUNTS_TABLES="users teams team_members project_members sessions projects tasks task_submission_versions task_reviews submission_attachments contribution_cases contribution_case_versions contribution_confirmations contribution_notifications contribution_attestations audit_events"
counts() {
  local db="$1"
  for table in $COUNTS_TABLES; do
    printf '%s=%s\n' "$table" "$(psql_db "$db" "SELECT count(*) FROM $table")"
  done
}
counts "$SRC_DB" >"$WORK/source-counts.txt"
counts "$RESTORE_DB" >"$WORK/restore-counts.txt"
diff -u "$WORK/source-counts.txt" "$WORK/restore-counts.txt" >/dev/null || die "restored row counts differ; see $WORK/restore-counts.txt"
SCHEMA_SRC="$(psql_db "$SRC_DB" "SELECT COALESCE(MAX(version_id),0) FROM goose_db_version WHERE is_applied")"
SCHEMA_RESTORE="$(psql_db "$RESTORE_DB" "SELECT COALESCE(MAX(version_id),0) FROM goose_db_version WHERE is_applied")"
[ "$SCHEMA_SRC" = "$SCHEMA_RESTORE" ] || die "schema version differs: $SCHEMA_SRC vs $SCHEMA_RESTORE"

ATTACH_ID="$(jq -r .attachment_id "$STATE")"
ATTACH_KEY="$(psql_db "$RESTORE_DB" "SELECT storage_key FROM submission_attachments WHERE id='$ATTACH_ID' AND status='bound' AND removed_at IS NULL")"
ATTACH_SIZE="$(psql_db "$RESTORE_DB" "SELECT byte_size FROM submission_attachments WHERE id='$ATTACH_ID'")"
ATTACH_SHA="$(psql_db "$RESTORE_DB" "SELECT sha256 FROM submission_attachments WHERE id='$ATTACH_ID'")"
case "$ATTACH_KEY" in
  */*.blob) ;;
  *) die "restored storage_key is not a scoped blob key: $ATTACH_KEY" ;;
esac
[ -f "$RESTORE_ATTACH/$ATTACH_KEY" ] || die "restored attachment file is missing: $ATTACH_KEY"
[ "$(file_size "$RESTORE_ATTACH/$ATTACH_KEY")" = "$ATTACH_SIZE" ] || die "restored attachment size differs"
[ "$(file_sha256 "$RESTORE_ATTACH/$ATTACH_KEY")" = "$ATTACH_SHA" ] || die "restored attachment hash differs"
[ "$(file_mode "$RESTORE_ATTACH")" = "0700" ] || die "restored attachment directory permissions are not 0700"
[ "$(file_mode "$RESTORE_ATTACH/$ATTACH_KEY")" = "0600" ] || die "restored attachment file permissions are not 0600"

# Every bound row must have a byte-identical file; no silent metadata-only success.
psql_db "$RESTORE_DB" "SELECT storage_key||'|'||byte_size||'|'||sha256 FROM submission_attachments WHERE status='bound' AND removed_at IS NULL ORDER BY storage_key" >"$WORK/bound-files.txt"
python3 - "$RESTORE_ATTACH" "$WORK/bound-files.txt" <<'PY'
import hashlib, pathlib, sys
root = pathlib.Path(sys.argv[1])
rows = [line for line in pathlib.Path(sys.argv[2]).read_text().splitlines() if line]
for row in rows:
    key, size, digest = row.split('|')
    path = root / key
    if not path.is_file():
        raise SystemExit(f"missing bound file: {key}")
    data = path.read_bytes()
    if len(data) != int(size):
        raise SystemExit(f"size mismatch: {key}")
    if hashlib.sha256(data).hexdigest() != digest:
        raise SystemExit(f"hash mismatch: {key}")
print(f"verified {len(rows)} bound attachment files")
PY

echo "m6-split: starting restored API on 127.0.0.1:$RESTORE_PORT"
NEXORA_ENV=test NEXORA_HTTP_ADDR="127.0.0.1:$RESTORE_PORT" NEXORA_LOG_LEVEL=warn \
  NEXORA_DATABASE_URL="$RESTORE_URL" NEXORA_ATTACHMENT_DIR="$RESTORE_ATTACH" NEXORA_MIGRATE_ON_START=false \
  "$BIN_DIR/nexora-api" >"$WORK/restore-api.log" 2>&1 &
RESTORE_PID=$!
wait_ready "$RESTORE_PORT" || die "restored API did not become ready; see $WORK/restore-api.log"

echo "m6-split: verifying restored business evidence and cross-Team permissions"
python3 infra/scripts/m6_flow.py verify \
  --base-url "http://127.0.0.1:$RESTORE_PORT" \
  --password "$ROOT_PASSWORD" \
  --state-file "$STATE"

echo "m6-split: measuring restored API latency and checking representative query plans"
python3 infra/scripts/m6_flow.py load \
  --base-url "http://127.0.0.1:$RESTORE_PORT" \
  --password "$ROOT_PASSWORD" \
  --state-file "$STATE" \
  --requests 200 --concurrency 8 | tee "$WORK/load.json"
TEAM_ID="$(jq -r .team_id "$STATE")"
PROJECT_ID="$(jq -r .project_id "$STATE")"
TASK_ID="$(jq -r .task_id "$STATE")"
SUBMISSION_ID="$(jq -r .submission_id "$STATE")"
WORKER_ID="$(jq -r .worker_id "$STATE")"
psql_db "$RESTORE_DB" "EXPLAIN (ANALYZE, BUFFERS) SELECT id FROM contribution_notifications WHERE team_id='$TEAM_ID' AND project_id='$PROJECT_ID' AND recipient_id='$WORKER_ID' ORDER BY prepared_at,id" >"$WORK/plan-notices.txt"
psql_db "$RESTORE_DB" "EXPLAIN (ANALYZE, BUFFERS) SELECT id FROM submission_attachments WHERE team_id='$TEAM_ID' AND project_id='$PROJECT_ID' AND task_id='$TASK_ID' AND submission_version_id='$SUBMISSION_ID' AND status='bound' AND removed_at IS NULL" >"$WORK/plan-attachments.txt"
grep -E "Index Scan|Seq Scan|Execution Time" "$WORK/plan-notices.txt" "$WORK/plan-attachments.txt"

echo "m6-split: simulating missing and corrupt attachment"
FILE="$RESTORE_ATTACH/$ATTACH_KEY"
mv "$FILE" "$FILE.missing"
python3 infra/scripts/m6_flow.py check-download --base-url "http://127.0.0.1:$RESTORE_PORT" --password "$ROOT_PASSWORD" --state-file "$STATE" --expect-status 503
mv "$FILE.missing" "$FILE"
cp "$FILE" "$FILE.good"
python3 - "$FILE" "$ATTACH_SIZE" <<'PY'
import pathlib, sys
path = pathlib.Path(sys.argv[1])
path.write_bytes(b"X" * int(sys.argv[2]))
PY
python3 infra/scripts/m6_flow.py check-download --base-url "http://127.0.0.1:$RESTORE_PORT" --password "$ROOT_PASSWORD" --state-file "$STATE" --expect-status 503
mv "$FILE.good" "$FILE"
python3 infra/scripts/m6_flow.py check-download --base-url "http://127.0.0.1:$RESTORE_PORT" --password "$ROOT_PASSWORD" --state-file "$STATE" --expect-status 200
RESTORE_END="$(python3 -c 'import time; print(time.time())')"
RESTORE_SECONDS="$(python3 -c "print(round(float('$RESTORE_END')-float('$RESTORE_START'),2))")"

python3 - "$SUMMARY" "$STAMP" "$SCHEMA_RESTORE" "$DB_SHA" "$FILE_COUNT" "$TOTAL_BYTES" "$ATTACH_KEY" "$ATTACH_SHA" "$RESTORE_SECONDS" "$WORK/source-counts.txt" <<'PY'
import json, sys
out, stamp, schema, db_sha, count, total, key, digest, seconds, counts_file = sys.argv[1:]
rows = {}
for line in open(counts_file, encoding="utf-8"):
    name, value = line.strip().split("=", 1)
    rows[name] = int(value)
json.dump({
    "timestamp": stamp,
    "schema_version": int(schema),
    "database": {"dump_sha256": db_sha, "source_rows": rows},
    "attachments": {"count": int(count), "bytes": int(total), "selected_storage_key": key, "selected_sha256": digest},
    "restore_seconds": float(seconds),
    "checks": {"row_counts_match": True, "schema_match": True, "bound_files_match": True,
               "download": True, "cross_team_denied": True, "anonymous_denied": True,
               "missing_returns_503": True, "corrupt_returns_503": True},
}, open(out, "w", encoding="utf-8"), indent=2, sort_keys=True)
PY

echo "m6-split: PASS"
cat "$SUMMARY"
echo "m6-split: evidence retained at $WORK"
