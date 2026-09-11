# Package mode with a split package library and per-file libraries

This test verifies that a hand-written target using the package library name
is preserved and regenerated in place alongside per-file `py_library` targets.
Sources owned by the per-file targets must not also appear in the generated
package library.
