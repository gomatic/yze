package yze

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	errs "github.com/gomatic/go-error"
	goyze "github.com/gomatic/go-yze"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const boom errs.Const = "boom"

// fakeEntry is a minimal non-directory fs.DirEntry for driving the walk callback
// without a real tree.
type fakeEntry struct{ name string }

func (f fakeEntry) Name() string             { return f.name }
func (fakeEntry) IsDir() bool                { return false }
func (fakeEntry) Type() fs.FileMode          { return 0 }
func (fakeEntry) Info() (fs.FileInfo, error) { return nil, nil }

// writeSQLDir writes one .sql and one unrelated file into a temp dir and returns it.
func writeSQLDir(t *testing.T, sql string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "q.sql"), []byte(sql), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore me"), 0o600))
	return dir
}

func TestSQLAnalyzersBundlesKeywordcase(t *testing.T) {
	t.Parallel()
	analyzers := SQLAnalyzers()
	require.Len(t, analyzers, 1)
	assert.Equal(t, goyze.AnalyzerName("keywordcase"), analyzers[0].Name)
	assert.Equal(t, []goyze.Category{"sql"}, analyzers[0].Categories)
	assert.NotEmpty(t, analyzers[0].Doc, "every bundled analyzer carries catalog metadata")
	assert.Equal(t, goyze.HelpURL("https://docs.gomatic.dev/yze/keywordcase"), analyzers[0].URL)
	assert.Equal(t, "yze/keywordcase", analyzers[0].RuleID(), "the flat rule-id scheme mirrors goyze.Registration")
}

func TestFilterSQLByCategory(t *testing.T) {
	t.Parallel()
	all := SQLAnalyzers()
	assert.Len(t, FilterSQL(all, nil), 1, "empty categories matches everything")
	assert.Len(t, FilterSQL(all, []goyze.Category{"sql"}), 1)
	assert.Empty(t, FilterSQL(all, []goyze.Category{"go"}))
}

func TestRootsOf(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"."}, RootsOf([]goyze.Pattern{"./..."}))
	assert.Equal(t, []string{"foo"}, RootsOf([]goyze.Pattern{"foo/..."}))
	assert.Equal(t, []string{"./bar"}, RootsOf([]goyze.Pattern{"./bar/..."}))
	assert.Equal(t, []string{"."}, RootsOf([]goyze.Pattern{""}))
}

func TestRunSQLFlagsKeywordsAndIgnoresNonSQL(t *testing.T) {
	t.Parallel()
	dir := writeSQLDir(t, "SELECT 1 FROM t;")
	report, err := RunSQL(os.ReadFile, filepath.WalkDir, SQLAnalyzers(), []string{dir})
	require.NoError(t, err)
	require.Len(t, report.Diagnostics, 2, "SELECT and FROM; the .txt file is ignored")
	assert.Equal(t, "yze/keywordcase", report.Diagnostics[0].Rule)
}

func TestRunSQLCleanForLowercase(t *testing.T) {
	t.Parallel()
	dir := writeSQLDir(t, "select 1 from t;")
	report, err := RunSQL(os.ReadFile, filepath.WalkDir, SQLAnalyzers(), []string{dir})
	require.NoError(t, err)
	assert.Empty(t, report.Diagnostics)
}

func TestRunSQLSkipsMissingRoot(t *testing.T) {
	t.Parallel()
	// A pattern's directory need not exist on disk; a missing root is no SQL, not
	// an error.
	report, err := RunSQL(os.ReadFile, filepath.WalkDir, SQLAnalyzers(), []string{filepath.Join(t.TempDir(), "absent")})
	require.NoError(t, err)
	assert.Empty(t, report.Diagnostics)
}

func TestRunSQLWrapsWalkError(t *testing.T) {
	t.Parallel()
	walk := func(string, fs.WalkDirFunc) error { return boom }
	_, err := RunSQL(os.ReadFile, walk, SQLAnalyzers(), []string{"."})
	assert.ErrorIs(t, err, ErrSQLWalk)
}

func TestRunSQLPropagatesWalkCallbackError(t *testing.T) {
	t.Parallel()
	// A walk that hands the callback an error (an unreadable directory entry).
	walk := func(_ string, fn fs.WalkDirFunc) error { return fn("bad", nil, boom) }
	_, err := RunSQL(os.ReadFile, walk, SQLAnalyzers(), []string{"."})
	assert.ErrorIs(t, err, ErrSQLWalk)
}

