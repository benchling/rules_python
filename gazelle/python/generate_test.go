package python

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/language"
	"github.com/bazelbuild/bazel-gazelle/rule"
	"github.com/emirpasic/gods/sets/treeset"
	godsutils "github.com/emirpasic/gods/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bazel-contrib/rules_python/gazelle/pythonconfig"
)

func newTestBuildFile(rules ...*rule.Rule) *rule.File {
	return &rule.File{
		Path:  "BUILD.bazel",
		Rules: rules,
	}
}

func newPyLibraryRule(name string, srcs []string) *rule.Rule {
	r := rule.NewRule("py_library", name)
	r.SetAttr("srcs", srcs)
	return r
}

func TestCollectExistingPythonSourceRulesSkipsExcludedOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "excluded_only.py"), []byte(""), 0o600))

	args := language.GenerateArgs{
		Dir:    dir,
		File:   newTestBuildFile(newPyLibraryRule("excluded_only", []string{"excluded_only.py"})),
		Config: &config.Config{},
	}
	knownSrcs := map[string]struct{}{"foo.py": {}}

	rules := collectExistingPythonSourceRules(args, pyLibraryKind, knownSrcs)
	assert.Empty(t, rules)
}

func TestCollectExistingPythonSourceRulesKeepsExcludedAlongsideKnown(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "foo.py"), []byte(""), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "excluded_only.py"), []byte(""), 0o600))

	args := language.GenerateArgs{
		Dir: dir,
		File: newTestBuildFile(newPyLibraryRule("custom", []string{
			"foo.py",
			"excluded_only.py",
		})),
		Config: &config.Config{},
	}
	knownSrcs := map[string]struct{}{"foo.py": {}}

	rules := collectExistingPythonSourceRules(args, pyLibraryKind, knownSrcs)
	require.Len(t, rules, 1)
	assert.Equal(t, "custom", rules[0].name)
	assert.Equal(t, 2, rules[0].srcs.Size())
}

func TestAdoptExcludedInitOnlyPackageLibraryForSplitLayout(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "__init__.py"), []byte(""), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "foo.py"), []byte(""), 0o600))

	packageLibraryName := "pkg"
	args := language.GenerateArgs{
		Dir: dir,
		File: newTestBuildFile(
			newPyLibraryRule("foo", []string{"foo.py"}),
			newPyLibraryRule(packageLibraryName, []string{pyLibraryEntrypointFilename}),
		),
		Config: &config.Config{},
	}
	knownSrcs := map[string]struct{}{"foo.py": {}}

	adopted := adoptExcludedInitOnlyPackageLibraryForSplitLayout(
		args,
		pyLibraryKind,
		packageLibraryName,
		knownSrcs,
		collectExistingPythonSourceRules(args, pyLibraryKind, knownSrcs),
	)
	require.Len(t, adopted, 2)
	assert.True(t, hasSplitPackageLibraryLayout(packageLibraryName, adopted))

	packageRule := adopted[1]
	assert.Equal(t, packageLibraryName, packageRule.name)
	assert.Equal(t, 1, packageRule.srcs.Size())
	assert.True(t, packageRule.srcs.Contains(pyLibraryEntrypointFilename))
}

func TestAdoptExcludedInitOnlyPackageLibraryIgnoredWithoutOtherLibraries(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "__init__.py"), []byte(""), 0o600))

	packageLibraryName := "pkg"
	args := language.GenerateArgs{
		Dir:    dir,
		File:   newTestBuildFile(newPyLibraryRule(packageLibraryName, []string{pyLibraryEntrypointFilename})),
		Config: &config.Config{},
	}

	adopted := adoptExcludedInitOnlyPackageLibraryForSplitLayout(
		args,
		pyLibraryKind,
		packageLibraryName,
		map[string]struct{}{},
		nil,
	)
	assert.Nil(t, adopted)
}

