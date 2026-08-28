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
- `DB`/`Close()` — wraps the opened pool. `Session(path)` is the CLI-facing handler. After successful startup validation, `Open` records the validated target's path, device, and inode on the `DB` as the request-boundary health reference (Issue #7; see [session-health.md](session-health.md)).
- Exact-two pool (Issue #5): `SetMaxOpenConns(2)` + `SetMaxIdleConns(2)` after opening; DSN carries `_busy_timeout=5000`, so every physical connection receives the five-second busy timeout at creation. Constants: `poolSize = 2`, `busyTimeoutMillis = 5000`, `sqlMaxLengthBytes = 64 MiB`.
- `DB.Lease(ctx)` / `Lease` type — dedicated lease acquisition: verifies original-path identity before admitting any connection (retained or newly opened/replacement) for use (Issue #7), then checks out one pooled connection and applies `sqlite.Limit(conn, SQLITE_LIMIT_LENGTH, 64 MiB)` before handing it over (the documented modernc mechanism for connection-local limits); concurrent callers get distinct connections, a third blocks until release, release is idempotent-safe, and `Conn()` panics on reuse of a released lease. Carries the narrow `interruptFn` seam (nil in production; tests install hooks).

Driver: pinned exact `modernc.org/sqlite v1.57.0` (pure Go/no-cgo), registered as `"sqlite"`. DSNs build from a percent-encoded `file:` URI so reserved characters in paths stay part of the filename.

### internal/connection/request.go

Reusable cancellable request lifecycle (Issue #6; semantics documented in [cancellation-infrastructure.md](cancellation-infrastructure.md)):

- `Request` — one cancellable database request exclusively owning its lease: process-unique atomic-counter ID, derived cancellable context (`Context()`), at-most-once idempotent `Cancel` that dispatches the connection-scoped interrupt once, observable `State()` (`running`/`cancelling`/`settled`) and `Settled()`, exactly-one `Settle(err) Outcome` with cancellation-wins late-success classification and settle-idempotence, and idempotent `Close` releasing the lease only after settlement. Never force-closes a connection.
- `Outcome`/`RequestState` — typed terminal classifications (`success`, `cancelled`, `failed`) and visible lifecycle states with `String()` renderings.
- `Lease.BeginRequest(parent)` — entry point; `Request.Run(op)` is the synchronous convenience form.

### internal/connection/health.go

Request-boundary database identity checks (Issue #7; semantics documented in [session-health.md](session-health.md)):

- `HealthKind` (`HealthDeleted`, `HealthReplaced`) and `HealthError{Path, Kind, Cause}` — typed classification with neutral diagnostic text only; terminal copy is Issue #46's.
- `VerifyHealth()` — stats the recorded original path and compares both identifiers once per call; nil means unchanged (including same-inode in-place mutation), absence of any kind (deletion, rename-away, or unstatable path) classifies deleted with preserved unwrappable cause, device/inode mismatch classifies replaced.
- `RunRequest(parent, op) RequestResult` — the reusable request boundary: identity verification inside Lease → cancellable op → settlement → post-error reverification where deletion/replacement takes precedence over ordinary error handling while preserving causes; successful results stand until the next boundary detects changes before work.
- No watcher, polling loop, or UI dependency.

### internal/connection/identity_unix.go / identity_other.go

Platform split for identity capture under build tags: Unix reads device/inode from `syscall.Stat_t`; non-Unix reports capture unsupported so verification trivially passes (Sqloid targets Linux/macOS).

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

### internal/connection/schema_test.go (Issue #9)

Connection-boundary behavior of catalog reads: success settles `OutcomeSuccess` with a populated catalog and positive schema version; two concurrent reads lease distinct pooled connections without blocking; an already-cancelled context fails with `context.Canceled` preserved through the refresh wrapper; and a same-path replacement at the boundary classifies typed `HealthReplaced` instead of ordinary SQLite error handling.

### internal/connection/tracer.go (Issue #10, disposable)

Disposable tracer execution path (replaced by Issue #22): `TracerResult{Columns []string, Rows [][]any}` typed transport (`nil`/`int64`/`float64`/`string`/`[]byte` values; copied row slices); `DB.RunTraceSelectAll(parent, target)` executes exactly one hardcoded quoted `SELECT *` from `schema.SelectAllSQL(target)` as one complete `RunRequest` — boundary identity checks, dedicated lease, cancellable read, settlement, post-error reclassification — without schema revalidation or query building. Failures wrap causes in neutral `tracerError` ("could not trace …") and return no result. See [early-integration-tracer.md](early-integration-tracer.md).

### internal/connection/tracer_test.go (Issue #10)

## internal/schema

Schema metadata module (Issue #9), documented in [schema-catalog.md](schema-catalog.md):

### internal/schema/schema.go

UI-independent catalog types: `ObjectKind` (`ordinary-table`/`virtual-table`/`view`), `RowidCapability` (`has-rowid`/`without-rowid`/`not-applicable`), `Object` (name, kind, `WriteEligible`, rowid capability, `RowidShadowed`, `InsertableCount`, declared-order `Columns`), `Column` (name, declared type as pure metadata, hidden/generated visibility, insertability), and `Catalog` (schema version plus name-sorted objects). No driver, Bubble Tea, `internal/ui`, or `internal/querybuilder` dependency; no type-specific input behavior.

### internal/schema/catalog.go

Pure catalog rules over gathered metadata rows: `MasterRow`/`ColumnRow`/`Input` inputs and `BuildCatalog` classification — kind detection (`CREATE VIRTUAL TABLE` prefix), suffix-based `WITHOUT ROWID` detection (trailing semicolon/whitespace/comment tolerated, last two tokens compared case-insensitively), rowid-alias shadowing (`rowid`/`_rowid_`/`oid`, case-insensitive), exclusions of `sqlite_%` (case-insensitive) and `_cf_METADATA`, index/trigger ignoring, ascending-name determinism, and insertability (`hidden==0` plus write eligibility, so views report zero insertable columns).

### internal/connection/schema.go (Issue #9)

In the Connection module: `DB.ReadCatalog(ctx)` runs one `RunRequest` that reads `PRAGMA schema_version`, eligible rows from `main.sqlite_master` (`WHERE type IN ('table','view') ORDER BY name`), and each object's columns from the bound-parameter pragma function `main.pragma_table_xinfo(?)`, then decodes everything with `internal/schema.BuildCatalog`. Failures wrap their cause losslessly in `catalogError` (`could not refresh: <step>: <cause>`) and return the failed `RequestResult`, so health classification (deletion/replacement) wins over ordinary error handling per Issue #7; a cancelled context yields `context.Canceled` preserved through the wrapper. No catalog rules live here — only gathering. See [schema-catalog.md](schema-catalog.md).

### internal/schema/tracer.go (Issue #10, disposable)

Catalog-to-tracer composition seam (replaced by Issue #22): `ChooseTracerTarget(cat, name)` returns the cataloged object (any selected kind — SELECT-only usage) or typed `*TracerError` (`"%q": not present in the refreshed schema catalog`) so execution never runs against stale identifiers; `SelectAllSQL(obj)` renders the one hardcoded `SELECT * FROM "<name>"` with embedded double quotes doubled. No revalidation, parameters, predicates, or builder behavior; see [early-integration-tracer.md](early-integration-tracer.md).

### internal/schema/tracer_test.go / tracer_integration_test.go (Issue #10)

Unit tests for safe identifier quoting (embedded quotes, spaces), catalog selection and typed rejection; Unix-tagged SQLite integration proving the chosen fixture table executes through Connection into typed headers/rows, an unusual identifier (`odd "name`), and a basic failure after a post-selection drop.


## internal/d1

### internal/d1/discovery.go

Local D1 candidate discovery (Issue #3): `Discover()` reads only the immediate entries of the working-directory-relative `.wrangler/state/v3/d1/miniflare-D1DatabaseObject` (`Dir` constant) and applies the PRD's exact rules in `eligible(name)` — case-sensitive `.sqlite` suffix, no lowercase `metadata` substring, no `-wal`/`-shm` sidecars, no recursion, no alternate layouts. Exactly one candidate returns its joined path unchanged; zero returns `ErrNoCandidate`, multiple return `ErrMultipleCandidates`, both typed outcomes for `internal/cli` (exact diagnostics are Issue #4). The package is a pure filesystem scan: it never opens SQLite.

### internal/cli/d1.go

The D1 startup glue (`RunD1`, backed by injectable `runD1With(discover, open)`): requests the sole candidate from `internal/d1.Discover` and passes that path unchanged to the shared `internal/connection.Session`. On any discovery failure, `mapDiscoveryDiagnostic` converts the typed outcomes to the exact Issue #4 process diagnostics — zero candidates become exactly two lines (typed message plus the expected-path and `sqloid sqlite <file>` explicit-open hint), multiple candidates become only the exact single line `There is more than one SQLite database in .wrangler` with no hint — and the opener is never invoked. No D1-specific validation or SQLite opening exists; `cmd/sqloid/main.go` wires `Handlers{SQLite: connection.Session, D1: cli.RunD1}`.
## internal/querybuilder

QueryBuilder module (Issues #11+), documented in [builder-command-table.md](builder-command-table.md):

### internal/querybuilder/builder.go

Package comment and scope: the QueryBuilder module stores command-specific structured state as UI-independent data with immutable value-level transitions. It consumes object kinds and eligibility from `internal/schema` instead of duplicating catalog rules; at this milestone it never imports `internal/ui`, renders no copy, implements popups, or builds SQL.

### internal/querybuilder/command_table.go (Issue #11)

Command and table selection lifecycle: `Command` kind (`CommandUnselected` plus SELECT/UPDATE/DELETE/INSERT, human-facing `String()`), `Field` identity (`FieldCommand`, `FieldTable`), and `QueryBuilder` with `NewQuery()` starting unselected and Command-focused. Transitions: `SelectCommand` replaces the command, bumps `DownstreamGeneration()` (all downstream command-specific state discarded), recomputes eligibility under Schema metadata (`WriteEligible`/kind), retains the selected table only while still eligible, clears it when absent from the latest refresh, and focuses Table; `RefreshSchema` swaps in a fresh catalog snapshot and drops vanished selections; `SelectTable` accepts only names present in the current eligible list; `EligibleTables()` returns every refreshed object for SELECT and only write-eligible kinds for writes.

## internal/ui

### internal/ui/model.go

The top-level Bubble Tea model (Issue #8): `Field` (labeled builder field with counted display lines), `Model` (`Width`/`Height`, `Fields`, `Focus`, `Scroll`, cancellable-request ownership via `ActiveCancellable` plus a `CancelCommand func() tea.Msg` seam, and the unexported suspension copy `suspendedModel`). `Update` handles `tea.WindowSizeMsg` through `resize` — which freezes the entire model unchanged behind the undersized message and restores it exactly on return to supported dimensions after clamping scroll — and contextual key handling in `handleKey`: while suspended only Ctrl+W routes (and only when hidden state owns active cancellable work); otherwise Tab/Shift+Tab/Up/Down move focus then adjust scroll, and Issue #11 routes plain S/U/D/I through `internal/ui/command_table.go` while Command holds focus. Issue #10 adds the isolated disposable tracer state field `Trace *TraceView` plus `StartTraceMsg`/`traceSettledMsg` handling: a start message always returns a command (executor runs inside the command, never in Update); completion stores fully owned fresh `*TraceView{Grid, Err, Settled}` replacing any prior trace (see [early-integration-tracer.md](early-integration-tracer.md)).

### internal/ui/layout.go

Pure region arithmetic with no rendering dependency. `CalculateLayout(totalHeight, fields)` returns footer=1 row, builder desired height inclusive of its 2 border rows and 2 padding rows capped at `floor(H/3)`, results height as every remaining row (> H/2), and `PageRows` as results minus its owned fixed rows (2 border + status/count + frozen header). `BuilderViewport` is interior content lines; `fieldSpans`/`adjustScroll` keep the complete focused field visible inside that viewport; `tooSmall` implements the exact 80×24 threshold; constants pin each region's fixed-row ownership so no border is shared.

### internal/ui/command_table.go (Issue #11)

Builder-field integration: field labels (`Command`, `Table`), plain-key mapping for `s`/`u`/`d`/`i`, and `builderFields(qb.QueryBuilder)` rendering Command always plus Table once any command is chosen. `applyBuilder` installs a transition snapshot, rebuilds the rendered fields, and aligns UI focus with the builder's next required field; `handleCommandKey` consumes one-key selection only while Command holds focus; `SchemaRefreshedMsg{Catalog}` routes refreshed Schema metadata through `applySchemaRefresh`, which keeps focus on the surviving labeled field. No database logic, popups (Issue #12), or idle-prompt rendering changes live here.

### internal/ui/view.go

Deterministic Lip Gloss composition: results box on top owning border/status/header, bordered padded builder below showing the visible field-line window starting at `Scroll` with `>` focused markers, one global footer row last, joining to exactly H rendered rows. While suspended, `View` returns exactly `terminal too small`. Styles are centralized; color never carries meaning alone. With settled tracer state, `renderResults` shows instead the minimal tracer grid (bold pipe-joined header row then pipe-joined data rows) or the plain failure text in the same bordered region — no feature claims beyond Issue #10 scope.

### internal/ui/tracer.go (Issue #10, disposable)

Bubble Tea composition path for the disposable tracer (replaced wholesale by Issue #22): `TraceGrid{Headers, Rows}` string cells; `TraceResult{Grid, Err}` typed completion translated at the composition seam (no connection/driver type crosses into UI); `StartTraceMsg{Execute func(ctx) TraceResult}` whose injected Schema/Connection-facing executor always runs inside a returned command; `traceSettledMsg`; isolated `TraceView{Grid, Err, Settled}` state; `SettledTracer()`; and nil-executor safety. No SQL, handles, or catalog queries here.