func TestRunSQLWrapsReadError(t *testing.T) {
	t.Parallel()
	walk := func(_ string, fn fs.WalkDirFunc) error { return fn("x.sql", fakeEntry{name: "x.sql"}, nil) }
	read := func(string) ([]byte, error) { return nil, boom }
	_, err := RunSQL(read, walk, SQLAnalyzers(), []string{"."})
	assert.ErrorIs(t, err, ErrSQLRead)
}

func TestRunSQLPropagatesAnalyzerError(t *testing.T) {
	t.Parallel()
	// An unterminated string literal is a lexical scan error from the analyzer.
	dir := writeSQLDir(t, "select 'unterminated")
	_, err := RunSQL(os.ReadFile, filepath.WalkDir, SQLAnalyzers(), []string{dir})
	require.Error(t, err)
}

func TestRunSQLPrunesFixtureAndHiddenDirs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, sub := range []string{"testdata", "vendor", ".hidden"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, sub), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, sub, "fixture.sql"), []byte("SELECT 1;"), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.sql"), []byte("SELECT 1;"), 0o644))
	report, err := RunSQL(os.ReadFile, filepath.WalkDir, SQLAnalyzers(), []string{dir})
	require.NoError(t, err)
	require.Len(t, report.Diagnostics, 1, "only real.sql is linted; testdata/vendor/hidden are pruned")
	assert.Equal(t, filepath.Join(dir, "real.sql"), report.Diagnostics[0].Path)
}

// TestPrunedDirMirrorsWhatTheGoToolNeverLoads names prunedDir's claim. Each
// pruned directory is a place where linting would be actively wrong rather than
// merely noisy: testdata fixtures are frequently INTENTIONALLY malformed (that
// is what makes them fixtures), vendor is someone else's code this repo does
// not own, and hidden trees are tooling state. Reporting in any of them
// produces findings nobody can act on, which is how a gate gets ignored.
//
// The dot-directory rule must not swallow "." and ".." — pruning "." would
// prune the walk root and collect nothing at all, silently reporting a clean
// tree for a repository full of SQL.
func TestPrunedDirMirrorsWhatTheGoToolNeverLoads(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name dirName
		why  string
		want bool
	}{
		{name: "testdata", want: true, why: "fixtures are deliberately wrong"},
		{name: "vendor", want: true, why: "vendored code is not this repo's to fix"},
		{name: ".git", want: true, why: "hidden trees are tooling state"},
		{name: ".github", want: true, why: "any hidden directory, not a fixed list"},
		{name: ".", want: false, why: "pruning the walk root would collect nothing at all"},
		{name: "..", want: false, why: "and neither is the parent a hidden tree"},
		{name: "sql", want: false, why: "an ordinary directory is walked"},
		{name: "testdata2", want: false, why: "the match is exact, not a prefix"},
		{name: "mytestdata", want: false, why: "and not a suffix either"},
	} {
		assert.Equal(t, tc.want, prunedDir(tc.name), "prunedDir(%q): %s", tc.name, tc.why)
	}
}

// TestVisitPrunesAndCollectsWhatPrunedDirDecides names visit's claim, which is
// the same rule seen from the walk: a pruned directory must return
// fs.SkipDir — not merely "collect nothing" — because descending into a large
// vendor or .git tree and filtering afterwards is the difference between a walk
// and a crawl. It must also propagate the walk's own error rather than swallow
// it, or an unreadable directory becomes a silently smaller file list.
func TestVisitPrunesAndCollectsWhatPrunedDirDecides(t *testing.T) {
	t.Parallel()

	var files []string
	collector := sqlCollector{files: &files}

	assert.Equal(t, fs.SkipDir, collector.visit("a/vendor", dirEntry{name: "vendor", dir: true}, nil),
		"a pruned directory must stop the descent, not just be ignored")
	assert.NoError(t, collector.visit("a/sql", dirEntry{name: "sql", dir: true}, nil))

	require.NoError(t, collector.visit("a/q.sql", dirEntry{name: "q.sql"}, nil))
	require.NoError(t, collector.visit("a/notes.md", dirEntry{name: "notes.md"}, nil))
	assert.Equal(t, []string{"a/q.sql"}, files, "only .sql files are collected")

	boom := errors.New("permission denied")
	assert.ErrorIs(t, collector.visit("a", dirEntry{name: "a", dir: true}, boom), boom,
		"a walk error must surface, not shrink the file list in silence")
}

// dirEntry is a minimal fs.DirEntry for driving the walk callback directly.
type dirEntry struct {
	name string
	dir  bool
}

func (e dirEntry) Name() string               { return e.name }
func (e dirEntry) IsDir() bool                { return e.dir }
func (e dirEntry) Type() fs.FileMode          { return 0 }
func (e dirEntry) Info() (fs.FileInfo, error) { return nil, nil }
