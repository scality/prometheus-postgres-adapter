ARG BUILDPLATFORM
ARG BUILDER_IMAGE=golang:1.24
ARG RUNNER_IMAGE=gcr.io/distroless/static-debian12

FROM --platform=$BUILDPLATFORM $BUILDER_IMAGE AS builder

LABEL org.opencontainers.image.source=https://github.com/scality/prometheus-postgres-adapter

ARG TARGETARCH
ARG TARGETOS
ARG APPLICATION_VERSION=dev

WORKDIR /go/src/app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w -X prometheus-postgres-adapter/cmd/config.ApplicationVersion=${APPLICATION_VERSION}" -o prometheus-postgres-adapter ./cmd/main.go

FROM $RUNNER_IMAGE AS runner

COPY --from=builder /go/src/app/prometheus-postgres-adapter /bin/prometheus-postgres-adapter

ENTRYPOINT ["/bin/prometheus-postgres-adapter"]
