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
  --build-arg BUILDER_IMAGE=golang:1.24 \
  --build-arg RUNNER_IMAGE=gcr.io/distroless/static-debian12 \
  .
```

#### Build Arguments

| Argument | Description | Default |
|----------|-------------|---------|
| `APPLICATION_VERSION` | Version tag for the application | `dev` |
| `BUILDER_IMAGE` | Base image used for building the application | `golang:1.24` |
| `RUNNER_IMAGE` | Base image for the final container | `gcr.io/distroless/static-debian12` |

The build process uses a multi-stage build approach:
1. Uses `BUILDER_IMAGE` to compile the Go application
2. Copies the compiled binary to `RUNNER_IMAGE` for a minimal production image


## Configuration

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
