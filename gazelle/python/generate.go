// Copyright 2023 The Bazel Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package python

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
	"github.com/bmatcuk/doublestar/v4"
	"github.com/emirpasic/gods/lists/singlylinkedlist"
	"github.com/emirpasic/gods/sets/treeset"
	godsutils "github.com/emirpasic/gods/utils"

	"github.com/bazel-contrib/rules_python/gazelle/pythonconfig"
)

const (
	pyLibraryEntrypointFilename = "__init__.py"
	pyBinaryEntrypointFilename  = "__main__.py"
	pyTestEntrypointFilename    = "__test__.py"
	pyTestEntrypointTargetname  = "__test__"
	conftestFilename            = "conftest.py"
	conftestTargetname          = "conftest"
)

var (
	buildFilenames = []string{"BUILD", "BUILD.bazel"}
)

// existingPythonSourceRule is a hand-written rule that Gazelle regenerates in
// place instead of replacing.
type existingPythonSourceRule struct {
	name string
	// srcs are the rule's srcs with the entries that no longer exist pruned.
	srcs *treeset.Set
	// declaredSrcCount is the number of srcs the rule lists in the BUILD file,
	// before pruning. Decisions about how a rule is treated are made on this
	// count so that they reflect only what the user wrote: deleting an unrelated
	// file must not change how Gazelle handles the rule.
	declaredSrcCount int
}

// Returns the mapped kind, or kind if no mapping is configured with the map_kind directive.
func getMappedKind(c *config.Config, kind string) string {
	if mapped, ok := c.KindMap[kind]; ok {
		return mapped.KindName
	}
	return kind
}

// kindMatches returns whether r matches the canonical Python rule kind `expected`, respecting `# gazelle:map_kind` and
// `# gazelle:alias_kind` directives in the config.Config c.
func kindMatches(c *config.Config, r *rule.Rule, expected string) bool {
	kind := r.Kind()
	return kind == getMappedKind(c, expected) || c.AliasMap[kind] == expected
}

func matchesAnyGlob(s string, globs []string) bool {
	// This function assumes that the globs have already been validated. If a glob is
	// invalid, it's considered a non-match and we move on to the next pattern.
	for _, g := range globs {
		if ok, _ := doublestar.Match(g, s); ok {
			return true
		}
	}
	return false
}

// isTargetSrc reports whether src is a label rather than a file path.
func isTargetSrc(src string) bool {
	return strings.HasPrefix(src, "@") || strings.HasPrefix(src, "//") || strings.HasPrefix(src, ":")
}

// collectExistingPythonSourceRules returns the rules of the canonical kind
// `kind` that Gazelle should regenerate in place rather than replace. knownSrcs
// holds the source files Gazelle would itself put in a generated target's srcs.
//
// A rule is only adopted if at least one of its srcs is in knownSrcs; a rule
// built entirely from sources Gazelle was told to leave alone is left alone too.
// Once adopted, srcs that exist but are absent from knownSrcs are still kept:
// python_ignore_files, gazelle:exclude and subdirectory sources are hidden from
// generation, which must not cause Gazelle to delete them from a hand-written
// target. Only srcs that no longer exist are pruned.
//
// A rule listing an entrypoint or conftest.py is never adopted: those sources
// have dedicated targets that Gazelle always generates, so adopting the rule
// would leave two targets owning the same file.
func collectExistingPythonSourceRules(args language.GenerateArgs, kind string, knownSrcs map[string]struct{}) []existingPythonSourceRule {
	if args.File == nil {
		return nil
	}

	genFiles := make(map[string]struct{}, len(args.GenFiles))
	for _, f := range args.GenFiles {
		genFiles[f] = struct{}{}
	}
	srcExists := func(src string) bool {
		if _, ok := genFiles[src]; ok {
			return true
		}
		_, err := os.Stat(filepath.Join(args.Dir, src))
		return err == nil
	}

	var sourceRules []existingPythonSourceRule
	for _, existingRule := range args.File.Rules {
		if !kindMatches(args.Config, existingRule, kind) {
			continue
		}

		srcs := existingRule.AttrStrings("srcs")
		if len(srcs) == 0 {
			continue
		}

		validSrcs := treeset.NewWith(godsutils.StringComparator)
		skip := false
		hasKnownSrc := false
		for _, src := range srcs {
			if isTargetSrc(src) || filepath.Ext(src) != ".py" {
				skip = true
				break
			}
			if src == pyBinaryEntrypointFilename ||
				src == pyTestEntrypointFilename ||
				src == conftestFilename {
				skip = true
				break
			}
			if _, ok := knownSrcs[src]; ok {
				hasKnownSrc = true
				validSrcs.Add(src)
			} else if srcExists(src) {
				validSrcs.Add(src)
			}
		}
		if skip {
			continue
		}
		if !hasKnownSrc {
			continue
		}

		sourceRules = append(sourceRules, existingPythonSourceRule{
			name:             existingRule.Name(),
			srcs:             validSrcs,
			declaredSrcCount: len(srcs),
		})
	}
	return sourceRules
}

// addSetValuesToMap copies every value in srcs into dst.
func addSetValuesToMap(srcs *treeset.Set, dst map[string]struct{}) {
	it := srcs.Iterator()
	for it.Next() {
		dst[it.Value().(string)] = struct{}{}
	}
}

// removeClaimedSrcs removes sources owned by rules from the generated source
// sets. A preserved rule claims a source when it becomes its sole generated
// owner rather than sharing it with another target Gazelle generates.
func removeClaimedSrcs(rules []existingPythonSourceRule, srcSets ...*treeset.Set) {
	for _, sourceRule := range rules {
		it := sourceRule.srcs.Iterator()
		for it.Next() {
			src := it.Value().(string)
			for _, srcSet := range srcSets {
				srcSet.Remove(src)
			}
		}
	}
}

// filterExistingPythonSourceRules returns the rules accepted by shouldKeep.
func filterExistingPythonSourceRules(
	rules []existingPythonSourceRule,
	shouldKeep func(existingPythonSourceRule) bool,
) []existingPythonSourceRule {
	filtered := make([]existingPythonSourceRule, 0, len(rules))
	for _, sourceRule := range rules {
		if shouldKeep(sourceRule) {
			filtered = append(filtered, sourceRule)
		}
	}
	return filtered
}

