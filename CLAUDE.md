# CLAUDE.md

Working guide for prometheus-postgres-adapter. The project's documentation is
imported below so it is always in context — read it and follow it instead of
rediscovering the conventions each time.

- Usage, configuration, and deployment: @README.md
- Architecture, the storage model, and design decisions: @DESIGN.md
- Workflow and coding conventions: @CONTRIBUTING.md

## Keep the docs in sync

The docs are part of every change, not an afterthought. In the same PR,
update the doc that owns what you changed:

- behavior, configuration, or deployment → @README.md
- architecture or a design decision → @DESIGN.md
- conventions or workflow → @CONTRIBUTING.md

If a change contradicts something written in these docs, fix the docs rather
than leaving them stale.
