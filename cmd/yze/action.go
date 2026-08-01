package main

import (
	goyze "github.com/gomatic/go-yze"
	"github.com/urfave/cli/v3"

	"github.com/gomatic/yze"
)

// Turning CLI flags and a config file into the analyzer set, category filter and
// patterns a run uses. Kept apart from main so the wiring that decides WHAT is
// analyzed is separate from the loop that analyzes it.

// config is the parsed invocation.
type config struct {
	format     yze.Format
	config     string
	emitRules  yze.RuleFormat
	categories []goyze.Category
	patterns   []goyze.Pattern
	isFix      bool
}

func configFromCmd(cmd *cli.Command) config {
	return config{
		format:     yze.Format(cmd.String("format")),
		categories: toCategories(cmd.StringSlice("category")),
		patterns:   patternsOf(cmd.Args().Slice()),
		config:     cmd.String("config"),
		emitRules:  yze.RuleFormat(cmd.String("emit-rules")),
		isFix:      cmd.Bool("fix"),
	}
}

// configPath is the path to the yze config file holding per-analyzer settings.
type configPath string

// configure applies per-analyzer settings from the config file, if one is given.
// Go settings apply to the registrations' analyzer flags; the SQL analyzers define
// no settings, so a config block supplying one for a bundled SQL analyzer is
// rejected explicitly rather than silently ignored.
func configure(regs []goyze.Registration, sqlAnalyzers []yze.SQLAnalyzer, path configPath) error {
	if string(path) == "" {
		return nil
	}
	settings, err := yze.LoadConfig(readFile, yze.ConfigPath(path))
	if err != nil {
		return err
	}
	if err := goyze.ApplyConfig(regs, settings); err != nil {
		return err
	}
	return yze.ApplySQLConfig(sqlAnalyzers, settings)
}

func toCategories(values []string) []goyze.Category {
	out := make([]goyze.Category, 0, len(values))
	for _, v := range values {
		out = append(out, goyze.Category(v))
	}
	return out
}

// patternsOf defaults to the current module when no packages are named.
// defaultPattern is what a run analyzes when the caller names nothing: the
// whole module. It is named once so the place that supplies the default and the
// places that recognise it stay in agreement.
const defaultPattern goyze.Pattern = "./..."

func patternsOf(args []string) []goyze.Pattern {
	if len(args) == 0 {
		return []goyze.Pattern{defaultPattern}
	}
	patterns := make([]goyze.Pattern, 0, len(args))
	for _, a := range args {
		patterns = append(patterns, goyze.Pattern(a))
	}
	return patterns
}

// preferredFixes collects each diagnostic's preferred suggested fix — the first
// one only. go/analysis defines a diagnostic's SuggestedFixes as alternative
// strategies of which "at most one may be applied", so applying every alternative
// would stack conflicting rewrites of the same source range; the first is the
// analyzer's preferred strategy.
func preferredFixes(report goyze.Report) []goyze.Fix {
	fixes := make([]goyze.Fix, 0, len(report.Diagnostics))
	for _, d := range report.Diagnostics {
		if len(d.Fixes) > 0 {
			fixes = append(fixes, d.Fixes[0])
		}
	}
	return fixes
}
