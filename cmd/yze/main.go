// Command yze runs the gomatic yze analyzer suite over the given package
// patterns and emits a normalized report (the stickler-json contract by default).
// It is the aggregator the stickler runner invokes; findings do not by themselves
// fail the run — that gate belongs to stickler.
package main

import (
	"context"
	"fmt"
	"os"

	goyze "github.com/gomatic/go-yze"
	"github.com/urfave/cli/v3"

	"github.com/gomatic/yze"
)

// Injected collaborators, so the command is testable without loading real
// packages or touching the filesystem.
var (
	driver    goyze.Driver     = goyze.CheckerDriver
	readFile  goyze.FileReader = os.ReadFile
	writeFile goyze.FileWriter = osWriteFile
)

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
		Usage:          "run the gomatic yze analyzer suite",
		ArgsUsage:      "[packages...]",
		ExitErrHandler: func(context.Context, *cli.Command, error) {},
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "format", Value: string(yze.FormatSticklerJSON), Usage: "output format (stickler-json, text)"},
			&cli.BoolFlag{Name: "fix", Usage: "apply suggested fixes in place"},
			&cli.StringSliceFlag{Name: "category", Usage: "restrict to analyzers carrying any of these categories"},
			&cli.StringFlag{Name: "config", Usage: "path to a yze config file (per-analyzer settings)"},
		},
		Action: action,
	}
}

// action runs the filtered analyzers and either applies fixes or emits a report.
func action(_ context.Context, cmd *cli.Command) error {
	cfg := configFromCmd(cmd)
	regs := yze.Filter(yze.Registrations(), cfg.categories)
	if err := configure(regs, cfg.config); err != nil {
		return err
	}
	report, err := goyze.Run(driver, regs, cfg.patterns)
	if err != nil {
		return err
	}
	if cfg.fix {
		return applyFixes(report)
	}
	return yze.Emit(cmd.Writer, cfg.format, report)
}

// applyFixes applies every suggested fix in the report through the shared engine.
func applyFixes(report goyze.Report) error {
	_, err := goyze.ApplyFixes(readFile, writeFile, goyze.GoFormat, allFixes(report))
	return err
}

// config is the parsed invocation.
type config struct {
	format     yze.Format
	config     string
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
