# File mode with inherited config, init package library, and per-file targets

Parent `util/BUILD` sets `gazelle:python_generation_mode file`. The child
`util/events` package has no local mode directive, a hand-written package
library named after the directory (`events`) that owns only `__init__.py`, and a
separate per-file library for `datadog.py`.

Gazelle must not merge unclaimed per-file sources into the package library in
file mode; doing so duplicates import specs with the per-file targets.
