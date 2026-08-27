# Sqloid Wiki Index

Content-oriented catalog of all wiki pages.

## Project

- [project-overview.md](project-overview.md) — high-level description of Sqloid, its stack, and module architecture.

## Source code

- [source-code.md](source-code.md) — catalog and summaries of all source files under `cmd/` and `internal/`.

## Concepts

- [cli-contract.md](cli-contract.md) — the full command-line contract: commands, flags, stream ownership, exit statuses, and silent successful startup.
- [d1-discovery.md](d1-discovery.md) — Issue #3 local D1 candidate discovery: the exact Wrangler directory, case-sensitive `.sqlite` rule, lowercase `metadata` and `-wal`/`-shm` exclusions, zero/one/multiple cardinality, and the `internal/d1` → `internal/cli` → `internal/connection` handoff.
- [sqlite-startup.md](sqlite-startup.md) — Issue #2 startup pipeline: pre-open validation order, failure classes, one-line diagnostics, mode=rw opening, schema probe, journal-mode preservation, no creation/modification, and no read-only fallback.

## Tests

- [unit-tests.md](unit-tests.md) — catalog and summaries of the Go unit and process-boundary tests.
