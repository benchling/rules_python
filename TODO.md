# Review follow-ups: preserving existing Python source targets

Status of review findings for the `py_library` / `py_test` preservation
feature. Line references are against `gazelle/python/generate.go` as of the
last commit on `martani/preserve-existing-targets-2`.

Background on why any of this is destructive: putting a hand-written target into
`result.Gen` moves it from *unmanaged* to *managed*. `srcs` is in
`MergeableAttrs` and `deps` / `pyi_deps` / `pyi_srcs` are in `ResolveAttrs`
(`gazelle/python/kinds.go:57-74` for `py_library`, `:81-98` for `py_test`), and
`rule.MergeRules` drops any value in the existing list that is not in the
generated set unless it carries a `# keep` comment. Attributes present in the
generated rule but absent from the existing one are copied in unconditionally,
regardless of mergeability.

## Explicitly decided, do not reopen

- **Pruning hand-written `deps`.** Preserved targets participate in the resolve
  phase, so a `deps` entry not derivable from an `import` statement (e.g. an
  `importlib` plugin, a `//third_party/...` runtime dep) is deleted. This is
  intended behavior and is covered by the release note and user documentation.

## Fixed

- **`visibility` injected into previously-unmanaged targets** — preserved
  targets no longer get `addVisibility`, since `MergeRules` copies a
  non-mergeable attribute the existing rule does not set and offers no `# keep`
  for it.
- **Entrypoints and `conftest.py` in a preserved rule** — a rule listing
  `__main__.py`, `__test__.py` or `conftest.py` is no longer adopted.
- **The claiming threshold** is now taken from the declared src count rather
  than the pruned set, so deleting an unrelated file cannot flip a target
  between claiming and not claiming.
- **Name collisions with generated targets** — the names Gazelle will generate
  are collected before claiming and existing rules with one of those names are
  not adopted. This covers the per-file names, the package library/test names,
  the `py_binary` name, `conftest`, and a `py_binary` extracted from a
  preserved target's own main module. Without it the two rules merged and
  orphaned the sources of whichever lost.
- **A preserved target whose sources are all main modules** is now left as
  written instead of emptied and deleted.
- **Stale `deps` inherited from an extracted main module** — dependencies are
  recomputed after main modules are removed from `srcs`.
- **User documentation and release notes** now describe eligibility, claiming,
  managed attributes, `# keep`, exclusions, and the possible dependency cycle.

## Correctness

### 1. Preserved and generated targets can form a dependency cycle

Once a preserved target claims its sources, imports can point both ways:
`custom`'s `bar.py` imports `foo` so `custom` gets `deps = [":pkg"]`, and if
`pkg`'s `foo.py` imports `bar` then `pkg` gets `deps = [":custom"]`. Bazel
rejects the cycle. Before the feature this was a self-import inside one target
and produced no edge.

Not reachable in the current fixtures, which are arranged so imports only flow
from the preserved target to the generated one. Detecting it properly needs the
resolve phase, which runs after generation, so this is likely a documentation
item rather than something to block on. It is also already reachable in file
mode without the feature.

## Test gaps

Ranked. None of these exist today.

1. `map_kind` / `alias_kind` with a target name that is not the generated
   name — the preservation path is entirely untested for renamed kinds. The
   three existing cases (`respect_alias_kind`, `respect_kind_mapping`,
   `respect_alias_and_map_kind`) all use names the filters exclude.
2. A preserved `py_test` in file mode.
3. `srcs = glob([...])`: `AttrStrings` returns nothing, so the rule is skipped
   and its files are also swept into generated targets. Probably the right
   conservative behavior, but it is silent and unasserted — and `glob` is the
   commonest hand-written form.
4. Label and non-`.py` srcs (`srcs = ["a.py", ":generated.py"]`,
   `srcs = ["a.py", "schema.json"]`), which disqualify the whole rule.
5. Project mode with a preserved target listing subdirectory sources, and any
   case in a subpackage — all preservation fixtures sit at the workspace root
   (`args.Rel == ""`), so `imports` rendering is never exercised.
6. Custom `python_library_naming_convention` /
   `python_test_naming_convention`, which feed the "is this the generated
   target?" filters via `RenderLibraryName` / `RenderTestName`.
7. A test-pattern file inside a preserved library's srcs — `knownPySrcs`
   merges library and test filenames, so a preserved `py_library` can claim
   `foo_test.py` and suppress the generated `py_test`.

## Diagnostics

The preservation code emits no log output at all. Every branch that declines to
preserve a rule (no srcs, a label or non-`.py` src, an entrypoint or
`conftest.py` src, no managed src, a name Gazelle generates) leaves Gazelle
generating a competing target over the same sources, which is exactly what the
user needs to know. Consider one log line per declined rule.

Separately, the `log.Fatalf` calls in `appendPyLibrary` and
`newPyTestTargetBuilder` name no target, and the same file is now parsed twice
(once for the preserved target, once for the generated one), so the message does
not identify which target failed.

## Cosmetic

- `testdata/per_file_respect_existing_multiple_srcs/BUILD.in:11` and
  `testdata/project_generation_mode_respect_existing_multiple_srcs/BUILD.in:19`
  indent `tags` with a literal tab; neighbouring lines use four spaces.
- `testdata/project_generation_mode_respect_existing_multiple_srcs/BUILD.in`
  has a stray double blank line, and its `load` statement omits `py_test`
  although the file uses it.
- The testdata READMEs added by the first preservation commit use Title Case
  headings; the dominant convention across the other ~80 cases is sentence case.
