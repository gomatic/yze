package main

import (
	"bytes"
	"context"
	"go/token"
	"testing"

	errs "github.com/gomatic/go-error"
	goyze "github.com/gomatic/go-yze"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
)

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

func TestActionFixAppliesOnlyFirstSuggestedFix(t *testing.T) {
	swapDriver(t, alternativesDriver(t))
	swapReadFile(t, "package p\n", nil)
	written := map[string]string{}
	swapWriteFile(t, func(path string, data []byte) error {
		written[path] = string(data)
		return nil
	})
	swapVerifier(t, func(_ []goyze.Pattern) (goyze.VerifyResult, error) {
		return goyze.VerifyResult{}, nil
	})

	out, err := runApp(t, appName, "--fix")

	require.NoError(t, err)
	assert.Equal(t, "applied 1 edit(s) across 1 file(s) in 1 round(s)\n", out)
	assert.Equal(t, map[string]string{"p.go": "package q\n"}, written,
		"only the first alternative is applied; the second would conflict")
}

// TestVerifyFixesRefusesToReportSuccessOnABrokenTree names verifyFixes' claim.
// The reload asks a question the analysis pass cannot: not "does any rule still
// fire" but "does the tree still COMPILE after those edits". A rename applied by
// --fix can break a _test.go caller that no analyzer had an opinion about, so
// without this step a fix run leaves the build broken and prints a cheerful
// summary. That summary is exactly what must not appear here.
func TestVerifyFixesRefusesToReportSuccessOnABrokenTree(t *testing.T) {
	restore := verifier
	defer func() { verifier = restore }()

	verifier = func([]goyze.Pattern) (goyze.VerifyResult, error) {
		return goyze.VerifyResult{Issues: []goyze.VerifyIssue{{Msg: "undefined: oldName"}}}, nil
	}

	var out bytes.Buffer
	err := verifyFixes(&out, fixState{edits: 3, rounds: 1}, []goyze.Pattern{"./..."})

	require.ErrorIs(t, err, errFixVerify)
	assert.NotContains(t, out.String(), "applied",
		"no success summary may be printed for a tree that no longer type-checks")
}

// TestVerifyFixesConfirmsACleanTreeAndSummarises is the other half: a clean
// reload must actually report what changed, or a successful --fix run is
// indistinguishable from one that did nothing.
func TestVerifyFixesConfirmsACleanTreeAndSummarises(t *testing.T) {
	restore := verifier
	defer func() { verifier = restore }()

	verifier = func([]goyze.Pattern) (goyze.VerifyResult, error) {
		return goyze.VerifyResult{}, nil
	}

	var out bytes.Buffer
	err := verifyFixes(&out, fixState{edits: 3, files: map[string]struct{}{"a.go": {}}, rounds: 2},
		[]goyze.Pattern{"./..."})

	require.NoError(t, err)
	assert.Contains(t, out.String(), "3 edit(s)")
	assert.Contains(t, out.String(), "1 file(s)")
	assert.Contains(t, out.String(), "2 round(s)")
}
