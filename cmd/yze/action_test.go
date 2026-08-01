package main

import (
	"go/token"
	"testing"

	errs "github.com/gomatic/go-error"
	goyze "github.com/gomatic/go-yze"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gomatic/yze"
)

func TestActionEmitRulesSARIF(t *testing.T) {
	out, err := runApp(t, appName, "--emit-rules", "sarif")

	require.NoError(t, err)
	assert.Contains(t, out, `"$schema"`)
	assert.Contains(t, out, `"yze/errconst"`)
	assert.Contains(t, out, `"yze/keywordcase"`, "the SQL analyzers are part of the catalog")
}

func TestActionEmitRulesGrit(t *testing.T) {
	out, err := runApp(t, appName, "--emit-rules", "grit")

	require.NoError(t, err)
	assert.Contains(t, out, "# yze rule catalog")
	assert.Contains(t, out, "yze/errconst")
	assert.Contains(t, out, "yze/keywordcase", "the SQL analyzers are part of the catalog")
}

func TestActionEmitRulesAppliesCategoryFilter(t *testing.T) {
	out, err := runApp(t, appName, "--emit-rules", "grit", "--category", "sql")

	require.NoError(t, err)
	assert.Contains(t, out, "yze/keywordcase")
	assert.NotContains(t, out, "yze/errconst")
}

func TestActionRejectsUnknownFormat(t *testing.T) {
	swapDriver(t, reportDriver(t))

	_, err := runApp(t, appName, "--format", "nope")

	require.Error(t, err)
}

func TestActionAppliesConfigFile(t *testing.T) {
	swapDriver(t, emptyDriver())
	swapReadFile(t, "analyzers:\n  ptrrecv:\n    allow:\n      - pkg.Foo\n", nil)
	t.Cleanup(func() {
		for _, reg := range yze.Registrations() {
			if reg.Name == "ptrrecv" {
				_ = reg.Analyzer.Flags.Set("allow", "")
			}
		}
	})

	_, err := runApp(t, appName, "--config", "yze.yaml")

	require.NoError(t, err)
}

func TestActionReportsConfigLoadError(t *testing.T) {
	swapDriver(t, emptyDriver())
	swapReadFile(t, "", errs.Const("no config file"))

	_, err := runApp(t, appName, "--config", "missing.yaml")

	require.Error(t, err)
}

func TestActionReportsConfigApplyError(t *testing.T) {
	swapDriver(t, emptyDriver())
	swapReadFile(t, "analyzers:\n  ptrrecv:\n    nope:\n      - x\n", nil)

	_, err := runApp(t, appName, "--config", "yze.yaml")

	require.Error(t, err)
}

func TestActionRejectsSQLAnalyzerConfigSettings(t *testing.T) {
	// The SQL analyzers define no settings, so a config block that supplies one
	// for a bundled SQL analyzer must error, not silently no-op.
	swapDriver(t, emptyDriver())
	swapReadFile(t, "analyzers:\n  keywordcase:\n    style:\n      - upper\n", nil)

	_, err := runApp(t, appName, "--config", "yze.yaml")

	require.ErrorIs(t, err, yze.ErrSQLSetting)
}

func TestActionRunsBundledSQLAnalyzer(t *testing.T) {
	// --category sql skips the Go analyzers and runs the SQL analyzers over the
	// .sql files under the pattern.
	out, err := runApp(t, appName, "--category", "sql", sqlDir(t, "SELECT 1 FROM t;"))
	require.NoError(t, err)
	assert.Contains(t, out, "yze/keywordcase")
	assert.Contains(t, out, "should be lowercase")
}

func TestActionReturnsSQLAnalyzerError(t *testing.T) {
	// A lexical scan failure in a .sql file surfaces as a run error.
	_, err := runApp(t, appName, "--category", "sql", sqlDir(t, "select 'unterminated"))
	require.Error(t, err)
}

func TestActionEmitsTextFormat(t *testing.T) {
	swapDriver(t, reportDriver(t))

	out, err := runApp(t, appName, "--format", "text")

	require.NoError(t, err)
	assert.Equal(t, "p.go:1:1: boom (yze/x)\n", out)
}

func TestActionEmitsSticklerJSONByDefault(t *testing.T) {
	swapDriver(t, reportDriver(t))

	out, err := runApp(t, appName)

	require.NoError(t, err)
	assert.Contains(t, out, `"diagnostics"`)
	assert.Contains(t, out, `"boom"`)
}

func TestActionAppliesCategoryFilter(t *testing.T) {
	var captured []goyze.Registration
	swapDriver(t, func(regs []goyze.Registration, _ []goyze.Pattern) (*token.FileSet, []goyze.DriverResult, error) {
		captured = regs
		return token.NewFileSet(), nil, nil
	})

	_, err := runApp(t, appName, "--category", "errors")

	require.NoError(t, err)
	require.Len(t, captured, 4)
	assert.Equal(t, "yze/errconst", captured[0].RuleID())
}

func TestActionPassesExplicitPatterns(t *testing.T) {
	var captured []goyze.Pattern
	swapDriver(t, func(_ []goyze.Registration, patterns []goyze.Pattern) (*token.FileSet, []goyze.DriverResult, error) {
		captured = patterns
		return token.NewFileSet(), nil, nil
	})

	_, err := runApp(t, appName, "./foo/...")

	require.NoError(t, err)
	assert.Equal(t, []goyze.Pattern{"./foo/..."}, captured)
}

func TestActionReturnsDriverError(t *testing.T) {
	swapDriver(t, func(_ []goyze.Registration, _ []goyze.Pattern) (*token.FileSet, []goyze.DriverResult, error) {
		return nil, nil, errs.Const("driver boom")
	})

	_, err := runApp(t, appName)

	require.Error(t, err)
}

func TestActionSkipsBothLanguagesWhenCategoryMatchesNothing(t *testing.T) {
	// A category no analyzer carries filters out both the Go and SQL analyzers, so
	// neither language runs and the report is empty.
	out, err := runApp(t, appName, "--category", "no-such-category", sqlDir(t, "SELECT 1;"))
	require.NoError(t, err)
	assert.NotContains(t, out, "yze/keywordcase")
}
