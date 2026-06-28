# yze

The aggregator for the gomatic **yze** analyzer family: it fans in every `yze-<group>-<name>` analyzer and runs them over Go packages through the [`go-yze`](https://github.com/gomatic/go-yze) driver, emitting a normalized report.

```
yze [--format stickler-json|text] [--fix] [--group <g>] [--category <c>...] [packages...]
```

- **`--format stickler-json`** (default) — the native [`Report`](https://github.com/gomatic/go-yze) JSON the [`stickler`](https://github.com/gomatic/stickler) runner consumes with no adapter. `text` prints one human line per finding.
- **`--fix`** — applies analyzers' suggested fixes in place via the shared `go-yze` engine.
- **`--group` / `--category`** — restrict the analyzer set. `group` is the stable name-segment axis (default `go`); `category` is the many-to-many tag axis.

Findings do not by themselves fail the run — that gate belongs to [`stickler`](https://github.com/gomatic/stickler). The current suite: `yze-go-errconst`, `yze-go-gotostmt`, `yze-go-namedtypes`.
