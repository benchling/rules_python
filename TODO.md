# Open follow-ups: preserving existing Python source targets

Review findings for the `py_library` / `py_test` preservation feature that are
**not** addressed on this branch. Line references are against
`gazelle/python/generate.go` as of the last commit on
`martani/preserve-existing-targets-2`.

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
  intended behavior. It still needs a release note and a documented `# keep`
  story — see "Release notes" and "Documentation" below.

## Correctness

### 1. `visibility` is injected into previously-unmanaged targets

`generate.go:548` (`addVisibility(visibility)` on the `py_library` built by
`appendPyLibrary`).

`visibility` is not in `MergeableAttrs`, but `MergeRules` copies attributes that
are absent from the existing rule, and there is no `# keep` path for an absent
attribute. A hand-written target relying on Bazel's default private visibility
silently becomes visible across the subtree.

Already enshrined in
`gazelle/python/testdata/simple_binary_with_library/BUILD.out`, whose comment
changed from "This target should be kept unmodified by Gazelle" to "Gazelle
should preserve this custom target". Decide whether preserved targets should get
`visibility` at all; if not, only apply it to targets Gazelle created. Either
way, keep a case in the suite asserting that Gazelle does not rewrite a target
it is not managing — that guarantee currently has no test.

### 2. Preserved and generated targets can form a dependency cycle

Once a preserved target claims its sources, imports can point both ways:
`custom`'s `bar.py` imports `foo` so `custom` gets `deps = [":pkg"]`, and if
`pkg`'s `foo.py` imports `bar` then `pkg` gets `deps = [":custom"]`. Bazel
rejects the cycle. Before the feature this was a self-import inside one target
and produced no edge.

Not reachable in the current fixtures, which are arranged so imports only flow
from the preserved target to the generated one. Needs a decision (detect and
warn? refuse to claim?) and a test either way.

### 3. Entrypoints and `conftest.py` in a preserved rule cause double ownership

`generate.go:397-413` adds `__main__.py`, `__test__.py` and `conftest.py` to
`knownPySrcs`, but they are never members of `pyLibraryFilenames` /
`pyTestFilenames` — the scan at `generate.go:260-279` routes them to dedicated
branches. So `removeClaimedSrcs` cannot remove them, and the `py_binary`,
`conftest` and package-level `py_test` targets are still generated from the same
file. Note `generate.go:702` adds `__test__.py` to `pyTestFilenames` *after*
claiming has run.

Probably best handled by declining to preserve a rule that lists an entrypoint or
`conftest.py`, leaving it untouched as before the feature.

### 4. The claiming threshold is computed on the pruned source set

`generate.go:433-440`: `sourceRule.srcs.Size() > 1 || cfg.CoarseGrainedGeneration()`,
where `srcs` has already had nonexistent entries removed. So
`srcs = ["bar.py", "baz.py"]` claims its sources but
`srcs = ["bar.py", "deleted.py"]` does not — deleting an unrelated file flips a
target between claiming and non-claiming. Claiming should not depend on
filesystem state this way.

Dropping the single-source carve-out entirely would resolve this and simplify the
feature, at the cost of changing `simple_binary_with_library` (`bar.py` would
leave the generated package library). Worth evaluating against
`testdata/dont_rename_target` and `testdata/invalid_imported_module/foo`.

### 5. Per-file name collisions are not checked

`sourceRuleMatchesGeneratedPerFileName` (`generate.go:183-192`) compares the rule
name only against the basenames of *its own* srcs. In file mode,
`py_library(name = "qux", srcs = ["bar.py", "baz.py"])` alongside an unclaimed
`qux.py` puts two `py_library(name = "qux")` rules into `result.Gen`;
`ensureNoCollision` cannot catch it because both are the same kind. Same gap for
`py_test` via the filter at `generate.go:424-432`.

Fix by collecting the names Gazelle will generate before the filters run and
rejecting existing rules whose name is in that set.

### 6. Stale `deps` inherited from an extracted main module

`generate.go:452` parses `srcs` at the top of `appendPyLibrary`, before
`srcs.Remove(name)` strips main modules further down. A preserved target
therefore keeps dependencies contributed by a source it no longer owns.

Pinned in
`testdata/per_file_respect_existing_with_init_and_main_module/BUILD.out`, where
`custom` has `deps = [":foo"]` although its only remaining src (`__init__.py`)
imports nothing. Fixing it needs per-source dep attribution rather than the
aggregated `allDeps`.

### 7. A preserved target whose sources are all main modules is still deleted

The `__init__.py` trigger for this path is fixed, but the underlying shape
remains: in file mode a preserved target whose every src is a main module has its
`srcs` emptied, then `generate.go:517-529` finds the same-named existing rule,
sets `generateEmptyLibrary`, and the built rule is `IsEmpty` — so the
hand-written target is deleted rather than preserved.

