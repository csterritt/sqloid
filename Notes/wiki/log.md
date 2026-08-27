# Wiki Log

## [2026-08-26] ingest | Issue #1 CLI shell and usage contracts

Initial wiki setup and ingest of the Issue #1 implementation: created `project-overview.md`, `source-code.md`, `cli-contract.md`, and `unit-tests.md`; documented the `mow.cli` command structure, the `sqlite <file>` and `d1` commands, help/version flags, exact stream ownership, exit statuses (0/2), silent successful startup, and the `internal/cli` vs `cmd/sqloid` roles. Updated `index.md`.
## [2026-08-26] ingest | Issue #2 SQLite startup validation and read-write errors

Ingested the Issue #2 implementation: pinned `modernc.org/sqlite v1.57.0`; created `internal/connection` (`Open`, `Session`, structured `StartupError` classes) with ordered non-mutating pre-open validation (existence → readability → exact header → writability proof → mode=rw open → schema probe); connected the sqlite handler in `internal/cli` to render exact one-line diagnostics with status 1 while `cmd/sqloid` stayed a thin entrypoint. Added [sqlite-startup.md](sqlite-startup.md); updated `cli-contract.md`, `source-code.md`, `unit-tests.md`, and `index.md`.
