# Contributing

Thanks for contributing to prometheus-postgres-adapter. This guide covers the
workflow and the conventions the codebase follows. For what the adapter does
see the [README](README.md); for how it is built and why see
[DESIGN.md](DESIGN.md).

## Development environment

You need Go 1.26+ and [golangci-lint](https://golangci-lint.run/) v2 on your
`PATH` (the CI pins the exact version). A reachable PostgreSQL instance is
required to run the adapter, but not to build it or run the unit tests.

The everyday commands are listed in the README's
[Development](README.md#development) section.

## Architecture

The project uses a layered architecture with an inward-only dependency rule
(`cmd → infrastructure → presentation → usecase → domain`). Before adding a
type, read [DESIGN.md](DESIGN.md#architecture) and place it in the layer that
owns its responsibility:

- entities go in `domain` (it depends on no other layer);
- application logic is a use case in `usecase`; it depends only on small port
  interfaces it declares itself, never on a concrete adapter;
- inbound/outbound edges — the HTTP handlers, the gRPC StoreAPI server, the
  PostgreSQL client, the message queue — live in `presentation`;
- the metric-writer pipeline, the SQL query builder, and the DI container live
  in `infrastructure`.

## Coding conventions

- **Logging**: standard-library `log/slog`. Use the `*Context` methods
  (`InfoContext`, `ErrorContext`, …) wherever a `context.Context` is in scope;
  structured key/value attributes, never `fmt`-style logging.
- **Errors**: [`github.com/scality/go-errors`](https://github.com/scality/go-errors),
  imported unaliased as `errors`. Define package-level `ErrXxx` sentinels and
  wrap at the failure site with
  `errors.Wrap(ErrXxx, errors.WithProperty(...), errors.CausedBy(rawErr))` so
  `errors.Is` keeps matching the category through the chain. Do not use
  `fmt.Errorf`/`%w`.
- **Tests**: [testify](https://github.com/stretchr/testify) (`assert` /
  `require`), table-driven with `t.Run` subtests; fakes and mocks are written
  by hand (no codegen) and live next to the code they exercise.
- **Linting & formatting**: `golangci-lint run` must pass; format with
  `golangci-lint fmt`.

## Commits and pull requests

- Conventional-commit subjects with an explicit action verb, e.g.
  `feat(storeapi): add …`, `fix(metricwriter): …`, `chore: …`,
  `refactor: …`.
- Keep each commit a single coherent change, and keep PRs focused.
- When the change is tied to a Jira ticket, add a trailing `Issue: <TICKET>`
  footer. Chores and refactors that are not tied to a ticket omit it.
- Make sure `go test ./...` and `golangci-lint run` pass before opening a PR.

## Keep the docs in sync

Treat the docs as part of the change, not an afterthought. In the same PR:

- a change to behavior, configuration, or deployment → update the
  [README](README.md);
- a change to architecture or a design decision → update
  [DESIGN.md](DESIGN.md);
- a change to conventions or workflow → update this file.

## License

By contributing you agree your contribution is licensed under the repository's
[LICENSE](LICENSE).
