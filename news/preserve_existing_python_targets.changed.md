(gazelle) Existing hand-written {obj}`py_library` and {obj}`py_test` targets
that own sources Gazelle manages can now be regenerated in place. Gazelle
manages their `srcs` and dependency attributes, and usually keeps their sources
out of generated targets; a single-source library remains shared outside
project mode. Dependencies not derived from imports are removed unless marked
with `# keep`. Add `# keep` above the rule to opt out. Rules named after a
generated target, and rules listing `__main__.py`, `__test__.py`, or
`conftest.py`, are excluded from this preservation behavior.
