# syntax=docker/dockerfile:1@sha256:87999aa3d42bdc6bea60565083ee17e86d1f3339802f543c0d03998580f9cb89

FROM golang:1.27@sha256:2920bb124f1a2e12a1d39f0cab756ae0068441ee1da7655dec7f5e374944095b AS builder

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

FROM gcr.io/distroless/static:nonroot@sha256:963fa6c544fe5ce420f1f54fb88b6fb01479f054c8056d0f74cc2c6000df5240

LABEL org.opencontainers.image.source=https://github.com/scality/prometheus-postgres-adapter

WORKDIR /
COPY --from=builder /workspace/prometheus-postgres-adapter /prometheus-postgres-adapter

USER 65532:65532

ENTRYPOINT ["/prometheus-postgres-adapter"]
