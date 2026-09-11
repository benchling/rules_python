# Per-File Generation With Preserved Target Owning `__init__.py` and a Main Module

This test verifies that extracting a main module from a preserved target does not
also drop a hand-written `__init__.py` source.

With `python_generation_mode_per_file_include_init`, Gazelle adds `__init__.py`
to the per-file targets it generates. It must not remove `__init__.py` from a
preserved target that listed it explicitly, which would empty the target's srcs
and delete it.

The preserved target gets no dependency on `:foo`: `cli.py` imports `foo`, but
dependencies are recomputed after the main module is extracted, so an import
from a source the target no longer owns does not survive in its `deps`.
