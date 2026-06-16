ARG BUILDPLATFORM

FROM --platform=$BUILDPLATFORM golang:1.26@sha256:792443b89f65105abba56b9bd5e97f680a80074ac62fc844a584212f8c8102c3 AS builder

ARG TARGETARCH
ARG TARGETOS
ARG APPLICATION_VERSION=dev

WORKDIR /go/src/app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w -X prometheus-postgres-adapter/cmd/config.ApplicationVersion=${APPLICATION_VERSION}" -o prometheus-postgres-adapter ./cmd/main.go

FROM gcr.io/distroless/static-debian12 AS runner

LABEL org.opencontainers.image.source=https://github.com/scality/prometheus-postgres-adapter

COPY --from=builder /go/src/app/prometheus-postgres-adapter /bin/prometheus-postgres-adapter

ENTRYPOINT ["/bin/prometheus-postgres-adapter"]
