// Command yze runs the gomatic yze analyzer suite over the given package
// patterns and emits a normalized report (the stickler-json contract by default).
// It is the aggregator the stickler runner invokes; findings do not by themselves
// fail the run — that gate belongs to stickler.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	errs "github.com/gomatic/go-error"
	goyze "github.com/gomatic/go-yze"
	"github.com/urfave/cli/v3"

	"github.com/gomatic/yze"
)

// Injected collaborators, so the command is testable without loading real
// packages or touching the filesystem.
var (
	driver    goyze.Driver     = goyze.CheckerDriver
	verifier  goyze.Verifier   = goyze.CheckerVerifier
	readFile  goyze.FileReader = os.ReadFile
	writeFile goyze.FileWriter = osWriteFile
	walkDir   yze.WalkDir      = filepath.WalkDir
	errWriter io.Writer        = os.Stderr
)

// errFixVerify reports that applied fixes left the tree failing to type-check;
// the residual errors were already printed to errWriter.
const errFixVerify errs.Const = "post-fix verification failed"

// osWriteFile writes data back to an existing file, preserving its mode.
func osWriteFile(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, info.Mode().Perm())
}

// appName is the CLI name.
const appName = "yze"

// version is the application version, exposed via --version. It defaults to "dev"
// and is overwritten at build time via ldflags: -X main.version={{.Version}}
// (see .goreleaser.yml).
var version = "dev"

// osExit is indirected so tests can observe the process exit code.
var osExit = os.Exit

func main() { osExit(run(os.Args)) }

// run builds and executes the CLI, returning the process exit code.
func run(args []string) int {
	if err := createApp().Run(context.Background(), args); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, appName+":", err)
		return 1
	}
	return 0
}

// createApp constructs the yze CLI. The ExitErrHandler is neutralized so Run
// returns errors to run() rather than exiting the process itself.
func createApp() *cli.Command {
	return &cli.Command{
		Name:           appName,
		Version:        version,
		Usage:          "run the gomatic yze analyzer suite",
		ArgsUsage:      "[packages...]",
		ExitErrHandler: func(context.Context, *cli.Command, error) {},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "format",
				Value: string(yze.FormatSticklerJSON),
				Usage: "output format (stickler-json, text)",
			},
			&cli.BoolFlag{Name: "fix", Usage: "apply suggested fixes in place"},
			&cli.StringSliceFlag{Name: "category", Usage: "restrict to analyzers carrying any of these categories"},
			&cli.StringFlag{Name: "config", Usage: "path to a yze config file (per-analyzer settings)"},
			&cli.StringFlag{Name: "emit-rules", Usage: "export the rule catalog (sarif, grit) instead of running"},
		},
		Action: action,
	}
}

// action runs the filtered analyzers and either applies fixes or emits a report.
func action(_ context.Context, cmd *cli.Command) error {
	cfg := configFromCmd(cmd)
	regs := yze.Filter(yze.Registrations(), cfg.categories)
	if cfg.emitRules != "" {
		return yze.EmitRules(cmd.Writer, cfg.emitRules, regs)
	}
	if err := configure(regs, pathParam(cfg.config)); err != nil {
		return err
	}
	sqlAnalyzers := yze.FilterSQL(yze.SQLAnalyzers(), cfg.categories)
	report, err := runAll(regs, sqlAnalyzers, cfg.patterns)
	if err != nil {
		return err
	}
	if cfg.fix {
		return applyFixes(cmd.Writer, regs, sqlAnalyzers, cfg.patterns, report)
	}
	return yze.Emit(cmd.Writer, cfg.format, report)
}

// runAll runs the Go analyzers over the package patterns and the SQL analyzers
// over the .sql files beneath them, merging their findings into one report. A
// language whose analyzer set is empty (e.g. filtered out by --category) is
// skipped entirely, so yze runs cleanly on a Go-only or SQL-only tree.
func runAll(regs []goyze.Registration, sqlAnalyzers []yze.SQLAnalyzer, patterns []goyze.Pattern) (goyze.Report, error) {
	report := goyze.Report{}
	if len(regs) > 0 {
		goReport, err := goyze.Run(driver, regs, patterns)
		if err != nil {
			return goyze.Report{}, err
		}
		report.Diagnostics = append(report.Diagnostics, goReport.Diagnostics...)
	}
	if len(sqlAnalyzers) > 0 {
		sqlReport, err := yze.RunSQL(readFile, walkDir, sqlAnalyzers, yze.RootsOf(patterns))
		if err != nil {
			return goyze.Report{}, err
		}
		report.Diagnostics = append(report.Diagnostics, sqlReport.Diagnostics...)
	}
	return report, nil
}

// maxFixRounds caps the analyze→apply fixpoint iteration; a tree that still
// yields edits after this many rounds is reported to errWriter and verified
// as-is.
const maxFixRounds = 10

// fixState accumulates what the fixpoint loop changed across rounds: total
// edits, the distinct files touched, the rounds that applied edits, and whether
// the loop stopped at the round cap rather than a fixpoint.
type fixState struct {
	files  map[string]struct{}
	edits  int
	rounds int
	capped bool
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
	s.capped = true
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
	if state.capped {
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

// applyRound applies one round's suggested fixes and returns the result
// alongside the fixes it applied (for file accounting).
func applyRound(report goyze.Report) (goyze.FixResult, []goyze.Fix, error) {
	fixes := allFixes(report)
	result, err := goyze.ApplyFixes(readFile, writeFile, goyze.GoFormat, fixes)
	return result, fixes, err
}

// verifyFixes reloads the fixed patterns — test files included, which the
// analysis driver never loads — and either confirms the applied edits or
// reports the residual errors and fails.
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

// config is the parsed invocation.
type config struct {
	format     yze.Format
	config     string
	emitRules  yze.RuleFormat
	categories []goyze.Category
	patterns   []goyze.Pattern
	fix        bool
}

func configFromCmd(cmd *cli.Command) config {
	return config{
		format:     yze.Format(cmd.String("format")),
		categories: toCategories(cmd.StringSlice("category")),
		patterns:   patternsOf(cmd.Args().Slice()),
		config:     cmd.String("config"),
		emitRules:  yze.RuleFormat(cmd.String("emit-rules")),
		fix:        cmd.Bool("fix"),
	}
}

// pathParam names the path parameter of configure; rename it to the real domain concept.
type pathParam string

// configure applies per-analyzer settings from the config file, if one is given.
func configure(regs []goyze.Registration, path pathParam) error {
	if string(path) == "" {
		return nil
	}
	settings, err := yze.LoadConfig(readFile, string(path))
	if err != nil {
		return err
	}
	return goyze.ApplyConfig(regs, settings)
}

func toCategories(values []string) []goyze.Category {
	out := make([]goyze.Category, 0, len(values))
	for _, v := range values {
		out = append(out, goyze.Category(v))
	}
	return out
}

// patternsOf defaults to the current module when no packages are named.
func patternsOf(args []string) []goyze.Pattern {
	if len(args) == 0 {
		return []goyze.Pattern{"./..."}
	}
	patterns := make([]goyze.Pattern, 0, len(args))
	for _, a := range args {
		patterns = append(patterns, goyze.Pattern(a))
	}
	return patterns
}

func allFixes(report goyze.Report) []goyze.Fix {
	var fixes []goyze.Fix
	for _, d := range report.Diagnostics {
		fixes = append(fixes, d.Fixes...)
	}
	return fixes
}
