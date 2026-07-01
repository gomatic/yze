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
	if err := configure(regs, cfg.config); err != nil {
		return err
	}
	report, err := runAll(regs, yze.FilterSQL(yze.SQLAnalyzers(), cfg.categories), cfg.patterns)
	if err != nil {
		return err
	}
	if cfg.fix {
		return applyFixes(cmd.Writer, report, cfg.patterns)
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

// applyFixes applies every suggested fix in the report through the shared
// engine, then — when at least one edit landed — verifies the tree still
// type-checks. A run that applied no edits is left untouched and unverified.
func applyFixes(w io.Writer, report goyze.Report, patterns []goyze.Pattern) error {
	result, err := goyze.ApplyFixes(readFile, writeFile, goyze.GoFormat, allFixes(report))
	if err != nil {
		return err
	}
	if result.EditsApplied == 0 {
		return nil
	}
	return verifyFixes(w, result, patterns)
}

// verifyFixes reloads the fixed patterns — test files included, which the
// analysis driver never loads — and either confirms the applied edits or
// reports the residual errors and fails.
func verifyFixes(w io.Writer, result goyze.FixResult, patterns []goyze.Pattern) error {
	verified, err := verifier(patterns)
	if err != nil {
		return err
	}
	if !verified.Clean() {
		reportIssues(verified)
		return errFixVerify
	}
	_, err = fmt.Fprintf(w, "applied %d edit(s) across %d file(s)\n", result.EditsApplied, result.FilesChanged)
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

// configure applies per-analyzer settings from the config file, if one is given.
func configure(regs []goyze.Registration, path string) error {
	if path == "" {
		return nil
	}
	settings, err := yze.LoadConfig(readFile, path)
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
