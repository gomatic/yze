package yze

import (
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

func (f fakeEntry) Name() string               { return f.name }
func (fakeEntry) IsDir() bool                  { return false }
func (fakeEntry) Type() fs.FileMode            { return 0 }
func (fakeEntry) Info() (fs.FileInfo, error)   { return nil, nil }

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
