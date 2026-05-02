# Package Mode With Existing Target Spanning Multiple Files

This test verifies that default package generation preserves a non-standard
`py_library` that already owns multiple sources.

Gazelle should prune sources that no longer exist, keep the target's
non-generated attributes, add generated dependencies, and put unclaimed sources
in the generated package target.
