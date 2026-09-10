# Simple binary with library

This test case asserts that a simple `py_binary` is generated as expected
referencing a `py_library`.

The existing custom `py_library` shares `bar.py` with the generated package
library. Gazelle should preserve that shared source in both targets and leave
the custom target otherwise untouched: it gets no `visibility`, because
injecting one would widen a hand-written target that relies on the default
private visibility.
