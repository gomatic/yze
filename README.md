# yze

The aggregator for the gomatic **yze** analyzer family. It fans in every `yze-<name>` analyzer and runs two kinds together: the `yze-go-*` analyzers over Go packages through the [`go-yze`](https://github.com/gomatic/go-yze) driver, and the `yze-sql-*` analyzers over the `.sql` files beneath the same pattern roots. Both emit one normalized report, so a single `yze` run lints Go and SQL. Each analyzer reports under the flat rule id `yze/<name>` and carries a language `--category` (`go`, `sql`).

```
yze [--format stickler-json|text] [--fix] [--category <c>...] [--config <path>] [packages...]
```

- **`--format stickler-json`** (default) — the native [`Report`](https://github.com/gomatic/go-yze) JSON the [`stickler`](https://github.com/gomatic/stickler) runner consumes with no adapter. `text` prints one human line per finding.
- **`--fix`** — applies analyzers' suggested fixes in place via the shared `go-yze` engine.
- **`--category`** — restricts the analyzer set to those carrying any of the named categories (the many-to-many tag axis). Omit it to run the whole suite.
- **`--config`** — path to a yze config file supplying per-analyzer settings.

Findings do not by themselves fail the run — that gate belongs to [`stickler`](https://github.com/gomatic/stickler). The current suite (14 analyzers): `yze/anonstruct`, `yze/boolname`, `yze/ctxfirst`, `yze/emptyiface`, `yze/errconst`, `yze/errlast`, `yze/gotostmt`, `yze/layout`, `yze/namedtypes`, `yze/pkgstd`, `yze/ptrparam`, `yze/ptrrecv`, `yze/stdlog`, `yze/testfile`.

## Suite governance — fleet dry-run before registration

A new analyzer (or a behavior change to an existing one) can collide with a sanctioned ecosystem pattern — globalvar v0.1.0 flagged the DI test-seam vars and the command-package `var cfg` pattern that pkgstd's own testdata uses. Registering into the suite and bumping the tools.txt pin adopts the change fleet-wide, so the collision must surface **before** registration, not at gate time.

Before registering an analyzer here (or tagging a release the pin will adopt), run the fleet dry-run:

```
make dry-run                    # working tree vs the pinned yze on GOBIN, fleet = ~/src/github.com/gomatic
go run dryrun.go -fleet <dir> -baseline <yze>   # explicit fleet roots / baseline
```

It builds the candidate from this working tree, runs candidate and baseline over every Go module in the fleet (including [`template.cli`](https://github.com/gomatic/template.cli), the sanctioned pattern source), and diffs findings per repo. Any repo GAINING findings fails the dry-run — review each wave: an intended tightening means those repos need fixing before the pin moves; an unintended one means the analyzer needs refinement (an allowlist gap, a pattern exemption) first. Cross-version note: a rewritten diagnostic message shows as a paired +/- for the same site.
