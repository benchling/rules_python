# Package mode with a single preserved library owning the only module

This test verifies that one preserved `py_library` covering the package's only
Gazelle-managed source is enough to suppress the generated package aggregate and
claim that source.
