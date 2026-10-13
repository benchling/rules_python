# Package Mode With Preserved Target Owning Unmanaged Sources

This test verifies that Gazelle only prunes sources that do not exist on disk.

`ignored.py` and `excluded.py` exist but are hidden from generation by
`python_ignore_files` and `gazelle:exclude`. Those directives suppress
generation; they must not cause Gazelle to delete the sources from a
hand-written target. Only `removed.py`, which does not exist, is pruned.
