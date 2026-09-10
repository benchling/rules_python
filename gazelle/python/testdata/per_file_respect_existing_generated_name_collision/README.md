# Per-file generation with an existing target named after a generated one

This test verifies that Gazelle refuses to adopt an existing target whose name
is one it generates itself.

`qux` is the per-file target name for `qux.py`, so the existing `qux` is not a
hand-written target to regenerate in place. Adopting it would let it claim
`bar.py` and `baz.py`, suppressing the per-file targets for them, and the
generated `qux` would then merge over it and drop both sources -- leaving no
target that owns them.

Instead the existing rule is left out of preservation and `bar.py` and `baz.py`
get their own per-file targets. Gazelle still merges the generated `qux` over
the existing rule of that name, which is its long-standing behavior for any
rule whose name matches a generated target.
