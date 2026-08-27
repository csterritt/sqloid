# Wiki Log

## [2026-08-26] ingest | Issue #1 CLI shell and usage contracts

Initial wiki setup and ingest of the Issue #1 implementation: created `project-overview.md`, `source-code.md`, `cli-contract.md`, and `unit-tests.md`; documented the `mow.cli` command structure, the `sqlite <file>` and `d1` commands, help/version flags, exact stream ownership, exit statuses (0/2), silent successful startup, and the `internal/cli` vs `cmd/sqloid` roles. Updated `index.md`.
## [2026-08-26] ingest | Issue #2 SQLite startup validation and read-write errors

Ingested the Issue #2 implementation: pinned `modernc.org/sqlite v1.57.0`; created `internal/connection` (`Open`, `Session`, structured `StartupError` classes) with ordered non-mutating pre-open validation (existence → readability → exact header → writability proof → mode=rw open → schema probe); connected the sqlite handler in `internal/cli` to render exact one-line diagnostics with status 1 while `cmd/sqloid` stayed a thin entrypoint. Added [sqlite-startup.md](sqlite-startup.md); updated `cli-contract.md`, `source-code.md`, `unit-tests.md`, and `index.md`.
## [2026-08-26] ingest | Issue #3 local D1 candidate discovery

Ingested the Issue #3 implementation: created `internal/d1` (`Discover`, exact-rule `eligible` filter, typed `ErrNoCandidate`/`ErrMultipleCandidates` sentinels) scanning only `.wrangler/state/v3/d1/miniflare-D1DatabaseObject` non-recursively; wired `internal/cli.RunD1` to pass the sole candidate unchanged to `internal/connection.Session` with no D1-specific opening path; fixed `mustFileURL` so relative paths render as opaque `file:` URIs (no invented URI authority). Created [d1-discovery.md](d1-discovery.md); updated `cli-contract.md`, `source-code.md`, `unit-tests.md`, and `index.md`.
## [2026-08-26] ingest | Issue #4 exact D1 discovery diagnostics

Ingested the Issue #4 implementation and golden tests: `internal/cli.mapDiscoveryDiagnostic` now maps typed `internal/d1` outcomes to exact process diagnostics — zero candidates (missing, unreadable, empty, or candidate-free Wrangler directory) emit exactly two stderr lines (`no candidate database found in .wrangler` plus the expected-path and `sqloid sqlite <file>` explicit-open hint), multiple candidates emit only `There is more than one SQLite database in .wrangler` with no hint; both bypass the opener, exit 1, and create nothing. Added discovery-failure section to [d1-discovery.md](d1-discovery.md); updated `cli-contract.md`, `source-code.md`, `unit-tests.md`, and `index.md`.
