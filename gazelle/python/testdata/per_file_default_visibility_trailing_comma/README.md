# Default visibility with a trailing comma in file generation mode

This test verifies that a `python_default_visibility` directive whose value
ends with a comma does not emit an empty visibility label on generated
per-file targets. That empty label is what Bazel lint rejects as an invalid
`""` label when a new per-file target is emitted after splitting a preserved
multi-source library.
