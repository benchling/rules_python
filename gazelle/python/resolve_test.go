package python

import (
	"testing"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
	"github.com/stretchr/testify/assert"

	"github.com/bazel-contrib/rules_python/gazelle/pythonconfig"
)

func TestImportsPerFileGenerationInitPackageTargetDoesNotIndexSiblingModules(t *testing.T) {
	t.Parallel()

	fileModeCfg := &pythonconfig.Config{}
	fileModeCfg.SetPerFileGeneration(true)
	c := &config.Config{
		Exts: map[string]interface{}{
			languageName: pythonconfig.Configs{
				"util/events": fileModeCfg,
			},
		},
	}
	f := &rule.File{Pkg: "util/events"}

	resolver := &Resolver{}
	eventsRule := rule.NewRule("py_library", "events")
	eventsRule.SetAttr("srcs", []string{"__init__.py"})
	datadogRule := rule.NewRule("py_library", "datadog")
	datadogRule.SetAttr("srcs", []string{"datadog.py"})

	eventsImports := resolver.Imports(c, eventsRule, f)
	datadogImports := resolver.Imports(c, datadogRule, f)

	assert.Equal(t, []string{"util.events"}, importImps(eventsImports))
	assert.Equal(t, []string{"util.events.datadog"}, importImps(datadogImports))
}

func TestImportsMergedPackageLibraryIndexesUnclaimedModules(t *testing.T) {
	t.Parallel()

	packageModeCfg := &pythonconfig.Config{}
	packageModeCfg.SetPerFileGeneration(false)
	c := &config.Config{
		Exts: map[string]interface{}{
			languageName: pythonconfig.Configs{
				"pkg": packageModeCfg,
			},
		},
	}
	f := &rule.File{Pkg: "pkg"}

	resolver := &Resolver{}
	pkgRule := rule.NewRule("py_library", "pkg")
	pkgRule.SetAttr("srcs", []string{"__init__.py", "datadog.py"})

	imports := resolver.Imports(c, pkgRule, f)
	assert.Equal(t, []string{"pkg", "pkg.datadog"}, importImps(imports))
}

func importImps(specs []resolve.ImportSpec) []string {
	out := make([]string, len(specs))
	for i, spec := range specs {
		out[i] = spec.Imp
	}
	return out
}
