# Package mode with an existing target named after the generated conftest

This test verifies that the generated-name check covers target names Gazelle
derives from something other than the generation mode.

`conftest` is the name of the target Gazelle generates for `conftest.py`, so the
existing `conftest` is not adopted. Were it adopted, it would claim `bar.py` and
`baz.py` and the generated `conftest` would then merge over it, dropping both
sources from the build.
