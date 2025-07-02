# Per-file generation with existing target spanning multiple files

This test case generates one `py_library` per file, but has an existing target containing 2 files. In this
case, the existing target should be preserved (and used for the existing 2 files), but new targets should be
created for new files.
