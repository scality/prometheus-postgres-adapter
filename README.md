# prometheus-postgres-adapter

A Prometheus remote write adapter for PostgreSQL storage.

## Overview

The prometheus-postgres-adapter allows you to store Prometheus metrics data in PostgreSQL.

This adapter implements the Prometheus remote read/write API, enabling long-term storage of time series data in a PostgreSQL database.

## Features

- Store Prometheus metrics in PostgreSQL
- Support for Prometheus remote write/read API
- Configurable metrics concurrent parser / writer

## Installation

### From Source

```bash
go mod download
go build -o prometheus-postgres-adapter
```


### Building with Docker

To build the Docker image manually, use the following command:

```bash
docker build -t prometheus-postgres-adapter:local \
  --build-arg APPLICATION_VERSION=dev \
  --build-arg BUILDER_IMAGE=golang:1.25 \
  --build-arg RUNNER_IMAGE=gcr.io/distroless/static-debian12 \
  .
```

#### Build Arguments

| Argument | Description | Default |
|----------|-------------|---------|
| `APPLICATION_VERSION` | Version tag for the application | `dev` |
| `BUILDER_IMAGE` | Base image used for building the application | `golang:1.25` |
| `RUNNER_IMAGE` | Base image for the final container | `gcr.io/distroless/static-debian12` |

The build process uses a multi-stage build approach:
1. Uses `BUILDER_IMAGE` to compile the Go application
2. Copies the compiled binary to `RUNNER_IMAGE` for a minimal production image


## Configuration

### Database Setup

Before using the adapter, ensure the PostgreSQL database is properly set up with the required schema.
Use the following SQL script to create the necessary tables and indexes:

```sql
CREATE TABLE
IF NOT EXISTS metric_labels
(
  metric_id BIGINT PRIMARY KEY,
  metric_name TEXT NOT NULL,
  metric_name_label TEXT NOT NULL,
  metric_labels jsonb,
  UNIQUE(metric_name, metric_labels)
);

CREATE INDEX
IF NOT EXISTS metric_labels_labels_idx
ON metric_labels
USING GIN (metric_labels);

CREATE TABLE
IF NOT EXISTS metric_values
(
  metric_id BIGINT,
  metric_time TIMESTAMPTZ,
  metric_value FLOAT8
);

CREATE INDEX
IF NOT EXISTS metric_values_id_time_idx
ON metric_values
USING btree
(
  metric_id,
  metric_time DESC
);

CREATE INDEX
IF NOT EXISTS metric_values_time_idx
ON metric_values
USING btree
(
  metric_time DESC
);
```

Run this script on your PostgreSQL database to initialize the schema required by the adapter.

The SQL script creates two tables: `metric_labels` and `metric_values`, along with indexes to optimize database performance:

- **`metric_labels` Table**:
  - Stores metadata about metrics, ensuring a unique combination of `metric_name` and `metric_labels`.
  - **GIN Index**: Applied to the `metric_labels` column for efficient querying of JSONB data, enabling fast searches and filtering based on labels.

- **`metric_values` Table**:
  - Stores time-series data, including metric ID, timestamp, and value.
  - **B-tree Index on `metric_id` and `metric_time DESC`**: Optimizes queries filtering by metric ID and retrieving the most recent data.
  - **B-tree Index on `metric_time DESC`**: Speeds up queries focused on retrieving recent data across all metrics.

These indexes are designed to enhance query performance, particularly for time-series and metadata-heavy workloads.

### Prometheus Setup

A Prometheus instance is needed with remote_read and remote_write configured as shown below:

```yaml
remote_read:
  - url: "http://adapter.service.url:9201/read"
remote_write:
  - url: "http://adapter.service.url:9201/write"
```

[Refer to the Prometheus documentation](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#remote_write)
to further customize Prometheus' remote writing capabilities

### Adapter Configuration

The adapter can be configured using environment variables:

#### Database Connection

````bash
POSTGRESQL_DATABASE_HOST=localhost           # Database host
POSTGRESQL_DATABASE_PORT=5432                # Database port
POSTGRESQL_DATABASE_USER=postgres            # Database user
POSTGRESQL_DATABASE_PASSWORD=password        # Database password
POSTGRESQL_DATABASE_DB=ltm                   # Database name
POSTGRESQL_DATABASE_SSL_MODE=disable         # SSL mode (disable, require, verify-ca, verify-full)
````

#### HTTP Server
```bash
HTTP_PORT=9201                    # Port for the HTTP server
```

#### Processing settings
```bash
METRIC_PARSER_COUNT=1             # Number of concurrent metric parsers
METRIC_WRITER_COUNT=1             # Number of concurrent metric writers
```

#### Logger
```bash
LOG_LEVEL=info                    # Log level (debug, info, warn, error)
```

## Kubernetes Usage Limitation
**Important**: This adapter is not intended to be deployed with multiple replicas in a Kubernetes context.

The application does not include a mechanism to propagate new metrics at runtime across multiple instances.
Running multiple replicas may result in inconsistent metric storage.
