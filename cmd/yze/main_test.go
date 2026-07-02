package main

import (
	"bytes"
	"context"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	errs "github.com/gomatic/go-error"
	goyze "github.com/gomatic/go-yze"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"

	"github.com/gomatic/yze"
)

func swapDriver(t *testing.T, d goyze.Driver) {
	t.Helper()
	original := driver
	t.Cleanup(func() { driver = original })
	driver = d
}

func fileSet(t *testing.T) (*token.FileSet, *token.File) {
	t.Helper()
	fset := token.NewFileSet()
	src := "package p\n"
	f := fset.AddFile("p.go", fset.Base(), len(src))
	f.SetLinesForContent([]byte(src))
	return fset, f
}

func sampleReg() goyze.Registration {
	return goyze.Registration{Name: "x", Analyzer: &analysis.Analyzer{Name: "x"}}
}

// reportDriver returns one diagnostic with no fix.
func reportDriver(t *testing.T) goyze.Driver {
	fset, f := fileSet(t)
	return func(_ []goyze.Registration, _ []goyze.Pattern) (*token.FileSet, []goyze.DriverResult, error) {
		return fset, []goyze.DriverResult{{
			Registration: sampleReg(),
			Diagnostics:  []analysis.Diagnostic{{Pos: f.Pos(0), Message: "boom"}},
		}}, nil
	}
}

func runApp(t *testing.T, args ...string) (string, error) {
	t.Helper()
	app := createApp()
	var buf bytes.Buffer
	app.Writer = &buf
	err := app.Run(context.Background(), args)
	return buf.String(), err
}

func TestVersionFlagReportsVersion(t *testing.T) {
	out, err := runApp(t, appName, "--version")

	require.NoError(t, err)
	assert.Contains(t, out, version)
}

func TestActionEmitRulesSARIF(t *testing.T) {
	out, err := runApp(t, appName, "--emit-rules", "sarif")

	require.NoError(t, err)
	assert.Contains(t, out, `"$schema"`)
	assert.Contains(t, out, `"yze/errconst"`)
}

