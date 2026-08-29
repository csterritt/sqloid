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

### internal/connection/firstpage.go (Issue #22)

Production first-page SELECT execution, documented in [first-select-result-grid.md](first-select-result-grid.md): `DB.RunFirstPage(parent, statement, params)` runs exactly one bound SELECT — always the QueryBuilder-rendered SQL and ordered parameters — as one complete `RunRequest` on its own dedicated lease, applying the Issue #6 request lifecycle and Issue #7 health classification unchanged. Eager scanning copies each row once and converts exactly once into the shared `internal/result` representation via `result.FromDriver`; failures wrap causes in neutral `firstPageError` ("could not run select — …") and return no page. No count request, later paging, cache, or finalization behavior.

### internal/connection/count.go (Issue #24)

Independent complete-SELECT result count execution, documented in [concurrent-page-count.md](concurrent-page-count.md): `DB.RunCount(parent, statement, params)` runs exactly one bound count statement — always the complete-SELECT subquery with the user's Limit inside it — as one complete `RunRequest` on its own dedicated leased connection, concurrent with (never serialized behind) the first page as an independent autocommit read. Failures wrap causes in neutral `countError` ("could not run count — …") and return the total zero plus the failed `RequestResult`; a count failure is never reinterpreted as a page failure.

### internal/connection/count_overlap_test.go (Issue #24)

Mandatory release- and dependency-upgrade-blocking capability suite: WAL and rollback-journal fixtures with mode recorded before opening, simultaneous barrier-held overlap via the documented test-only hooks (`DB.beforeFirstPage`/`DB.beforeCount`), two distinct physical connections from the exact-two pool, third-lease blocking, unchanged journal mode, independent-snapshot drift across an interleaved external writer, rollback-journal delay or ordinary `database is locked` error, count-failure isolation, and clean lease release on success and error.

### internal/connection/firstpage_test.go (Issue #22)

SQLite-backed coverage of the first-page boundary: typed NULL/INTEGER/REAL/TEXT/BLOB rows with original driver labels crossing into `result.Page` without string coercion, typed NULLs, zero rows, an ordinary query failure (`no such table` cause preserved, no terminal claim), ordered parameter binding, a pre-cancelled context, and scan-level typing without loss.

### internal/connection/page.go and page_test.go (Issue #25)

Paged-page SELECT execution, documented in [serialized-vertical-paging.md](serialized-vertical-paging.md): `DB.ExecutePage(parent, statement, params)` runs exactly one bound page statement — already carrying its exact LIMIT/OFFSET range from the QueryBuilder page seam — as one complete `RunRequest` on its own dedicated leased connection, with the same eager typed scanning and single `result.FromDriver` conversion as a first page and causes preserved through the neutral `firstPageError` wrapper. No adjacent-offset arithmetic, serialization, or caching lives here. The SQLite-backed tests cover adjacent disjoint ranges, an offset beyond the data as a typed empty page, and ordinary failure classification with the driver cause preserved.

### internal/connection/started_request.go (Issue #28)

Externally cancellable started-request handles for SELECT page and count work, documented in [scoped-select-cancellation.md](scoped-select-cancellation.md): `StartFirstPage`/`StartPage`/`StartCount` each lease a dedicated connection, begin an Issue #6 `Request`, and run the statement in a separate goroutine so cancellation is requested from another goroutine; `Cancel` is idempotent and connection-scoped, the lease releases only at true settlement, `Wait`/`SettledChan` expose terminal classification, lease-acquisition failure pre-settles as failed, and every result flows through the Issue #6/#7 classification unchanged. Statements stay owned by the QueryBuilder rendering seam.

## internal/result

