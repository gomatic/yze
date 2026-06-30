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
