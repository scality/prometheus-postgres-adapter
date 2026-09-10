FROM mcr.microsoft.com/devcontainers/go:1.27-trixie@sha256:17bf9e845391f8dfad8d967f203ecd66e2be1c1b67016e56292ea37001edc23f

USER vscode

# renovate: datasource=github-releases depName=golangci/golangci-lint extractVersion=^v(?<version>.+)$
ARG GOLANGCI_VERSION=2.13.2
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
  | sudo sh -s -- v${GOLANGCI_VERSION}
