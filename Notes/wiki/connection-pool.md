# Connection Pool and Dedicated Leasing

How `internal/connection` owns the exact-two SQLite pool, dedicated leases, per-connection busy handling and length limits, and journal-mode preservation (Issue #5; `Notes/PRD-sqloid.md` §Connection pool, limits, and busy handling). Startup validation itself lives in [sqlite-startup.md](sqlite-startup.md).

## Pool ownership

`Open` owns the pool: after validation succeeds it configures the wrapped `*sql.DB` with `SetMaxOpenConns(2)` and `SetMaxIdleConns(2)`, so the pool's minimum and maximum are both exactly two (constant `poolSize`). Two connections exist because two concurrent page/count requests must each lease a distinct physical connection without serializing or growing beyond two. Closing the session (`DB.Close`) releases the whole pool regardless of outstanding leases.

## Lease lifecycle

- `DB.Lease(ctx) (*Lease, error)` checks out one dedicated connection from the pool. Concurrent callers receive distinct physical connections; a third concurrent lease blocks until one of the two is released — there is no hidden serialization of independent leases.
- Every lease is configured before any query runs: `sqlite.Limit(conn, SQLITE_LIMIT_LENGTH, 64 MiB)` applies the exact 67108864-byte connection-local length limit. Limits are inherently connection-local and have no DSN equivalent; leasing through `*sql.Conn` is the documented modernc mechanism (`Conn()` exposes the underlying connection).
- `Lease.Conn()` runs the caller's work on that exclusive connection. It panics if called after `Release`, refusing reuse of a released lease (reusing one would silently run against a different pooled connection).
- `Lease.Release(ctx)` returns the connection to the pool and is idempotent-safe on success, error, and repeated calls; a released lease can no longer be observed through `Conn()`. Release closes the connection back into the pool rather than destroying it, so both leased connections remain pooled as idle afterward.

## Busy handling and length limits

| Invariant | Value | Mechanism |
| --- | --- | --- |
| Busy timeout | exactly 5000 ms | DSN parameter `_busy_timeout=5000`; travels with every physical connection the pool creates |
| `SQLITE_LIMIT_LENGTH` | exactly 67108864 bytes (64 MiB) | `sqlite.Limit` on each lease, applied at acquisition before any work |

Both values are constants in `internal/connection`. Neither modifies the database file: the busy timeout is connection state, and the length limit is reset whenever the driver would recycle the connection — Sqloid re-applies it on every lease acquisition.

## Journal invariants

WAL and rollback-journal modes are neither set nor changed anywhere in `Connection`: the opener records fixture mode before opening, and opening plus concurrent lease use leaves `PRAGMA journal_mode` identical (`delete` fixtures stay `delete`, `wal` stays `wal`). No journal pragma appears in any DSN, hook, or lease configuration.

## Verification

- `internal/connection/pool_config_test.go`
  - `TestPoolHoldsExactlyTwoUsableConnections` — maximum pool size pins to 2; two concurrent goroutines acquire distinct leases simultaneously within an explicit bound; each lease answers `PRAGMA schema_version` while the other is held; after release the pool retains both as idle (floor of two).
  - `TestEveryConnectionHasFiveSecondBusyTimeout` / `TestEveryConnectionHasExactLengthLimit` — inspect individual leases via helpers over concurrent acquisition, reading back `PRAGMA busy_timeout` and `SQLITE_LIMIT_LENGTH` (-1 query form), with failure messages naming the absent invariant.
- `internal/connection/lease_test.go`
  - `TestConcurrentLeasesAreDistinctConnections/{delete,wal}` — barrier-driven (channel synchronization; explicit bound only for deadlock detection): fixtures pre-placed in rollback-journal and WAL mode, journal mode recorded before open, both leases held concurrently, underlying driver connections compared for pointer identity (distinctness), each lease's busy timeout and 64 MiB limit verified, journal mode re-checked unchanged after use.
  - `TestLeaseReleaseIsSafeAndRefusesReuse` — release after successful use, repeated-release safety, and `Conn()` panicking with the released-lease message on reuse.
- Race-detector run over `internal/connection` passes (`CGO_ENABLED=1 go test -race ./internal/connection/`).

Cross-references: [sqlite-startup.md](sqlite-startup.md), [source-code.md](source-code.md), [unit-tests.md](unit-tests.md).
