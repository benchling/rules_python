# Per-File Generation With Preserved Target Containing a Main Module

This test verifies that per-file generation still extracts a `py_binary` when a
preserved target contains a source with `if __name__ == "__main__":`.

Gazelle should remove the main-module source from the preserved `py_library`,
keep the remaining sources and non-generated attributes, and generate the
matching `py_binary`.
