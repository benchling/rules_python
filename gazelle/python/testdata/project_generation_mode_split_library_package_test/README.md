# Project generation with a split package library and a package test

This test verifies that a hand-written package library alongside per-file
libraries does not cause Gazelle to treat the existing package test as a
separate preserved target. The package test is still generated, so it must
own every unclaimed test source, including a newly added one in a
subdirectory. Otherwise the preserved and generated targets share a name and
Gazelle alternates between their source lists on every run.
