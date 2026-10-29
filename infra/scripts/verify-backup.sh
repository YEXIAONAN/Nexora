#!/usr/bin/env bash
# Restores the newest backup into a scratch database, checks the baseline
# schema, and drops the scratch database again.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

BACKUP_DIR="${NEXORA_BACKUP_DIR:-$ROOT_DIR/backups}"
LATEST="${1:-}"

if [[ -z "$LATEST" ]]; then
  LATEST="$(ls -1t "$BACKUP_DIR"/nexora_*.dump 2>/dev/null | head -n 1 || true)"
fi

if [[ -z "$LATEST" || ! -f "$LATEST" ]]; then
  echo "no backup archive found in $BACKUP_DIR; run infra/scripts/backup.sh first" >&2
  exit 1
fi

SERVICE="${NEXORA_POSTGRES_SERVICE:-postgres}"
DB_USER="${NEXORA_POSTGRES_USER:-nexora}"
SCRATCH_DB="nexora_restore_check_$(date -u +%Y%m%d%H%M%S)"

cleanup() {
  docker compose exec -T "$SERVICE" dropdb -U "$DB_USER" --if-exists "$SCRATCH_DB" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "verifying $LATEST"
"$ROOT_DIR/infra/scripts/restore.sh" "$LATEST" --target-db "$SCRATCH_DB" --yes

INSTALLATION_ID="$(docker compose exec -T "$SERVICE" psql -U "$DB_USER" -d "$SCRATCH_DB" -tAc \
  "SELECT value #>> '{}' FROM system_settings WHERE key = 'installation_id'")"

if [[ -z "$INSTALLATION_ID" ]]; then
  echo "restore verification failed: installation_id is missing" >&2
  exit 1
fi

SCHEMA_VERSION="$(docker compose exec -T "$SERVICE" psql -U "$DB_USER" -d "$SCRATCH_DB" -tAc \
  "SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version WHERE is_applied")"

echo "restore verified successfully"
echo "  archive:          $LATEST"
echo "  installation_id:  $INSTALLATION_ID"
echo "  schema_version:   $SCHEMA_VERSION"
