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
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/repo"
	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
	bzl "github.com/bazelbuild/buildtools/build"
	"github.com/emirpasic/gods/sets/treeset"
	godsutils "github.com/emirpasic/gods/utils"

	"github.com/bazel-contrib/rules_python/gazelle/pythonconfig"
)

const languageName = "py"

const (
	// resolvedDepsKey is the attribute key used to pass dependencies that don't
	// need to be resolved by the dependency resolver in the Resolver step.
	resolvedDepsKey = "_gazelle_python_resolved_deps"
	// depsToRemoveKey is the attribute key used to store the deps_to_remove list
	depsToRemoveKey = "_gazelle_python_deps_to_remove"
	// srcsForOrderingKey is the attribute key used to store source files for ordering constraints
	srcsForOrderingKey = "_gazelle_python_srcs_for_ordering"
	// depsOrderFilename is the name of the file that contains the dependency order
	depsOrderFilename = "deps-order.txt"
)

// DepsOrderResolver holds the dependency order information parsed from deps-order.txt
type DepsOrderResolver struct {
	fileToIndex    map[string]int
	loaded         bool
	// importToSrcs maps import names to their source files (pkg-relative paths)
	importToSrcs   map[string][]string
}

// NewDepsOrderResolver creates a new DepsOrderResolver
func NewDepsOrderResolver() *DepsOrderResolver {
	return &DepsOrderResolver{
		fileToIndex:  make(map[string]int),
		loaded:       false,
		importToSrcs: make(map[string][]string),
	}
}

// LoadDepsOrder loads the deps-order.txt file from the repository root
func (d *DepsOrderResolver) LoadDepsOrder(repoRoot string) error {
	if d.loaded {
		return nil
	}

	depsOrderPath := filepath.Join(repoRoot, depsOrderFilename)
	file, err := os.Open(depsOrderPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, which is fine - we just won't use deps ordering
			d.loaded = true
			return nil
		}
		return fmt.Errorf("failed to open %s: %v", depsOrderFilename, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	index := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}
		d.fileToIndex[line] = index
		index++
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read %s: %v", depsOrderFilename, err)
	}

	d.loaded = true
	return nil
}

// GetAverageIndex calculates the average index for a set of source files
func (d *DepsOrderResolver) GetAverageIndex(srcs []string) float64 {
	if len(d.fileToIndex) == 0 {
		return 0 // No ordering file, return 0
	}

	totalIndex := 0
	validSrcs := 0
	for _, src := range srcs {
		// Try both the full path and just the filename
		filename := filepath.Base(src)
		if index, exists := d.fileToIndex[src]; exists {
			totalIndex += index
			validSrcs++
		} else if index, exists := d.fileToIndex[filename]; exists {
			totalIndex += index
			validSrcs++
		}
	}

	if validSrcs == 0 {
		return float64(len(d.fileToIndex)) // Files not in order get max index
	}

	return float64(totalIndex) / float64(validSrcs)
}

// ShouldAddToDepsToRemove returns true if the dependency should be added to deps_to_remove based on ordering constraints
func (d *DepsOrderResolver) ShouldAddToDepsToRemove(currentTargetSrcs []string, depTargetSrcs []string) bool {
	if len(d.fileToIndex) == 0 {
		return false // No ordering constraints
	}

	currentAvg := d.GetAverageIndex(currentTargetSrcs)
	depAvg := d.GetAverageIndex(depTargetSrcs)

	// If current target has lower average index than dependency, the dependency should be removed
	return currentAvg < depAvg
}

// RegisterImportSources registers the mapping between import specs and their source files
func (d *DepsOrderResolver) RegisterImportSources(importSpecs []resolve.ImportSpec, pkgPath string, srcs []string) {
	// Convert sources to repo-relative paths
	repoRelativeSrcs := make([]string, 0, len(srcs))
	for _, src := range srcs {
		repoRelativeSrcs = append(repoRelativeSrcs, filepath.Join(pkgPath, src))
	}
	
	// Register each import spec
	for _, spec := range importSpecs {
		d.importToSrcs[spec.Imp] = repoRelativeSrcs
	}
}

// getSourcesForImport gets the source files for a given import name using the registered mappings
func (d *DepsOrderResolver) getSourcesForImport(importName string) []string {
	if srcs, ok := d.importToSrcs[importName]; ok {
		return srcs
	}
	return []string{}
}

// Resolver satisfies the resolve.Resolver interface. It resolves dependencies
// in rules generated by this extension.
type Resolver struct{
	depsOrderResolver *DepsOrderResolver
}