// existingRulesShareSrcs reports whether two preserved rules list the same
// source file.
func existingRulesShareSrcs(a, b existingPythonSourceRule) bool {
	it := a.srcs.Iterator()
	for it.Next() {
		if b.srcs.Contains(it.Value()) {
			return true
		}
	}
	return false
}

// hasSplitPackageLibraryLayout reports whether the package already defines the
// generated package library name alongside other preserved libraries whose
// sources are disjoint from it. This is the layout where per-file libraries own
// individual modules and the package library owns the remainder.
func hasSplitPackageLibraryLayout(packageLibraryName string, rules []existingPythonSourceRule) bool {
	var packageLibrary *existingPythonSourceRule
	for i := range rules {
		if rules[i].name == packageLibraryName {
			packageLibrary = &rules[i]
			break
		}
	}
	if packageLibrary == nil || len(rules) < 2 {
		return false
	}
	for _, other := range rules {
		if other.name == packageLibraryName {
			continue
		}
		if existingRulesShareSrcs(*packageLibrary, other) {
			return false
		}
	}
	return true
}

// hasExplicitSourceOwnershipLayout reports whether preserved non-package
// libraries collectively own every Gazelle-managed library source without
// overlapping each other and without using the generated package library name.
// When true, Gazelle must not emit a package-level library and every preserved
// library must claim its sources, regardless of per-target source counts.
func hasExplicitSourceOwnershipLayout(
	packageLibraryName string,
	rules []existingPythonSourceRule,
	libraryFilenames *treeset.Set,
) bool {
	if libraryFilenames == nil || libraryFilenames.Empty() || len(rules) == 0 {
		return false
	}
	for _, sourceRule := range rules {
		if sourceRule.name == packageLibraryName {
			return false
		}
	}
	for i := range rules {
		for j := i + 1; j < len(rules); j++ {
			if existingRulesShareSrcs(rules[i], rules[j]) {
				return false
			}
		}
	}
	covered := make(map[string]struct{})
	for _, sourceRule := range rules {
		it := sourceRule.srcs.Iterator()
		for it.Next() {
			covered[it.Value().(string)] = struct{}{}
		}
	}
	it := libraryFilenames.Iterator()
	for it.Next() {
		if _, ok := covered[it.Value().(string)]; !ok {
			return false
		}
	}
	return true
}

// adoptExcludedInitOnlyPackageLibraryForSplitLayout appends the hand-written
// package library when it lists only an excluded __init__.py. That target is
// otherwise not adopted because none of its srcs are Gazelle-managed, but it
// is still the package aggregate in a split layout and must be preserved so
// Gazelle does not emit a competing package-level library.
func adoptExcludedInitOnlyPackageLibraryForSplitLayout(
	args language.GenerateArgs,
	kind string,
	packageLibraryName string,
	knownSrcs map[string]struct{},
	rules []existingPythonSourceRule,
) []existingPythonSourceRule {
	if args.File == nil || len(rules) == 0 {
		return rules
	}
	for _, sourceRule := range rules {
		if sourceRule.name == packageLibraryName {
			return rules
		}
	}

	genFiles := make(map[string]struct{}, len(args.GenFiles))
	for _, f := range args.GenFiles {
		genFiles[f] = struct{}{}
	}
	srcExists := func(src string) bool {
		if _, ok := genFiles[src]; ok {
			return true
		}
		_, err := os.Stat(filepath.Join(args.Dir, src))
		return err == nil
	}

	for _, existingRule := range args.File.Rules {
		if existingRule.Name() != packageLibraryName || !kindMatches(args.Config, existingRule, kind) {
			continue
		}
		srcs := existingRule.AttrStrings("srcs")
		if len(srcs) != 1 || srcs[0] != pyLibraryEntrypointFilename {
			return rules
		}
		if _, ok := knownSrcs[pyLibraryEntrypointFilename]; ok {
			return rules
		}
		if !srcExists(pyLibraryEntrypointFilename) {
			return rules
		}

		validSrcs := treeset.NewWith(godsutils.StringComparator, pyLibraryEntrypointFilename)
		candidate := existingPythonSourceRule{
			name:             packageLibraryName,
			srcs:             validSrcs,
			declaredSrcCount: 1,
		}
		for _, other := range rules {
			if existingRulesShareSrcs(candidate, other) {
				return rules
			}
		}
		return append(rules, candidate)
	}
	return rules
}

// adoptEmptyAggregatePackageLibraryForSplitLayout appends the hand-written
// package library when it omits srcs entirely. That target is otherwise not
// adopted because collectExistingPythonSourceRules ignores rules without srcs,
// but it is still the deps-only package aggregate in a split layout and must
// be preserved so Gazelle does not emit a competing package-level library.
// Only applies when every other adopted library is a single-module target.
func adoptEmptyAggregatePackageLibraryForSplitLayout(
	args language.GenerateArgs,
	kind string,
	packageLibraryName string,
	rules []existingPythonSourceRule,
) []existingPythonSourceRule {
	if args.File == nil || len(rules) == 0 {
		return rules
	}
	for _, sourceRule := range rules {
		if sourceRule.name == packageLibraryName {
			return rules
		}
	}
	for _, other := range rules {
		if other.declaredSrcCount != 1 {
			return rules
		}
	}

	for _, existingRule := range args.File.Rules {
		if existingRule.Name() != packageLibraryName || !kindMatches(args.Config, existingRule, kind) {
			continue
		}
		if len(existingRule.AttrStrings("srcs")) != 0 {
			return rules
		}

		candidate := existingPythonSourceRule{
			name:             packageLibraryName,
			srcs:             treeset.NewWith(godsutils.StringComparator),
			declaredSrcCount: 0,
		}
		for _, other := range rules {
			if existingRulesShareSrcs(candidate, other) {
				return rules
			}
		}
		return append(rules, candidate)
	}
	return rules
}

// addTargetNamesForSrcs records the per-file target name Gazelle derives from
// each of srcs.
func addTargetNamesForSrcs(srcs *treeset.Set, dst map[string]struct{}) {
	it := srcs.Iterator()
	for it.Next() {
		src := it.Value().(string)
		dst[strings.TrimSuffix(filepath.Base(src), ".py")] = struct{}{}
	}
}

