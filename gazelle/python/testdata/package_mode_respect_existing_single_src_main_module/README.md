# Package Mode With Preserved Single-Source Target Containing a Main Module

This test verifies that a main module owned by a preserved target yields exactly
one `py_binary`.

A preserved target with a single source does not claim it, so the source is also
part of the generated package target. Gazelle must still extract the main module
only once instead of emitting a duplicate `py_binary` for each target that owns
the source.
