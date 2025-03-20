FROM --platform=$BUILDPLATFORM golang:1.24-alpine3.21 as builder

ARG BUILDPLATFORM
ARG TARGETARCH
ARG TARGETOS
ARG APP_VERSION

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w -X prometheus-postgres-adapter/cmd/config.ApplicationVersion=${APP_VERSION}" -o prometheus-postgres-adapter ./cmd/main.go

FROM alpine:3.21

COPY --from=builder /app/prometheus-postgres-adapter /bin/prometheus-postgres-adapter

ENTRYPOINT ["/bin/prometheus-postgres-adapter"]
