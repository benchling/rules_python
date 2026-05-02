# Project Generation With Existing Target Spanning Multiple Files

This test verifies that project generation preserves existing non-standard
`py_library` and `py_test` targets while still generating project-wide targets
for unclaimed sources.

Gazelle should prune sources that no longer exist, keep non-generated
attributes, add generated dependencies, and leave the existing `__init__` target
unchanged.