// findConftestPaths returns package paths containing conftest.py, from currentPkg
// up through ancestors, stopping at module root.
func findConftestPaths(repoRoot, currentPkg, pythonProjectRoot string, includeAncestorConftest bool) []string {
	var result []string
	for pkg := currentPkg; ; pkg = filepath.Dir(pkg) {
		if pkg == "." {
			pkg = ""
		}
		if _, err := os.Stat(filepath.Join(repoRoot, pkg, conftestFilename)); err == nil {
			result = append(result, pkg)
		}
		// We traverse up the tree to find conftest files and we start in
		// the current package. Thus if we find one in the current package
		// and do not want ancestors, we break early.
		if !includeAncestorConftest {
			break
		}
		if pkg == "" {
			break
		}
	}
	return result
}

// GenerateRules extracts build metadata from source files in a directory.
// GenerateRules is called in each directory where an update is requested
// in depth-first post-order.
func (py *Python) GenerateRules(args language.GenerateArgs) language.GenerateResult {
	cfgs := args.Config.Exts[languageName].(pythonconfig.Configs)
	cfg := cfgs[args.Rel]

	if !cfg.ExtensionEnabled() {
		return language.GenerateResult{}
	}

	if !isBazelPackage(args.Dir) {
		if cfg.CoarseGrainedGeneration() {
			// Determine if the current directory is the root of the coarse-grained
			// generation. If not, return without generating anything.
			parent := cfg.Parent()
			if parent != nil && parent.CoarseGrainedGeneration() {
				return language.GenerateResult{}
			}
		}
	}

	pythonProjectRoot := cfg.PythonProjectRoot()

	packageName := filepath.Base(args.Dir)

	pyLibraryFilenames := treeset.NewWith(godsutils.StringComparator)
	pyTestFilenames := treeset.NewWith(godsutils.StringComparator)
	pyFileNames := treeset.NewWith(godsutils.StringComparator)

	// hasPyBinaryEntryPointFile controls whether a single py_binary target should be generated for
	// this package or not.
	hasPyBinaryEntryPointFile := false

	// hasPyTestEntryPointFile and hasPyTestEntryPointTarget control whether a py_test target should
	// be generated for this package or not.
	hasPyTestEntryPointFile := false
	hasPyTestEntryPointTarget := false
	hasConftestFile := false

	testFileGlobs := cfg.TestFilePattern()

	for _, f := range args.RegularFiles {
		if cfg.IgnoresFile(filepath.Base(f)) {
			continue
		}
		ext := filepath.Ext(f)
		if ext == ".py" {
			pyFileNames.Add(f)
			if !hasPyBinaryEntryPointFile && f == pyBinaryEntrypointFilename {
				hasPyBinaryEntryPointFile = true
			} else if !hasPyTestEntryPointFile && f == pyTestEntrypointFilename {
				hasPyTestEntryPointFile = true
			} else if f == conftestFilename {
				hasConftestFile = true
			} else if matchesAnyGlob(f, testFileGlobs) {
				pyTestFilenames.Add(f)
			} else {
				pyLibraryFilenames.Add(f)
			}
		}
	}

	// If a __test__.py file was not found on disk, search for targets that are
	// named __test__.
	if !hasPyTestEntryPointFile && args.File != nil {
		for _, rule := range args.File.Rules {
			if rule.Name() == pyTestEntrypointTargetname {
				hasPyTestEntryPointTarget = true
				break
			}
		}
	}

	// Add files from subdirectories if they meet the criteria.
	for _, d := range args.Subdirs {
		// boundaryPackages represents child Bazel packages that are used as a
		// boundary to stop processing under that tree.
		boundaryPackages := make(map[string]struct{})
		err := filepath.WalkDir(
			filepath.Join(args.Dir, d),
			func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				// Ignore the path if it crosses any boundary package. Walking
				// the tree is still important because subsequent paths can
				// represent files that have not crossed any boundaries.
				for bp := range boundaryPackages {
					if strings.HasPrefix(path, bp) {
						return nil
					}
				}
				if entry.IsDir() {
					// If we are visiting a directory, we determine if we should
					// halt digging the tree based on a few criterias:
					//   1. We are using per-file generation.
					//   2. The directory has a BUILD or BUILD.bazel files. Then
					//       it doesn't matter at all what it has since it's a
					//       separate Bazel package.
					if cfg.PerFileGeneration() {
						return fs.SkipDir
					}

					if isBazelPackage(path) {
						boundaryPackages[path] = struct{}{}
						return nil
					}

					if !cfg.CoarseGrainedGeneration() {
						return fs.SkipDir
					}

					return nil
				}
				if filepath.Ext(path) == ".py" {
					if cfg.CoarseGrainedGeneration() || !isEntrypointFile(path) {
						srcPath, _ := filepath.Rel(args.Dir, path)
						repoPath := filepath.Join(args.Rel, srcPath)
						excludedPatterns := cfg.ExcludedPatterns()
						if excludedPatterns != nil {
							it := excludedPatterns.Iterator()
							for it.Next() {
								excludedPattern := it.Value().(string)
								isExcluded, err := doublestar.Match(excludedPattern, repoPath)
								if err != nil {
									return err
								}
								if isExcluded {
									return nil
								}
							}
						}
						baseName := filepath.Base(path)
						if matchesAnyGlob(baseName, testFileGlobs) {
							pyTestFilenames.Add(srcPath)
						} else {
							pyLibraryFilenames.Add(srcPath)
						}
					}
				}
				return nil
			},
		)
		if err != nil {
			log.Printf("ERROR: %v\n", err)
			return language.GenerateResult{}
		}
	}

	parser := newPython3Parser(args.Config.RepoRoot, args.Rel, cfg.IgnoresDependency)
	visibility := cfg.Visibility()

	var result language.GenerateResult
	result.Gen = make([]*rule.Rule, 0)

	if cfg.GenerateProto() {
		generateProtoLibraries(args, cfg, pythonProjectRoot, visibility, &result)
	}

	collisionErrors := singlylinkedlist.New()
	// Create a validFilesMap of mainModules to validate if python macros have valid srcs.
	validFilesMap := make(map[string]struct{})

	// Determine whether we have an __init__.py file in this package and whether we'll implicitly include it in srcs.
	var hasPopulatedInit bool
	var autoIncludeInit bool
	if cfg.PerFileGeneration() {
		var hasInit bool
		hasInit, hasPopulatedInit = hasLibraryEntrypointFile(args.Dir)
		autoIncludeInit = cfg.PerFileGenerationIncludeInit() && hasInit && hasPopulatedInit
	}

	// knownPySrcs is the set of source files Gazelle manages in this package, i.e.
	// the ones it would put in a generated target's srcs. It is narrower than "the
	// .py files that exist here": files hidden by python_ignore_files or
	// gazelle:exclude, and subdirectory files in per-package mode, are absent.
	// The entrypoints and conftest.py are diverted out of pyLibraryFilenames and
	// pyTestFilenames by the scan above, so they are added back explicitly.
	knownPySrcs := make(map[string]struct{})
	addSetValuesToMap(pyLibraryFilenames, knownPySrcs)
	addSetValuesToMap(pyTestFilenames, knownPySrcs)
	for _, src := range []struct {
		name    string
		present bool
	}{
		{pyBinaryEntrypointFilename, hasPyBinaryEntryPointFile},
		{pyTestEntrypointFilename, hasPyTestEntryPointFile},
		{conftestFilename, hasConftestFile},
	} {
		if src.present {
			knownPySrcs[src.name] = struct{}{}
		}
	}

	// generatedTargetNames holds per-file target names Gazelle generates in this
	// package. In file mode, an existing rule with one of those names is not a
	// hand-written target to adopt: adopting it would let it claim sources that
	// belong in other per-file targets, and the generated rule of the same name
	// would merge over it and drop them. Package- and project-level library and
	// test names are intentionally absent: those targets are regenerated in
	// place. Dedicated binary and conftest target names are always excluded.
	packageLibraryName := cfg.RenderLibraryName(packageName)
	existingPyLibraries := collectExistingPythonSourceRules(args, pyLibraryKind, knownPySrcs)
	if !cfg.PerFileGeneration() {
		existingPyLibraries = adoptExcludedInitOnlyPackageLibraryForSplitLayout(
			args,
			pyLibraryKind,
			packageLibraryName,
			knownPySrcs,
			existingPyLibraries,
		)
		existingPyLibraries = adoptEmptyAggregatePackageLibraryForSplitLayout(
			args,
			pyLibraryKind,
			packageLibraryName,
			existingPyLibraries,
		)
	}
	existingPyTests := collectExistingPythonSourceRules(args, pyTestKind, knownPySrcs)
	splitPackageLibraryLayout := false
	if !cfg.PerFileGeneration() {
		splitPackageLibraryLayout = hasSplitPackageLibraryLayout(packageLibraryName, existingPyLibraries)
		if !splitPackageLibraryLayout {
			splitPackageLibraryLayout = hasExplicitSourceOwnershipLayout(
				packageLibraryName,
				existingPyLibraries,
				pyLibraryFilenames,
			)
		}
	}

	generatedTargetNames := make(map[string]struct{})
	if cfg.PerFileGeneration() {
		addTargetNamesForSrcs(pyLibraryFilenames, generatedTargetNames)
		addTargetNamesForSrcs(pyTestFilenames, generatedTargetNames)
	} else if !splitPackageLibraryLayout {
		generatedTargetNames[packageLibraryName] = struct{}{}
		generatedTargetNames[cfg.RenderTestName(packageName)] = struct{}{}
	}
	if hasPyBinaryEntryPointFile {
		generatedTargetNames[cfg.RenderBinaryName(packageName)] = struct{}{}
	}
	if hasConftestFile {
		generatedTargetNames[conftestTargetname] = struct{}{}
	}
	isNotGeneratedTargetName := func(sourceRule existingPythonSourceRule) bool {
		_, isGenerated := generatedTargetNames[sourceRule.name]
		return !isGenerated
	}

	existingPyLibraries = filterExistingPythonSourceRules(
		existingPyLibraries,
		isNotGeneratedTargetName,
	)
	existingPyTests = filterExistingPythonSourceRules(existingPyTests, isNotGeneratedTargetName)
	hasPreservedPackageLibrary := splitPackageLibraryLayout
	// A library that owns a single source does not claim it: the source stays in
	// the generated target as well, which is what users of the long-standing
	// "extra target over one file" pattern expect. Coarse-grained generation has
	// a single library for the whole tree, so there claiming is unconditional or
	// the source would be owned twice. When a hand-written target already uses
	// the package library name alongside per-file libraries, every other
	// preserved library claims its sources so the generated package library does
	// not duplicate them. A py_test always claims, because a source pulled into
	// two test targets is executed twice.
	claimingPyLibraries := filterExistingPythonSourceRules(
		existingPyLibraries,
		func(sourceRule existingPythonSourceRule) bool {
			if sourceRule.declaredSrcCount > 1 || cfg.CoarseGrainedGeneration() {
				return true
			}
			if hasPreservedPackageLibrary && sourceRule.name != packageLibraryName {
				return true
			}
			return false
		},
	)
	removeClaimedSrcs(claimingPyLibraries, pyLibraryFilenames, pyTestFilenames)
	removeClaimedSrcs(existingPyTests, pyLibraryFilenames, pyTestFilenames)

	// extractedMainModules tracks the main modules that already have a generated
	// py_binary target. A source file can be owned by both a preserved target and
	// a generated one, in which case appendPyLibrary sees it twice and would
	// otherwise emit a duplicate py_binary for it.
	extractedMainModules := make(map[string]struct{})

	// autoIncludedInit reports whether the caller added pyLibraryEntrypointFilename
	// to srcs itself, rather than it being a source the user listed by hand. Only
	// in the former case may it be removed again when a main module is extracted.
	//
	// isPreserved reports whether the target is an existing hand-written one being
	// regenerated in place, as opposed to one Gazelle created.
	appendPyLibrary := func(
		srcs *treeset.Set,
		pyLibraryTargetName string,
		autoIncludedInit, isPreserved bool,
	) {
		allDeps, mainModules, annotations, err := parser.parse(srcs)
		if err != nil {
			log.Fatalf("ERROR: %v\n", err)
		}
		for name := range mainModules {
			validFilesMap[name] = struct{}{}
		}

		srcsChanged := false
		if !hasPyBinaryEntryPointFile {
			// Creating one py_binary target per main module when __main__.py doesn't exist.
			mainFileNames := make([]string, 0, len(mainModules))
			for name := range mainModules {
				// A py_binary named after the target it would be extracted from
				// cannot be generated: both rules would land in result.Gen under
				// the same name and merge into one.
				if isPreserved && strings.TrimSuffix(filepath.Base(name), ".py") == pyLibraryTargetName {
					continue
				}
				mainFileNames = append(mainFileNames, name)

				// Remove the file from srcs if we're doing per-file library generation so
				// that we don't also generate a py_library target for it.
				if cfg.PerFileGeneration() {
					srcs.Remove(name)
					srcsChanged = true
					// Also remove the __init__.py that was added earlier.
					if autoIncludedInit {
						srcs.Remove(pyLibraryEntrypointFilename)
					}
				}
			}

			sort.Strings(mainFileNames)
			for _, filename := range mainFileNames {
				if _, ok := extractedMainModules[filename]; ok {
					continue
				}
				extractedMainModules[filename] = struct{}{}

				pyBinaryTargetName := strings.TrimSuffix(filepath.Base(filename), ".py")
				if err := ensureNoCollision(args.Config, args.File, pyBinaryTargetName, pyBinaryKind); err != nil {
					fqTarget := label.New("", args.Rel, pyBinaryTargetName)
					log.Printf("failed to generate target %q of kind %q: %v",
						fqTarget.String(), getMappedKind(args.Config, pyBinaryKind), err)
					continue
				}

				// Add any sibling .pyi files to pyi_srcs
				filenames := treeset.NewWith(godsutils.StringComparator, filename)
				pyiSrcs, _ := getPyiFilenames(filenames, cfg.GeneratePyiSrcs(), args.Dir)

				pyBinaryBuilder := newTargetBuilder(pyBinaryKind, pyBinaryTargetName, pythonProjectRoot, args.Rel, pyFileNames, cfg.ResolveSiblingImports()).
					addVisibility(visibility).
					addSrc(filename).
					addPyiSrcs(pyiSrcs).
					addModuleDependencies(mainModules[filename]).
					addResolvedDependencies(annotations.includeDeps).
					generateImportsAttribute().
					setAnnotations(*annotations)

				if autoIncludeInit {
					pyBinaryBuilder.addSrc(pyLibraryEntrypointFilename)
				}

				pyBinary := pyBinaryBuilder.build()
				result.Gen = append(result.Gen, pyBinary)
				result.Imports = append(result.Imports, pyBinary.PrivateAttr(config.GazelleImportsKey))
			}
		}

		// If we're doing per-file generation, srcs could be empty at this point, meaning we shouldn't make a py_library.
		// If there is already a package named py_library target before, we should generate an empty py_library.
		if srcs.Empty() {
			// Leave a preserved target exactly as it was written instead. Falling
			// through would build an empty rule, which Gazelle reports as removable
			// and so deletes a hand-written target.
			if isPreserved {
				return
			}
			if args.File == nil {
				return
			}
			generateEmptyLibrary := false
			for _, r := range args.File.Rules {
				if r.Name() == pyLibraryTargetName && kindMatches(args.Config, r, pyLibraryKind) {
					generateEmptyLibrary = true
				}
			}
			if !generateEmptyLibrary {
				return
			}
		}

		if srcsChanged {
			// The dependencies above were derived from the srcs the target had
			// before the main modules were extracted. Recompute them so the target
			// does not keep dependencies contributed by a source it no longer owns.
			allDeps, _, annotations, err = parser.parse(srcs)
			if err != nil {
				log.Fatalf("ERROR: %v\n", err)
			}
		}

		// Add any sibling .pyi files to pyi_srcs
		pyiSrcs, _ := getPyiFilenames(srcs, cfg.GeneratePyiSrcs(), args.Dir)

		// Check if a target with the same name we are generating already
		// exists, and if it is of a different kind from the one we are
		// generating. If so, we have to throw an error since Gazelle won't
		// generate it correctly.
		if err := ensureNoCollision(args.Config, args.File, pyLibraryTargetName, pyLibraryKind); err != nil {
			fqTarget := label.New("", args.Rel, pyLibraryTargetName)
			err := fmt.Errorf("failed to generate target %q of kind %q: %w. "+
				"Use the '# gazelle:%s' directive to change the naming convention.",
				fqTarget.String(), getMappedKind(args.Config, pyLibraryKind), err, pythonconfig.LibraryNamingConvention)
			collisionErrors.Add(err)
		}

		pyLibraryBuilder := newTargetBuilder(
			pyLibraryKind,
			pyLibraryTargetName,
			pythonProjectRoot,
			args.Rel,
			pyFileNames,
			cfg.ResolveSiblingImports(),
		).
			addSrcs(srcs).
			addPyiSrcs(pyiSrcs).
			addModuleDependencies(allDeps).
			addResolvedDependencies(annotations.includeDeps).
			generateImportsAttribute().
			setAnnotations(*annotations)

		// visibility is not a mergeable attribute, so rule.MergeRules copies it
		// into an existing rule that does not set one and offers no '# keep' to
		// prevent that. Injecting it would silently widen a hand-written target
		// that relies on Bazel's default private visibility.
		if !isPreserved {
			pyLibraryBuilder.addVisibility(visibility)
		}

		pyLibrary := pyLibraryBuilder.build()

		if pyLibrary.IsEmpty(py.Kinds()[pyLibrary.Kind()]) {
			result.Empty = append(result.Empty, pyLibrary)
		} else {
			result.Gen = append(result.Gen, pyLibrary)
			result.Imports = append(result.Imports, pyLibrary.PrivateAttr(config.GazelleImportsKey))
		}
	}

	for _, existingPyLibrary := range existingPyLibraries {
		if existingPyLibrary.name == packageLibraryName &&
			existingPyLibrary.declaredSrcCount == 0 &&
			existingPyLibrary.srcs.Empty() {
			continue
		}
		srcs := existingPyLibrary.srcs
		if existingPyLibrary.name == packageLibraryName &&
			existingPyLibrary.declaredSrcCount > 0 &&
			!cfg.PerFileGeneration() {
			mergedSrcs := treeset.NewWith(godsutils.StringComparator)
			srcs.Each(func(index int, filename interface{}) {
				mergedSrcs.Add(filename)
			})
			pyLibraryFilenames.Each(func(index int, filename interface{}) {
				mergedSrcs.Add(filename)
			})
			srcs = mergedSrcs
		}
		appendPyLibrary(srcs, existingPyLibrary.name, false, true)
	}

	if cfg.PerFileGeneration() {
		pyLibraryFilenames.Each(func(index int, filename interface{}) {
			pyLibraryTargetName := strings.TrimSuffix(filepath.Base(filename.(string)), ".py")
			if filename == pyLibraryEntrypointFilename && !hasPopulatedInit {
				return // ignore empty __init__.py.
			}
			srcs := treeset.NewWith(godsutils.StringComparator, filename)
			if autoIncludeInit {
				srcs.Add(pyLibraryEntrypointFilename)
			}
			appendPyLibrary(srcs, pyLibraryTargetName, autoIncludeInit, false)
		})
	} else if !hasPreservedPackageLibrary {
		appendPyLibrary(pyLibraryFilenames, packageLibraryName, false, false)
	}

	if hasPyBinaryEntryPointFile {
		deps, _, annotations, err := parser.parseSingle(pyBinaryEntrypointFilename)
		if err != nil {
			log.Fatalf("ERROR: %v\n", err)
		}

		pyBinaryTargetName := cfg.RenderBinaryName(packageName)

		// Check if a target with the same name we are generating already
		// exists, and if it is of a different kind from the one we are
		// generating. If so, we have to throw an error since Gazelle won't
		// generate it correctly.
		if err := ensureNoCollision(args.Config, args.File, pyBinaryTargetName, pyBinaryKind); err != nil {
			fqTarget := label.New("", args.Rel, pyBinaryTargetName)
			err := fmt.Errorf("failed to generate target %q of kind %q: %w. "+
				"Use the '# gazelle:%s' directive to change the naming convention.",
				fqTarget.String(), getMappedKind(args.Config, pyBinaryKind), err, pythonconfig.BinaryNamingConvention)
			collisionErrors.Add(err)
		}

		// Add any sibling .pyi files to pyi_srcs
		filenames := treeset.NewWith(godsutils.StringComparator, pyBinaryEntrypointFilename)
		pyiSrcs, _ := getPyiFilenames(filenames, cfg.GeneratePyiSrcs(), args.Dir)

		pyBinaryTarget := newTargetBuilder(pyBinaryKind, pyBinaryTargetName, pythonProjectRoot, args.Rel, pyFileNames, cfg.ResolveSiblingImports()).
			setMain(pyBinaryEntrypointFilename).
			addVisibility(visibility).
			addSrc(pyBinaryEntrypointFilename).
			addPyiSrcs(pyiSrcs).
			addModuleDependencies(deps).
			addResolvedDependencies(annotations.includeDeps).
			setAnnotations(*annotations).
			generateImportsAttribute()

		pyBinary := pyBinaryTarget.build()

		result.Gen = append(result.Gen, pyBinary)
		result.Imports = append(result.Imports, pyBinary.PrivateAttr(config.GazelleImportsKey))
	}

	var conftest *rule.Rule
	if hasConftestFile {
		deps, _, annotations, err := parser.parseSingle(conftestFilename)
		if err != nil {
			log.Fatalf("ERROR: %v\n", err)
		}

		// Check if a target with the same name we are generating already
		// exists, and if it is of a different kind from the one we are
		// generating. If so, we have to throw an error since Gazelle won't
		// generate it correctly.
		if err := ensureNoCollision(args.Config, args.File, conftestTargetname, pyLibraryKind); err != nil {
			fqTarget := label.New("", args.Rel, conftestTargetname)
			err := fmt.Errorf("failed to generate target %q of kind %q: %w. ",
				fqTarget.String(), getMappedKind(args.Config, pyLibraryKind), err)
			collisionErrors.Add(err)
		}

		// Add any sibling .pyi files to pyi_srcs
		filenames := treeset.NewWith(godsutils.StringComparator, conftestFilename)
		pyiSrcs, _ := getPyiFilenames(filenames, cfg.GeneratePyiSrcs(), args.Dir)

		conftestTarget := newTargetBuilder(pyLibraryKind, conftestTargetname, pythonProjectRoot, args.Rel, pyFileNames, cfg.ResolveSiblingImports()).
			addSrc(conftestFilename).
			addPyiSrcs(pyiSrcs).
			addModuleDependencies(deps).
			addResolvedDependencies(annotations.includeDeps).
			setAnnotations(*annotations).
			addVisibility(visibility).
			setTestonly().
			generateImportsAttribute()

		conftest = conftestTarget.build()

		result.Gen = append(result.Gen, conftest)
		result.Imports = append(result.Imports, conftest.PrivateAttr(config.GazelleImportsKey))
	}

	var pyTestTargets []*targetBuilder
	newPyTestTargetBuilder := func(srcs *treeset.Set, pyTestTargetName string) *targetBuilder {
		deps, _, annotations, err := parser.parse(srcs)
		if err != nil {
			log.Fatalf("ERROR: %v\n", err)
		}
		// Check if a target with the same name we are generating already
		// exists, and if it is of a different kind from the one we are
		// generating. If so, we have to throw an error since Gazelle won't
		// generate it correctly.
		if err := ensureNoCollision(args.Config, args.File, pyTestTargetName, pyTestKind); err != nil {
			fqTarget := label.New("", args.Rel, pyTestTargetName)
			err := fmt.Errorf("failed to generate target %q of kind %q: %w. "+
				"Use the '# gazelle:%s' directive to change the naming convention.",
				fqTarget.String(), getMappedKind(args.Config, pyTestKind), err, pythonconfig.TestNamingConvention)
			collisionErrors.Add(err)
		}

		// Add any sibling .pyi files to pyi_srcs
		pyiSrcs, _ := getPyiFilenames(srcs, cfg.GeneratePyiSrcs(), args.Dir)

		return newTargetBuilder(pyTestKind, pyTestTargetName, pythonProjectRoot, args.Rel, pyFileNames, cfg.ResolveSiblingImports()).
			addSrcs(srcs).
			addPyiSrcs(pyiSrcs).
			addModuleDependencies(deps).
			addResolvedDependencies(annotations.includeDeps).
			setAnnotations(*annotations).
			generateImportsAttribute()
	}

	for _, existingPyTest := range existingPyTests {
		pyTestTargets = append(pyTestTargets, newPyTestTargetBuilder(existingPyTest.srcs, existingPyTest.name))
	}

	if (!cfg.PerPackageGenerationRequireTestEntryPoint() || hasPyTestEntryPointFile || hasPyTestEntryPointTarget || cfg.CoarseGrainedGeneration()) && !cfg.PerFileGeneration() {
		// Create one py_test target per package
		if hasPyTestEntryPointFile {
			// Only add the pyTestEntrypointFilename to the pyTestFilenames if
			// the file exists on disk.
			pyTestFilenames.Add(pyTestEntrypointFilename)
		}
		if hasPyTestEntryPointTarget || !pyTestFilenames.Empty() {
			pyTestTargetName := cfg.RenderTestName(packageName)
			pyTestTarget := newPyTestTargetBuilder(pyTestFilenames, pyTestTargetName)

			if hasPyTestEntryPointTarget {
				entrypointTarget := fmt.Sprintf(":%s", pyTestEntrypointTargetname)
				main := fmt.Sprintf(":%s", pyTestEntrypointFilename)
				pyTestTarget.
					addSrc(entrypointTarget).
					addResolvedDependency(entrypointTarget).
					setMain(main)
			} else if hasPyTestEntryPointFile {
				pyTestTarget.setMain(pyTestEntrypointFilename)
			} /* else:
			main is not set, assuming there is a test file with the same name
			as the target name, or there is a macro wrapping py_test and setting its main attribute.
			*/
			pyTestTargets = append(pyTestTargets, pyTestTarget)
		}
	} else {
		// Create one py_test target per file
		pyTestFilenames.Each(func(index int, testFile interface{}) {
			srcs := treeset.NewWith(godsutils.StringComparator, testFile)
			pyTestTargetName := strings.TrimSuffix(filepath.Base(testFile.(string)), ".py")
			pyTestTarget := newPyTestTargetBuilder(srcs, pyTestTargetName)

			if hasPyTestEntryPointTarget {
				entrypointTarget := fmt.Sprintf(":%s", pyTestEntrypointTargetname)
				main := fmt.Sprintf(":%s", pyTestEntrypointFilename)
				pyTestTarget.
					addSrc(entrypointTarget).
					addResolvedDependency(entrypointTarget).
					setMain(main)
			} else if hasPyTestEntryPointFile {
				pyTestTarget.addSrc(pyTestEntrypointFilename)
				pyTestTarget.setMain(pyTestEntrypointFilename)
			}
			pyTestTargets = append(pyTestTargets, pyTestTarget)
		})
	}

	for _, pyTestTarget := range pyTestTargets {
		shouldAddConftest := pyTestTarget.annotations.includePytestConftest == nil ||
			*pyTestTarget.annotations.includePytestConftest

		if shouldAddConftest {
			for _, conftestPkg := range findConftestPaths(args.Config.RepoRoot, args.Rel, pythonProjectRoot, cfg.IncludeAncestorConftest()) {
				pyTestTarget.addModuleDependency(
					Module{
						Name:     importSpecFromSrc(pythonProjectRoot, conftestPkg, conftestFilename).Imp,
						Filepath: filepath.Join(conftestPkg, conftestFilename),
					},
				)
			}
		}
		pyTest := pyTestTarget.build()

		result.Gen = append(result.Gen, pyTest)
		result.Imports = append(result.Imports, pyTest.PrivateAttr(config.GazelleImportsKey))
	}
	emptyRules := py.getRulesWithInvalidSrcs(args, validFilesMap)
	result.Empty = append(result.Empty, emptyRules...)
	if !collisionErrors.Empty() {
		it := collisionErrors.Iterator()
		for it.Next() {
			log.Printf("ERROR: %v\n", it.Value())
		}
		os.Exit(1)
	}

	return result
}