// Name returns the name of the language. This is the prefix of the kinds of
// rules generated. E.g. py_library and py_binary.
func (*Resolver) Name() string { return languageName }

// Imports returns a list of ImportSpecs that can be used to import the rule
// r. This is used to populate RuleIndex.
//
// If nil is returned, the rule will not be indexed. If any non-nil slice is
// returned, including an empty slice, the rule will be indexed.
func (py *Resolver) Imports(c *config.Config, r *rule.Rule, f *rule.File) []resolve.ImportSpec {
	cfgs := c.Exts[languageName].(pythonconfig.Configs)
	cfg := cfgs[f.Pkg]
	srcs := r.AttrStrings("srcs")
	provides := make([]resolve.ImportSpec, 0, len(srcs)+1)
	for _, src := range srcs {
		ext := filepath.Ext(src)
		if ext != ".py" {
			continue
		}
		if cfg.PerFileGeneration() && len(srcs) > 1 && src == pyLibraryEntrypointFilename {
			// Do not provide import spec from __init__.py when it is being included as
			// part of another module.
			continue
		}
		pythonProjectRoot := cfg.PythonProjectRoot()
		provide := importSpecFromSrc(pythonProjectRoot, f.Pkg, src)
		provides = append(provides, provide)
	}
	if len(provides) == 0 {
		return nil
	}
	
	// Register the import-to-source mappings for dependency ordering
	py.depsOrderResolver.RegisterImportSources(provides, f.Pkg, srcs)
	
	return provides
}

// importSpecFromSrc determines the ImportSpec based on the target that contains the src so that
// the target can be indexed for import statements that match the calculated src relative to the its
// Python project root.
func importSpecFromSrc(pythonProjectRoot, bzlPkg, src string) resolve.ImportSpec {
	pythonPkgDir := filepath.Join(bzlPkg, filepath.Dir(src))
	relPythonPkgDir, err := filepath.Rel(pythonProjectRoot, pythonPkgDir)
	if err != nil {
		panic(fmt.Errorf("unexpected failure: %v", err))
	}
	if relPythonPkgDir == "." {
		relPythonPkgDir = ""
	}
	pythonPkg := strings.ReplaceAll(relPythonPkgDir, "/", ".")
	filename := filepath.Base(src)
	if filename == pyLibraryEntrypointFilename {
		if pythonPkg != "" {
			return resolve.ImportSpec{
				Lang: languageName,
				Imp:  pythonPkg,
			}
		}
	}
	moduleName := strings.TrimSuffix(filename, ".py")
	var imp string
	if pythonPkg == "" {
		imp = moduleName
	} else {
		imp = fmt.Sprintf("%s.%s", pythonPkg, moduleName)
	}
	return resolve.ImportSpec{
		Lang: languageName,
		Imp:  imp,
	}
}

// Embeds returns a list of labels of rules that the given rule embeds. If
// a rule is embedded by another importable rule of the same language, only
// the embedding rule will be indexed. The embedding rule will inherit
// the imports of the embedded rule.
func (py *Resolver) Embeds(r *rule.Rule, from label.Label) []label.Label {
	// TODO(f0rmiga): implement.
	return make([]label.Label, 0)
}

// addDependency adds a dependency to either the regular deps or pyiDeps set based on
// whether the module is type-checking only. If a module is added as both
// non-type-checking and type-checking, it should end up in deps and not pyiDeps.
func addDependency(dep string, typeCheckingOnly bool, deps, pyiDeps *treeset.Set) {
	if typeCheckingOnly {
		if !deps.Contains(dep) {
			pyiDeps.Add(dep)
		}
	} else {
		deps.Add(dep)
		pyiDeps.Remove(dep)
	}
}