Shared UI-independent result representation (Issue #22), documented in [first-select-result-grid.md](first-select-result-grid.md) and [concurrent-page-count.md](concurrent-page-count.md):

### internal/result/result.go

The single result seam consumed by `internal/ui`'s grid and future CSV/JSON exporters: typed `Value`/`Kind` (NULL, INTEGER, REAL, TEXT, BLOB never collapsed), `RealToken` (exact finite-REAL shortest round-trip token with `.0` restoration, locale-independent; Issue #23 adds the exact non-finite tokens `Inf`/`-Inf`/`NaN`, one token for every NaN payload, selected only after the REAL kind is known), `GridText`/`DecodeText` (visible tab/newline symbols and maximal invalid-UTF-8 replacement to one U+FFFD each), `DeduplicateNames` (full-set left-to-right collision-safe output-name rule), `Page` with original `Columns` beside `HeaderNames()` deduplicated display/export names, `InvalidUTF` warning metadata, and `FromDriver` converting the plain driver value set once at the Connection boundary with BLOB bytes retained exactly. Independent of Bubble Tea, driver concrete types, and exporter formats; see [non-finite-real-grid.md](non-finite-real-grid.md).

### internal/result/count.go and select_identity.go (Issue #24)

The independent result-count lifecycle state (`CountStatus` pending/success/unavailable; `CountState` with `Header()` rendering exactly `Counting rows…`, `Result count: N`, `Result count: N (after Limit M)`, or `Count unavailable`, with HasLimit/Limit as executed metadata and a zero value rendering nothing) and the SELECT execution identity seam (`NextSelectExecutionID`/`NextSelectRequestID` monotonic nonzero IDs, `SelectRole` first-page/count with no interchangeable identity, `SelectRequest`, `SelectTracker` enforcing the two-level identity rule with at-most-once role consumption). Documented in [concurrent-page-count.md](concurrent-page-count.md).

### internal/result/count_test.go and select_identity_test.go (Issue #24)

Exact wording variants plus zero-value emptiness, and distinct monotonic execution/request IDs with exact-acceptance, wrong-role, stale-execution, duplicate, and swapped-role rejection.

### internal/result/result_test.go and architecture_test.go (Issue #22)

Table-driven contract tests for full-set name deduplication (empty labels, pre-suffixed labels, repeated `COUNT(*)`, collision chains), finite REAL tokens (integral-looking values, signed zero, exponents, precision edges, locale independence, round-tripping), Issue #23 non-finite REAL tokens (signed infinities, quiet/payload/negative NaN patterns all rendering `NaN`, finite REAL tokens unchanged, TEXT `Inf`/`-Inf`/`NaN` verbatim, `FromDriver` rows retaining kinds, float64 bits including NaN payloads, and no invalid-UTF metadata), control-character transformation, maximal invalid-UTF-8 replacement with warning metadata, exact BLOB retention and `[BLOB n bytes]` display, typed-cell distinction (numeric-looking TEXT never coerced), and driver conversion including unsupported-value rejection. `architecture_test.go` parses repository source to pin that `internal/result` imports neither Bubble Tea nor the driver, that `internal/ui` owns no private REAL/BLOB/UTF-8/deduplication formatting, and that no tracer execution route survives.

## internal/schema

Schema metadata module (Issue #9), documented in [schema-catalog.md](schema-catalog.md):

### internal/schema/schema.go

UI-independent catalog types: `ObjectKind` (`ordinary-table`/`virtual-table`/`view`), `RowidCapability` (`has-rowid`/`without-rowid`/`not-applicable`), `Object` (name, kind, `WriteEligible`, rowid capability, `RowidShadowed`, `InsertableCount`, declared-order `Columns`), `Column` (name, declared type as pure metadata, hidden/generated visibility, insertability), and `Catalog` (schema version plus name-sorted objects). No driver, Bubble Tea, `internal/ui`, or `internal/querybuilder` dependency; no type-specific input behavior.

### internal/schema/catalog.go

Pure catalog rules over gathered metadata rows: `MasterRow`/`ColumnRow`/`Input` inputs and `BuildCatalog` classification — kind detection (`CREATE VIRTUAL TABLE` prefix), suffix-based `WITHOUT ROWID` detection (trailing semicolon/whitespace/comment tolerated, last two tokens compared case-insensitively), rowid-alias shadowing (`rowid`/`_rowid_`/`oid`, case-insensitive), exclusions of `sqlite_%` (case-insensitive) and `_cf_METADATA`, index/trigger ignoring, ascending-name determinism, and insertability (`hidden==0` plus write eligibility, so views report zero insertable columns).

### internal/connection/schema.go (Issue #9)

In the Connection module: `DB.ReadCatalog(ctx)` runs one `RunRequest` that reads `PRAGMA schema_version`, eligible rows from `main.sqlite_master` (`WHERE type IN ('table','view') ORDER BY name`), and each object's columns from the bound-parameter pragma function `main.pragma_table_xinfo(?)`, then decodes everything with `internal/schema.BuildCatalog`. Failures wrap their cause losslessly in `catalogError` (`could not refresh: <step>: <cause>`) and return the failed `RequestResult`, so health classification (deletion/replacement) wins over ordinary error handling per Issue #7; a cancelled context yields `context.Canceled` preserved through the wrapper. No catalog rules live here — only gathering. See [schema-catalog.md](schema-catalog.md).

### internal/schema/refresh.go (Issue #13)

Typed refresh lifecycle: `RefreshStatus` (`RefreshOK` with complete refreshed catalog, `RefreshFailed` carrying only its cause, terminal `RefreshDeleted`/`RefreshReplaced` mapping Connection `HealthError` kinds), `Attempt` payload consistency enforced by `NewSuccess`/`NewFailure`/`NewTerminal` + `Valid()`, and human-facing status strings. Pure data, no driver or UI dependency; see [stale-schema-refresh.md](stale-schema-refresh.md).

### internal/schema/refresh_test.go (Issue #13)

Contract tests for attempt validity (success carries catalog only, failure cause-only so prior catalogs are retained verbatim, terminals carry neither) and classification strings; the prior catalog stays immutable test data.


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

### internal/querybuilder/projection.go (Issue #15)

SELECT wildcard and COUNT(*) projection path, documented in [projection-count-star.md](projection-count-star.md): `ProjectionKind` (`ProjectionWildcard`, `ProjectionCountStar`, `ProjectionColumn`), `AggregateValue` over the shared `Aggregate` enum, `ProjectionCandidate` with identity-separate `Display()` and collision-free `Key()` encodings, `ProjectionEntry`, and `ProjectionOutcome{Builder, ReopenColumns, PendingAggregate}`. Derivation: empty projections offer wildcard first, bare `COUNT(*)` second, then visible columns in Schema order; any entry hides both synthetics; a committed wildcard leaves nothing. Transitions: `AcceptProjection` (wildcard/sentinel commit directly with Column(s) focus and sentinel reopen; named columns return pending aggregate choices without committing), `CompleteProjectionAggregate` (appends `(column, aggregate)` after validating visible-column membership — making `MIN(*)`/`MAX(*)`/`AVG(*)`/`SUM(*)` unrepresentable by construction), `RemoveProjection(index)` (guarded), plus `ProjectionEntries()`/`ProjectionEmpty()`. State on `QueryBuilder` clears under command replacement and table-loss refresh; Issue #16 completes ordered editing, documented in [projection-ordered-editing.md](projection-ordered-editing.md).

### internal/querybuilder/value.go, sql_atoms.go, sql_literal.go (Issue #14)

Universal parsing and SQL-safety atoms, documented in [sql-atoms-and-literals.md](sql-atoms-and-literals.md). `value.go` owns `ParsedKind`/`Value`, verbatim `ParseValue` (INTEGER `-?[0-9]+` fitting int64 first; finite `strconv.ParseFloat` REAL second with leading-`+` rejection; exact TEXT fallback), shared `realToken` PRD formatting and `quoteTextLiteral` helpers, and `ParamValue()` returning stable concrete bind types (`int64`/`float64`/string) where typed NULL and empty input stay strings. `sql_atoms.go` owns schema-derived quoting (`ObjectIdentifier`, `ColumnIdentifier`, single-atom `quoteIdentifierAtom`) plus closed `Operator`/`Aggregate`/`Direction` enums with token renderers that reject invalid values, and `Predicate` assembly keeping user values on the parameter list. `sql_literal.go` owns the sole canonical standalone renderer `RenderSQLLiteral` over `Literal`/`LiteralKind` (exact INTEGER decimal, shortest round-trip REAL with `.0` restoration and non-finite rejection, quote-doubled TEXT, `NULL`, lowercase-hex `X'...'` BLOB), exposed for future Issues #40/#48 without UI or modal coupling.

## internal/ui

### internal/ui/model.go

The top-level Bubble Tea model (Issue #8): `Field` (labeled builder field with counted display lines), `Model` (`Width`/`Height`, `Fields`, `Focus`, `Scroll`, cancellable-request ownership via `ActiveCancellable` plus a `CancelCommand func() tea.Msg` seam, and the unexported suspension copy `suspendedModel`). `Update` handles `tea.WindowSizeMsg` through `resize` — which freezes the entire model unchanged behind the undersized message and restores it exactly on return to supported dimensions after clamping scroll — and contextual key handling in `handleKey`: while suspended only Ctrl+W routes (and only when hidden state owns active cancellable work); a terminal health classification (Issue #13) consumes every key so no workflow revives; otherwise Tab/Shift+Tab/Up/Down move focus then adjust scroll — blocked while the stale-schema flow is active — and Issue #11 routes plain S/U/D/I through `internal/ui/command_table.go` while Command holds focus. Issue #10 adds the isolated disposable tracer state field `Trace *TraceView` plus `StartTraceMsg`/`traceSettledMsg` handling, removed wholesale by Issue #22 (documented in [first-select-result-grid.md](first-select-result-grid.md)), which replaces it with the settled first-page `Result *ResultView` state and the wired `Select SelectExecutor` field: `ExecutionStartedMsg` now appends query history and issues the one first-page request via `startSelectPage()`, and `SelectSettledMsg` stores the typed page or ordinary execution error as fresh result state. Issue #13 adds the wired `Refresher CatalogRefresher`, stale/pending attempt fields, terminal-state handling of `SchemaRefreshSettledMsg`/`RetrySchemaRefreshMsg`/`CancelStaleRefreshMsg`, and stale navigation gating; see [stale-schema-refresh.md](stale-schema-refresh.md).

### internal/ui/runnable_feedback.go (Issue #19)

Base-context Enter gating documented in [runnable-state-feedback.md](runnable-state-feedback.md): `handleBaseEnter` consulting `QueryBuilder.RunnableReport()` after every higher-precedence context (pending refresh/requests, stale overlay handled in `model.go`); invalid data consumed with `focusRunnableField` mapping typed report fields onto exact field-bar labels (including the new `Set`/`Insert` write targets) and `showRunnableReason` rendering the reason verbatim inline (transient — the next `applyBuilder` rebuild removes superseded feedback); runnable data emitting only the `PreExecutionRequestedMsg` seam for Issue #21; opener fields keeping Issue #12–#18 Enter behavior, with the authoritative Limit exception (nonempty invalid committed text shows the exact reason instead of reopening entry, or defers to an earlier report field). `setFieldContent`/`insertFieldContent` render UPDATE/INSERT prompt states.

### internal/ui/layout.go

Pure region arithmetic with no rendering dependency. `CalculateLayout(totalHeight, fields)` returns footer=1 row, builder desired height inclusive of its 2 border rows and 2 padding rows capped at `floor(H/3)`, results height as every remaining row (> H/2), and `PageRows` as results minus its owned fixed rows (2 border + status/count + frozen header). `BuilderViewport` is interior content lines; `fieldSpans`/`adjustScroll` keep the complete focused field visible inside that viewport; `tooSmall` implements the exact 80×24 threshold; constants pin each region's fixed-row ownership so no border is shared.

### internal/ui/command_table.go (Issue #11)

Builder-field integration: field labels (`Command`, `Table`), plain-key mapping for `s`/`u`/`d`/`i`, and `builderFields(qb.QueryBuilder)` rendering Command always plus Table once any command is chosen; Issue #15 adds the `Column(s)` field once a SELECT has a selected table, whose content renders committed projection entries via `projectionEntryLabels`. `applyBuilder` installs a transition snapshot, rebuilds the rendered fields, and aligns UI focus with the builder's next required field (`FieldColumns` maps onto `Column(s)`); `handleCommandKey` consumes one-key selection only while Command holds focus; `SchemaRefreshedMsg{Catalog}` routes refreshed Schema metadata through `applySchemaRefresh`, which keeps focus on the surviving labeled field. No database logic, popups (Issue #12), or idle-prompt rendering changes live here.

### internal/querybuilder/predicate.go (Issue #17)

Guided WHERE predicate state documented in [where-guided-predicates.md](where-guided-predicates.md): `WherePredicate` with structural states (`WhereAbsent`/`WhereColumnChosen`/`WhereAwaitingValue`/`WhereComplete`) over immutable transitions (`SelectColumn`, `ChooseOperator`, `SubmitValue`, `Entered()` for byte-exact restoration), null operators completing immediately with stale value discard, `SQL()`/`Params()` emitting quoted column + fixed token + one `?` and at most one parsed parameter; QueryBuilder integration adds the committed predicate plus seeded draft lifecycle (`StartWhere(column)` validating eligible identities, `ApplyWhereDraft`/`CancelWhereDraft`/`CommitWhereDraft`, `HasWhere`/`WherePredicate`/`WhereParams`), `FixedOperators()` order stability, `WhereCandidates()` visible-column derivation, wholesale clearing on command replacement or table loss, and zero SELECT-consumable reuse unchanged by UPDATE/DELETE.

### internal/querybuilder/write_state.go (Issue #19)

Placeholder typed write-state seam documented in [runnable-state-feedback.md](runnable-state-feedback.md): `SetAssignment` (UPDATE {Value, NULL} choices with structural `SetChoiceNone/Value/Null`, parsed submission, verbatim entered text, submitted marker) and `InsertColumn` (INSERT {Value, NULL, Default/Omit} with `InsertChoiceOmit`), getters (`Choice()`, `SubmittedValue()`, `Entered()`), and immutable transitions — `AcceptSetColumn`/`ChooseSetAssignment`/`SubmitSetValue` (empty text completes as empty TEXT; typed `NULL` stays TEXT, distinct from the SQL-NULL choice), `BeginInsertPrompts` seeding one incomplete prompt per Schema-derived insertable column, `ChooseInsertColumn`/`SubmitInsertValue`, `InsertableColumns()`, and the forward-compatible `WithSetAssignments` installer used by tests to represent states (duplicate SET columns) the guided flow never constructs. State clears on command replacement and table loss via `discardSelectors`.

### internal/querybuilder/runnable.go (Issue #19)

The authoritative runnable report documented in [runnable-state-feedback.md](runnable-state-feedback.md): `RunField` typed targets in visual builder order (Command, Table, Projection, SetAssignments, InsertColumns, Where, GroupBy, OrderBy, Limit) and `RunnableReport{Runnable, Field, Reason}` from pure `QueryBuilder.RunnableReport()`. Selected-command, refreshed-identifier (table, WHERE column, groups, ordering), and no-incomplete-value-prompt common gates; per-command prerequisites in visual order reusing the Issue #18 grouping/ORDER BY/LIMIT validators; test-asserted reason constants (`ReasonNoCommand` … `ReasonUnsubmittedValueFmt`) plus exact reuse of `LimitInvalidReason`.

### internal/querybuilder/whole_value.go (Issue #19)

Reusable immutable whole-value clearing documented in [runnable-state-feedback.md](runnable-state-feedback.md): `ClearWhereValue()` reopening a completed value-taking committed predicate as an incomplete awaiting-value draft preserving column/operator (absent, null-operator, no-submission, and open-draft no-ops), `ClearLimitValue()` restoring the valid unbounded state, and `ClearSetValue(column)`/`ClearInsertValue(column)` keeping the Value choice and column identity while dropping the whole submission.

### internal/ui/value_input.go (Issue #17)

The universal value-entry prompt consumed by the WHERE flow: `ValuePrompt` keeps entered text verbatim in a rune buffer with an insertion cursor (runes append/left/right/home/end/backspace/delete/ctrl+u) and hands the exact buffer to the owning feature's hook on Enter while Esc is owned by the model's cancel path. Prompt lines render the labeled buffer row with a reversed cursor cell (never color-only), the caller-supplied inline hint, contextual help lines, and the fixed `Enter submits · Esc cancels` footer; parsing stays entirely inside QueryBuilder so typed `NULL` and empty input remain TEXT by construction.

### internal/ui/projection_popup.go (Issue #15)

Column(s) popup wiring, documented in [projection-count-star.md](projection-count-star.md): `columnsFieldLabel` as the field-bar label and popup opener, Enter-on-Column(s) opening a fresh searchable single-select popup from `QueryBuilder.ProjectionCandidates()` (typed identity preserved via candidate keys, viewport 8), accept routing through the builder's own outcomes — direct sentinel/wildcard commits followed by `reopenColumnsPopup` (exact focus + deterministic reset) only when requested, named columns to the scroll-only `Value/Count/Min/Max/Avg/Sum` aggregate popup whose acceptance calls `CompleteProjectionAggregate` — and `projectionEntryLabels` rendering committed entries for the field bar. No projection rules are duplicated and no aggregate-on-wildcard choice is constructible in UI code. Issue #16 adds the issue's own pure remove-latest transition (`RemoveLatestProjection`); `internal/ui/model.go` routes Backspace/Delete from the base Column(s) field through it before popup/table-open handling, and `internal/ui/projection_editing_test.go` scripts the editing flow end-to-end.

### internal/ui/view.go

Deterministic Lip Gloss composition: results box on top owning border/status/header, bordered padded builder below showing the visible field-line window starting at `Scroll` with `>` focused markers, one global footer row last, joining to exactly H rendered rows. While suspended, `View` returns exactly `terminal too small`. Issue #13 adds terminal-health precedence after suspension: a deletion/replacement state renders the whole shell as exactly `Database file no longer exists — session ended` or `Database file was replaced — session ended`, overriding every region and overlay. While the stale-schema flow is active with no popup open, `renderResults` leads content with exactly the persistent stale status plus inline cause lines; Issue #22 routes a settled first page through `renderResultContent` (deduplicated frozen header, absolute range status, typed rows, or the ordinary execution error) while the pre-execution placeholder remains otherwise. Styles are centralized; color never carries meaning alone.

### internal/ui/first_select.go and results_grid.go (Issue #22)

The production SELECT orchestration and grid documented in [first-select-result-grid.md](first-select-result-grid.md): the `SelectExecutor`/`FirstPageResult`/`SelectSettledMsg`/`ResultView` seam (no database or driver type crosses into Bubble Tea state), `startSelectPage` capturing the builder's exact SQL/parameters at the actual-execution boundary, and `renderResultPage`/`renderResultContent` rendering the frozen deduplicated header, absolute range status, typed cells, invalid-UTF warning, exact `No rows`, and ordinary execution errors — every display token through `internal/result`, with no UI-private formatting. Issue #24 extends this: `startSelectPage` assigns one execution ID plus distinct first-page/count request IDs, installs the `SelectTracker`, captures the executed Limit as count metadata, and issues both requests together via `tea.Batch` without waiting for either; `SelectSettledMsg`/`CountSettledMsg` carry both identity levels and Update gates every completion through the tracker; `internal/ui/count.go` adds the `CountExecutor`/`CountResult`/`CountSettledMsg` seam, `applyCountSettled` explicit count state, and `CountHelpLines` help; the status/count line composes the exact count wording with the independently displayed range. Documented in [concurrent-page-count.md](concurrent-page-count.md).

### internal/ui/count.go (Issue #24)

See the Issue #24 additions above; the count lifecycle state, executor seam, settlement, and help live here.

### internal/ui/paging.go and vertical_paging_test.go (Issue #25)

Serialized vertical result paging, documented in [serialized-vertical-paging.md](serialized-vertical-paging.md): the `PageExecutor`/`PageSettledMsg` seam; `handlePageKey` consuming Page Up/Down with exactly the adjacent absolute logical range through `QB.PageSQL` at the layout-derived complete-visible-row size, low/high/user-Limit boundary suppression, and repeated/opposite key consumption while one page is pending; `pageRange` adjacency arithmetic; `applyPageSettled` installing matched completions with absolute-range status and marking the high boundary on short pages; `resetPagingState` on fresh executions; and the scripted tests holding responses behind barriers to prove request counts, loading feedback, local horizontal movement, count-plus-one-page overlap, and exact sizes at supported heights plus after resize.

### internal/ui/horizontal_layout.go and horizontal_keys.go (Issue #29)

Whole-column horizontal scrolling, documented in [whole-column-horizontal-scrolling.md](whole-column-horizontal-scrolling.md): the pure seam — `visibleGridLayout` recomputing visible columns and widths afresh from the output columns, rendered cell display widths, available grid width, and the first-visible index, with oversized-first-column capping and no offset anywhere; `horizontalStep` boundary arithmetic; `clampFirstColumn` resize normalization; `naturalGridWidths` shared with the grid — and the model wiring: `Model.firstColumn` as the grid's only horizontal position, `.`/`,` and the xterm Shift+Page CSI bridge (`ShiftPageMsg`/`shiftPageDirection`) each moving exactly one whole column with boundary no-ops, no database command, movement local while any SELECT request is pending, resize preserving then clamping the index, and `renderResultPage` rendering only the laid-out whole columns.

### internal/ui/inflight_gate.go and in-flight gating (Issue #27)

The generic request-in-flight gate and read-phase feedback, documented in [in-flight-gating.md](in-flight-gating.md): `selectRequestPending()` deriving pending solely from the `firstPagePending`/`pagePending`/`countPendingFlag` ownership flags; `handleInFlightGate` consuming Enter (phase wording plus the exact `press Ctrl+W to cancel` hint), Ctrl+P/N, Ctrl+E/Y, Ctrl+S, and Ctrl+X with exact explanatory feedback recorded in `inFlightNotice` (rendered in the footer) and no command dispatch, while horizontal `,`/`.` movement, page keys, and field navigation fall through to base handling; `q`/Ctrl+C opening the shared quit confirmation (`quitConfirm`/`quitSuspended` with Enter/y/Ctrl+C confirming after `deactivateActiveSelect` cleanup and Esc/n restoring the exact context); Ctrl+W routing only to an owned cancellable request, marking `selectCancelling` before dispatching `SelectCancelRequestedMsg`; and the status/count line gaining the exact `Running…` and `cancelling…` parts. Issue #44 owns write-phase labels and write-side gate integration.

### internal/ui/concurrent_count_test.go (Issue #24)

Scripted model coverage of the concurrent launch (one execution ID, two distinct nonzero role request IDs, exact count SQL/parameters), count-before-page and after-page settlement, after-Limit wording with no clamping, `Count unavailable` isolation with retained rows, page-failure independence, wrong-role/duplicate/superseded/delayed-count rejection, and the help content.

### internal/ui/first_select_test.go and results_grid_test.go (Issue #22)

Scripted model coverage of the builder→validation→execution→settlement route (including history timing and consecutive-identical suppression at the actual-execution boundary, failed validation running no execution, and ordinary error settlement) and focused view coverage of deduplicated frozen headers, absolute `rows 1-N` range, typed cell distinctions, visible control characters, the persistent invalid-UTF warning, exact `No rows`, startup-prompt distinction, and the result-error boundary.

### internal/ui/where_popup.go (Issue #17)

WHERE popup wiring documented in [where-guided-predicates.md](where-guided-predicates.md): `whereFieldLabel` ("Where") with the field bar entry rendered for SELECT/UPDATE/DELETE trailing projection fields via `command_table.go`; Enter on Where opening the searchable single-select column popup from `QueryBuilder.WhereCandidates()`, acceptance beginning/revising through `StartWhere`; the scroll-only fixed-operator popup (`operatorPopupCandidates` from `FixedOperators()` tokens, viewport 8, revision highlight restoration); routing acceptance by operator transition results — null operators commit immediately via `CommitWhereDraft` with focus kept on Where, value-taking ones open `NewValuePrompt` seeded from the untouched commitment's exact entered text on same-column revision; `handleValuePromptKey` submitting/cancelling with whole-draft discard restoring any prior completion; `applyWhereFlow` pinning staged-focus to the Where field across every transition; and the overlay drawing `drawValuePromptOverlay` composing hint/help content without region reflow. No operator semantics or parsing rules are duplicated in UI code.

### internal/ui/popup.go (Issue #12)

Reusable popup interaction state: `PopupMode` (searchable versus scroll-only), `PopupCandidate` pairing identity with displayed text, `EnterResult{Outcome, ID}` over `EnterNone/Accepted/Added/Duplicate`, and `Popup` with `install`-based constructors (`NewSearchablePopup`, `NewMultiSearchablePopup`, `NewScrollOnlyPopup`). `matchesSubsequence` implements the case-insensitive subsequence rule; filtering preserves source order; empty search shows everything; exhausted filters and empty candidate data stay open with exactly `no matches`. Every actual search-text change (`SetSearch`, rune append, Backspace) resets highlight to the first visible result and viewport top to zero; Up/Down clamp at both boundaries and shift `viewportTop` minimally via `scrollIntoView`; `viewportHeight <= 0` is unwindowed. Enter: single-select accepts the highlighted ID; multi-select adds a nonduplicate completion in insertion order and stays open (duplicate becomes `EnterDuplicate`); no-match Enters are ignored. Esc closes while `Completed()` keeps finished selections for preservation. Model routing lives here too: `handlePopupKey` consumes Up/Down/Enter/Esc/printables before base-context handling (scroll-only variants ignore appends), restoring exact opener focus through `closePopupRestore` on cancel and close-then-commit on accept. Issue #13 adds `ReplaceCandidates` (whole-catalog replacement preserving search while deterministically resetting highlight/viewport — never partial substitution), blocks single-select acceptance while `schemaStale`, and turns Esc under stale schema into `CancelStaleRefreshMsg` handling (stale-flow cancellation); see [stale-schema-refresh.md](stale-schema-refresh.md).

### internal/ui/schema_refresh.go (Issue #13)

Refresh lifecycle inside the UI: the narrow `CatalogRefresher` seam (`RefreshCatalog() schema.Attempt`, invoked only inside tea.Cmd functions), `RetrySchemaRefreshMsg`/`CancelStaleRefreshMsg`, identified `SchemaRefreshSettledMsg` delivery via per-issue attempt identity, `issueRefresh` (nil refresher yields no command), `applyRefreshSettled` (superseded/invalid/terminal-guarded discards; success installs the catalog through QueryBuilder + clears indicators atomically + continues the popup with refreshed candidates; ordinary failure raises exact indicators over the untouched prior catalog; deletion/replacement transitions to the established terminal state suppressing controls and causes), `applyCancel` (restores captured opener focus and pre-open state, bumps identity so late results cannot mutate anything), `enterTerminal`, exact constants (`StaleSchemaStatus`, `DeletedSessionEndedMessage`, `ReplacedSessionEndedMessage`), and accessors `SchemaStale()`/`ContinuationBlocked()`/`TerminalState()`. See [stale-schema-refresh.md](stale-schema-refresh.md).

### internal/ui/popup_view.go (Issue #12)

Deterministic popup presentation: `RenderPopup` draws a rounded-bordered box with one `Search: <text>_` line for searchable variants only, status lines such as exact `no matches`, then the visible candidate window with `> `/`  ` prefixes for highlighted/plain rows; lines truncate by whole runes at interior width and window clipping respects viewport height. `composeOverlay` splices overlay text onto composed shell lines at row/col without reflowing anything outside its extent. Cell-width math uses go-runewidth so glyphs never split mid-sequence.

### internal/ui/table_popup.go (Issue #12)

Table as first end-to-end searchable single-select consumer: Enter on the focused Table field installs a fresh popup from `QueryBuilder.EligibleTables()` — eligibility filtered entirely through builder rules — mapping each cataloged object name to both candidate identity and display, capping the viewport at 8 rows. The accept hook commits identity via `QB.SelectTable(id)` then `applyBuilder`; `tableFocused` guards against popups already open or suspension. Issue #13 renames the opener to `beginTablePopup` (returning the mandatory fresh-catalog request as a tea.Cmd) and `openPopupCmd` now feeds that command out of `handleKey`; Issue #15 makes `openPopupCmd` route Enter on a focused Column(s) field to `beginColumnsPopup` instead; see [stale-schema-refresh.md](stale-schema-refresh.md) and [projection-count-star.md](projection-count-star.md).

### internal/querybuilder/group_by.go (Issue #18)

GROUP BY state documented in [group-order-limit.md](group-order-limit.md): `GroupByCandidates()` (visible-column Schema-order derivation excluding committed names), `AcceptGroupColumn` (acceptance-order appends; unknown/hidden/empty/exact-duplicate identities rejected as immutable no-ops), `GroupByEntries`/`RemoveLatestGroup`, `validateGrouping` applying the full matrix (wildcard × GROUP BY exclusivity, every-nonaggregate-grouped rule for mixed projections, extra groups permitted, all-aggregate validity without GROUP BY, stale-group detection) with the exact reason constants (`MixedAggregationNeedsGroupReason`, `WildcardGroupedByReason`, `StaleGroupColumnReason`), plus `renderProjection`/`joinSQLList` helpers shared by the SELECT statement assembly.

### internal/querybuilder/order_by.go (Issue #18)

ORDER BY state documented in [group-order-limit.md](group-order-limit.md): typed `OrderByCandidate{Key, Display}` identities (reserved-prefix keys keeping equal labels distinct), `OrderByCandidates()` (table columns in Schema order for ordinary SELECTs; grouped columns, selected aggregates in projection order, then bare `COUNT(*)` for aggregate/grouped contexts), `AcceptOrderBy` (atomic sole-selection replacement with ASC default), `SetOrderDirection`/`ToggleOrderDirection`/`ClearOrderBy`/`OrderBySelection` over the closed ASC/DESC direction, and `validateOrderBy` staleness reporting `StaleOrderByExpressionReason`.

### internal/querybuilder/limit.go (Issue #18)

LIMIT state documented in [group-order-limit.md](group-order-limit.md): verbatim `limitInput` beside the optional parsed integer, `SetLimitInput`/`LimitInput`/`LimitValue`, the strict `parseLimitText` grammar (empty = unbounded; base-10 integers within `[1, 9223372036854775807]` only; zero, signs, whitespace, decimal/exponent/hex forms, nonnumeric text, and overflow all rejected), the exact `LimitInvalidReason` wording, `validateLimit`, and canonical `renderLimit` output.

### internal/querybuilder/select_sql.go and validation.go (Issue #18)

`SelectSQL()` assembles the deterministic statement — quoted projection over the quoted table, then WHERE, GROUP BY (commit order), ORDER BY (one committed expression with direction), and LIMIT in grammar order — refusing to render any piece whose identity no longer resolves, and `SelectParams()` returning only the WHERE predicate's parameters (projection, grouping, ordering, and LIMIT contribute none). Issue #24 adds `CountSQL()` (`SELECT COUNT(*) FROM (<SelectSQL()>)` with the user's Limit inside the subquery) and `CountParams()` (exactly `SelectParams()`, order preserved) in `count_sql.go`, documented in [concurrent-page-count.md](concurrent-page-count.md). Issue #25 adds `page_sql.go` — `PageSQL(pageLimit, offset)` rendering the base SELECT with any user ORDER BY byte-for-byte, the single eligible implicit `ORDER BY rowid` fallback, and an exact integer LIMIT/OFFSET range clamped around the user's Limit (beyond-Limit and invalid ranges yielding empty, never a partially valid statement), plus `PageParams()` (exactly `SelectParams()`) — documented in [serialized-vertical-paging.md](serialized-vertical-paging.md). `validation.go` defines the first-invalid contract types (`InvalidIssue{Field, Reason}`, `FieldIdentityGroupBy/OrderBy/Limit`) consumed by `FirstInvalidIssue()` in fixed rule order: grouping, then ORDER BY, then LIMIT.

### internal/ui/group_by_popup.go, order_by_popup.go, limit_field.go (Issue #18)

Field wiring documented in [group-order-limit.md](group-order-limit.md): `groupByFieldLabel` ("Group By"), `orderByFieldLabel` ("Order By"), and `limitFieldLabel` ("Limit") with focused-guards, popup/prompt openers over the builder's own candidates, `groupByAcceptHook` (commit + reopen with remaining candidates; rejected acceptances reopen unchanged), `orderByAcceptHook` (atomic commit with focus restoration), base-field `removeLatestGroup`/`clearOrderByField`/`clearLimitField`/`toggleOrderDirectionInBaseField` (Backspace removal and Up/Down direction toggling without popup or focus movement), `beginLimitPrompt` seeding the Issue #14 entry byte-for-byte, `limitPromptAccepted`/`limitPromptCancelled` (no second validator; prior value preserved on cancel), `orderByFieldContent` (`expression DIRECTION`), and `limitFieldContent` appending QueryBuilder's invalid reason verbatim. `command_table.go` renders the three SELECT entries in Query Grammar order and maps the new `FieldGroupBy`/`FieldOrderBy`/`FieldLimit` identities; `model.go` routes base-field keys below popup/value-prompt modality.

### internal/querybuilder/history_state.go (Issue #20)

The history-ready execution snapshot documented in [query-history-append.md](query-history-append.md): `QueryBuilder.HistoryState()` returning the canonical `HistoryState` (command, stable table identity, ordered projection entries, WHERE presence/column/operator/entered value/parsed bound type, GROUP BY acceptance order, ORDER BY expression key/direction, Limit empty-versus-accepted-number, ordered UPDATE SET assignments and INSERT per-column choices with submitted values and exact entered text) with freshly allocated slices and no receiver mutation, plus `HistoryState.Equal` proving every significant field equality-significant — entered representation, concrete bound type, structural choice, column/projection/group order, direction, and empty-vs-number Limit — while excluding transient focus, drafts, cursors, errors, layout, and request state.

### internal/history/query_cursor.go and internal/querybuilder/restore.go (Issue #35)

The Issue #35 navigation and restoration seams documented in [query-history-navigation.md](query-history-navigation.md): `Store.Newest`/`Oldest`/`OlderThan`/`NewerThan` as pure stable-ID cursor primitives (fresh deep copies, deterministic boundary and missing-ID false results, no append/ID-allocation/reorder), and the pure `RestoreBuilder(state, catalog)` seam rebuilding a complete `QueryBuilder` through canonical immutable transitions (command/table identity, ordered projection, guided WHERE with exact entered text, GROUP BY order, ORDER BY key/direction, Limit, ordered SET/INSERT choices) with false on any unresolvable identity so a partial builder is never installed.

### internal/history/query_store.go and query_append.go (Issue #20)

The session-only history store documented in [query-history-append.md](query-history-append.md): `Capacity = 20`, `EntryID` stable nonzero identities allocated monotonically and never renumbered or reused, `Store.Append` deep-copying the state on retention and on `Entries()`/`Lookup()` retrieval with oldest-first eviction of exactly one entry per overflow append, deterministic empty and not-found behavior, no database or Bubble Tea dependency, and the single `Store.AppendExecution` entry point suppressing only normalized-equal consecutive executions (no ID consumed, no eviction) so A→B→A keeps both A entries.

### internal/ui/model.go history seam (Issue #20)

The execution-start timing seam documented in [query-history-append.md](query-history-append.md), extended by Issue #35 (see [query-history-navigation.md](query-history-navigation.md)) with the `internal/ui/query_history.go` mode/cursor/notice state (`historyMode`, `historyCursorID`, `historyNotice`), Ctrl+P/N entering at the newest entry and stepping by stable ID, Ctrl+N-at-newest and Esc exit, `ExecutionStartedMsg` detaching the cursor before the append seam, and `Update`-time defensive selected-ID eviction validation (exact `Previously viewed query was evicted from history` notice, new-oldest fallback, empty-store base return) plus the `internal/history/query_cursor.go` primitives: the `History *history.Store` model field (nil = unwired no-op), `ExecutionStartedMsg` routed in `Update` to `appendQueryHistoryAtExecutionStart()` which appends through `AppendExecution` only for SELECT and INSERT, `PreExecutionRequestedMsg` explicitly appending nothing, and UPDATE/DELETE intentionally never appending through this seam until their confirmation-driven write flow exists (Issues #37/#38); Issue #22 owns emitting the start message after successful validation, so failed executions retain their start append; the settled result is stored separately in `Result *ResultView`.

### internal/schema/revalidate.go, internal/querybuilder/revalidate.go, internal/connection/schema.go revalidation, and internal/ui/schema_validation.go (Issue #21)

Pre-execution schema-version validation documented in [schema-validation-workflow.md](schema-validation-workflow.md): pure `schema.Revalidate`/`Revalidation`/`VersionAttempt` typed outcomes, `ReadSchemaVersion` as one cancellable request, the immutable `QueryBuilder.Revalidate` repair transition with dependent-only clearing and the authoritative post-repair runnable report, and the `internal/ui` validation workflow (`VersionReader` seam, preparation identities, `ValidationSettledMsg`/`CancelValidationMsg`, unchanged cache reuse, changed-version repair with first-reason focus, stale retry/cancel, exact `cancelling…`, terminal precedence, execution-start route only on settled success).

## internal/resultcache

The UI-independent positional result cache (Issue #30), documented in [positional-result-cache.md](positional-result-cache.md):

### internal/resultcache/cache.go

One active SELECT's fetched rows as a single contiguous inclusive range of absolute logical positions: `Position` as one-based absolute identity keeping duplicate-valued rows distinct, `Page`/`Row` over `result.Value` payloads copied at acceptance, `Merge(page, dir)` accepting exactly when the page/retained union stays contiguous after accounting for overlap (two-way sorted merge preserving ascending order for backward prepends, same-position overlap replacement without duplication, atomic rejection of stale gap-forming pages), the independent `MaxPositions` cap with deterministic low-end (forward) versus high-end (backward) eviction and cumulative `RowCapEvictions` metadata, and `Start()`/`End()`/`Len()`/`Rows()` range metadata for `internal/ui` and later immutable snapshots. The 64 MiB byte cap is deferred to Issue #31.

### internal/ui/active_select.go and internal/history/result_entry.go (Issue #34)

The active-SELECT lifetime seam and immutable result snapshots, documented in [active-select-lifetime.md](active-select-lifetime.md): `selectActive`/`activeExecID`/`finalizedExecID` kept distinct from request and generation identity; `activateSelect` (new execution finalizes the previous lifetime first), `enterResultHistory`, `acceptedQuitCleanup`, and the idempotent `finalizeActiveSelect` marking finalization before any duplicate or late message can append; `appendFinalizedResultEntry` converting the authoritative cache rows and Issue #33 facts exactly once into tabular success/count-failed/partial-failed/cancelled-after-rows/failure-after-rows snapshots or non-tabular Cancelled/error entries; and `history.ResultEntry`/`ResultStore.AppendFinalized` with `ResultKind`, deep-copied immutable rows, and deterministic rejection of a second entry per execution ID. Model changes: the `ResultHistory` store, first-page cancelled/failure finalization after settlement, later-page recorded endings cleared by healthy settlement, and page dispatch/recovery refusal once inactive.