func TestGetRulesWithInvalidSrcsKeepsExcludedSourcesOnDisk(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "pkg")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "__init__.py"), []byte(""), 0o600))

	buildFile := newTestBuildFile(newPyLibraryRule("pkg", []string{pyLibraryEntrypointFilename}))
	pkgConfig := pythonconfig.New(dir, "")
	args := language.GenerateArgs{
		Dir:          dir,
		Rel:          "pkg",
		File:         buildFile,
		Config: &config.Config{
			Exts: map[string]interface{}{
				"py": pythonconfig.Configs{"pkg": pkgConfig},
			},
		},
		RegularFiles: []string{"foo.py"},
	}

	py := &Python{}
	invalid := py.getRulesWithInvalidSrcs(args, map[string]struct{}{})
	assert.Empty(t, invalid)
}

func TestHasSplitPackageLibraryLayout(t *testing.T) {
	t.Parallel()

	packageLibraryName := "pkg"
	fooSrcs := treeset.NewWith(godsutils.StringComparator, "foo.py")
	pkgSrcs := treeset.NewWith(godsutils.StringComparator, "aux.py")

	rules := []existingPythonSourceRule{
		{name: "foo", srcs: fooSrcs, declaredSrcCount: 1},
		{name: packageLibraryName, srcs: pkgSrcs, declaredSrcCount: 1},
	}
	assert.True(t, hasSplitPackageLibraryLayout(packageLibraryName, rules))

	overlapPkg := treeset.NewWith(godsutils.StringComparator, "foo.py", "aux.py")
	overlapRules := []existingPythonSourceRule{
		{name: "foo", srcs: fooSrcs, declaredSrcCount: 1},
		{name: packageLibraryName, srcs: overlapPkg, declaredSrcCount: 2},
	}
	assert.False(t, hasSplitPackageLibraryLayout(packageLibraryName, overlapRules))
}

func TestEmptyAggregateFixtureSplitLayout(t *testing.T) {
	t.Parallel()

	packageLibraryName := "package_mode_respect_existing_split_package_library_empty_aggregate"
	dir := filepath.Join(t.TempDir(), packageLibraryName)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "foo.py"), []byte(""), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bar.py"), []byte(""), 0o600))

	pkgRule := rule.NewRule("py_library", packageLibraryName)
	pkgRule.SetAttr("deps", []string{":foo"})
	args := language.GenerateArgs{
		Dir: dir,
		File: newTestBuildFile(
			newPyLibraryRule("foo", []string{"foo.py"}),
			newPyLibraryRule("bar", []string{"bar.py"}),
			pkgRule,
		),
		Config: &config.Config{},
	}
	knownSrcs := map[string]struct{}{"foo.py": {}, "bar.py": {}}

	rules := collectExistingPythonSourceRules(args, pyLibraryKind, knownSrcs)
	rules = adoptEmptyAggregatePackageLibraryForSplitLayout(args, pyLibraryKind, packageLibraryName, rules)
	require.Len(t, rules, 3)
	assert.True(t, hasSplitPackageLibraryLayout(packageLibraryName, rules))
}

func TestAdoptEmptyAggregatePackageLibraryForSplitLayout(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "foo.py"), []byte(""), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bar.py"), []byte(""), 0o600))

	packageLibraryName := "pkg"
	args := language.GenerateArgs{
		Dir: dir,
		File: newTestBuildFile(
			rule.NewRule("py_library", packageLibraryName),
			newPyLibraryRule("foo", []string{"foo.py"}),
			newPyLibraryRule("bar", []string{"bar.py"}),
		),
		Config: &config.Config{},
	}
	knownSrcs := map[string]struct{}{"foo.py": {}, "bar.py": {}}

	adopted := adoptEmptyAggregatePackageLibraryForSplitLayout(
		args,
		pyLibraryKind,
		packageLibraryName,
		collectExistingPythonSourceRules(args, pyLibraryKind, knownSrcs),
	)
	require.Len(t, adopted, 3)
	assert.True(t, hasSplitPackageLibraryLayout(packageLibraryName, adopted))

	var packageRule *existingPythonSourceRule
	for i := range adopted {
		if adopted[i].name == packageLibraryName {
			packageRule = &adopted[i]
			break
		}
	}
	require.NotNil(t, packageRule)
	assert.Equal(t, 0, packageRule.srcs.Size())
	assert.Equal(t, 0, packageRule.declaredSrcCount)
}

