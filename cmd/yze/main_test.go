package main

import (
	"bytes"
	"context"
	"go/token"
	"os"
	"testing"

	errs "github.com/gomatic/go-error"
	goyze "github.com/gomatic/go-yze"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
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
	return goyze.Registration{Name: "x", Group: "go", Analyzer: &analysis.Analyzer{Name: "x"}}
}

// reportDriver returns one diagnostic with no fix.
func reportDriver(t *testing.T) goyze.Driver {
	fset, f := fileSet(t)
	return func(_ []goyze.Registration, _ []string) (*token.FileSet, []goyze.DriverResult, error) {
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

func TestActionEmitsTextFormat(t *testing.T) {
	swapDriver(t, reportDriver(t))

	out, err := runApp(t, "yze", "--format", "text")

	require.NoError(t, err)
	assert.Equal(t, "p.go:1:1: boom (yze/go/x)\n", out)
}

func TestActionEmitsSticklerJSONByDefault(t *testing.T) {
	swapDriver(t, reportDriver(t))

	out, err := runApp(t, "yze")

	require.NoError(t, err)
	assert.Contains(t, out, `"diagnostics"`)
	assert.Contains(t, out, `"boom"`)
}

func TestActionAppliesGroupFilter(t *testing.T) {
	var captured []goyze.Registration
	swapDriver(t, func(regs []goyze.Registration, _ []string) (*token.FileSet, []goyze.DriverResult, error) {
		captured = regs
		return token.NewFileSet(), nil, nil
	})

	_, err := runApp(t, "yze", "--group", "go")
	require.NoError(t, err)
	assert.Len(t, captured, 5)

	_, err = runApp(t, "yze", "--group", "sql")
	require.NoError(t, err)
	assert.Empty(t, captured)
}

func TestActionAppliesCategoryFilter(t *testing.T) {
	var captured []goyze.Registration
	swapDriver(t, func(regs []goyze.Registration, _ []string) (*token.FileSet, []goyze.DriverResult, error) {
		captured = regs
		return token.NewFileSet(), nil, nil
	})

	_, err := runApp(t, "yze", "--category", "errors")

	require.NoError(t, err)
	require.Len(t, captured, 1)
	assert.Equal(t, "yze/go/errconst", captured[0].RuleID())
}

func TestActionPassesExplicitPatterns(t *testing.T) {
	var captured []string
	swapDriver(t, func(_ []goyze.Registration, patterns []string) (*token.FileSet, []goyze.DriverResult, error) {
		captured = patterns
		return token.NewFileSet(), nil, nil
	})

	_, err := runApp(t, "yze", "./foo/...")

	require.NoError(t, err)
	assert.Equal(t, []string{"./foo/..."}, captured)
}

func TestActionReturnsDriverError(t *testing.T) {
	swapDriver(t, func(_ []goyze.Registration, _ []string) (*token.FileSet, []goyze.DriverResult, error) {
		return nil, nil, errs.Const("driver boom")
	})

	_, err := runApp(t, "yze")

	require.Error(t, err)
}

func TestActionRejectsUnknownFormat(t *testing.T) {
	swapDriver(t, reportDriver(t))

	_, err := runApp(t, "yze", "--format", "nope")

	require.Error(t, err)
}

func TestActionFixWithNoFixesSucceeds(t *testing.T) {
	swapDriver(t, reportDriver(t))

	_, err := runApp(t, "yze", "--fix")

	require.NoError(t, err)
}

func TestActionFixPropagatesApplyError(t *testing.T) {
	fset, f := fileSet(t)
	swapDriver(t, func(_ []goyze.Registration, _ []string) (*token.FileSet, []goyze.DriverResult, error) {
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

	_, err := runApp(t, "yze", "--fix")

	require.Error(t, err)
}

func TestRunReturnsZeroOnSuccessAndOneOnError(t *testing.T) {
	swapDriver(t, func(_ []goyze.Registration, _ []string) (*token.FileSet, []goyze.DriverResult, error) {
		return token.NewFileSet(), nil, nil
	})

	assert.Equal(t, 0, run([]string{"yze"}))
	assert.Equal(t, 1, run([]string{"yze", "--format", "nope"}))
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
	swapDriver(t, func(_ []goyze.Registration, _ []string) (*token.FileSet, []goyze.DriverResult, error) {
		return token.NewFileSet(), nil, nil
	})
	originalExit, originalArgs := osExit, os.Args
	t.Cleanup(func() { osExit, os.Args = originalExit, originalArgs })

	var code int
	osExit = func(c int) { code = c }
	os.Args = []string{"yze"}

	main()

	assert.Equal(t, 0, code)
}
