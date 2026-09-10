# Package mode with an existing target owning entrypoint sources

This test verifies that a rule listing an entrypoint or `conftest.py` is left
untouched rather than preserved.

`__main__.py` and `conftest.py` always get their own generated targets, and
Gazelle has no way to hand them over to another target. Regenerating `custom`
in place would therefore leave two targets owning each of those sources, so
`custom` keeps all three of its sources and gains no generated attributes.