## Release notes

`CONTRIBUTING.md:176-191` requires a news fragment per change; this branch has
none. Add `news/<pr>.changed.md`. It must call out that Gazelle now manages
`srcs` and `deps` on previously hand-written `py_library` / `py_test` targets,
that `deps` not derivable from imports will be removed, and that `# keep` is the
escape hatch.

## Documentation

`gazelle/docs/installation_and_usage.md:166-169` still says "all source files are
collected into the `srcs` of the `py_library`", which is no longer true — sources
claimed by a preserved target are excluded. The `### Tests` section
(`:178`) should state that existing `py_test` targets are preserved and always
claim their srcs, unlike libraries.

Also document, since none of it is written down anywhere:

- Which attributes Gazelle takes over on a preserved target (`srcs`, `deps`) and
  which it leaves alone (`tags`, and `visibility` when already present).
- That `# keep` is the only way to protect a value.
- That there is no directive to opt out of preservation. `gazelle/docs/directives.md`
  is unchanged by this branch; consider whether an opt-out is needed.

## Code comments

`generate.go` documents 9 of its 12 pre-existing package-level functions, and the
three that it doesn't are self-describing. These new declarations still have no
doc comment:

- `existingPythonSourceRule` (`:51`)
- `isTargetSrc` (`:82`) — worth noting the polarity differs by call site: a label
  src disqualifies a rule from preservation (`:128`) but marks it *valid* in
  `getRulesWithInvalidSrcs` (`:819`).
- `addSetValuesToMap` (`:154`), `removeClaimedSrcs` (`:161`),
  `filterExistingPythonSourceRules` (`:173`),
  `sourceRuleMatchesGeneratedPerFileName` (`:183`)

`removeClaimedSrcs` is the place to define "claim", which is load-bearing
vocabulary used nowhere else. The policy block at `generate.go:415-442` also needs
the *why* for two rules a reader cannot derive: why a single-source library does
not claim while a `py_test` always does, and why targets whose name matches a
generated name are excluded from preservation.

## Test gaps

Ranked. None of these exist today.

1. `map_kind` / `alias_kind` with a target name that is not the generated name —
   the preservation path is entirely untested for renamed kinds. The three
   existing cases (`respect_alias_kind`, `respect_kind_mapping`,
   `respect_alias_and_map_kind`) all use names the filters exclude.
2. A preserved `py_test` in file mode, and the collision shape where a preserved
   `py_test` named `foo_test` has `srcs = ["bar_test.py"]`.
3. `srcs = glob([...])`: `AttrStrings` returns nothing, so the rule is skipped and
   its files are also swept into generated targets. Probably the right
   conservative behavior, but it is silent and unasserted — and `glob` is the
   commonest hand-written form.
4. Label and non-`.py` srcs (`srcs = ["a.py", ":generated.py"]`,
   `srcs = ["a.py", "schema.json"]`), which disqualify the whole rule.
5. Project mode with a preserved target listing subdirectory sources, and any case
   in a subpackage — all preservation fixtures sit at the workspace root
   (`args.Rel == ""`), so `imports` rendering is never exercised.
6. Custom `python_library_naming_convention` /
   `python_test_naming_convention`, which feed the "is this the generated
   target?" filters via `RenderLibraryName` / `RenderTestName`.
7. A test-pattern file inside a preserved library's srcs — `knownPySrcs` merges
   library and test filenames, so a preserved `py_library` can claim `foo_test.py`
   and suppress the generated `py_test`.

## Diagnostics

The preservation code emits no log output at all. Every branch that declines to
preserve a rule (`generate.go:120-122` no srcs, `:139-141` the `skip` bail-out
decided at `:128` for a label or non-`.py` src, `:142-144` no managed src) leaves
Gazelle generating a competing target over the same sources, which is exactly
what the user needs to know. Consider one log line per declined rule.

Separately, `log.Fatalf` at `generate.go:457` and `:668` names no target, and the
same file is now parsed twice (once for the preserved target, once for the
generated one), so the message does not identify which target failed. The
`mainModules` loop at `:453-455` also runs before the `err` check at `:456`.

## Cosmetic

- `testdata/per_file_respect_existing_multiple_srcs/BUILD.in:11` and
  `testdata/project_generation_mode_respect_existing_multiple_srcs/BUILD.in:19`
  indent `tags` with a literal tab; neighbouring lines use four spaces.
- `testdata/project_generation_mode_respect_existing_multiple_srcs/BUILD.in`
  has a stray double blank line, and its `load` statement omits `py_test`
  although the file uses it.
- New testdata READMEs use Title Case headings; the dominant convention across
  the other ~80 cases is sentence case.
- The new `test.yaml` files omit the license header that most existing ones carry.
