# Per-File Generation With Existing Target Spanning Multiple Files

This test verifies that per-file generation preserves a non-standard
`py_library` that already owns multiple sources.

Gazelle should prune sources that no longer exist, keep the target's
non-generated attributes, add generated dependencies, and create per-file
targets only for unclaimed files.
