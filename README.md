# yze

The aggregator for the gomatic **yze** analyzer family: it fans in every `yze-<name>` analyzer and runs them over Go packages through the [`go-yze`](https://github.com/gomatic/go-yze) driver, emitting a normalized report. Each analyzer reports under the rule id `yze/<name>`.

```
yze [--format stickler-json|text] [--fix] [--category <c>...] [--config <path>] [packages...]
```

- **`--format stickler-json`** (default) — the native [`Report`](https://github.com/gomatic/go-yze) JSON the [`stickler`](https://github.com/gomatic/stickler) runner consumes with no adapter. `text` prints one human line per finding.
- **`--fix`** — applies analyzers' suggested fixes in place via the shared `go-yze` engine.
- **`--category`** — restricts the analyzer set to those carrying any of the named categories (the many-to-many tag axis). Omit it to run the whole suite.
- **`--config`** — path to a yze config file supplying per-analyzer settings.

Findings do not by themselves fail the run — that gate belongs to [`stickler`](https://github.com/gomatic/stickler). The current suite (14 analyzers): `yze/anonstruct`, `yze/boolname`, `yze/ctxfirst`, `yze/emptyiface`, `yze/errconst`, `yze/errlast`, `yze/gotostmt`, `yze/layout`, `yze/namedtypes`, `yze/pkgstd`, `yze/ptrparam`, `yze/ptrrecv`, `yze/stdlog`, `yze/testfile`.
