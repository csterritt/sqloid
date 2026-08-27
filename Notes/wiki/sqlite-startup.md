# SQLite Startup Validation and Read-Write Opening

How Sqloid starts a session on a SQLite database: ordered pre-open validation, read-write-only opening, the schema probe, and the exact failure diagnostics (Issue #2; `Notes/PRD-sqloid.md` §Startup validation and errors). The CLI shell contract itself lives in [cli-contract.md](cli-contract.md).

## Ownership

`internal/connection` owns SQLite connection startup for both explicit (`sqloid sqlite <file>`) and discovered (D1) paths. It exposes:

- `Open(path) (*DB, error)` — validation + read-write opening + probe; returns `*DB` wrapping the pooled `*sql.DB`.
- `Session(path) error` — CLI-facing handler that opens a session and closes it when the TUI does not consume the handle yet.
- `Lease(ctx) (*Lease, error)` / `(*Lease).Conn` / `(*Lease).Release` — dedicated-connection leasing over the exact-two pool; see [connection-pool.md](connection-pool.md) for ownership, busy timeout, length limits, and journal invariants (Issue #5).
- `StartupError` / `FailureKind` / `DB` / `Close` as the public surface for classification and rendering.

`internal/cli` only renders diagnostics and exit statuses; `cmd/sqloid/main.go` wires `Handlers{SQLite: connection.Session}` and stays a thin entrypoint.

## Validation order (non-mutating)

Open performs exactly these checks in order, failing fast at the first failing step and never creating or modifying the target:

1. **Existence** — `os.Stat`; missing targets produce no file afterwards.
2. **Readability** — open-read of the target proves it can be read at all before content is inspected.
3. **Header check** — directories and any file whose first 16 bytes are not exactly `SQLite format 3\0` are rejected. The header is identified by direct file read, without opening through the driver.
4. **Read-write open** (`mode=rw`) — an OS-level `O_RDWR` open (no create flag) proves genuine writability *before* the driver is involved, because SQLite would silently fall back to `O_RDONLY`. The driver then opens with DSN `file:<path>?mode=rw`, which forbids creating missing databases.
5. **Schema probe** — a harmless `PRAGMA schema_version` query confirms the file is operational. Journal mode is never set or changed.

## Failure classes

| Class | Diagnostic (exactly one stderr line) |
| --- | --- |
| Missing | `<path>: no such file or directory` |
| Unreadable | `<path>: permission denied` |
| Directory / invalid header | `<path>: not a SQLite database` |
| mode=rw EACCES/EPERM | `cannot open database read-write: <path>: permission denied` |
| mode=rw EROFS | `cannot open database read-write: <path>: read-only file system` |
| Other open/probe causes | `cannot open database read-write: <path>: <raw OS/driver cause>` |

Classification is structured, not textual: errors are `*connection.StartupError` with `Path`, `Kind`, and a preserved unwrappable `Cause`, so `errors.Is/As` work. `internal/cli` prints `err.Error()` verbatim via one `Fprintln` and exits 1. Successful startup is silent with status 0. There is deliberately **no read-only fallback**.

## Driver pin

The pure-Go driver is pinned to exact version `modernc.org/sqlite v1.57.0` (added through `go get`, not hand-edited). Production builds stay no-cgo; imports register under database name `"sqlite"`.

## Verification

- `internal/connection/startup_test.go` — table-driven pre-open validation: existence/readability/header rejection classes, mandated ordering (an unreadable invalid-header file reports readability), and snapshot proofs that failing validation neither creates a missing target nor modifies an existing one.
- `internal/connection/opener_test.go` — valid databases open read-write and answer queries; journal mode (`delete` and `wal`) is unchanged by opening; a non-writable valid-header database yields exactly `cannot open database read-write: <path>: permission denied`; EPERM/EACCES/EROFS/raw-driver detail mapping; all diagnostics are single-line.
- `internal/cli/startup_test.go` — real fixtures through `Main`: every failure class prints exactly its documented one line on stderr, nothing on stdout, status 1; valid databases are fully silent with status 0.
- `cmd/sqloid/main_test.go::TestSQLiteStartupProcessBehavior` — process-level behavior with the production connection handler (`SQLOID_CLI_REAL=1`): silent success plus exact stderr lines/status 1.

Cross-references: [cli-contract.md](cli-contract.md), [connection-pool.md](connection-pool.md), [source-code.md](source-code.md), [unit-tests.md](unit-tests.md).
