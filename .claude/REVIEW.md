# Review criteria

Read by the `/review-pr` skill (Scality agent hub) and by anyone reviewing by hand.
Flag problems only — see "What not to flag" at the end.

## What this repo is

`prometheus-postgres-adapter` stores Prometheus time series in PostgreSQL for
long-term retention: it accepts remote-write, serves remote-read, and exposes the
same data to Thanos through a native gRPC StoreAPI. Layers follow the inward-only
rule `cmd → infrastructure → presentation → usecase → domain`. Design decisions
live in `DESIGN.md`.

## Criteria

| Area | What to check |
|------|---------------|
| Error handling | Uses `github.com/scality/go-errors` (imported unaliased as `errors`). New failure categories are package-level `ErrXxx` sentinels; failure sites wrap with `errors.Wrap(ErrXxx, errors.WithProperty(...), errors.CausedBy(rawErr))` so `errors.Is` keeps matching the category through the chain. No `fmt.Errorf`/`%w`. |
| Logging | `log/slog` only. Use the `*Context` methods (`InfoContext`, `ErrorContext`, …) wherever a `context.Context` is in scope; structured key/value attrs (`slog.String`, `slog.Any`, …), never `fmt`-style logging. |
| Context propagation | `context.Context` threaded through call chains and cancellation respected — the ingestion goroutines and the gRPC/HTTP handlers must exit on `ctx` cancel rather than blocking forever. |
| Goroutine lifecycle | The ingestion goroutines (parser × `METRIC_PARSER_COUNT`, saver × `METRIC_WRITER_COUNT`) keep clear exit conditions; `ErrorChan` ownership stays with the saver, which closes it on context cancellation. No leaked goroutines or double-close. |
| Concurrency & single-replica invariant | metric-id allocation stays guarded (a `sync.Map` plus a mutex-protected, double-checked counter). The shared `labelRows`/`valuesRows` buffers have no lock of their own, so the safe defaults are one parser and one writer — flag anything that raises parser/writer concurrency or shares more mutable state without synchronization, and any assumption that more than one replica can run. |
| Layered architecture | Inward-only dependency rule `cmd → infrastructure → presentation → usecase → domain`. `domain` imports no other layer; a use case depends only on the small port interfaces it declares itself, never on a concrete adapter. Flag imports that point outward or skip a layer. |
| SQL safety (querybuilder) | `BuildSQLQuery` composes SQL by string interpolation, so it must only ever receive trusted matcher input from the remote-read / StoreAPI decoders. Verify values stay escaped, that no untrusted/user-controlled path reaches it, and that matcher translation is intact (equality → JSONB containment `@>`, inequality/regex → `metric_labels->>'k'` with anchored `~`/`!~`). |
| Thanos StoreAPI correctness | Series are sorted by label set; labels are built by ranging over the set, NOT via `labelpb.ZLabelsFromPromLabels` (its `unsafe` cast corrupts labels under the default `stringlabels` build); XOR chunks batched at 120 samples; `Info` time-range semantics preserved (empty DB advertises `MaxInt64` min so Thanos skips the store; max is `MaxInt64`); external labels applied to every series with precedence on name collision. |
| Configuration & breaking changes | New/renamed env vars are wired through `cmd/config` and documented; defaults preserved; changes to the `/write` and `/read` wire contracts, the StoreAPI, or the `metric_labels`/`metric_values` schema are backward compatible or explicitly called out. |
| Tests | testify (`assert` / `require`), table-driven with `t.Run` subtests; fakes and mocks are hand-written (no codegen) and live next to the code they exercise. |
| Docs in sync | A behavior/config/deployment change updates `README.md`; an architecture or design-decision change updates `DESIGN.md`; a conventions/workflow change updates `CONTRIBUTING.md`. A change that contradicts the docs must fix them in the same PR. |
| Security | The SQL trust boundary above; no credentials, connection strings, or secrets in code or logs; PostgreSQL connection and SSL-mode handling unchanged unless that is the intent. |

## What not to flag

- Anything the linters already own: `golangci-lint`, `gofmt`, `goimports` —
  formatting, import order, unused variables, naming.
- Markdown or comment wording preferences.
- Refactors unrelated to the PR's purpose.
