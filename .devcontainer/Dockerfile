FROM mcr.microsoft.com/devcontainers/go:1.27-trixie@sha256:c83c320b68e27dc36903cd5ee07394870c5330dfd77b0897e78c7e1aa00ddfd0
# The devcontainers image tag has no Go patch version, and the image sets
# GOTOOLCHAIN=local. Take Go from the Dockerfile's golang image, so the
# patch is pinned and matches go.mod.
COPY --from=golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 /usr/local/go /usr/local/go

USER vscode

# renovate: datasource=github-releases depName=golangci/golangci-lint extractVersion=^v(?<version>.+)$
ARG GOLANGCI_VERSION=2.15.0
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
  | sudo sh -s -- v${GOLANGCI_VERSION}
