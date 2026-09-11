# Package Mode With An Existing Test Named After The Package

This test verifies that a hand-written `py_test` using the package library name
is preserved in a package that has no library sources. Gazelle generates no
package library there, so the name is free, and the target's sources must not
also appear in a generated `py_test`.
