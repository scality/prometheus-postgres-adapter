# Design

This document records how prometheus-postgres-adapter is built and the
decisions behind it. For usage and configuration see the [README](README.md).

## Goals

- Provide **long-term storage** for Prometheus metrics in PostgreSQL through
  the Prometheus remote-write and remote-read APIs.
- Let **Thanos** read that long-term data directly, by exposing a native
  Thanos **StoreAPI** (gRPC) — so a Thanos Query can federate it like any
  other store, without a Prometheus `remote_read` passthrough behind a
  sidecar.
- Stay a small, focused adapter: ingest samples, store them, serve them back.

### Non-goals

- Not a general-purpose TSDB or a Prometheus replacement: no alerting, no
  recording rules, no PromQL evaluation (PromQL runs in Prometheus/Thanos,
  which read through this adapter).
- No downsampling or compaction; samples are stored as written.
- Not horizontally scalable. A single instance owns the in-memory metric-id
  state; see [Concurrency](#concurrency-and-the-single-replica-rule).

## Architecture

The code is layered with an inward-only dependency rule.

```
cmd/                  composition root: config, signals, starts the servers
pkg/
  domain/             entities (Samples, SampleLabels, ...) — no other layer
  usecase/            application logic; depends only on ports it declares
  presentation/       inbound/outbound edges:
                        http/handler  — /write, /read, /health
                        storeapi      — Thanos Store + Info gRPC server
                        database      — PostgreSQL client (pgx)
                        messagequeue  — in-process sample queue
  infrastructure/     metricwriter (ingestion pipeline), querybuilder (SQL),
                        di (the container that wires everything)
```

- **domain** holds the entities and imports no other layer.
- **usecase** holds the application logic (`PushPrometheusSamples`,
  `ReadPrometheusSamples`, `CheckDatabaseHealth`). Each use case depends only
  on small port interfaces it declares itself (e.g. `messageQueuePusher`,
  `SQLQueryBuilder`, `SQLQuerier`), never on a concrete adapter.
- **presentation** holds the edges that talk to the outside world.
- **infrastructure** holds the ingestion pipeline, the SQL builder, and a
  hand-rolled DI container (`pkg/infrastructure/di`) whose lazy getters
  construct and cache each component.
- **cmd** parses the environment config, installs signal handling, and starts
  the HTTP and gRPC servers.

## Ingestion (write path)

Ingestion is a small producer/consumer pipeline so an HTTP request returns as
soon as its samples are queued, decoupled from the database writes.

```
POST /write
  └─ decode snappy + protobuf prompb.WriteRequest
  └─ protoToSamples → domain.Samples
  └─ PushPrometheusSamples.Execute → messagequeue.Chan.Push   (blocks until popped)
                                          │
        ┌─────────────────────────────────┘
        ▼
  parser goroutine × METRIC_PARSER_COUNT      (default 1)
    - Pop() a batch of samples
    - for each sample, transformToMetricString(metric) → "name{\"k\":\"v\", ...}"
    - resolve its metric_id from an in-memory sync.Map; a miss allocates the
      next id from an in-memory counter under a mutex (double-checked), and
      appends a new row to labelRows
    - append (metric_id, time, value) to valuesRows
        │
        ▼
  saver goroutine × METRIC_WRITER_COUNT       (default 1)
    - every 10ms, if there are buffered values:
        - INSERT new label rows (atomic INSERT ... WHERE NOT EXISTS, so a
          duplicate is a no-op)
        - COPY the value rows into metric_values (bulk load)
```

The message queue (`messagequeue.Chan`) is an unbuffered channel, so the push
use case blocks until a parser is ready — natural backpressure.

On startup, `registerExistingMetrics` loads every `metric_name_label → id`
mapping from `metric_labels` into the `sync.Map` and seeds the id counter from
the maximum id found, so ids stay stable and unique across restarts.

Errors from the parser and saver goroutines are reported on an `ErrorChan`
that `main` drains and logs; the saver owns the channel and closes it when the
context is cancelled.

## Querying (read paths)

Both read paths share the same SQL query builder and PostgreSQL querier; they
differ only in their wire protocol.

### Prometheus remote-read (HTTP)

```
POST /read
  └─ decode snappy + protobuf prompb.ReadRequest
  └─ ReadPrometheusSamples.Execute
       └─ querybuilder.BuildSQLQuery(prompb.Query) → SQL
       └─ database.QueryDatabaseSamples → rows
       └─ assemble prompb.ReadResponse (one TimeSeries per label set)
  └─ encode snappy + protobuf
```

### Thanos StoreAPI (gRPC)

The adapter also serves the Thanos StoreAPI so Thanos Query can read it as a
first-class store. `pkg/presentation/storeapi` implements both
`storepb.StoreServer` and `infopb.InfoServer`:

- **Info** advertises the store's metadata: the external labels (see below)
  and a time range whose minimum is the oldest sample in the database
  (`MaxInt64` when empty, so Thanos skips an empty store) and whose maximum is
  `MaxInt64` (so Thanos always considers it for recent queries; it simply
  returns nothing for ranges it has no data for).
- **Series** converts the `SeriesRequest` (matchers + time range) into a
  `prompb.Query`, reuses `BuildSQLQuery` and the querier, groups the rows into
  sorted series, and streams them encoded as XOR chunks.
- **LabelNames / LabelValues** return the distinct label names and values
  (a superset — matchers and time range are not applied, which the StoreAPI
  permits).

The gRPC server also registers the standard `grpc_health_v1` health service
for Kubernetes probes. It runs alongside the HTTP server and both are stopped
gracefully on `SIGINT`/`SIGTERM` (`grpcServer.GracefulStop()` then
`httpServer.Shutdown()`).

Both read paths currently materialise the full result set in memory before
responding: `QueryDatabaseSamples` reads every matching row, then the rows are
grouped, sorted and (for the StoreAPI) XOR-encoded before the first byte is
sent. A wide range query against this long-term store can therefore pull a
large result set into memory at once -- the same characteristic the remote-read
path already had, but easier to trigger now that Thanos Query reaches the store
directly. Streaming per series from an ordered cursor is a planned improvement.

## Storage model

Two tables (see the [README](README.md#database-setup) for the DDL):

- **`metric_labels`** — one row per unique series: `metric_id` (primary key),
  `metric_name`, `metric_name_label` (the full canonical string, `UNIQUE`),
  and `metric_labels` (`jsonb`, with a GIN index for label queries).
- **`metric_values`** — the samples: `metric_id`, `metric_time`
  (`TIMESTAMPTZ`), `metric_value` (`FLOAT8`), indexed by
  `(metric_id, metric_time DESC)` and by `metric_time DESC`.

`querybuilder` turns a `prompb.Query` into SQL joining the two tables: matchers
on `__name__` become conditions on `metric_name`, all four reading the column
bare, since the schema declares it `NOT NULL` -- defending against a NULL on
the equality alone made the answer to one depend on which matcher asked; every
equality label matcher becomes a JSONB containment of its own (`metric_labels
@> '{"k":"v"}'`, which the GIN index serves, and two of them on one label state
the contradiction PromQL answers with nothing); inequality and regex matchers
become `COALESCE(metric_labels->>'k', '')` comparisons (regex via PostgreSQL
`~`/`!~`); the query's time window becomes `metric_time` bounds, clamped to the
years RFC 3339 can write, since Prometheus and Thanos say "no bound" with the
extremes of int64 milliseconds and PostgreSQL stores no such date. Two PromQL
rules shape that translation: a series that does not carry a label is matched
as if it carried it empty, hence the `COALESCE`; and a matcher pattern is
anchored as a whole, `^(?:...)$`, not by appending `^` and `$` to it, which
would bind them to the first and last branch of an alternation only. Values are
escaped, but the builder composes SQL by string interpolation, so it must only
ever be fed trusted matcher input from the remote-read / StoreAPI decoders.

Exposing the StoreAPI widens that surface: PromQL label and regex values from
any Thanos Query client now reach the builder. Two consequences follow. First,
escaping is single-quote doubling, which is injection-safe only while
PostgreSQL runs with `standard_conforming_strings=on` (the default since 9.1) --
do not disable it. Second, regex matchers (`~`/`!~`) hand the client's pattern
straight to PostgreSQL's regex engine over many rows, so a pathological pattern
is a cheap denial-of-service (ReDoS) vector that escaping does not address.
Binding values as query parameters instead of interpolating them is the robust
long-term fix and is tracked as future work.

That engine is also not the one the client wrote its pattern for: PromQL uses
RE2, PostgreSQL uses POSIX ARE, and the builder hands the pattern over as it
is. The two agree on most of what a dashboard writes -- literals, classes,
alternations, groups, bounds -- and part company in ways this adapter does not
yet address:

- the character classes are ASCII in RE2 and Unicode-aware here, so
  `job=~"\w+"` selects `é` here and would not in Prometheus;
- `\b` is a word boundary in PromQL and a literal backspace here, so such a
  matcher selects nothing rather than failing;
- `\z`, `\p`, `\P` and `\Q` are escapes PostgreSQL does not know, an embedded
  option group (`(?i)`) is one it reads only at the very start of an
  expression, and a repetition bound above 255 is one it refuses. Those fail
  the query, which the read paths report as this store breaking rather than as
  the matcher being wrong.

Checking a pattern against RE2 before it is spliced in, translating what the
two engines read differently, and answering the rest as a bad request rather
than an internal error are the fixes, and none of them is here yet.

## Design decisions

### PostgreSQL as the long-term store

The adapter exists so that metrics labelled for long-term retention can live
in a PostgreSQL database that is independent of any Prometheus' local TSDB
retention, while remaining queryable through the Prometheus/Thanos read APIs.
Writes are batched (queue → parser → saver) and values are bulk-loaded with
`COPY`, which is the cheap path for append-only time-series in PostgreSQL.

### Why a native Thanos StoreAPI

A Thanos sidecar only exposes its Prometheus' **local TSDB** time range to
Thanos Query (it advertises `max(--min-time, prometheus_tsdb_lowest_timestamp)`
and Thanos prunes queries to it). It cannot surface data that a Prometheus
serves through its own `remote_read` to this adapter. Exposing the StoreAPI
directly from the adapter makes the PostgreSQL data a first-class Thanos store:
Thanos reads it for the whole range it actually holds, independently of any
Prometheus' local retention, and independently of sidecar version behaviour.

### External labels

Thanos identifies and de-duplicates stores by their external labels. The
adapter applies the labels configured in `STORE_API_EXTERNAL_LABELS` to every
series it returns — with precedence on name collision, exactly as a Thanos
sidecar applies a Prometheus' external labels — and advertises them in `Info`.
Configure a label that identifies *this* store (e.g.
`source=postgres-adapter`) so queries can isolate adapter-served data;
`prometheus_replica`-style labels are handled by Thanos' existing
`--query.replica-label` deduplication.

### XOR chunks

The StoreAPI returns samples as XOR-encoded chunks (Prometheus'
`tsdb/chunkenc`), the format Thanos expects, batched at 120 samples per chunk.
Series are sorted by label set, as the StoreAPI requires. Labels are converted
to `labelpb.ZLabel` by ranging over the set rather than through
`labelpb.ZLabelsFromPromLabels`, whose `unsafe` cast assumes the slice-based
labels representation and corrupts labels under Prometheus' default
`stringlabels` build.

### Concurrency and the single-replica rule

Within one instance the metric-id allocation is guarded (a `sync.Map` plus a
mutex-protected counter), but the buffered `labelRows`/`valuesRows` slices are
shared between the parser and saver goroutines without their own lock, so the
defaults of one parser and one writer (`METRIC_PARSER_COUNT=1`,
`METRIC_WRITER_COUNT=1`) are the safe configuration.

Across instances there is no coordination: each instance keeps its own
in-memory `metric_name_label → id` map and id counter, so running multiple
replicas would let them allocate conflicting ids for new metrics. **Deploy a
single replica.**
