//go:build ignore

// Command dryrun is the fleet dry-run for suite changes: it runs a candidate
// yze (built from this working tree) and the pinned baseline yze side by side
// over every Go module in the fleet, and reports the findings the candidate
// ADDS. Run it before registering a new analyzer or bumping the tools.txt pin:
//
//	go run dryrun.go                     # fleet = ~/src/github.com/gomatic
//	go run dryrun.go -fleet <dir> -fleet <dir> -baseline <path-to-yze>
//
// A new analyzer that collides with a sanctioned ecosystem pattern (the
// globalvar-vs-DI-seam incident) shows up here as a wave of new findings
// across conforming repos — before adoption, not at gate time. The run exits
// non-zero when any repo GAINS findings relative to its own baseline (so a
// not-yet-clean repo still contributes signal); removed findings print as
// informational. A repo whose baseline RUN fails (broken build, workspace
// mismatch) is skipped — it can't be diffed.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	goyze "github.com/gomatic/go-yze"
)

// repoResult is one repository's dry-run outcome.
type repoResult struct {
	repo     string
	added    []string
	removed  []string
	baseline error
}

func main() {
	var fleets fleetList
	flag.Var(&fleets, "fleet", "fleet root directory holding repos (repeatable; default ~/src/github.com/gomatic)")
	baseline := flag.String("baseline", "", "baseline yze binary (default: yze on GOBIN, the pinned suite)")
	details := flag.Int("details", 5, "max added findings printed per repo")
	flag.Parse()

	candidate, cleanup := buildCandidate()
	defer cleanup()
	results := run(fleets.roots(), resolveBaseline(*baseline), candidate)
	os.Exit(report(results, *details))
}

// fleetList accumulates repeated -fleet flags.
type fleetList []string

func (f *fleetList) String() string     { return strings.Join(*f, ",") }
func (f *fleetList) Set(v string) error { *f = append(*f, v); return nil }

// roots yields the configured fleet roots, defaulting to the gomatic org.
func (f fleetList) roots() []string {
	if len(f) > 0 {
		return f
	}
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	return []string{filepath.Join(home, "src", "github.com", "gomatic")}
}

// buildCandidate compiles the working tree's yze into a temp dir.
func buildCandidate() (string, func()) {
	dir, err := os.MkdirTemp("", "yze-dryrun-")
	if err != nil {
		log.Fatal(err)
	}
	bin := filepath.Join(dir, "yze")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/yze")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Fatalf("build candidate: %v\n%s", err, out)
	}
	return bin, func() { _ = os.RemoveAll(dir) }
}

// resolveBaseline returns the baseline binary: the -baseline flag, or the
// pinned yze on GOBIN (falling back to PATH).
func resolveBaseline(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if gobin := gobinDir(); gobin != "" {
		if p := filepath.Join(gobin, "yze"); executable(p) {
			return p
		}
	}
	p, err := exec.LookPath("yze")
	if err != nil {
		log.Fatal("no baseline yze found: set -baseline or install the pinned yze")
	}
	return p
}

// gobinDir resolves GOBIN via the go tool (empty when unset).
func gobinDir() string {
	out, err := exec.Command("go", "env", "GOBIN").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func executable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// run dry-runs every repo under every fleet root.
func run(roots []string, baseline, candidate string) []repoResult {
	var results []repoResult
	for _, root := range roots {
		for _, repo := range modules(root) {
			results = append(results, diffRepo(repo, baseline, candidate))
		}
	}
	return results
}

// modules lists the fleet root's immediate subdirectories that are Go modules,
// skipping docs/site/governance repos (docs.*, www.*, _*) which carry no Go.
func modules(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		log.Fatalf("fleet root %s: %v", root, err)
	}
	var repos []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") ||
			strings.HasPrefix(name, "docs.") || strings.HasPrefix(name, "www.") {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, name, "go.mod")); err == nil {
			repos = append(repos, filepath.Join(root, name))
		}
	}
	sort.Strings(repos)
	return repos
}

// diffRepo runs both binaries over one repo and diffs their findings.
func diffRepo(repo, baseline, candidate string) repoResult {
	base, baseErr := findings(baseline, repo)
	if baseErr != nil {
		return repoResult{repo: repo, baseline: baseErr}
	}
	cand, candErr := findings(candidate, repo)
	if candErr != nil {
		return repoResult{repo: repo, baseline: fmt.Errorf("candidate: %w", candErr)}
	}
	return repoResult{repo: repo, added: minus(cand, base), removed: minus(base, cand)}
}

// findings runs one yze binary over a repo and returns its finding keys.
func findings(bin, repo string) (map[string]bool, error) {
	cmd := exec.Command(bin, "--format", "stickler-json", "--", "./...")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "GOWORK=off")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	report, err := goyze.UnmarshalReport(stdout.Bytes())
	if err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	for _, d := range report.Diagnostics {
		rel, relErr := filepath.Rel(repo, d.Path)
		if relErr != nil {
			rel = d.Path
		}
		keys[fmt.Sprintf("%s:%d:%d: %s (%s)", rel, d.Line, d.Col, d.Message, d.Rule)] = true
	}
	return keys, nil
}

// minus returns the sorted keys of a that b lacks.
func minus(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// report prints the fleet summary and returns the exit code: non-zero when
// any conforming repo gains findings under the candidate.
func report(results []repoResult, details int) int {
	gained := 0
	for _, r := range results {
		gained += printRepo(r, details)
	}
	if gained > 0 {
		fmt.Printf("\nDRY-RUN FAIL: %d repo(s) gain findings under the candidate suite\n", gained)
		return 1
	}
	fmt.Println("\nDRY-RUN PASS: no conforming repo gains findings under the candidate suite")
	return 0
}

// printRepo prints one repo's outcome, returning 1 when it gains findings.
func printRepo(r repoResult, details int) int {
	name := filepath.Base(r.repo)
	switch {
	case r.baseline != nil:
		fmt.Printf("%-24s SKIP (baseline not clean: %v)\n", name, truncate(r.baseline.Error(), 120))
		return 0
	case len(r.added) == 0 && len(r.removed) == 0:
		fmt.Printf("%-24s ok\n", name)
		return 0
	default:
		fmt.Printf("%-24s +%d -%d\n", name, len(r.added), len(r.removed))
		for i, f := range r.added {
			if i == details {
				fmt.Printf("    ... and %d more\n", len(r.added)-details)
				break
			}
			fmt.Printf("    + %s\n", f)
		}
		return gainedFlag(r.added)
	}
}

func gainedFlag(added []string) int {
	if len(added) > 0 {
		return 1
	}
	return 0
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
