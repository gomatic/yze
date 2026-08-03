package yze

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initRepo makes dir a git repository. git check-ignore needs neither commits
// nor an identity, so a bare init suffices.
func initRepo(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())
}

// writeIgnoredSQLTree writes a tracked q.sql and a dist/g.sql (both carrying
// the same keyword-case violations) plus a .gitignore ignoring dist/ into a
// fresh temp dir.
func writeIgnoredSQLTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("dist/\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "dist"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "q.sql"), []byte("SELECT 1 FROM t;"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dist", "g.sql"), []byte("SELECT 1 FROM t;"), 0o600))
	return dir
}

// ignoreIn adapts gitCheckIgnoreIn to a CheckIgnore anchored at dir, so a test
// can drive RunSQL against an isolated repository regardless of the process
// working directory.
func ignoreIn(dir string) CheckIgnore {
	return func(files []string) (map[string]bool, error) {
		return gitCheckIgnoreIn(workDir(dir), files)
	}
}

// TestRunSQLDropsGitIgnoredFilesButJudgesTracked pins the filter's contract in
// both directions at once: the git-ignored dist/g.sql carries the exact
// violations of the tracked q.sql, and only q.sql's may be reported — an
// ignored file is not the repository's owned surface, mirroring the policy the
// Go-package side already applies.
func TestRunSQLDropsGitIgnoredFilesButJudgesTracked(t *testing.T) {
	t.Parallel()
	dir := writeIgnoredSQLTree(t)
	initRepo(t, dir)

	report, err := RunSQL(os.ReadFile, filepath.WalkDir, ignoreIn(dir), SQLAnalyzers(), []string{dir})

	require.NoError(t, err)
	require.Len(t, report.Diagnostics, 2, "SELECT and FROM in the tracked file only")
	for _, d := range report.Diagnostics {
		assert.Equal(t, filepath.Join(dir, "q.sql"), d.Path, "no finding may name the ignored dist/g.sql")
	}
}

// TestRunSQLOutsideRepositoryJudgesEverything pins the not-a-repo contract:
// with no git repository to consult, the filter fails open and behavior is
// identical to the unfiltered walk — every .sql file is judged.
func TestRunSQLOutsideRepositoryJudgesEverything(t *testing.T) {
	t.Parallel()
	dir := writeIgnoredSQLTree(t)

	report, err := RunSQL(os.ReadFile, filepath.WalkDir, ignoreIn(dir), SQLAnalyzers(), []string{dir})

	require.NoError(t, err)
	assert.Len(t, report.Diagnostics, 4, "both files judged: SELECT and FROM in each")
}

// TestRunSQLFailsOpenLoudlyWhenGitCannotAnswer pins the fail-open direction as
// RunSQL's own contract: an erroring CheckIgnore must not abort the run and
// must not shrink it — the findings still appear, because silently dropping
// real SQL over an unanswerable question would turn a gate green over
// unanalyzed code.
func TestRunSQLFailsOpenLoudlyWhenGitCannotAnswer(t *testing.T) {
	t.Parallel()
	dir := writeIgnoredSQLTree(t)
	failing := func([]string) (map[string]bool, error) { return nil, boom }

	report, err := RunSQL(os.ReadFile, filepath.WalkDir, failing, SQLAnalyzers(), []string{dir})

	require.NoError(t, err, "an ignore failure fails open, never the run")
	assert.Len(t, report.Diagnostics, 4, "every file stays judged when git cannot answer")
}

// TestKeepTrackedDropsIgnoredAndFailsOpen names keepTracked's three answers:
// an ignored subset is dropped in walk order, an empty answer keeps
// everything, and an error keeps everything (fail open).
func TestKeepTrackedDropsIgnoredAndFailsOpen(t *testing.T) {
	t.Parallel()
	files := []string{"a.sql", "dist/b.sql", "c.sql"}

	dropped := keepTracked(func([]string) (map[string]bool, error) {
		return map[string]bool{"dist/b.sql": true}, nil
	}, files)
	assert.Equal(t, []string{"a.sql", "c.sql"}, dropped)

	assert.Equal(t, files, keepTracked(noIgnore, files), "nothing ignored keeps everything")
	assert.Equal(t, files, keepTracked(func([]string) (map[string]bool, error) { return nil, boom }, files),
		"an error fails open and keeps everything")
}

// TestGitCheckIgnoreRunsFromProcessWorkingDirectory exercises the default
// (empty workDir) path against the repository the tests run in: go.mod is
// tracked and never ignored, while dist/ is the canonical build-output ignore.
func TestGitCheckIgnoreRunsFromProcessWorkingDirectory(t *testing.T) {
	t.Parallel()

	ignored, err := GitCheckIgnore([]string{"go.mod"})
	require.NoError(t, err)
	assert.Empty(t, ignored, "go.mod is owned source; git check-ignore answers 'nothing ignored'")

	ignored, err = GitCheckIgnore([]string{"dist/probe.sql", "go.mod"})
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"dist/probe.sql": true}, ignored)
}

// TestGitCheckIgnoreEmptyFileList pins the no-files shortcut: git is not
// invoked and the answer is an empty set.
func TestGitCheckIgnoreEmptyFileList(t *testing.T) {
	t.Parallel()
	ignored, err := GitCheckIgnore(nil)
	require.NoError(t, err)
	assert.Empty(t, ignored)
}

// TestGitCheckIgnoreInNothingIgnored pins the exit-1 reading: a repository
// that ignores nothing is a meaningful empty answer, not an error.
func TestGitCheckIgnoreInNothingIgnored(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initRepo(t, dir)

	ignored, err := gitCheckIgnoreIn(workDir(dir), []string{"a.sql"})

	require.NoError(t, err)
	assert.Empty(t, ignored)
}

// TestGitCheckIgnoreInSurfacesFailure pins the error surface the caller fails
// open on: outside any repository git check-ignore fails whole (exit 128),
// and that failure must reach the caller rather than masquerade as an answer.
func TestGitCheckIgnoreInSurfacesFailure(t *testing.T) {
	t.Parallel()
	_, err := gitCheckIgnoreIn(workDir(t.TempDir()), []string{"a.sql"})
	assert.Error(t, err)
}

// TestExitCode names the three shapes of a command's ending: success, an
// ordinary non-zero exit, and a start failure that has no exit code at all.
func TestExitCode(t *testing.T) {
	t.Parallel()

	code, ok := exitCode(nil)
	assert.True(t, ok)
	assert.Equal(t, 0, code)

	exitErr := exec.Command("false").Run()
	require.Error(t, exitErr)
	code, ok = exitCode(exitErr)
	assert.True(t, ok)
	assert.Equal(t, 1, code)

	_, ok = exitCode(boom)
	assert.False(t, ok, "a non-exec error carries no exit code")
}

// TestIgnoredSet pins the -z output reading: NUL-delimited paths echoed
// exactly as given, with the trailing empty field dropped.
func TestIgnoredSet(t *testing.T) {
	t.Parallel()
	assert.Equal(t, map[string]bool{"a.sql": true, "dist/b.sql": true}, ignoredSet([]byte("a.sql\x00dist/b.sql\x00")))
	assert.Empty(t, ignoredSet(nil))
}
