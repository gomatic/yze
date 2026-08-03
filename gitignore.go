package yze

// Dropping git-ignored SQL files before analysis. A file git ignores is not
// the repository's owned surface — a generated dist/ artifact, a vendored
// dump — and no gate may judge it. The Go-package side already applies this
// policy (go-yze's git-ignore package filter drops packages whose files git
// ignores); the SQL walk mirrors it here because go-yze's machinery is
// unexported.
//
// The filter fails OPEN: when git is unavailable, the files are not in a
// repository, or the check cannot be run, every file is kept. Silently
// dropping real SQL would turn a gate green over unanalyzed code — the one
// outcome worse than judging a stray generated file.

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
)

// CheckIgnore reports which of the given files git ignores. It is the seam a
// test replaces to drive the filter without a real repository; the command
// wires [GitCheckIgnore].
type CheckIgnore func(files []string) (map[string]bool, error)

// GitCheckIgnore is the default CheckIgnore: one `git check-ignore` over all
// files at once, run from the process working directory — which during a lint
// pass is inside the repository — reading the ignored subset from its
// NUL-delimited output.
func GitCheckIgnore(files []string) (map[string]bool, error) {
	return gitCheckIgnoreIn("", files)
}

// workDir is the directory git check-ignore runs from; empty means the process
// working directory, which during a lint pass is inside the repository.
type workDir string

// gitCheckIgnoreIn runs the check from a specific working directory, so a test
// can drive it against an isolated repository. An empty dir uses the process
// working directory.
//
// Unlike go-yze's package filter, no out-of-tree pre-filtering is needed: the
// walked files all come from the roots the patterns name, which during a lint
// pass lie under the working directory. A root genuinely outside the work tree
// makes git check-ignore fail whole (exit 128), which the caller turns into
// fail-open — identical to the unfiltered behavior such a run had before.
func gitCheckIgnoreIn(dir workDir, files []string) (map[string]bool, error) {
	if len(files) == 0 {
		return map[string]bool{}, nil
	}
	cmd := exec.Command("git", "check-ignore", "--stdin", "-z")
	cmd.Dir = string(dir)
	cmd.Stdin = strings.NewReader(strings.Join(files, "\x00"))
	var out bytes.Buffer
	cmd.Stdout = &out
	// git check-ignore exits 1 when NOTHING is ignored and 128 on a real
	// failure (not a repository, git missing). Only 0 and 1 are meaningful
	// answers; anything else is surfaced so the caller can fail open.
	err := cmd.Run()
	if code, ok := exitCode(err); ok && code == 1 {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	return ignoredSet(out.Bytes()), nil
}

// exitCode extracts a command's process exit code, reporting whether err is an
// ordinary non-zero exit (as opposed to a start failure like a missing binary).
func exitCode(err error) (int, bool) {
	if err == nil {
		return 0, true
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), true
	}
	return 0, false
}

// ignoredSet parses git check-ignore -z output (a NUL-delimited list of the
// ignored paths, echoed exactly as given) into a set keyed by the walked
// paths.
func ignoredSet(out []byte) map[string]bool {
	ignored := map[string]bool{}
	for _, path := range strings.Split(string(out), "\x00") {
		if path != "" {
			ignored[path] = true
		}
	}
	return ignored
}

// keepTracked drops every collected file git ignores, keeping the rest in
// walk order. On any error it fails open and returns the files unchanged, so
// the findings still appear rather than vanishing over an unanswerable
// question — mirroring go-yze's package filter.
func keepTracked(ignore CheckIgnore, files []string) []string {
	ignored, err := ignore(files)
	if err != nil || len(ignored) == 0 {
		return files
	}
	kept := make([]string, 0, len(files))
	for _, f := range files {
		if !ignored[f] {
			kept = append(kept, f)
		}
	}
	return kept
}