// ruleListsGazelleManagedSrc reports whether any src is one Gazelle would place
// in a generated target for this package.
func ruleListsGazelleManagedSrc(srcs []string, managed map[string]struct{}) bool {
	for _, src := range srcs {
		if isTargetSrc(src) || filepath.Ext(src) != ".py" {
			continue
		}
		if _, ok := managed[src]; ok {
			return true
		}
	}
	return false
}

// isExcludedInitOnlyPackageLibrarySrcOnDisk reports whether src is the only
// source of the hand-written package library and exists on disk but is hidden
// from Gazelle generation (for example via gazelle:exclude).
func isExcludedInitOnlyPackageLibrarySrcOnDisk(
	args language.GenerateArgs,
	packageLibraryName string,
	existingRule *rule.Rule,
	src string,
) bool {
	if !kindMatches(args.Config, existingRule, pyLibraryKind) {
		return false
	}
	if existingRule.Name() != packageLibraryName || src != pyLibraryEntrypointFilename {
		return false
	}
	srcs := existingRule.AttrStrings("srcs")
	if len(srcs) != 1 {
		return false
	}
	_, err := os.Stat(filepath.Join(args.Dir, src))
	return err == nil
}

// getRulesWithInvalidSrcs checks existing Python rules in the BUILD file and return the rules with invalid source files.
// Invalid source files are files that do not exist or not a target.
func (py *Python) getRulesWithInvalidSrcs(args language.GenerateArgs, validFilesMap map[string]struct{}) (invalidRules []*rule.Rule) {
	if args.File == nil {
		return
	}
	packageLibraryName := filepath.Base(args.Dir)
	if args.Config != nil {
		if raw, ok := args.Config.Exts[languageName]; ok && raw != nil {
			cfg := raw.(pythonconfig.Configs)[args.Rel]
			if cfg != nil {
				packageLibraryName = cfg.RenderLibraryName(packageLibraryName)
			}
		}
	}

	for _, file := range args.GenFiles {
		validFilesMap[file] = struct{}{}
	}

	// allFilesMap extends validFilesMap with all regular files on disk.
	// py_binary uses validFilesMap (main modules + generated files), while py_library
	// and py_test use allFilesMap since any file is a valid src for them.
	allFilesMap := make(map[string]struct{}, len(validFilesMap)+len(args.RegularFiles))
	for file := range validFilesMap {
		allFilesMap[file] = struct{}{}
	}
	for _, file := range args.RegularFiles {
		allFilesMap[file] = struct{}{}
	}
	for _, existingRule := range args.File.Rules {
		var matchedKind string
		var filesMap map[string]struct{}
		if kindMatches(args.Config, existingRule, pyBinaryKind) {
			matchedKind = pyBinaryKind
			filesMap = validFilesMap
		} else if kindMatches(args.Config, existingRule, pyLibraryKind) {
			matchedKind = pyLibraryKind
			filesMap = allFilesMap
		} else if kindMatches(args.Config, existingRule, pyTestKind) {
			matchedKind = pyTestKind
			filesMap = allFilesMap
		} else {
			continue
		}

		srcs := existingRule.AttrStrings("srcs")
		if len(srcs) == 0 {
			continue
		}
		var hasValidSrcs bool
		for _, src := range srcs {
			if isTargetSrc(src) {
				hasValidSrcs = true
				break
			}
			if _, ok := filesMap[src]; ok {
				hasValidSrcs = true
				break
			}
			if isExcludedInitOnlyPackageLibrarySrcOnDisk(
				args,
				packageLibraryName,
				existingRule,
				src,
			) {
				hasValidSrcs = true
				break
			}
		}
		if !hasValidSrcs && matchedKind != pyBinaryKind &&
			!ruleListsGazelleManagedSrc(srcs, validFilesMap) {
			for _, src := range srcs {
				if isTargetSrc(src) {
					hasValidSrcs = true
					break
				}
				if _, err := os.Stat(filepath.Join(args.Dir, src)); err == nil {
					hasValidSrcs = true
					break
				}
			}
		}
		if !hasValidSrcs {
			invalidRules = append(invalidRules, newTargetBuilder(matchedKind, existingRule.Name(), "", "", nil, false).build())
		}
	}
	return invalidRules
}

