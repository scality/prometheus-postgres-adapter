FROM --platform=$BUILDPLATFORM $BUILDER_IMAGE AS builder

ARG BUILDPLATFORM
ARG BUILDER_IMAGE
ARG RUNNER_IMAGE_TAG
ARG TARGETARCH
ARG TARGETOS
ARG APPLICATION_VERSION

WORKDIR /go/src/app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w -X prometheus-postgres-adapter/cmd/config.ApplicationVersion=${APPLICATION_VERSION}" -o prometheus-postgres-adapter ./cmd/main.go

FROM $RUNNER_IMAGE_TAG AS runner

COPY --from=builder /go/src/app/prometheus-postgres-adapter /bin/prometheus-postgres-adapter

ENTRYPOINT ["/bin/prometheus-postgres-adapter"]
