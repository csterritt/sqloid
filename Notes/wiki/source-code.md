# Source Code Catalog

Summaries of all source files under `cmd/` and `internal/`.

## cmd/sqloid

### cmd/sqloid/main.go

The executable entrypoint and thin process boundary. Maps `os.Exit(cli.Main(os.Args, handlers))` with `handlers.SQLite = connection.Session`, connecting the shell to real database startup; all other construction and dispatch live in `internal/cli`. `cmd/sqloid/main_test.go` re-executes the test binary as the CLI to assert exact streams and exit statuses, including the production connection path via `SQLOID_CLI_REAL=1` (see [unit-tests.md](unit-tests.md)).

## internal/cli

### internal/cli/cli.go

The mow.cli command shell (PRD-mandated structure):

- `Version` — build version string (`dev` by default; overridable with `-ldflags -X ...cli.Version=...`).
- `Handlers` — injectable `SQLite(path) error` and `D1() error` functions invoked after successful routing; nil handlers are no-ops so the shell is testable before `internal/connection`/`internal/d1` exist.
- `New(h)` — builds the app: `sqlite` command with `Spec = "FILE"` and one string argument, `d1` command, `--version`/`-v` bool flag, root action printing the version line to stdout or long help. Sets `flag.ContinueOnError` so mow.cli returns usage errors instead of exiting directly, keeping the exit-status decision inside the package.
- `Main(args, h) int` — runs the app and maps a usage-error return onto status 2; successful dispatch returns 0 with no CLI-authored output. Returns the status instead of calling `os.Exit` so tests and the entrypoint control termination.

Cross-references: [cli-contract.md](cli-contract.md), [project-overview.md](project-overview.md).

## internal/connection

### internal/connection/startup.go

