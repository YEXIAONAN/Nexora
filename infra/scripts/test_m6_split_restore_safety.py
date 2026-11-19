"""Non-destructive regression tests for the M6 split restore drill.

The drill is destructive by design, so every safety check must run before any
database is dropped. These tests shadow `docker` with a recorder, run the real
script inside a throwaway tree and prove that no `DROP DATABASE` (and no
`docker exec` at all) is issued when the environment is invalid.

A real development database is never reachable from these tests: the script is
copied into a temporary directory, so its ROOT_DIR (and cached credential path)
is also temporary, and the fake docker is the first entry on PATH.
"""

import os
import pathlib
import shutil
import subprocess
import tempfile
import unittest

SCRIPT = pathlib.Path(__file__).resolve().parent / "m6-split-restore.sh"

FAKE_DOCKER = """#!/usr/bin/env bash
printf '%s\\n' "$*" >> {log}
case "$1" in
  inspect)
    case "$*" in
      *Config.Labels*) printf '%s\\n' "${{FAKE_LABEL:-phase04-disposable}}" ;;
      *State.Running*) printf '%s\\n' "${{FAKE_RUNNING:-true}}" ;;
      *) printf '\\n' ;;
    esac
    ;;
  port) printf '%s\\n' "${{FAKE_PORT:-127.0.0.1:5433}}" ;;
  *) : ;;
esac
"""


class SplitRestoreSafetyTest(unittest.TestCase):
    def run_drill(self, **overrides):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        root = pathlib.Path(temporary.name)
        scripts = root / "infra" / "scripts"
        scripts.mkdir(parents=True)
        copied = scripts / SCRIPT.name
        shutil.copy2(SCRIPT, copied)
        copied.chmod(0o755)
        fake_bin = root / "fake-bin"
        fake_bin.mkdir()
        log = root / "docker.log"
        fake = fake_bin / "docker"
        fake.write_text(FAKE_DOCKER.format(log=log), encoding="utf-8")
        fake.chmod(0o755)
        environment = dict(os.environ)
        environment.update(overrides)
        environment["PATH"] = str(fake_bin) + os.pathsep + environment.get("PATH", "")
        # The temporary ROOT_DIR has no cached credential file, so a test that
        # reaches the credential check always fails there without side effects.
        completed = subprocess.run(
            ["bash", str(copied)],
            cwd=root,
            env=environment,
            capture_output=True,
            text=True,
            timeout=60,
        )
        return completed, log.read_text(encoding="utf-8")

    def assert_refused_without_database_access(self, **overrides):
        completed, docker_log = self.run_drill(**overrides)
        output = completed.stdout + completed.stderr
        self.assertNotEqual(completed.returncode, 0, output)
        self.assertNotIn("building API binaries", output)
        self.assertNotIn("DROP DATABASE", docker_log)
        self.assertNotIn("exec", docker_log)
        return output

    def test_wrong_container_label_is_refused_before_any_database_drop(self):
        output = self.assert_refused_without_database_access(FAKE_LABEL="not-a-disposable-label")
        self.assertIn("disposable label", output)

    def test_wrong_port_is_refused_before_any_database_drop(self):
        output = self.assert_refused_without_database_access(FAKE_PORT="127.0.0.1:5432")
        self.assertIn("127.0.0.1:5433", output)

    def test_stopped_container_is_refused_before_any_database_drop(self):
        output = self.assert_refused_without_database_access(FAKE_RUNNING="false")
        self.assertIn("not running", output)

    def test_missing_credentials_are_refused_before_any_database_drop(self):
        output = self.assert_refused_without_database_access()
        self.assertIn("missing", output)


if __name__ == "__main__":
    unittest.main()