func TestActionEmitRulesGrit(t *testing.T) {
	out, err := runApp(t, appName, "--emit-rules", "grit")

	require.NoError(t, err)
	assert.Contains(t, out, "# yze rule catalog")
	assert.Contains(t, out, "yze/errconst")
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
	require.Len(t, captured, 2)
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

func TestActionRejectsUnknownFormat(t *testing.T) {
	swapDriver(t, reportDriver(t))

	_, err := runApp(t, appName, "--format", "nope")

	require.Error(t, err)
}

func swapVerifier(t *testing.T, v goyze.Verifier) {
	t.Helper()
	original := verifier
	t.Cleanup(func() { verifier = original })
	verifier = v
}

func swapWriteFile(t *testing.T, w goyze.FileWriter) {
	t.Helper()
	original := writeFile
	t.Cleanup(func() { writeFile = original })
	writeFile = w
}

func swapErrWriter(t *testing.T) *bytes.Buffer {
	t.Helper()
	original := errWriter
	t.Cleanup(func() { errWriter = original })
	var buf bytes.Buffer
	errWriter = &buf
	return &buf
}

// fixingDriver returns one diagnostic carrying a suggested fix on the first
// call and a clean result afterwards, so --fix reaches its fixpoint after one
// applied round.
func fixingDriver(t *testing.T) goyze.Driver {
	return sequenceDriver(t, 1, nil)
}

// sequenceDriver returns a driver that yields one fixable diagnostic per call
// for the first fixRounds calls and a clean result afterwards, counting each
// invocation in calls (when non-nil). It fakes the analyzers whose fix sets
// shrink round over round, which is what the --fix fixpoint loop iterates on.
func sequenceDriver(t *testing.T, fixRounds int, calls *int) goyze.Driver {
	t.Helper()
	fset, f := fileSet(t)
	count := 0
	return func(_ []goyze.Registration, _ []goyze.Pattern) (*token.FileSet, []goyze.DriverResult, error) {
		count++
		if calls != nil {
			*calls = count
		}
		if count > fixRounds {
			return fset, nil, nil
		}
		return fset, []goyze.DriverResult{{
			Registration: sampleReg(),
			Diagnostics: []analysis.Diagnostic{{
				Pos:     f.Pos(0),
				Message: "boom",
				SuggestedFixes: []analysis.SuggestedFix{{
					Message:   "rewrite",
					TextEdits: []analysis.TextEdit{{Pos: f.Pos(8), End: f.Pos(9), NewText: []byte("q")}},
				}},
			}},
		}}, nil
	}
}

// swapAppliedFix arranges a --fix run that successfully applies one edit:
// the driver offers a fix, the file reads back as valid Go, and the write
// lands in the returned map.
func swapAppliedFix(t *testing.T) map[string]string {
	t.Helper()
	swapDriver(t, fixingDriver(t))
	swapReadFile(t, "package p\n", nil)
	written := map[string]string{}
	swapWriteFile(t, func(path string, data []byte) error {
		written[path] = string(data)
		return nil
	})
	return written
}

func TestActionFixWithNoFixesSucceeds(t *testing.T) {
	swapDriver(t, reportDriver(t))
	swapVerifier(t, func(_ []goyze.Pattern) (goyze.VerifyResult, error) {
		t.Fatal("verifier must not run when no edits were applied")
		return goyze.VerifyResult{}, nil
	})

	out, err := runApp(t, appName, "--fix")

	require.NoError(t, err)
	assert.Empty(t, out, "a no-edit --fix run stays silent")
}

func TestActionFixVerifiesAndPrintsSummaryWhenClean(t *testing.T) {
	written := swapAppliedFix(t)
	var captured []goyze.Pattern
	swapVerifier(t, func(patterns []goyze.Pattern) (goyze.VerifyResult, error) {
		captured = patterns
		return goyze.VerifyResult{}, nil
	})

	out, err := runApp(t, appName, "--fix", "./foo/...")

	require.NoError(t, err)
	assert.Equal(t, "applied 1 edit(s) across 1 file(s) in 1 round(s)\n", out)
	assert.Equal(t, []goyze.Pattern{"./foo/..."}, captured, "the verifier must reload the same patterns")
	assert.Equal(t, map[string]string{"p.go": "package q\n"}, written)
}

// swapVerifierCounting installs a clean verifier that counts its invocations,
// to prove verification runs exactly once after the final round.
func swapVerifierCounting(t *testing.T) *int {
	t.Helper()
	calls := 0
	swapVerifier(t, func(_ []goyze.Pattern) (goyze.VerifyResult, error) {
		calls++
		return goyze.VerifyResult{}, nil
	})
	return &calls
}

func TestActionFixIteratesToFixpoint(t *testing.T) {
	driverCalls := 0
	swapDriver(t, sequenceDriver(t, 2, &driverCalls))
	swapReadFile(t, "package p\n", nil)
	swapWriteFile(t, func(string, []byte) error { return nil })
	verifyCalls := swapVerifierCounting(t)
	stderr := swapErrWriter(t)

	out, err := runApp(t, appName, "--fix")

	require.NoError(t, err)
	assert.Equal(
		t,
		"applied 2 edit(s) across 1 file(s) in 2 round(s)\n",
		out,
		"totals aggregate across rounds; files dedup",
	)
	assert.Equal(t, 3, driverCalls, "two fixing rounds plus the clean re-analysis that ends the loop")
	assert.Equal(t, 1, *verifyCalls, "verification runs once, after the final round")
	assert.Empty(t, stderr.String(), "no cap warning below maxFixRounds")
}

func TestActionFixStopsAtRoundCap(t *testing.T) {
	driverCalls := 0
	swapDriver(t, sequenceDriver(t, 1000, &driverCalls))
	swapReadFile(t, "package p\n", nil)
	swapWriteFile(t, func(string, []byte) error { return nil })
	verifyCalls := swapVerifierCounting(t)
	stderr := swapErrWriter(t)

	out, err := runApp(t, appName, "--fix")

	require.NoError(t, err)
	assert.Equal(t, "applied 10 edit(s) across 1 file(s) in 10 round(s)\n", out)
	assert.Contains(t, stderr.String(), "fix rounds capped at 10; fixes may remain")
	assert.Equal(t, maxFixRounds, driverCalls, "the capped round must not trigger another re-analysis")
	assert.Equal(t, 1, *verifyCalls, "a capped run is still verified, once")
}

func TestActionFixPropagatesReanalysisError(t *testing.T) {
	boom := errs.Const("reanalysis boom")
	inner := sequenceDriver(t, 1, nil)
	calls := 0
	swapDriver(
		t,
		func(regs []goyze.Registration, patterns []goyze.Pattern) (*token.FileSet, []goyze.DriverResult, error) {
			calls++
			if calls > 1 {
				return nil, nil, boom
			}
			return inner(regs, patterns)
		},
	)
	swapReadFile(t, "package p\n", nil)
	swapWriteFile(t, func(string, []byte) error { return nil })
	swapVerifier(t, func(_ []goyze.Pattern) (goyze.VerifyResult, error) {
		t.Fatal("verifier must not run when a re-analysis round fails")
		return goyze.VerifyResult{}, nil
	})

	_, err := runApp(t, appName, "--fix")

	require.ErrorIs(t, err, boom)
}

func TestActionFixFailsWhenTreeNoLongerTypeChecks(t *testing.T) {
	swapAppliedFix(t)
	stderr := swapErrWriter(t)
	swapVerifier(t, func(_ []goyze.Pattern) (goyze.VerifyResult, error) {
		return goyze.VerifyResult{Issues: []goyze.VerifyIssue{
			{Pos: "p_test.go:5:2", Msg: "too many arguments in call to f"},
		}}, nil
	})

	_, err := runApp(t, appName, "--fix")

	require.ErrorIs(t, err, errFixVerify)
	assert.Contains(t, stderr.String(), "p_test.go:5:2: too many arguments in call to f\n")
	assert.Contains(t, stderr.String(),
		"fixes applied, but 1 file(s) need follow-up "+
			"(the tree no longer type-checks — likely _test.go callers of retyped functions)\n")
}

func TestActionFixPropagatesVerifierError(t *testing.T) {
	swapAppliedFix(t)
	boom := errs.Const("verify boom")
	swapVerifier(t, func(_ []goyze.Pattern) (goyze.VerifyResult, error) {
		return goyze.VerifyResult{}, boom
	})

	_, err := runApp(t, appName, "--fix")

	require.ErrorIs(t, err, boom)
}

// failWriter fails every write, to prove the fix summary's write error surfaces.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errs.Const("write boom") }

func TestActionFixReportsSummaryWriteError(t *testing.T) {
	swapAppliedFix(t)
	swapVerifier(t, func(_ []goyze.Pattern) (goyze.VerifyResult, error) {
		return goyze.VerifyResult{}, nil
	})
	app := createApp()
	app.Writer = failWriter{}

	err := app.Run(context.Background(), []string{appName, "--fix"})

	require.Error(t, err)
}

func TestActionFixPropagatesApplyError(t *testing.T) {
	fset, f := fileSet(t)
	swapDriver(t, func(_ []goyze.Registration, _ []goyze.Pattern) (*token.FileSet, []goyze.DriverResult, error) {
		return fset, []goyze.DriverResult{{
			Registration: sampleReg(),
			Diagnostics: []analysis.Diagnostic{{
				Pos:     f.Pos(0),
				Message: "boom",
				SuggestedFixes: []analysis.SuggestedFix{{
					Message:   "rewrite",
					TextEdits: []analysis.TextEdit{{Pos: f.Pos(0), End: f.Pos(7), NewText: []byte("package q")}},
				}},
			}},
		}}, nil
	})
	originalRead := readFile
	t.Cleanup(func() { readFile = originalRead })
	readFile = func(string) ([]byte, error) { return nil, errs.Const("read boom") }

	_, err := runApp(t, appName, "--fix")

	require.Error(t, err)
}

func TestRunReturnsZeroOnSuccessAndOneOnError(t *testing.T) {
	swapDriver(t, func(_ []goyze.Registration, _ []goyze.Pattern) (*token.FileSet, []goyze.DriverResult, error) {
		return token.NewFileSet(), nil, nil
	})

	assert.Equal(t, 0, run([]string{appName}))
	assert.Equal(t, 1, run([]string{appName, "--format", "nope"}))
}

func TestOSWriteFilePreservesAndRejectsMissing(t *testing.T) {
	path := t.TempDir() + "/f.go"
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))

	require.NoError(t, osWriteFile(path, []byte("new")))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))

	assert.Error(t, osWriteFile(t.TempDir()+"/missing.go", []byte("x")))
}

