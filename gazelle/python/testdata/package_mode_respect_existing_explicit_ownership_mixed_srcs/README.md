# Package mode with explicit source ownership (mixed single- and multi-source)

This test verifies that Gazelle does not emit a package-level `py_library`
when preserved libraries collectively own every module without overlap, including
a multi-source target alongside single-source targets.
