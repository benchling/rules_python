# Package mode with an excluded-only hand-written target

This test verifies that a `py_library` listing only `gazelle:exclude` sources
stays unmanaged while Gazelle still adopts other targets that own
Gazelle-managed modules.
