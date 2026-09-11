# Per-file generation with a preserved target of only main modules

This test verifies that a preserved target is never deleted by having its
sources extracted out from under it.

In per-file generation a main module is removed from the library's srcs so that
only the `py_binary` owns it. Here every source is a main module, so the target
would be left with empty srcs, and an empty rule is one Gazelle reports as
removable. Rather than delete a hand-written target, Gazelle leaves it exactly
as written and still generates the two `py_binary` targets.