// isBazelPackage determines if the directory is a Bazel package by probing for
// the existence of a known BUILD file name.
func isBazelPackage(dir string) bool {
	for _, buildFilename := range buildFilenames {
		path := filepath.Join(dir, buildFilename)
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

// hasEntrypointFile determines if the directory has any of the established
// entrypoint filenames.
func hasEntrypointFile(dir string) bool {
	for _, entrypointFilename := range []string{
		pyLibraryEntrypointFilename,
		pyBinaryEntrypointFilename,
		pyTestEntrypointFilename,
	} {
		path := filepath.Join(dir, entrypointFilename)
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

// hasLibraryEntrypointFile returns if the given directory has the library
// entrypoint file, and if it is non-empty.
func hasLibraryEntrypointFile(dir string) (bool, bool) {
	stat, err := os.Stat(filepath.Join(dir, pyLibraryEntrypointFilename))
	if os.IsNotExist(err) {
		return false, false
	}
	if err != nil {
		log.Fatalf("ERROR: %v\n", err)
	}
	return true, stat.Size() != 0
}

// isEntrypointFile returns whether the given path is an entrypoint file. The
// given path can be absolute or relative.
func isEntrypointFile(path string) bool {
	basePath := filepath.Base(path)
	switch basePath {
	case pyLibraryEntrypointFilename,
		pyBinaryEntrypointFilename,
		pyTestEntrypointFilename:
		return true
	default:
		return false
	}
}

func ensureNoCollision(c *config.Config, file *rule.File, targetName, kind string) error {
	if file == nil {
		return nil
	}
	for _, t := range file.Rules {
		if t.Name() == targetName && !kindMatches(c, t, kind) {
			return fmt.Errorf("a target of kind %q with the same name already exists", t.Kind())
		}
	}
	return nil
}

func generateProtoLibraries(args language.GenerateArgs, cfg *pythonconfig.Config, pythonProjectRoot string, visibility []string, res *language.GenerateResult) {
	// First, enumerate all the proto_library in this package.
	var protoRuleNames []string
	for _, r := range args.OtherGen {
		if r.Kind() != "proto_library" {
			continue
		}
		protoRuleNames = append(protoRuleNames, r.Name())
	}
	sort.Strings(protoRuleNames)

	// Next, enumerate all the pre-existing py_proto_library in this package, so we can delete unnecessary rules later.
	pyProtoRules := map[string]bool{}
	pyProtoRulesForProto := map[string]string{}
	if args.File != nil {
		for _, r := range args.File.Rules {
			if kindMatches(args.Config, r, pyProtoLibraryKind) {
				pyProtoRules[r.Name()] = false

				protos := r.AttrStrings("deps")
				for _, proto := range protos {
					pyProtoRulesForProto[strings.TrimPrefix(proto, ":")] = r.Name()
				}
			}
		}
	}

	emptySiblings := treeset.Set{}
	// Generate a py_proto_library for each proto_library.
	for _, protoRuleName := range protoRuleNames {
		pyProtoLibraryName := cfg.RenderProtoName(protoRuleName)
		if ruleName, ok := pyProtoRulesForProto[protoRuleName]; ok {
			// There exists a pre-existing py_proto_library for this proto. Keep this name.
			pyProtoLibraryName = ruleName
		}

		pyProtoLibrary := newTargetBuilder(pyProtoLibraryKind, pyProtoLibraryName, pythonProjectRoot, args.Rel, &emptySiblings, false).
			addVisibility(visibility).
			addResolvedDependency(":" + protoRuleName).
			generateImportsAttribute().build()

		res.Gen = append(res.Gen, pyProtoLibrary)
		res.Imports = append(res.Imports, pyProtoLibrary.PrivateAttr(config.GazelleImportsKey))
		pyProtoRules[pyProtoLibrary.Name()] = true

	}

	// Finally, emit an empty rule for each pre-existing py_proto_library that we didn't already generate.
	for ruleName, generated := range pyProtoRules {
		if generated {
			continue
		}

		emptyRule := newTargetBuilder(pyProtoLibraryKind, ruleName, pythonProjectRoot, args.Rel, &emptySiblings, false).build()
		res.Empty = append(res.Empty, emptyRule)
	}

}

// getPyiFilenames returns a set of existing .pyi source file names for a given set of source
// file names if GeneratePyiSrcs is set. Otherwise, returns an empty set.
func getPyiFilenames(filenames *treeset.Set, generatePyiSrcs bool, basePath string) (*treeset.Set, error) {
	pyiSrcs := treeset.NewWith(godsutils.StringComparator)
	if !generatePyiSrcs {
		return pyiSrcs, nil
	}

	it := filenames.Iterator()
	for it.Next() {
		pyiFilename := it.Value().(string) + "i" // foo.py --> foo.pyi

		_, err := os.Stat(filepath.Join(basePath, pyiFilename))
		// If the file DNE or there's some other error, there's nothing to do.
		if err == nil {
			// pyi file exists, add it
			pyiSrcs.Add(pyiFilename)
		}
	}
	return pyiSrcs, nil
}
