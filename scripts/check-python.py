"""Validate Python for cusic without installing or modifying packages.

--runtime-only: check interpreter version before creating a venv.
default: also check installed NumPy/SciPy versions and that imports work.
"""
import sys


def main():
    if sys.version_info < (3, 10):
        return 1
    if "--runtime-only" in sys.argv[1:]:
        return 0
    try:
        import numpy
        import scipy

        numpy_major = int(numpy.__version__.split(".")[0])
        scipy_parts = [int(part) for part in scipy.__version__.split(".")[:2]]
        if not (2 <= numpy_major < 3):
            return 1
        if not ((1, 13) <= tuple(scipy_parts) < (2, 0)):
            return 1
    except (ImportError, ValueError, IndexError, OSError):
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
