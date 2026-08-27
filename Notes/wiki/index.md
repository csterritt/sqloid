# Sqloid Wiki Index

Content-oriented catalog of all wiki pages.

## Project

- [project-overview.md](project-overview.md) — high-level description of Sqloid, its stack, and module architecture.

## Source code

- [source-code.md](source-code.md) — catalog and summaries of all source files under `cmd/` and `internal/`.

## Concepts

- [cli-contract.md](cli-contract.md) — the full command-line contract: commands, flags, stream ownership, exit statuses, and silent successful startup.
- [d1-discovery.md](d1-discovery.md) — Issue #3 local D1 candidate discovery: the exact Wrangler directory, case-sensitive `.sqlite` rule, lowercase `metadata` and `-wal`/`-shm` exclusions, exact zero/multiple-candidate failure diagnostics (Issue #4), cardinality outcomes, and the `internal/d1` → `internal/cli` → `internal/connection` handoff.
- [sqlite-startup.md](sqlite-startup.md) — Issue #2 startup pipeline: pre-open validation order, failure classes, one-line diagnostics, mode=rw opening, schema probe, journal-mode preservation, no creation/modification, and no read-only fallback.
- [connection-pool.md](connection-pool.md) — Issue #5 exact-two pool and dedicated leasing: minimum/maximum pool size of two, lease lifecycle and release safety, five-second busy timeout on every connection, exact 64 MiB connection-local SQLITE_LIMIT_LENGTH, and unchanged WAL/rollback-journal modes.
- [cancellation-infrastructure.md](cancellation-infrastructure.md) — Issue #6 cancellable request lifecycle: unique request identity, context ownership, idempotent Cancel with exactly-one connection-scoped interrupt dispatch, cancelling-until-settlement visibility, cancellation-wins late-success classification, lease release only after settlement, no force-close with safe reuse, the one-second CPU and five-second lock-wait bounds on Linux/macOS, and the modernc v1.57.0 assumptions these depend on (SELECT/write/UI wiring deferred to Issue #28).
- [session-health.md](session-health.md) — Issue #7 request-boundary database identity checks: startup device/inode recording on Linux/macOS, verification before every request and before any newly opened/replacement pooled connection is admitted for use, typed `HealthDeleted`/`HealthReplaced` outcomes with preserved causes, rename-away as absence, same-inode in-place mutation as ordinary SQLite behavior, post-error reclassification precedence, successful-result-stands race handling, exactly one pre-BEGIN check per phased write, no watcher/polling/UI dependency, and terminal copy deferred to Issue #46.

- [responsive-tui-shell.md](responsive-tui-shell.md) — Issue #8 responsive Bubble Tea shell: pinned Bubble Tea/Lip Gloss versions, one-row global footer, builder desired border/padding-inclusive height capped at floor(H/3), all remaining rows to an independently bordered results region exceeding half-height, complete-row page-area arithmetic, focused-field internal scrolling, the exact 80×24 minimum, exact `terminal too small` suspension with ignored input, conditional Ctrl+W cancellation routing, and exact state restoration on resize.

## Tests

- [unit-tests.md](unit-tests.md) — catalog and summaries of the Go unit and process-boundary tests.
