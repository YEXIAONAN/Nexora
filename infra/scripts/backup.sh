#!/usr/bin/env bash
# Creates a verified PostgreSQL custom-format backup from the compose database.
# Success is only reported after pg_restore can read the archive.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

BACKUP_DIR="${NEXORA_BACKUP_DIR:-$ROOT_DIR/backups}"
RETENTION_DAYS="${NEXORA_BACKUP_RETENTION_DAYS:-14}"
DB_NAME="${NEXORA_POSTGRES_DB:-nexora}"
DB_USER="${NEXORA_POSTGRES_USER:-nexora}"
SERVICE="${NEXORA_POSTGRES_SERVICE:-postgres}"

if ! [[ "$RETENTION_DAYS" =~ ^[0-9]+$ ]]; then
  echo "NEXORA_BACKUP_RETENTION_DAYS must be a non-negative integer" >&2
  exit 2
fi

mkdir -p "$BACKUP_DIR"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
BACKUP_FILE="$BACKUP_DIR/nexora_${DB_NAME}_${STAMP}.dump"

if ! docker compose ps --status running --services 2>/dev/null | grep -qx "$SERVICE"; then
  echo "postgres service '$SERVICE' is not running; start it with: docker compose up -d postgres" >&2
  exit 1
fi

echo "dumping database '$DB_NAME' -> $BACKUP_FILE"
docker compose exec -T "$SERVICE" pg_dump -U "$DB_USER" -d "$DB_NAME" \
  --format=custom --no-owner --no-privileges >"$BACKUP_FILE"

if [[ ! -s "$BACKUP_FILE" ]]; then
  echo "backup file is empty" >&2
  rm -f "$BACKUP_FILE"
  exit 1
fi

# A dump that cannot be listed cannot be restored; verify before reporting success.
if ! docker compose exec -T "$SERVICE" pg_restore --list <"$BACKUP_FILE" >/dev/null; then
  echo "backup verification failed: pg_restore cannot read $BACKUP_FILE" >&2
  rm -f "$BACKUP_FILE"
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "$BACKUP_FILE" >"$BACKUP_FILE.sha256"
else
  shasum -a 256 "$BACKUP_FILE" >"$BACKUP_FILE.sha256"
fi

if [[ "$RETENTION_DAYS" -gt 0 ]]; then
  find "$BACKUP_DIR" -maxdepth 1 -type f -name "nexora_*.dump" -mtime +"$RETENTION_DAYS" -print -delete
  find "$BACKUP_DIR" -maxdepth 1 -type f -name "nexora_*.dump.sha256" -mtime +"$RETENTION_DAYS" -print -delete
fi

echo "backup verified: $BACKUP_FILE"
echo "checksum: $(cat "$BACKUP_FILE.sha256")"