func TestAdoptEmptyAggregatePackageLibraryIgnoredWithoutPerFileLibraries(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	packageLibraryName := "pkg"
	args := language.GenerateArgs{
		Dir:    dir,
		File:   newTestBuildFile(rule.NewRule("py_library", packageLibraryName)),
		Config: &config.Config{},
	}

	adopted := adoptEmptyAggregatePackageLibraryForSplitLayout(
		args,
		pyLibraryKind,
		packageLibraryName,
		nil,
	)
	assert.Nil(t, adopted)
}

func TestHasExplicitSourceOwnershipLayout(t *testing.T) {
	t.Parallel()

	packageLibraryName := "pkg"
	fooSrcs := treeset.NewWith(godsutils.StringComparator, "foo.py")
	barSrcs := treeset.NewWith(godsutils.StringComparator, "bar.py")
	libraryFilenames := treeset.NewWith(godsutils.StringComparator, "foo.py", "bar.py")

	rules := []existingPythonSourceRule{
		{name: "foo", srcs: fooSrcs, declaredSrcCount: 1},
		{name: "bar", srcs: barSrcs, declaredSrcCount: 1},
	}
	assert.True(t, hasExplicitSourceOwnershipLayout(packageLibraryName, rules, libraryFilenames))

	withPackageLib := append(rules, existingPythonSourceRule{
		name:             packageLibraryName,
		srcs:             treeset.NewWith(godsutils.StringComparator),
		declaredSrcCount: 0,
	})
	assert.False(t, hasExplicitSourceOwnershipLayout(packageLibraryName, withPackageLib, libraryFilenames))

	multiSrc := []existingPythonSourceRule{
		{name: "custom", srcs: libraryFilenames, declaredSrcCount: 2},
	}
	assert.True(t, hasExplicitSourceOwnershipLayout(packageLibraryName, multiSrc, libraryFilenames))

	authSrcs := treeset.NewWith(godsutils.StringComparator, "auth.py", "oauth2.py")
	mixedFilenames := treeset.NewWith(godsutils.StringComparator, "foo.py", "auth.py", "oauth2.py")
	mixed := []existingPythonSourceRule{
		{name: "foo", srcs: fooSrcs, declaredSrcCount: 1},
		{name: "auth", srcs: authSrcs, declaredSrcCount: 2},
	}
	assert.True(t, hasExplicitSourceOwnershipLayout(packageLibraryName, mixed, mixedFilenames))

	onlySrcs := treeset.NewWith(godsutils.StringComparator, "only.py")
	singleTarget := []existingPythonSourceRule{
		{name: "only", srcs: onlySrcs, declaredSrcCount: 1},
	}
	singleFilenames := treeset.NewWith(godsutils.StringComparator, "only.py")
	assert.True(t, hasExplicitSourceOwnershipLayout(packageLibraryName, singleTarget, singleFilenames))

	overlap := []existingPythonSourceRule{
		{name: "a", srcs: fooSrcs, declaredSrcCount: 1},
		{name: "b", srcs: libraryFilenames, declaredSrcCount: 2},
	}
	assert.False(t, hasExplicitSourceOwnershipLayout(packageLibraryName, overlap, libraryFilenames))

	partial := []existingPythonSourceRule{
		{name: "foo", srcs: fooSrcs, declaredSrcCount: 1},
	}
	assert.False(t, hasExplicitSourceOwnershipLayout(packageLibraryName, partial, libraryFilenames))
}

func TestAdoptEmptyAggregatePackageLibraryIgnoredWithMultiSrcLibrary(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "foo.py"), []byte(""), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bar.py"), []byte(""), 0o600))

	packageLibraryName := "pkg"
	args := language.GenerateArgs{
		Dir: dir,
		File: newTestBuildFile(
			rule.NewRule("py_library", packageLibraryName),
			newPyLibraryRule("custom", []string{"foo.py", "bar.py"}),
		),
		Config: &config.Config{},
	}
	knownSrcs := map[string]struct{}{"foo.py": {}, "bar.py": {}}

	adopted := adoptEmptyAggregatePackageLibraryForSplitLayout(
		args,
		pyLibraryKind,
		packageLibraryName,
		collectExistingPythonSourceRules(args, pyLibraryKind, knownSrcs),
	)
	require.Len(t, adopted, 1)
	assert.Equal(t, "custom", adopted[0].name)
}
