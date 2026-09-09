[![Post Merge](https://github.com/scality/prometheus-postgres-adapter/actions/workflows/post-merge.yaml/badge.svg)](https://github.com/scality/prometheus-postgres-adapter/actions/workflows/post-merge.yaml)
[![GitHub release](https://img.shields.io/github/v/release/scality/prometheus-postgres-adapter)](https://github.com/scality/prometheus-postgres-adapter/releases/latest)
[![Go version](https://img.shields.io/github/go-mod/go-version/scality/prometheus-postgres-adapter)](go.mod)
[![License](https://img.shields.io/github/license/scality/prometheus-postgres-adapter)](LICENSE)

# prometheus-postgres-adapter

A Prometheus remote-write/read adapter that stores time series in PostgreSQL
for long-term retention, and exposes them to Thanos through a native Thanos
StoreAPI.

It lets metrics live in a PostgreSQL database independent of any Prometheus'
local TSDB retention, while staying queryable through the Prometheus and
Thanos read APIs.

## How it works

Prometheus pushes samples to the adapter with `remote_write`. The adapter
decodes them, queues them in-process, and a small parser/writer pipeline bulk-
loads them into PostgreSQL (a `metric_labels` table for the series and a
`metric_values` table for the samples).

Reads are served two ways from the same storage:

- **Prometheus `remote_read`** over HTTP (`/read`).
- **Thanos StoreAPI** over gRPC, so a Thanos Query can federate the long-term
  data directly — without a Prometheus `remote_read` passthrough behind a
  sidecar.

For the architecture and the reasoning behind these choices, see
[DESIGN.md](DESIGN.md).

## Database setup

Supported majors are **16, 17 and 18**: every pull request runs the read
queries against all three, so an engine that reads a matcher differently fails
the build rather than the deployment.

Create the schema before starting the adapter:

```sql
CREATE TABLE IF NOT EXISTS metric_labels (
  metric_id BIGINT PRIMARY KEY,
  metric_name TEXT NOT NULL,
  metric_name_label TEXT NOT NULL,
  metric_labels jsonb,
  UNIQUE(metric_name, metric_labels)
);

CREATE INDEX IF NOT EXISTS metric_labels_labels_idx
  ON metric_labels USING GIN (metric_labels);

CREATE TABLE IF NOT EXISTS metric_values (
  metric_id BIGINT,
  metric_time TIMESTAMPTZ,
  metric_value FLOAT8
);

CREATE INDEX IF NOT EXISTS metric_values_id_time_idx
  ON metric_values USING btree (metric_id, metric_time DESC);

CREATE INDEX IF NOT EXISTS metric_values_time_idx
  ON metric_values USING btree (metric_time DESC);
```

`metric_labels` holds one row per unique series (the GIN index makes JSONB
label lookups fast); `metric_values` holds the samples (the b-tree indexes
speed up per-series and recent-data queries). See
[DESIGN.md](DESIGN.md#storage-model) for details.

## Prometheus setup

Point a Prometheus instance's `remote_write` (and, if you query it directly,
`remote_read`) at the adapter:

```yaml
remote_write:
  - url: "http://adapter.service.url:9201/write"
remote_read:
  - url: "http://adapter.service.url:9201/read"
```

See the [Prometheus documentation](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#remote_write)
to further customize remote writing.

## Thanos integration (StoreAPI)

The adapter exposes a Thanos StoreAPI over gRPC (`GRPC_ADDR`, default
`:10901`). Register it as an endpoint on Thanos Query so it federates the
PostgreSQL data directly:

```bash
thanos query \
  --endpoint=dns+postgresql-prometheus-adapter.<namespace>.svc:10901 \
  ...
```

Through the StoreAPI `Info` call, the adapter advertises a time range starting
at the oldest sample in the database (an empty database is advertised so that
Thanos skips it) and the external labels configured via
`STORE_API_EXTERNAL_LABELS`. Those external labels are applied to every series
the adapter returns, mirroring how a Thanos sidecar applies a Prometheus'
external labels — so set one that identifies this store, e.g.
`STORE_API_EXTERNAL_LABELS=source:postgres-adapter`, and queries can isolate
adapter-served data with `source="postgres-adapter"`.

Expose the gRPC port with a Kubernetes Service:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: postgresql-prometheus-adapter-grpc
spec:
  selector:
    app: postgresql-prometheus-adapter
  ports:
    - name: grpc
      port: 10901
      targetPort: 10901
```

## Configuration

The adapter is configured through environment variables:

| Variable | Description | Default |
| --- | --- | --- |
| `HTTP_ADDR` | Listen address for the HTTP server (`/write`, `/read`, `/health`). | `:9201` |
| `GRPC_ADDR` | Listen address for the gRPC Thanos StoreAPI server. | `:10901` |
| `STORE_API_EXTERNAL_LABELS` | External labels advertised to Thanos, as `key:value,key:value`. | none |
| `POSTGRESQL_DATABASE_HOST` | Database host. | |
| `POSTGRESQL_DATABASE_PORT` | Database port. | |
| `POSTGRESQL_DATABASE_USER` | Database user. | |
| `POSTGRESQL_DATABASE_PASSWORD` | Database password. | |
| `POSTGRESQL_DATABASE_NAME` | Database name. | |
| `POSTGRESQL_DATABASE_SSL_MODE` | SSL mode (`disable`, `require`, `verify-ca`, `verify-full`). | |
| `METRIC_PARSER_COUNT` | Number of concurrent metric parsers. | `1` |
| `METRIC_WRITER_COUNT` | Number of concurrent metric writers. | `1` |
| `LOGGER_LOG_LEVEL` | Log level (`debug`, `info`, `warn`, `error`). | `info` |

> **Single replica only.** Each instance keeps its own in-memory metric-id
> state, so running multiple replicas can produce inconsistent storage. Deploy
> a single replica. See [DESIGN.md](DESIGN.md#concurrency-and-the-single-replica-rule).

## Container image

The image is distroless, statically linked, and runs as a non-root user
(uid `65532`):

```bash
docker run --rm \
  -e POSTGRESQL_DATABASE_HOST=db -e POSTGRESQL_DATABASE_PORT=5432 \
  -e POSTGRESQL_DATABASE_USER=postgres -e POSTGRESQL_DATABASE_PASSWORD=secret \
  -e POSTGRESQL_DATABASE_NAME=ltm -e POSTGRESQL_DATABASE_SSL_MODE=disable \
  -p 9201:9201 -p 10901:10901 \
  ghcr.io/scality/prometheus-postgres-adapter:latest
```

To build it locally:

```bash
docker build --build-arg VERSION=dev -t prometheus-postgres-adapter:local .
```

## Development

Requires Go 1.26+ and [golangci-lint](https://golangci-lint.run/) v2.

```bash
go test ./...        # run the test suite
golangci-lint run    # lint
golangci-lint fmt    # format
go build ./cmd       # build the binary
docker build -t prometheus-postgres-adapter .
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow and the
coding conventions, and [DESIGN.md](DESIGN.md) for how the adapter is built.

## License

See [LICENSE](LICENSE).
