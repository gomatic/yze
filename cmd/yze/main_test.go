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

// failWriter fails every write, to prove the fix summary's write error surfaces.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errs.Const("write boom") }

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

	require.NoError(t, osWriteFile(sourcePath(path), []byte("new")))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))

	assert.Error(t, osWriteFile(sourcePath(t.TempDir()+"/missing.go"), []byte("x")))
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

// alternativesDriver returns one diagnostic carrying two alternative suggested
// fixes on the first call and a clean result afterwards. go/analysis defines a
// diagnostic's SuggestedFixes as alternative strategies of which at most one may
// be applied, so the run must apply only the first.
func alternativesDriver(t *testing.T) goyze.Driver {
	t.Helper()
	fset, f := fileSet(t)
	count := 0
	return func(_ []goyze.Registration, _ []goyze.Pattern) (*token.FileSet, []goyze.DriverResult, error) {
		count++
		if count > 1 {
			return fset, nil, nil
		}
		return fset, []goyze.DriverResult{{
			Registration: sampleReg(),
			Diagnostics: []analysis.Diagnostic{{
				Pos:     f.Pos(0),
				Message: "boom",
				SuggestedFixes: []analysis.SuggestedFix{
					{
						Message:   "rename to q",
						TextEdits: []analysis.TextEdit{{Pos: f.Pos(8), End: f.Pos(9), NewText: []byte("q")}},
					},
					{
						Message:   "rename to r",
						TextEdits: []analysis.TextEdit{{Pos: f.Pos(8), End: f.Pos(9), NewText: []byte("r")}},
					},
				},
			}},
		}}, nil
	}
}

// sqlDir writes a single .sql file with the given contents into a fresh temp dir
// and returns the recursive pattern naming it.
func sqlDir(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "q.sql"), []byte(contents), 0o600))
	return dir + "/..."
}
