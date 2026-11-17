import contextlib
import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import secret_scan


class SecretScanTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory(prefix="nexora-secret-test-")
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        subprocess.run(["git", "init", "-q", str(self.root)], check=True, capture_output=True)

    def write(self, path, value):
        target = self.root / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(value)

    def execute(self):
        output = io.StringIO()
        with contextlib.redirect_stdout(output), contextlib.redirect_stderr(output):
            code = secret_scan.main(["--root", str(self.root)])
        return code, output.getvalue()

    def test_detects_synthetic_leaks_without_printing_values(self):
        password = "synthetic-fixture-password"
        uri = "postgres://audit:" + password + "@localhost/nexora"
        samples = [
            "AKIA" + "A" * 16, "ghp_" + "a" * 36, "github_pat_" + "a" * 50,
            "xoxb-" + "a" * 30, "-----BEGIN " + "OPENSSH PRIVATE KEY-----",
            "-----BEGIN " + "PRIVATE KEY-----", uri,
            "postgresql://audit@localhost/db?pass%77ord=" + password,
            "postgres://audit:" + password + "%zz@[invalid/db",
            "POSTGRES_PASSWORD=" + password,
        ]
        for index, sample in enumerate(samples):
            with self.subTest(index=index):
                self.write("apps/source.txt", sample)
                code, output = self.execute()
                self.assertEqual(code, 1)
                self.assertIn("apps/source.txt:1", output)
                self.assertNotIn(sample, output)
                self.assertNotIn(password, output)

    def test_exact_allowlist_cannot_hide_changed_credentials_or_other_files(self):
        value = "postgres://fixture:" + "synthetic-example" + "@localhost/test"
        self.write("example.txt", value)
        self.write("infra/scripts/secret-scan-allowlist.json", json.dumps([{
            "path": "example.txt", "rule": "postgres-url-password",
            "sha256": secret_scan.digest(value), "reason": "single synthetic fixture",
        }]))
        self.assertEqual(self.execute()[0], 0)
        self.write("example.txt", value.replace("synthetic-example", "changed-password"))
        self.assertEqual(self.execute()[0], 1)
        self.write("example.txt", value)
        self.write("another.txt", value)
        self.assertEqual(self.execute()[0], 1)

    def test_safe_source_and_environment_references_pass(self):
        self.write("source.txt", "postgres://audit@localhost/db?sslmode=require\nPOSTGRES_PASSWORD: ${INJECTED_PASSWORD}\n")
        self.assertEqual(self.execute()[0], 0)

    def test_cli_returns_nonzero_and_never_prints_the_match(self):
        value = "ghp_" + "b" * 36
        self.write("leak.txt", value)
        result = subprocess.run(
            [sys.executable, "-B", secret_scan.__file__, "--root", str(self.root)],
            capture_output=True, text=True,
        )
        self.assertEqual(result.returncode, 1)
        self.assertIn("leak.txt:1", result.stderr)
        self.assertNotIn(value, result.stdout + result.stderr)

    def test_tracked_environment_file_is_rejected_without_reading_it(self):
        self.write(".env", "placeholder")
        subprocess.run(["git", "-C", str(self.root), "add", ".env"], check=True, capture_output=True)
        self.assertEqual(self.execute()[0], 1)

    def test_invalid_allowlist_and_missing_repository_fail_closed(self):
        self.write("infra/scripts/secret-scan-allowlist.json", "invalid json")
        self.assertEqual(self.execute()[0], 2)
        with contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(secret_scan.main(["--root", str(self.root / "missing")]), 2)


if __name__ == "__main__":
    unittest.main()
