#!/usr/bin/env bash
# Restores a custom-format backup into a NEW database by default.
# Restoring over the primary database requires explicit opt-in.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

usage() {
  cat <<'EOF'
Usage: infra/scripts/restore.sh <backup.dump> [options]

Options:
  --target-db <name>   Target database (default: nexora_restore_<UTC timestamp>)
  --drop-existing      Drop the target database if it already exists
  --yes                Do not prompt before destructive operations
  --help               Show this help

Restoring into the primary database requires NEXORA_ALLOW_PRIMARY_RESTORE=1
in addition to --target-db and --yes.
EOF
}

[[ $# -ge 1 ]] || { usage; exit 2; }
BACKUP_FILE="${1:-}"
shift || true

TARGET_DB=""
DROP_EXISTING=0
ASSUME_YES=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target-db) TARGET_DB="${2:-}"; shift 2 ;;
    --drop-existing) DROP_EXISTING=1; shift ;;
    --yes) ASSUME_YES=1; shift ;;
    --help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage; exit 2 ;;
  esac
done

[[ -f "$BACKUP_FILE" && -s "$BACKUP_FILE" ]] || { echo "backup file not found or empty: $BACKUP_FILE" >&2; exit 1; }
command -v docker >/dev/null 2>&1 || { echo "docker is required" >&2; exit 1; }

SERVICE="${NEXORA_POSTGRES_SERVICE:-postgres}"
DB_USER="${NEXORA_POSTGRES_USER:-nexora}"
PRIMARY_DB="${NEXORA_POSTGRES_DB:-nexora}"
TARGET_DB="${TARGET_DB:-nexora_restore_$(date -u +%Y%m%d%H%M%S)}"

[[ "$TARGET_DB" =~ ^[A-Za-z0-9_]+$ ]] || { echo "invalid target database name: $TARGET_DB" >&2; exit 1; }

if [[ "$TARGET_DB" == "$PRIMARY_DB" && "${NEXORA_ALLOW_PRIMARY_RESTORE:-}" != "1" ]]; then
  echo "refusing to restore over the primary database '$PRIMARY_DB'." >&2
  echo "set NEXORA_ALLOW_PRIMARY_RESTORE=1 only after taking a verified backup." >&2
  exit 1
fi

docker compose ps --status running --services 2>/dev/null | grep -qx "$SERVICE" || {
  echo "postgres service '$SERVICE' is not running; start it with: docker compose up -d postgres" >&2
  exit 1
}

docker compose exec -T "$SERVICE" pg_restore --list <"$BACKUP_FILE" >/dev/null || {
  echo "backup archive is not readable by pg_restore: $BACKUP_FILE" >&2
  exit 1
}

if [[ "$ASSUME_YES" -ne 1 ]]; then
  read -r -p "Restore '$BACKUP_FILE' into database '$TARGET_DB'? [y/N] " answer
  [[ "$answer" =~ ^[Yy]$ ]] || { echo "aborted"; exit 1; }
fi

exists="$(docker compose exec -T "$SERVICE" psql -U "$DB_USER" -d postgres -tAc \
  "SELECT 1 FROM pg_database WHERE datname = '$TARGET_DB'")"

if [[ -n "$exists" ]]; then
  if [[ "$DROP_EXISTING" -ne 1 ]]; then
    echo "target database '$TARGET_DB' already exists; pass --drop-existing to replace it" >&2
    exit 1
  fi
  echo "dropping existing database '$TARGET_DB'"
  docker compose exec -T "$SERVICE" dropdb -U "$DB_USER" --if-exists "$TARGET_DB"
fi

echo "creating database '$TARGET_DB'"
docker compose exec -T "$SERVICE" createdb -U "$DB_USER" "$TARGET_DB"

echo "restoring archive"
docker compose exec -T "$SERVICE" pg_restore -U "$DB_USER" -d "$TARGET_DB" \
  --no-owner --no-privileges <"$BACKUP_FILE"

SETTINGS_COUNT="$(docker compose exec -T "$SERVICE" psql -U "$DB_USER" -d "$TARGET_DB" -tAc \
  "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'system_settings'")"

if [[ "$SETTINGS_COUNT" != "1" ]]; then
  echo "restore verification failed: system_settings table is missing in '$TARGET_DB'" >&2
  exit 1
fi

echo "restore verified: database '$TARGET_DB' contains the baseline schema"
echo "to remove the scratch database: docker compose exec $SERVICE dropdb -U $DB_USER $TARGET_DB"