func TestMainExits(t *testing.T) {
	swapDriver(t, func(_ []goyze.Registration, _ []goyze.Pattern) (*token.FileSet, []goyze.DriverResult, error) {
		return token.NewFileSet(), nil, nil
	})
	originalExit, originalArgs := osExit, os.Args
	t.Cleanup(func() { osExit, os.Args = originalExit, originalArgs })

	var code int
	osExit = func(c int) { code = c }
	os.Args = []string{appName}

	main()

	assert.Equal(t, 0, code)
}

func swapReadFile(t *testing.T, content string, err error) {
	t.Helper()
	original := readFile
	t.Cleanup(func() { readFile = original })
	readFile = func(string) ([]byte, error) {
		if err != nil {
			return nil, err
		}
		return []byte(content), nil
	}
}

func emptyDriver() goyze.Driver {
	return func(_ []goyze.Registration, _ []goyze.Pattern) (*token.FileSet, []goyze.DriverResult, error) {
		return token.NewFileSet(), nil, nil
	}
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

// sqlDir writes a single .sql file with the given contents into a fresh temp dir
// and returns the recursive pattern naming it.
func sqlDir(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "q.sql"), []byte(contents), 0o600))
	return dir + "/..."
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

func TestActionSkipsBothLanguagesWhenCategoryMatchesNothing(t *testing.T) {
	// A category no analyzer carries filters out both the Go and SQL analyzers, so
	// neither language runs and the report is empty.
	out, err := runApp(t, appName, "--category", "no-such-category", sqlDir(t, "SELECT 1;"))
	require.NoError(t, err)
	assert.NotContains(t, out, "yze/keywordcase")
}
