# Package mode with excluded `__init__.py` and a split package library

This test verifies that a hand-written package library owning only an excluded
`__init__.py` is still preserved alongside per-file `py_library` targets. The
parent `gazelle:exclude **/__init__.py` pattern matches Benchling monolith
packages where init-only package libraries must not cause Gazelle to regenerate
a competing package-level target over the remaining modules.
