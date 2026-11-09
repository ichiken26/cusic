"""The installer must distinguish an available Python from missing modules."""
import pathlib
import subprocess
import sys
import unittest

CHECKER = pathlib.Path(__file__).resolve().parents[2] / "scripts" / "check-python.py"


class PythonDependencyCheckerTests(unittest.TestCase):
    def test_runtime_only_succeeds_without_site_packages(self):
        result = subprocess.run([sys.executable, "-S", str(CHECKER), "--runtime-only"],
                                capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_missing_modules_require_install(self):
        result = subprocess.run([sys.executable, "-S", str(CHECKER)],
                                capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)

    def test_dependencies_present_are_reused(self):
        result = subprocess.run([sys.executable, str(CHECKER)],
                                capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)


if __name__ == "__main__":
    unittest.main()
