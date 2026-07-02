package yze

import (
	"errors"
	"io/fs"
	"slices"
	"strings"

	errs "github.com/gomatic/go-error"
	goyze "github.com/gomatic/go-yze"
	keywordcase "github.com/gomatic/yze-sql-keywordcase"
)

// SQL run errors.
const (
	// ErrSQLWalk reports that walking a root for .sql files failed.
	ErrSQLWalk errs.Const = "cannot walk for SQL files"
	// ErrSQLRead reports that a .sql file could not be read.
	ErrSQLRead errs.Const = "cannot read SQL file"
)

// sqlExtension is the suffix that marks a file as SQL the suite should analyze.
const sqlExtension = ".sql"

// SQLAnalyzer is a source analyzer the suite runs over .sql files, as opposed to
// the go/analysis analyzers it runs over Go packages. Its diagnostics already use
// the shared go-yze contract, so they merge into the same report.
type SQLAnalyzer struct {
	Analyze    func(path, source string) ([]goyze.Diagnostic, error)
	Name       goyze.AnalyzerName
	Categories []goyze.Category
}

// WalkDir is [fs.WalkDir]'s signature. It's injected so a test can drive the file
// walk without a real directory tree.
type WalkDir func(root string, fn fs.WalkDirFunc) error

// SQLAnalyzers returns every SQL source analyzer bundled into the suite, in stable
// rule-id order.
func SQLAnalyzers() []SQLAnalyzer {
	return []SQLAnalyzer{
		{
			Name:       keywordcase.Name,
			Categories: []goyze.Category{keywordcase.Category},
			Analyze:    keywordcase.Diagnostics,
		},
	}
}

// FilterSQL selects the SQL analyzers matching the given categories, mirroring
// [Filter]. An empty category set matches every analyzer.
func FilterSQL(analyzers []SQLAnalyzer, categories []goyze.Category) []SQLAnalyzer {
	out := make([]SQLAnalyzer, 0, len(analyzers))
	for _, a := range analyzers {
		if matchesAny(a.Categories, categories) {
			out = append(out, a)
		}
	}
	return out
}

// matchesAny reports whether have shares any category with want, treating an empty
// want as matching everything.
func matchesAny(have, want []goyze.Category) bool {
	if len(want) == 0 {
		return true
	}
	for _, w := range want {
		if slices.Contains(have, w) {
			return true
		}
	}
	return false
}

// RootsOf turns package patterns into the directories to walk for .sql files: the
// recursive "/..." suffix is dropped, and a bare or empty pattern becomes ".".
func RootsOf(patterns []goyze.Pattern) []string {
	roots := make([]string, 0, len(patterns))
	for _, p := range patterns {
		root := strings.TrimSuffix(string(p), "...")
		root = strings.TrimSuffix(root, "/")
		if root == "" {
			root = "."
		}
		roots = append(roots, root)
	}
	return roots
}

// RunSQL finds every .sql file under the roots and runs the analyzers over each,
// returning the merged diagnostics. A walk or read failure aborts the run.
func RunSQL(read goyze.FileReader, walk WalkDir, analyzers []SQLAnalyzer, roots []string) (goyze.Report, error) {
	files, err := sqlFiles(walk, roots)
	if err != nil {
		return goyze.Report{}, err
	}
	report := goyze.Report{}
	for _, file := range files {
		diags, err := analyzeSQLFile(read, analyzers, fileParam(file))
		if err != nil {
			return goyze.Report{}, err
		}
		report.Diagnostics = append(report.Diagnostics, diags...)
	}
	return report, nil
}

// sqlFiles collects every .sql file under each root.
func sqlFiles(walk WalkDir, roots []string) ([]string, error) {
	var files []string
	for _, root := range roots {
		if err := collectUnder(walk, rootParam(root), &files); err != nil {
			return nil, err
		}
	}
	return files, nil
}

// rootParam names the root parameter of collectUnder; rename it to the real domain concept.
type rootParam string

// collectUnder appends the .sql files under one root to files. A root that doesn't
// exist is not an error — a Go package pattern like "./foo/..." need not name a
// real directory, and a tree with no SQL simply contributes none — so it's
// skipped; any other walk failure is wrapped in [ErrSQLWalk].
func collectUnder(walk WalkDir, root rootParam, files *[]string) error {
	err := walk(string(root), appendSQLFiles(files))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return ErrSQLWalk.With(err, "root", string(root))
	}
	return nil
}

// appendSQLFiles is a walk callback that appends every .sql file it visits to
// files.
func appendSQLFiles(files *[]string) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, sqlExtension) {
			*files = append(*files, path)
		}
		return nil
	}
}

// fileParam names the file parameter of analyzeSQLFile; rename it to the real domain concept.
type fileParam string

// analyzeSQLFile reads one file and runs every analyzer over it, collecting their
// diagnostics. A read failure or an analyzer error (e.g. a lexical scan failure)
// aborts.
func analyzeSQLFile(read goyze.FileReader, analyzers []SQLAnalyzer, file fileParam) ([]goyze.Diagnostic, error) {
	data, err := read(string(file))
	if err != nil {
		return nil, ErrSQLRead.With(err, "path", string(file))
	}
	var diags []goyze.Diagnostic
	for _, a := range analyzers {
		found, err := a.Analyze(string(file), string(data))
		if err != nil {
			return nil, err
		}
		diags = append(diags, found...)
	}
	return diags, nil
}
