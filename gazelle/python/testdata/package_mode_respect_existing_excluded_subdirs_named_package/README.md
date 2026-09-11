# Package mode with excluded subdirs and a package-named library

Mirrors monolith layouts where `benchling/`, `inductive/`, and `tunelab/`
subdirectories are excluded from Gazelle but a hand-written `py_library` named
after the package aggregates their sources, `data`, `deps`, and custom attrs.
Gazelle must leave that target unmanaged.
