# Package mode with only per-file libraries and no package aggregate

This test verifies that Gazelle does not emit a package-level `py_library`
when every module is already owned by a single-source preserved target, and
that those targets claim their sources so nothing is duplicated.
