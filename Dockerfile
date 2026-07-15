# syntax=docker/dockerfile:1@sha256:87999aa3d42bdc6bea60565083ee17e86d1f3339802f543c0d03998580f9cb89

FROM golang:1.26@sha256:792443b89f65105abba56b9bd5e97f680a80074ac62fc844a584212f8c8102c3 AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION="dev"

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY pkg/ pkg/

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build \
    -ldflags "-X prometheus-postgres-adapter/cmd/config.ApplicationVersion=${VERSION}" \
    -o prometheus-postgres-adapter ./cmd

FROM gcr.io/distroless/static:nonroot@sha256:f7f8f729987ad0fdf6b05eeeae94b26e6a0f613bdf46feea7fc40f7bd72953e6

LABEL org.opencontainers.image.source=https://github.com/scality/prometheus-postgres-adapter

WORKDIR /
COPY --from=builder /workspace/prometheus-postgres-adapter /prometheus-postgres-adapter

USER 65532:65532

ENTRYPOINT ["/prometheus-postgres-adapter"]