// Resolve translates imported libraries for a given rule into Bazel
// dependencies. Information about imported libraries is returned for each
// rule generated by language.GenerateRules in
// language.GenerateResult.Imports. Resolve generates a "deps" attribute (or
// the appropriate language-specific equivalent) for each import according to
// language-specific rules and heuristics.
func (py *Resolver) Resolve(
	c *config.Config,
	ix *resolve.RuleIndex,
	rc *repo.RemoteCache,
	r *rule.Rule,
	modulesRaw interface{},
	from label.Label,
) {
	// TODO(f0rmiga): may need to be defensive here once this Gazelle extension
	// join with the main Gazelle binary with other rules. It may conflict with
	// other generators that generate py_* targets.
	deps := treeset.NewWith(godsutils.StringComparator)
	pyiDeps := treeset.NewWith(godsutils.StringComparator)
	cfgs := c.Exts[languageName].(pythonconfig.Configs)
	cfg := cfgs[from.Pkg]

	if modulesRaw != nil {
		pythonProjectRoot := cfg.PythonProjectRoot()
		modules := modulesRaw.(*treeset.Set)
		it := modules.Iterator()
		explainDependency := os.Getenv("EXPLAIN_DEPENDENCY")
		// Resolve relative paths for package generation
		isPackageGeneration := !cfg.PerFileGeneration() && !cfg.CoarseGrainedGeneration()
		hasFatalError := false
	MODULES_LOOP:
		for it.Next() {
			mod := it.Value().(Module)
			moduleName := mod.Name
			// Transform relative imports `.` or `..foo.bar` into the package path from root.
			if strings.HasPrefix(mod.From, ".") {
				if !cfg.ExperimentalAllowRelativeImports() || !isPackageGeneration {
					continue MODULES_LOOP
				}

				// Count number of leading dots in mod.From (e.g., ".." = 2, "...foo.bar" = 3)
				relativeDepth := strings.IndexFunc(mod.From, func(r rune) bool { return r != '.' })
				if relativeDepth == -1 {
					relativeDepth = len(mod.From)
				}

				// Extract final symbol (e.g., "some_function") from mod.Name
				imported := mod.Name
				if idx := strings.LastIndex(mod.Name, "."); idx >= 0 {
					imported = mod.Name[idx+1:]
				}

				// Optional subpath in 'from' clause, e.g. "from ...my_library.foo import x"
				fromPath := strings.TrimLeft(mod.From, ".")
				var fromParts []string
				if fromPath != "" {
					fromParts = strings.Split(fromPath, ".")
				}

				// Current Bazel package as path segments
				pkgParts := strings.Split(from.Pkg, "/")

				if relativeDepth-1 > len(pkgParts) {
					log.Printf("ERROR: Invalid relative import %q in %q: exceeds package root.", mod.Name, mod.Filepath)
					continue MODULES_LOOP
				}

				// Go up relativeDepth - 1 levels
				baseParts := pkgParts
				if relativeDepth > 1 {
					baseParts = pkgParts[:len(pkgParts)-(relativeDepth-1)]
				}
				// Build absolute module path
				absParts := append([]string{}, baseParts...)       // base path
				absParts = append(absParts, fromParts...)          // subpath from 'from'
				absParts = append(absParts, imported)              // actual imported symbol

				moduleName = strings.Join(absParts, ".")
			}

			moduleParts := strings.Split(moduleName, ".")
			possibleModules := []string{moduleName}
			for len(moduleParts) > 1 {
				// Iterate back through the possible imports until
				// a match is found.
				// For example, "from foo.bar import baz" where baz is a module, we should try `foo.bar.baz` first, then
				// `foo.bar`, then `foo`.
				// In the first case, the import could be file `baz.py` in the directory `foo/bar`.
				// Or, the import could be variable `baz` in file `foo/bar.py`.
				// The import could also be from a standard module, e.g. `six.moves`, where
				// the dependency is actually `six`.
				moduleParts = moduleParts[:len(moduleParts)-1]
				possibleModules = append(possibleModules, strings.Join(moduleParts, "."))
			}
			errs := []error{}
		POSSIBLE_MODULE_LOOP:
			for _, moduleName := range possibleModules {
				imp := resolve.ImportSpec{Lang: languageName, Imp: moduleName}
				if override, ok := resolve.FindRuleWithOverride(c, imp, languageName); ok {
					if override.Repo == "" {
						override.Repo = from.Repo
					}
					if !override.Equal(from) {
						if override.Repo == from.Repo {
							override.Repo = ""
						}
						dep := override.Rel(from.Repo, from.Pkg).String()
						addDependency(dep, mod.TypeCheckingOnly, deps, pyiDeps)
						if explainDependency == dep {
							log.Printf("Explaining dependency (%s): "+
								"in the target %q, the file %q imports %q at line %d, "+
								"which resolves using the \"gazelle:resolve\" directive.\n",
								explainDependency, from.String(), mod.Filepath, moduleName, mod.LineNumber)
						}
						continue MODULES_LOOP
					}
				} else {
					if dep, distributionName, ok := cfg.FindThirdPartyDependency(moduleName); ok {
						addDependency(dep, mod.TypeCheckingOnly, deps, pyiDeps)
						// Add the type and stub dependencies if they exist.
						modules := []string{
							fmt.Sprintf("%s_stubs", strings.ToLower(distributionName)),
							fmt.Sprintf("%s_types", strings.ToLower(distributionName)),
							fmt.Sprintf("types_%s", strings.ToLower(distributionName)),
							fmt.Sprintf("stubs_%s", strings.ToLower(distributionName)),
						}
						for _, module := range modules {
							if dep, _, ok := cfg.FindThirdPartyDependency(module); ok {
								// Type stub packages are added as type-checking only.
								addDependency(dep, true, deps, pyiDeps)
							}
						}
						if explainDependency == dep {
							log.Printf("Explaining dependency (%s): "+
								"in the target %q, the file %q imports %q at line %d, "+
								"which resolves from the third-party module %q from the wheel %q.\n",
								explainDependency, from.String(), mod.Filepath, moduleName, mod.LineNumber, mod.Name, dep)
						}
						continue MODULES_LOOP
					} else {
						matches := ix.FindRulesByImportWithConfig(c, imp, languageName)
						if len(matches) == 0 {
							// Check if the imported module is part of the standard library.
							if isStdModule(Module{Name: moduleName}) {
								continue MODULES_LOOP
							} else if cfg.ValidateImportStatements() {
								err := fmt.Errorf(
									"%[1]q, line %[2]d: %[3]q is an invalid dependency: possible solutions:\n"+
										"\t1. Add it as a dependency in the requirements.txt file.\n"+
										"\t2. Use the '# gazelle:resolve py %[3]s TARGET_LABEL' BUILD file directive to resolve to a known dependency.\n"+
										"\t3. Ignore it with a comment '# gazelle:ignore %[3]s' in the Python file.\n",
									mod.Filepath, mod.LineNumber, moduleName,
								)
								errs = append(errs, err)
								continue POSSIBLE_MODULE_LOOP
							}
						}
						filteredMatches := make([]resolve.FindResult, 0, len(matches))
						for _, match := range matches {
							if match.IsSelfImport(from) {
								// Prevent from adding itself as a dependency.
								continue MODULES_LOOP
							}
							filteredMatches = append(filteredMatches, match)
						}
						if len(filteredMatches) == 0 {
							continue POSSIBLE_MODULE_LOOP
						}
						if len(filteredMatches) > 1 {
							sameRootMatches := make([]resolve.FindResult, 0, len(filteredMatches))
							for _, match := range filteredMatches {
								if strings.HasPrefix(match.Label.Pkg, pythonProjectRoot) {
									sameRootMatches = append(sameRootMatches, match)
								}
							}
							if len(sameRootMatches) != 1 {
								err := fmt.Errorf(
									"%[1]q, line %[2]d: multiple targets (%[3]s) may be imported with %[4]q: possible solutions:\n"+
										"\t1. Disambiguate the above multiple targets by removing duplicate srcs entries.\n"+
										"\t2. Use the '# gazelle:resolve py %[4]s TARGET_LABEL' BUILD file directive to resolve to one of the above targets.\n",
									mod.Filepath, mod.LineNumber, targetListFromResults(filteredMatches), moduleName)
								errs = append(errs, err)
								continue POSSIBLE_MODULE_LOOP
							}
							filteredMatches = sameRootMatches
						}
						matchLabel := filteredMatches[0].Label.Rel(from.Repo, from.Pkg)
						dep := matchLabel.String()
						
						// Register the mapping from dependency label to its source files
						// This allows us to look up source files during deps_to_remove creation
						match := filteredMatches[0]
						depSrcsPaths := make([]string, 0)
						// Try to infer source file from the import name
						if strings.Contains(moduleName, ".") {
							parts := strings.Split(moduleName, ".")
							srcFile := parts[len(parts)-1] + ".py"
							depSrcsPaths = append(depSrcsPaths, filepath.Join(match.Label.Pkg, srcFile))
						} else {
							srcFile := moduleName + ".py"
							depSrcsPaths = append(depSrcsPaths, filepath.Join(match.Label.Pkg, srcFile))
						}
						py.depsOrderResolver.importToSrcs[dep] = depSrcsPaths
						
						addDependency(dep, mod.TypeCheckingOnly, deps, pyiDeps)
						if explainDependency == dep {
							log.Printf("Explaining dependency (%s): "+
								"in the target %q, the file %q imports %q at line %d, "+
								"which resolves from the first-party indexed labels.\n",
								explainDependency, from.String(), mod.Filepath, moduleName, mod.LineNumber)
						}
						continue MODULES_LOOP
					}
				}
			} // End possible modules loop.
			if len(errs) > 0 {
				// If, after trying all possible modules, we still haven't found anything, error out.
				joinedErrs := ""
				for _, err := range errs {
					joinedErrs = fmt.Sprintf("%s%s\n", joinedErrs, err)
				}
				log.Printf("ERROR: failed to validate dependencies for target %q:\n\n%v", from.String(), joinedErrs)
				hasFatalError = true
			}
		}
		if hasFatalError {
			os.Exit(1)
		}
	}

	addResolvedDeps(r, deps)

	// Load deps order constraints if available
	err := py.depsOrderResolver.LoadDepsOrder(c.RepoRoot)
	if err != nil {
		log.Printf("Warning: failed to load deps-order.txt: %v", err)
	}

	// Get current rule's sources for ordering comparison
	currentSrcs := r.AttrStrings("srcs")
	// Convert relative paths to paths relative to repo root
	currentSrcsPaths := make([]string, 0, len(currentSrcs))
	for _, src := range currentSrcs {
		currentSrcsPaths = append(currentSrcsPaths, filepath.Join(from.Pkg, src))
	}

	// Function to create deps_to_remove based on ordering constraints
	createDepsToRemove := func(allDeps *treeset.Set) *treeset.Set {
		depsToRemove := treeset.NewWith(godsutils.StringComparator)
		
		// If we have ordering constraints, check each dependency
		if len(py.depsOrderResolver.fileToIndex) > 0 {
			allDeps.Each(func(_ int, dep interface{}) {
				depLabel := dep.(string)
				
				// Get the source files for this dependency using the registered mappings
				depSrcs := py.depsOrderResolver.getSourcesForImport(depLabel)
				
				// Check if this dependency should be added to deps_to_remove based on ordering
				if py.depsOrderResolver.ShouldAddToDepsToRemove(currentSrcsPaths, depSrcs) {
					depsToRemove.Add(dep)
				}
			})
		}
		
		return depsToRemove
	}

	if cfg.GeneratePyiDeps() {
		if !deps.Empty() {
			r.SetAttr("deps", convertDependencySetToExpr(deps))
			depsToRemove := createDepsToRemove(deps)
			if !depsToRemove.Empty() {
				r.SetAttr("deps_to_remove", convertDependencySetToExpr(depsToRemove))
			}
		}
		if !pyiDeps.Empty() {
			r.SetAttr("pyi_deps", convertDependencySetToExpr(pyiDeps))
		}
	} else {
		// When generate_pyi_deps is false, merge both deps and pyiDeps into deps
		combinedDeps := treeset.NewWith(godsutils.StringComparator)
		combinedDeps.Add(deps.Values()...)
		combinedDeps.Add(pyiDeps.Values()...)

		if !combinedDeps.Empty() {
			r.SetAttr("deps", convertDependencySetToExpr(combinedDeps))
			depsToRemove := createDepsToRemove(combinedDeps)
			if !depsToRemove.Empty() {
				r.SetAttr("deps_to_remove", convertDependencySetToExpr(depsToRemove))
			}
		}
	}
}

// addResolvedDeps adds the pre-resolved dependencies from the rule's private attributes
// to the provided deps set.
func addResolvedDeps(
	r *rule.Rule,
	deps *treeset.Set,
) {
	resolvedDeps := r.PrivateAttr(resolvedDepsKey).(*treeset.Set)
	if !resolvedDeps.Empty() {
		it := resolvedDeps.Iterator()
		for it.Next() {
			deps.Add(it.Value())
		}
	}
}

// targetListFromResults returns a string with the human-readable list of
// targets contained in the given results.
func targetListFromResults(results []resolve.FindResult) string {
	list := make([]string, len(results))
	for i, result := range results {
		list[i] = result.Label.String()
	}
	return strings.Join(list, ", ")
}

// convertDependencySetToExpr converts the given set of dependencies to an
// expression to be used in the deps attribute.
func convertDependencySetToExpr(set *treeset.Set) bzl.Expr {
	deps := make([]bzl.Expr, set.Size())
	it := set.Iterator()
	for it.Next() {
		dep := it.Value().(string)
		deps[it.Index()] = &bzl.StringExpr{Value: dep}
	}
	return &bzl.ListExpr{List: deps}
}
