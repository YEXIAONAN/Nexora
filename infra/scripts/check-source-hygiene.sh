#!/usr/bin/env bash
# Reject Finder-style duplicate Go sources (names like "schema 2.go") before the
# API build. Untracked copies are invisible to Git, but the Go toolchain still
# compiles every ".go" file whose name does not start with "." or "_", so such
# duplicates redeclare package symbols and fail `go build` with
# "redeclared in this block". Accepts an optional scan root for testing.
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCAN_ROOT="${1:-$ROOT_DIR/apps/api}"

if [[ ! -d "$SCAN_ROOT" ]]; then
  printf 'check-source-hygiene: scan root not found: %s\n' "$SCAN_ROOT" >&2
  exit 1
fi

duplicates="$(find "$SCAN_ROOT" -type f -name '* 2.go' -print | sort)"
if [[ -z "$duplicates" ]]; then
  printf 'check-source-hygiene: OK (no duplicate Go sources under %s)\n' "$SCAN_ROOT"
  exit 0
fi

printf 'check-source-hygiene: duplicate Go sources found; `go build` fails with "redeclared in this block":\n' >&2
while IFS= read -r duplicate; do
  printf '  %s\n' "$duplicate" >&2
done <<< "$duplicates"
printf 'These copies usually come from macOS Finder duplication. They are not\n' >&2
printf 'tracked by Git but still compiled. Compare each copy with the file of the\n' >&2
printf 'same name without the " 2" suffix, keep anything you still need elsewhere,\n' >&2
printf 'then delete the copies and retry.\n' >&2
exit 1
