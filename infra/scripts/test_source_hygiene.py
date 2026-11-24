import subprocess
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("check-source-hygiene.sh")


class SourceHygieneTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory(prefix="nexora-hygiene-test-")
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name) / "apps" / "api"
        self.root.mkdir(parents=True)

    def write(self, name):
        target = self.root / name
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text("package fixture\n")

    def run_check(self):
        result = subprocess.run(
            ["bash", str(SCRIPT), str(self.root)],
            capture_output=True,
            text=True,
        )
        return result.returncode, result.stdout + result.stderr

    def test_passes_on_clean_tree_and_reports_the_scan_root(self):
        self.write("internal/schema.go")
        code, output = self.run_check()
        self.assertEqual(code, 0)
        self.assertIn("OK", output)
        self.assertIn(str(self.root), output)

    def test_rejects_finder_style_duplicate_go_sources_with_paths_and_guidance(self):
        self.write("internal/database/schema.go")
        self.write("internal/database/schema 2.go")
        self.write("internal/tasks/reports.go")
        self.write("internal/tasks/reports 2.go")
        code, output = self.run_check()
        self.assertEqual(code, 1)
        self.assertIn("schema 2.go", output)
        self.assertIn("reports 2.go", output)
        self.assertIn("redeclared", output)
        self.assertIn("delete the copies", output)

    def test_ignores_non_go_duplicates_and_ordinary_numbered_names(self):
        self.write("internal/notes 2.md")
        self.write("internal/task2.go")
        code, _ = self.run_check()
        self.assertEqual(code, 0)

    def test_missing_scan_root_fails(self):
        missing = Path(self.directory.name) / "nowhere"
        result = subprocess.run(
            ["bash", str(SCRIPT), str(missing)],
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 1)
        self.assertIn("not found", result.stderr)


if __name__ == "__main__":
    unittest.main()
