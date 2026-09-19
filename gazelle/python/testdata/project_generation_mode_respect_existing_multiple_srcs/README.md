# Project Generation With Existing Target Spanning Multiple Files

This test verifies that project generation preserves existing non-standard
`py_library` and `py_test` targets while still generating project-wide targets
for unclaimed sources.

Gazelle should prune sources that no longer exist, keep non-generated
attributes, and add generated dependencies.

Unlike the other generation modes, project mode has a single generated library
for the whole tree, so every preserved target claims its sources even when it
has only one. That is why `__init__.py` is absent from the generated project
library, and why the existing `__init__` target appears unchanged: regenerating
it produces identical content.
