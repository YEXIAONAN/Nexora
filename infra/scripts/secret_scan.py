#!/usr/bin/env python3
"""Small, dependency-free credential gate; findings contain locations, never values."""

import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys
from urllib.parse import parse_qsl, urlsplit


PATTERNS = {
    "aws-access-key": re.compile(r"\bAKIA[0-9A-Z]{16}\b"),
    "github-token": re.compile(r"\b(?:gh[pousr]_[A-Za-z0-9]{36,}|github_pat_[A-Za-z0-9_]{22,})\b"),
    "slack-token": re.compile(r"\bxox[baprs]-[A-Za-z0-9-]{10,}\b"),
    "private-key": re.compile(r"-----BEGIN (?:(?:RSA|OPENSSH|EC|PGP|DSA|ENCRYPTED) )?PRIVATE KEY-----"),
}
POSTGRES_URL = re.compile(r"\bpostgres(?:ql)?://[^\s\"'`<>]+", re.IGNORECASE)
PASSWORD_ASSIGNMENT = re.compile(r"^[ \t]*(?:NEXORA_)?POSTGRES_PASSWORD[ \t]*[:=][ \t]*(\S+)", re.MULTILINE)
# A pure shell expansion such as ${PGPASSWORD} carries no literal secret; the
# value is injected at runtime. Both Postgres rules ignore it consistently.
SHELL_EXPANSION = re.compile(r"[\"']?\$\{[A-Z_][A-Z0-9_]*\}[\"']?")
RULES = set(PATTERNS) | {"postgres-url-password", "postgres-password-assignment"}


def digest(value):
    return hashlib.sha256(value.encode()).hexdigest()


def is_real_secret(value):
    return bool(value) and not SHELL_EXPANSION.fullmatch(value)


def load_allowlist(path):
    entries = json.loads(path.read_text()) if path.exists() else []
    allowed = set()
    for entry in entries:
        relative = Path(entry["path"])
        if (relative.is_absolute() or ".." in relative.parts or entry["rule"] not in RULES
                or not re.fullmatch(r"[a-f0-9]{64}", entry["sha256"]) or not entry["reason"].strip()):
            raise ValueError("invalid exact-match allowlist entry")
        allowed.add((relative.as_posix(), entry["rule"], entry["sha256"]))
    return allowed


def candidates(content):
    for rule, pattern in PATTERNS.items():
        for match in pattern.finditer(content):
            yield rule, match.start(), match.group()
    for match in POSTGRES_URL.finditer(content):
        value = match.group()
        try:
            parsed = urlsplit(value)
            secrets = [parsed.password] if parsed.password else []
            secrets += [
                secret
                for key, secret in parse_qsl(parsed.query, keep_blank_values=True)
                if key.lower().replace("_", "") in {"password", "sslpassword"}
            ]
            has_password = any(is_real_secret(secret) for secret in secrets)
        except ValueError:
            userinfo = value.split("@", 1)[0].split("://", 1)[-1] if "@" in value else ""
            password = userinfo.split(":", 1)[1] if ":" in userinfo else ""
            has_password = is_real_secret(password)
        if has_password:
            yield "postgres-url-password", match.start(), value
    for match in PASSWORD_ASSIGNMENT.finditer(content):
        if is_real_secret(match.group(1)):
            yield "postgres-password-assignment", match.start(), match.group().strip()


def scan(root, allowed):
    result = subprocess.run(
        ["git", "-C", str(root), "ls-files", "--cached", "--others", "--exclude-standard", "-z"],
        capture_output=True, check=True,
    )
    findings = []
    for name in sorted(set(result.stdout.decode().split("\0")) - {""}):
        relative = Path(name)
        path = root / relative
        if relative.name.startswith(".env") and relative.name != ".env.example":
            findings.append((name, 1, "environment-file"))
            continue  # Do not read actual environment credentials.
        if path.is_symlink():
            if path.suffix.lower() in {".pem", ".key"}:
                findings.append((name, 1, "private-key-symlink"))
            continue  # Never follow a link outside the repository.
        if not path.is_file():
            continue
        content = path.read_bytes().decode("utf-8", errors="replace")
        for rule, offset, value in candidates(content):
            if (relative.as_posix(), rule, digest(value)) not in allowed:
                findings.append((name, content.count("\n", 0, offset) + 1, rule))
    return findings


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, required=True)
    args = parser.parse_args(argv)
    root = args.root.resolve()
    try:
        allowed = load_allowlist(root / "infra/scripts/secret-scan-allowlist.json")
        findings = scan(root, allowed)
    except (OSError, ValueError, KeyError, TypeError, subprocess.CalledProcessError):
        print("secret scan could not complete; check Git access and allowlist configuration", file=sys.stderr)
        return 2
    for path, line, rule in findings:
        print(f"ERROR: {path}:{line} [{rule}] credential value omitted", file=sys.stderr)
    if findings:
        print(f"secret scan failed: {len(findings)} finding(s)", file=sys.stderr)
        return 1
    print("secret scan passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