Owns SQLite connection startup for explicit and discovered paths (Issue #2):

- `FailureKind` — structured failure classes: `FailureMissing`, `FailureUnreadable`, `FailureNotADatabase`, `FailureReadWrite`.
- `StartupError` — carries `Path`, `Kind`, and a preserved unwrappable `Cause`; `Error()` already produces the exact one-line diagnostics rendered verbatim by `internal/cli` (see [sqlite-startup.md](sqlite-startup.md) for the message table).
- `Open(path)` — ordered, non-mutating pre-open validation: existence → readability → exact 16-byte `SQLite format 3\0` header (no driver involvement) → OS-level writability proof (`O_RDWR` open; prevents the driver's silent O_RDONLY fallback) → driver open with DSN `file:<path>?mode=rw` (never creates) → harmless `PRAGMA schema_version` probe. Journal mode is never changed; there is no read-only fallback.
- `DB`/`Close()` — wraps the opened pool. `Session(path)` is the CLI-facing handler.
- Exact-two pool (Issue #5): `SetMaxOpenConns(2)` + `SetMaxIdleConns(2)` after opening; DSN carries `_busy_timeout=5000`, so every physical connection receives the five-second busy timeout at creation. Constants: `poolSize = 2`, `busyTimeoutMillis = 5000`, `sqlMaxLengthBytes = 64 MiB`.
- `DB.Lease(ctx)` / `Lease` type — dedicated lease acquisition: checks out one pooled connection and applies `sqlite.Limit(conn, SQLITE_LIMIT_LENGTH, 64 MiB)` before handing it over (the documented modernc mechanism for connection-local limits); concurrent callers get distinct connections, a third blocks until release, release is idempotent-safe, and `Conn()` panics on reuse of a released lease. Carries the narrow `interruptFn` seam (nil in production; tests install hooks).

Driver: pinned exact `modernc.org/sqlite v1.57.0` (pure Go/no-cgo), registered as `"sqlite"`. DSNs build from a percent-encoded `file:` URI so reserved characters in paths stay part of the filename.

### internal/connection/request.go

Reusable cancellable request lifecycle (Issue #6; semantics documented in [cancellation-infrastructure.md](cancellation-infrastructure.md)):

- `Request` — one cancellable database request exclusively owning its lease: process-unique atomic-counter ID, derived cancellable context (`Context()`), at-most-once idempotent `Cancel` that dispatches the connection-scoped interrupt once, observable `State()` (`running`/`cancelling`/`settled`) and `Settled()`, exactly-one `Settle(err) Outcome` with cancellation-wins late-success classification and settle-idempotence, and idempotent `Close` releasing the lease only after settlement. Never force-closes a connection.
- `Outcome`/`RequestState` — typed terminal classifications (`success`, `cancelled`, `failed`) and visible lifecycle states with `String()` renderings.
- `Lease.BeginRequest(parent)` — entry point; `Request.Run(op)` is the synchronous convenience form.

### internal/connection/startup_test.go

Table-driven pre-open validation: missing file, directory, invalid/corrupt/short headers, and readability-before-header ordering (an unreadable invalid-header file classifies as unreadable). Every case snapshots size/mode/mtime/body before and after to prove pre-open validation neither creates a missing target nor modifies an existing one.

### internal/connection/pool_config_test.go (Issue #5)

Pool and per-connection configuration integration tests: maximum pool size pinned to 2; two concurrent leases acquired simultaneously within an explicit bound with both usable under load (`InUse`/`OpenConnections` = 2); released pool retains both connections as idle (floor of two); every inspected connection carries exactly the 5000 ms busy timeout and exactly 67108864-byte `SQLITE_LIMIT_LENGTH` (read back via the -1 query form).

### internal/connection/lease_test.go (Issue #5)

Barrier-driven dedicated-leasing tests against fixtures pre-placed in rollback-journal (`delete`) and WAL mode: two goroutines hold leases concurrently, underlying driver connections are compared by pointer identity to prove distinctness; each lease answers `PRAGMA schema_version`, has the five-second busy timeout and exact 64 MiB limit while held; journal mode recorded before open is unchanged after use. Release-safety test covers successful release, repeated-release idempotence, and the panic guarding reuse of a released lease.

### internal/connection/request_test.go (Issue #6)

Fake-backed lifecycle tests without a live driver: unique IDs and context ownership; eight racing Cancellers dispatching exactly one interrupt via a fake hook plus post-settlement idempotence; visible cancelling-until-settlement state; late-success discarded as cancelled; error classification matrix; real-pool lease-hold-before-settlement with third-lease refusal; no-force-close and safe same-lease/released-pool reuse after settled cancellation.

### internal/connection/interrupt_unix_test.go (Issue #6)

Linux/macOS mandatory barrier-based capability tests against modernc v1.57.0 (release-blocking): CPU-bound 200k-row probe scan settles under one second of cancellation; lock-wait INSERT blocked behind a held SHARED read lock settles within the five-second bound; independent second-lease isolation during interruption; deliberately released late success classified cancelled and discarded; subsequent harmless work on the interrupted physical connection and pool reuse prove no force-close.

### internal/connection/opener_test.go

Opener integration: real databases open read-write and answer queries; journal mode (`delete` and `wal`) is unchanged by opening (`TestOpenPreservesJournalMode`); non-writable valid-header databases yield exactly `cannot open database read-write: <path>: permission denied`; direct classifier tests pin EACCES→`permission denied`, EPERM→`permission denied`, EROFS→`read-only file system`, and raw driver causes preserved verbatim; all diagnostics stay single-line.

## internal/d1

### internal/d1/discovery.go

Local D1 candidate discovery (Issue #3): `Discover()` reads only the immediate entries of the working-directory-relative `.wrangler/state/v3/d1/miniflare-D1DatabaseObject` (`Dir` constant) and applies the PRD's exact rules in `eligible(name)` — case-sensitive `.sqlite` suffix, no lowercase `metadata` substring, no `-wal`/`-shm` sidecars, no recursion, no alternate layouts. Exactly one candidate returns its joined path unchanged; zero returns `ErrNoCandidate`, multiple return `ErrMultipleCandidates`, both typed outcomes for `internal/cli` (exact diagnostics are Issue #4). The package is a pure filesystem scan: it never opens SQLite.

### internal/cli/d1.go

The D1 startup glue (`RunD1`, backed by injectable `runD1With(discover, open)`): requests the sole candidate from `internal/d1.Discover` and passes that path unchanged to the shared `internal/connection.Session`. On any discovery failure, `mapDiscoveryDiagnostic` converts the typed outcomes to the exact Issue #4 process diagnostics — zero candidates become exactly two lines (typed message plus the expected-path and `sqloid sqlite <file>` explicit-open hint), multiple candidates become only the exact single line `There is more than one SQLite database in .wrangler` with no hint — and the opener is never invoked. No D1-specific validation or SQLite opening exists; `cmd/sqloid/main.go` wires `Handlers{SQLite: connection.Session, D1: cli.RunD1}`.
