package main

import (
	"fmt"
	"io"

	goyze "github.com/gomatic/go-yze"

	"github.com/gomatic/yze"
)

// The --fix loop: apply, re-analyze, repeat to a fixpoint, then VERIFY. The
// verification step is the one that keeps a fix run honest — an applied edit
// can break a _test.go caller no analyzer has an opinion about, so without it a
// --fix run can leave the build broken and report success.

// maxFixRounds caps the analyze→apply fixpoint iteration; a tree that still
// yields edits after this many rounds is reported to errWriter and verified
// as-is.
const maxFixRounds = 10

// fixState accumulates what the fixpoint loop changed across rounds: total
// edits, the distinct files touched, the rounds that applied edits, and whether
// the loop stopped at the round cap rather than a fixpoint.
type fixState struct {
	files    map[string]struct{}
	edits    int
	rounds   int
	isCapped bool
}

// absorb folds one applied round into the running totals and returns the
// updated state.
func (s fixState) absorb(result goyze.FixResult, fixes []goyze.Fix) fixState {
	s.edits += result.EditsApplied
	s.rounds++
	for _, fix := range fixes {
		for _, fe := range fix.Files {
			if len(fe.Edits) > 0 {
				s.files[fe.Path] = struct{}{}
			}
		}
	}
	return s
}

// atCap marks the state as stopped by the round cap rather than a fixpoint.
func (s fixState) atCap() fixState {
	s.isCapped = true
	return s
}

// applyFixes drives --fix to a fixpoint: it applies the report's suggested
// fixes and, while a round applied edits, re-analyzes and applies again — some
// analyzers (e.g. namedtypes' first-by-position name dedupe) intentionally
// defer conflicting fixes to a later round. When at least one edit landed, the
// tree is verified once, after the final round. A run whose first round applied
// no edits is left untouched, unverified, and silent.
func applyFixes(
	w io.Writer,
	regs []goyze.Registration,
	sqlAnalyzers []yze.SQLAnalyzer,
	patterns []goyze.Pattern,
	report goyze.Report,
) error {
	state, err := fixpoint(regs, sqlAnalyzers, patterns, report)
	if err != nil {
		return err
	}
	if state.edits == 0 {
		return nil
	}
	if state.isCapped {
		_, _ = fmt.Fprintf(errWriter, "fix rounds capped at %d; fixes may remain — re-run --fix\n", maxFixRounds)
	}
	return verifyFixes(w, state, patterns)
}

// fixpoint runs apply→re-analyze rounds until a round applies no edits or the
// round cap is reached, accumulating totals across the applied rounds.
func fixpoint(
	regs []goyze.Registration,
	sqlAnalyzers []yze.SQLAnalyzer,
	patterns []goyze.Pattern,
	report goyze.Report,
) (fixState, error) {
	state := fixState{files: map[string]struct{}{}}
	for {
		result, fixes, err := applyRound(report)
		switch {
		case err != nil:
			return state, err
		case result.EditsApplied == 0:
			return state, nil
		}
		state = state.absorb(result, fixes)
		if state.rounds == maxFixRounds {
			return state.atCap(), nil
		}
		if report, err = runAll(regs, sqlAnalyzers, patterns); err != nil {
			return state, err
		}
	}
}

// applyRound applies one round's preferred suggested fixes and returns the
// result alongside the fixes it applied (for file accounting).
func applyRound(report goyze.Report) (goyze.FixResult, []goyze.Fix, error) {
	fixes := preferredFixes(report)
	result, err := goyze.ApplyFixes(readFile, writeFile, goyze.GoFormat, fixes)
	return result, fixes, err
}

// verifyFixes reloads the fixed patterns and either confirms the applied edits
// or reports the residual errors and fails.
//
// The reload asks a different question from the analysis pass that produced the
// fixes: not "does any rule still fire" but "does the tree still type-check
// after those edits". A fix that renames a symbol can break a _test.go caller
// no analyzer had an opinion about, so an unverified --fix run can leave the
// build broken while reporting success.
func verifyFixes(w io.Writer, state fixState, patterns []goyze.Pattern) error {
	verified, err := verifier(patterns)
	if err != nil {
		return err
	}
	if !verified.Clean() {
		reportIssues(verified)
		return errFixVerify
	}
	_, err = fmt.Fprintf(
		w,
		"applied %d edit(s) across %d file(s) in %d round(s)\n",
		state.edits,
		len(state.files),
		state.rounds,
	)
	return err
}

// reportIssues prints each residual error and a follow-up summary to errWriter.
func reportIssues(verified goyze.VerifyResult) {
	for _, issue := range verified.Issues {
		_, _ = fmt.Fprintln(errWriter, issue)
	}
	_, _ = fmt.Fprintf(errWriter,
		"fixes applied, but %d file(s) need follow-up "+
			"(the tree no longer type-checks — likely _test.go callers of retyped functions)\n",
		verified.Files())
}
