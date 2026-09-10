FROM mcr.microsoft.com/devcontainers/go:1.26-trixie@sha256:232b16db710cedd1ad955e78ef973d6252ddc1149f299b1eaeb1caaa8c61caf8

USER vscode

# renovate: datasource=github-releases depName=golangci/golangci-lint extractVersion=^v(?<version>.+)$
ARG GOLANGCI_VERSION=2.13.2
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
  | sudo sh -s -- v${GOLANGCI_VERSION}
