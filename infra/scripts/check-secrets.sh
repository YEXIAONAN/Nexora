#!/usr/bin/env bash
# Lightweight secret scan for tracked files. Complements host-side tooling;
# it is intentionally dependency free so CI can always run it.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "not inside a git repository" >&2
  exit 1
fi

fail=0

if git ls-files --error-unmatch .env >/dev/null 2>&1; then
  echo "ERROR: .env is tracked by git" >&2
  fail=1
fi

patterns=(
  'AKIA[0-9A-Z]{16}'
  'gh[pousr]_[A-Za-z0-9]{36,}'
  'xox[baprs]-[A-Za-z0-9-]{10,}'
  '-----BEGIN (RSA|OPENSSH|EC|PGP|DSA) PRIVATE KEY-----'
  'postgres://[^:/[:space:]]+:[^@[:space:]]+@'
)

for pattern in "${patterns[@]}"; do
  # Development credentials in .env.example and docker-compose defaults are
  # intentional; everything else must be clean.
  if git grep -nIE "$pattern" -- \
      ':!*.example' ':!docker-compose.yml' ':!infra/docker/*' ':!*.lock' ':!apps/web/pnpm-lock.yaml' \
      >/tmp/nexora-secret-scan.out 2>/dev/null; then
    echo "ERROR: potential secret matched pattern: $pattern" >&2
    cat /tmp/nexora-secret-scan.out >&2
    fail=1
  fi
done

rm -f /tmp/nexora-secret-scan.out

if [[ "$fail" -ne 0 ]]; then
  echo "secret scan failed" >&2
  exit 1
fi

echo "secret scan passed"
