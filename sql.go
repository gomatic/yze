package yze

import (
	"errors"
	"io/fs"
	"slices"
	"strings"

	errs "github.com/gomatic/go-error"
	sql "github.com/gomatic/go-sql"
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
// the shared go-yze contract, so they merge into the same report; Doc and URL are
// the catalog metadata mirroring [goyze.Registration], so the rule exports list
// SQL rules alongside the Go ones.
type SQLAnalyzer struct {
	Analyze    func(path, source string) ([]goyze.Diagnostic, error)
	Name       goyze.AnalyzerName
	Doc        string
	URL        goyze.HelpURL
	Categories []goyze.Category
}

// RuleID returns the stable rule identifier "yze/<name>" carried by every
// diagnostic the analyzer emits, mirroring [goyze.Registration.RuleID] so both
// languages share one flat id scheme.
func (a SQLAnalyzer) RuleID() string {
	return "yze/" + string(a.Name)
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
			Doc:        "reports SQL keywords that are not written in lowercase, per the gomatic SQL standard",
			URL:        "https://docs.gomatic.dev/yze/keywordcase",
			Categories: []goyze.Category{keywordcase.Category},
			Analyze: func(path, source string) ([]goyze.Diagnostic, error) {
				return keywordcase.Diagnostics(keywordcase.Path(path), sql.SQL(source))
			},
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

// RunSQL finds every .sql file under the roots, drops the ones git ignores
// (an ignored file is not the repository's owned surface — the same policy
// the Go-package side applies, failing open when git cannot answer), and runs
// the analyzers over each, returning the merged diagnostics. A walk or read
// failure aborts the run.
func RunSQL(
	read goyze.FileReader,
	walk WalkDir,
	ignore CheckIgnore,
	analyzers []SQLAnalyzer,
	roots []string,
) (goyze.Report, error) {
	files, err := sqlFiles(walk, roots)
	if err != nil {
		return goyze.Report{}, err
	}
	files = keepTracked(ignore, files)
	report := goyze.Report{}
	for _, file := range files {
		diags, err := analyzeSQLFile(read, analyzers, sqlPath(file))
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
	c := sqlCollector{files: &files}
	for _, root := range roots {
		if err := collectUnder(walk, sqlRoot(root), c); err != nil {
			return nil, err
		}
	}
	return files, nil
}

// sqlCollector accumulates .sql paths across walk callbacks; the slice lives
// behind a pointer field so the value handle's copies share it.
type sqlCollector struct{ files *[]string }

// sqlRoot is a directory root walked for .sql files.
type sqlRoot string

// collectUnder appends the .sql files under one root to files. A root that doesn't
// exist is not an error — a Go package pattern like "./foo/..." need not name a
// real directory, and a tree with no SQL simply contributes none — so it's
// skipped; any other walk failure is wrapped in [ErrSQLWalk].
func collectUnder(walk WalkDir, root sqlRoot, c sqlCollector) error {
	err := walk(string(root), c.visit)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return ErrSQLWalk.With(err, "root", string(root))
	}
	return nil
}

// visit is the walk callback: it prunes the directories Go tooling never
// lints — testdata (fixtures are deliberate, often deliberately wrong),
// vendor, and hidden trees — and collects every .sql file.
func (c sqlCollector) visit(path string, d fs.DirEntry, err error) error {
	if err != nil {
		return err
	}
	if d.IsDir() && prunedDir(dirName(d.Name())) {
		return fs.SkipDir
	}
	if !d.IsDir() && strings.HasSuffix(path, sqlExtension) {
		*c.files = append(*c.files, path)
	}
	return nil
}

// dirName is a directory's base name as seen by the walk.
type dirName string

// The directories a SQL walk skips, named so the walk and its tests share one
// definition of "exempt".
const (
	// fixturesDir holds deliberately-malformed inputs; a finding there
	// describes the fixture rather than a defect.
	fixturesDir dirName = "testdata"
	// vendorDir holds code this repository does not own.
	vendorDir dirName = "vendor"
)

// prunedDir reports whether a directory is exempt from SQL linting: test
// fixtures, vendored code, and hidden trees — mirroring the go tool, which
// never loads these. "." and ".." are NOT hidden trees; pruning "." would prune
// the walk root and report a clean tree for a repository full of SQL.
func prunedDir(name dirName) bool {
	return name == fixturesDir || name == vendorDir ||
		(strings.HasPrefix(string(name), ".") && name != "." && name != "..")
}

// sqlPath is the path to one .sql file the suite analyzes.
type sqlPath string

// analyzeSQLFile reads one file and runs every analyzer over it, collecting their
// diagnostics. A read failure or an analyzer error (e.g. a lexical scan failure)
// aborts.
func analyzeSQLFile(read goyze.FileReader, analyzers []SQLAnalyzer, file sqlPath) ([]goyze.Diagnostic, error) {
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
