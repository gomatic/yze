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
	writeFile goyze.FileWriter = func(path string, data []byte) error { return osWriteFile(sourcePath(path), data) }
	walkDir   yze.WalkDir      = filepath.WalkDir
	errWriter io.Writer        = os.Stderr
)

// errFixVerify reports that applied fixes left the tree failing to type-check;
// the residual errors were already printed to errWriter.
const errFixVerify errs.Const = "post-fix verification failed"

// sourcePath is an on-disk path of a file a fix rewrites.
type sourcePath string

// osWriteFile writes data back to an existing file, preserving its mode.
func osWriteFile(path sourcePath, data []byte) error {
	info, err := os.Stat(string(path))
	if err != nil {
		return err
	}
	return os.WriteFile(string(path), data, info.Mode().Perm())
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
				Name:    "format",
				Value:   string(yze.FormatSticklerJSON),
				Sources: cli.EnvVars("YZE_FORMAT"),
				Usage:   "output format (stickler-json, text)",
			},
			&cli.BoolFlag{
				Name:    "fix",
				Sources: cli.EnvVars("YZE_FIX"),
				Usage:   "apply suggested fixes in place",
			},
			&cli.StringSliceFlag{
				Name:    "category",
				Sources: cli.EnvVars("YZE_CATEGORY"),
				Usage:   "restrict to analyzers carrying any of these categories",
			},
			&cli.StringFlag{
				Name:    "config",
				Value:   "",
				Sources: cli.EnvVars("YZE_CONFIG"),
				Usage:   "path to a yze config file (per-analyzer settings)",
			},
			&cli.StringFlag{
				Name:    "emit-rules",
				Value:   "",
				Sources: cli.EnvVars("YZE_EMIT_RULES"),
				Usage:   "export the rule catalog (sarif, grit) instead of running",
			},
		},
		Action: action,
	}
}

// action runs the filtered analyzers and either applies fixes or emits a report.
func action(_ context.Context, cmd *cli.Command) error {
	cfg := configFromCmd(cmd)
	regs := yze.Filter(yze.Registrations(), cfg.categories)
	sqlAnalyzers := yze.FilterSQL(yze.SQLAnalyzers(), cfg.categories)
	if cfg.emitRules != "" {
		return yze.EmitRules(cmd.Writer, cfg.emitRules, yze.CatalogRules(regs, sqlAnalyzers))
	}
	if err := configure(regs, sqlAnalyzers, configPath(cfg.config)); err != nil {
		return err
	}
	report, err := runAll(regs, sqlAnalyzers, cfg.patterns)
	if err != nil {
		return err
	}
	if cfg.isFix {
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
