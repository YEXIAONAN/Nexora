#!/usr/bin/env bash
# Scan the working tree using the same rules locally and in CI. Values are never printed.
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
exec python3 -B "$ROOT_DIR/infra/scripts/secret_scan.py" --root "$ROOT_DIR" "$@"
