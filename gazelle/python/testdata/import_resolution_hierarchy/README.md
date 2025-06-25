# Python Import Resolution Hierarchy

This test case verifies that Python imports are resolved correctly following Python's module search path semantics, especially when there are multiple files with the same name in different directories.

## Test Scenario

The directory structure is:
```
a/
├── bar.py          # Module at project level
├── foo.py          # Imports "bar" - should resolve to a/bar.py
└── b/
    ├── bar.py      # Local module in subdirectory
    └── foo.py      # Imports "bar" - should resolve to a/bar.py (NOT a/b/bar.py)
```

## Expected Behavior

When `a/b/foo.py` contains `import bar`, it should resolve to `//a:bar` (the module at the parent level), not `//a/b:bar` (the local module), following Python's import resolution semantics where imports are resolved from the project root.

## Bug Fixed

This test case was created to verify the fix for a bug where `import bar` from `a/b/foo.py` was incorrectly resolving to `:bar` (local `a/b/bar.py`) instead of `//a:bar` (parent-level `a/bar.py`).

## Generation Mode

Uses per-file generation mode where each `.py` file generates its own `py_library` target.

`__init__.py` files are left empty so no target is generated for them.
