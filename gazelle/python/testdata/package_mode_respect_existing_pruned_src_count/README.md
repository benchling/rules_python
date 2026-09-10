# Package mode with a preserved target whose declared sources are pruned

This test verifies that whether a preserved library claims its sources depends
only on what the BUILD file declares.

A library owning a single source does not claim it, but the count is taken from
the srcs the user wrote rather than from the srcs that survive pruning. `custom`
declares two sources, so it claims `bar.py` even though `removed.py` no longer
exists, and `bar.py` stays out of the generated package library. Deleting an
unrelated file must not flip a target between claiming and not claiming.
