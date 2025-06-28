# Clearing deps / pyi_deps

This test case asserts that an existing `py_library` specifying `deps` and
`pyi_deps` have these attributes removed if the corresponding imports are
removed.
