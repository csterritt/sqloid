# Unit and Process-Boundary Test Catalog

Go tests live beside the code they cover. Run with `go test ./...` from the repository root.

## internal/cli

### internal/cli/cli_test.go

In-process, table-driven tests of the shell's routing and output contracts:

- `TestMainRouting` — table of `args → (status, sqlite path, d1 called)`: covers `sqlite <file>` and `d1` dispatch, missing and unexpected arguments, unknown commands, and help/version flags. Handlers are fakes recording dispatches.
- `TestVersionOutput` — `--version` and `-v` write exactly `sqloid <Version>\n` (captured by swapping `os.Stdout` through a pipe) with status 0 and no command dispatch.
- `TestVersionOutputIsSilentOnSuccess` — successful `d1` dispatch writes nothing to stdout.

## cmd/sqloid

### cmd/sqloid/main_test.go

Process-boundary tests asserting the real binary's exact streams and exit statuses. `TestMain` re-executes the test binary as `sqloid` when `SQLOID_CLI_UNDER_TEST` is set, running the CLI with handlers that record dispatches to a file (`SQLOID_CLI_RECORD`):

- `TestCLIStreamsAndStatuses` — table of `args → (status, exact stdout)`: usage failures exit 2, help exits 0, version stdout is exactly `sqloid <version>\n`, and successful `sqlite`/`d1` dispatch is completely silent on both streams.
- `TestUsageFailureWritesErrorAndUsageToStderr` — missing `sqlite` argument: status 2, empty stdout, stderr contains both `Error:` and the usage block.
- `TestHelpGoesToStderr` — `--help` exits 0, lists `sqlite` and `d1`, and writes only to stderr.
- `TestSuccessfulDispatchRunsHandlersSilently` — `sqlite example.db` and `d1` exit 0 with empty streams, and the recorded dispatch lines prove the handlers ran with the routed arguments.

Cross-references: [cli-contract.md](cli-contract.md), [source-code.md](source-code.md).

## internal/cli

### internal/cli/startup_test.go (Issue #2)

Startup-failure rendering with real fixtures through `Main` using the production `connection.Session` handler:

- `TestStartupFailuresRenderOneLineOnStderr` — table over missing, unreadable, directory, invalid-header, and non-writable fixtures: each prints exactly its documented one stderr line, nothing on stdout, and exits 1. Skips under root, where mode-based unreadability cannot be exercised.
- `TestSuccessfulStartupIsSilent` — a valid database exits 0 with completely empty streams.
- `TestStartupFailureKeepsStructuredCause` — startup errors remain `*connection.StartupError`, guarding structured classification for rendering.

### cmd/sqloid/main_test.go — TestSQLiteStartupProcessBehavior

Runs the re-executed test binary with the production connection handler (`SQLOID_CLI_REAL=1` env selects `connection.Session` instead of the recording stub). Asserts silent status-0 success on a valid fixture plus exact one-line stderr diagnostics and status 1 for missing, invalid-header, and non-writable databases — process-level verification of the Issue #2 contract.

Cross-references: [cli-contract.md](cli-contract.md), [sqlite-startup.md](sqlite-startup.md).

## internal/d1

### internal/d1/discovery_test.go (Issue #3)

Table-driven filesystem tests via `t.Chdir(t.TempDir())`: sole-candidate selection with exact joined path; zero candidates across directory-absent/empty/metadata-only/sidecar-only/wrong-case/nested/alternate-layout fixtures (`ErrNoCandidate`, empty path); multiple candidates including uppercase-`Metadata` names remaining eligible under the case-sensitive rule (`ErrMultipleCandidates`); exclusions leaving a surviving top-level candidate while metadata, sidecar, nested, and alternate-layout files are ignored.

## Additional Issue #3 tests elsewhere

- `internal/cli/d1_test.go` — `TestRunD1PassesSoleCandidateUnchangedToSharedOpener` proves the discovery→opener handoff passes the exact path unchanged through injected seams; `TestD1EndToEndOpensSoleDiscoveredCandidate` runs `sqloid d1` through `Main` on a mixed fixture (real SQLite candidate plus ignored metadata, sidecar, wrong-case, nested, and alternate-layout files) asserting silent status-0 success.

## Issue #4 discovery-failure golden tests (internal/cli/d1_test.go)

- `TestRunD1DiscoveryFailureMapsExactDiagnosticAndSkipsOpener` — with each typed `internal/d1` outcome injected: asserts the mapped diagnostic's exact spelling, line count (2 for zero candidates with the expected-path + `sqloid sqlite <file>` hint; 1 for multiple candidates without a hint), and that the shared opener is never invoked.
- `TestD1DiscoveryFailureProcessBehavior` — runs `sqloid d1` through `Main` on real fixtures via `t.Chdir`: missing, empty, and candidate-free Wrangler directories produce exactly the two-line zero-candidate diagnostic; multiple `.sqlite` candidates produce exactly the single line. Each case asserts stderr equality byte-for-byte, silence on stdout, status 1, and — by walking the working directory before and after — that no database or stray file was created.
- `TestD1DiscoveryUnreadableDirectoryProcessBehavior` — an unreadable (`0o000`) Wrangler directory yields the identical zero-candidate two lines with status 1 and no creation. Permission cases skip under root.
- `internal/connection/opener_test.go` — `TestOpenRelativePath` pins read-write mode=rw opening of a working-directory-relative discovered path.

## internal/connection Issue #5 pool and leasing tests

- `internal/connection/pool_config_test.go` — exact-two pool contract: maximum size 2, simultaneous usable distinct leases within an explicit bound, retained floor of two idle connections after release, per-connection five-second busy timeout, and per-connection exact 64 MiB `SQLITE_LIMIT_LENGTH` (cross-checked on every inspected connection).
- `internal/connection/lease_test.go` — dedicated-leasing high-risk coverage from the PRD: barrier-driven concurrent lease pairs in WAL and rollback-journal fixtures prove distinct physical connections, all per-connection invariants hold on each held lease, journal mode is neither set nor changed, and release/reuse safety is enforced.

## internal/connection Issue #6 cancellation lifecycle tests

- `internal/connection/request_test.go` — fake-backed (no live driver) request lifecycle: unique request IDs and context ownership; exactly-one interrupt dispatch under eight racing Cancellers and after settlement; visible `running → cancelling → settled` state where cancelling persists until Settle even after internal work completion; late success discarded as cancelled; error classification matrix with settle-idempotence; lease-hold-before-settlement proven by fast-failing third-lease acquisition on the real two-connection pool; no-force-close plus safe same-lease and released-pool reuse after settled cancellation.
- `internal/connection/interrupt_unix_test.go` — Linux/macOS mandatory barrier-based pinned-driver capability evidence (`//go:build unix`, release-blocking): controlled CPU-bound probe scan settles under one second of cancellation; lock-wait INSERT behind a held SHARED read lock settles within five seconds; independent second-lease isolation during interruption; deliberately released late success classified cancelled and discarded; subsequent harmless PRAGMA work and pool reuse prove the interrupted connection was never force-closed.

## internal/connection Issue #7 request-boundary identity tests

- `internal/connection/health_test.go` — Linux/macOS barrier-driven integration tests (`//go:build unix`): startup records the validated target's device/inode; `VerifyHealth` classification table (unchanged → nil, deletion/rename-away → typed deleted with cause unwrapping to `fs.ErrNotExist`, same-path replacement with a distinct inode → typed replaced, in-place same-inode mutation through an outside connection → nil ordinary behavior); `Lease` refuses admission after deletion or replacement; `RunRequest` exercises schema-style, count-style, and estimate-style reads plus two barrier-rendezvoused pooled requests on distinct physical connections; changed targets block before work with exactly one boundary check and the operation never started; post-error reclassification races (deletion/replacement applied after a successful precheck while a failing request is in flight) take precedence over the preserved SQLite error; raced replacement after success stands until the next boundary detects replacement before work; phased writes (commit and rollback) receive exactly one pre-BEGIN check with none between statement execution and COMMIT.

Cross-references: [cancellation-infrastructure.md](cancellation-infrastructure.md), [connection-pool.md](connection-pool.md), [session-health.md](session-health.md), [sqlite-startup.md](sqlite-startup.md).

## internal/schema Issue #9 schema catalog tests

- `internal/schema/catalog_test.go` — table-driven pure contract tests over synthetic metadata: object kind classification (ordinary/virtual/view incl. odd-case CREATE VIRTUAL TABLE and WITHOUT ROWID suffix variants with semicolon/whitespace/trailing comment), write eligibility, rowid capability plus declared-rowid shadowing for all three aliases (`rowid`, `_rowid_`, `oid`, case-insensitive), generated/hidden columns as noninsertable with declared-type passthrough, zero-insertable tables, exact exclusion of `sqlite_%` (any case) and `_cf_METADATA` while look-alikes survive and indexes/triggers are ignored, ascending-name determinism across input orders, empty-column-map objects, and the human-facing kind/capability strings.
- `internal/schema/catalog_integration_test.go` — SQLite-backed (`//go:build unix`) fixture covering an ordinary table, an fts5 virtual table with its five shadow tables (config/idx genuinely WITHOUT ROWID), a view, a WITHOUT ROWID table, a declared-`rowid` shadowing table, AUTOINCREMENT's auto-created `sqlite_sequence`, `_cf_METADATA`, and a generated-columns mix. Asserts catalog version equals direct `PRAGMA schema_version`; the reserved objects exist in the DB yet never surface in the catalog; per-object kinds, rowid capabilities, shadowing, eligibility sets, virtual-table hidden columns, view SELECT-only zero insertable columns, and column declared types/insertability; repeated reads deep-equal; and DROP raises the version and removes the object from a refreshed catalog.

## internal/connection Issue #9 catalog boundary tests

- `internal/connection/schema_test.go` — success result with populated catalog; two concurrent catalog reads leasing distinct pool connections; cancelled context classified failed with `context.Canceled` unwrappable; same-path replacement at the request boundary yields typed `HealthReplaced` (see [schema-catalog.md](schema-catalog.md)).

## internal/connection and internal/schema Issue #10 tracer boundary tests

- `internal/connection/tracer_test.go` — SQLite-backed: typed row transport (`int64` INTEGER, `string` TEXT, headers in result order), execution against the safely quoted unusual identifier `odd "name`, and a basic query failure (`no_such_table`) settling as failed `RequestResult` with a preserved cause naming the failing object and no terminal-classification claims. Composition always goes through `ReadCatalog` + `schema.ChooseTracerTarget`.
- `internal/schema/tracer_test.go` — unit tests for identifier quoting (embedded double quotes doubled, spaces/mixed case preserved), catalog selection by name across kinds, and typed `*TracerError` rejection of uncataloged names with diagnostic text identifying the object.
- `internal/schema/tracer_integration_test.go` (unix) — end-to-end fixture flow: ordinary table executes into typed headers/rows via Connection; unusual identifier succeeds; a post-selection drop makes execution alone fail as an ordinary basic failure. No builder/validation/paging/count/history/cancellation/write behavior exercised.

## internal/ui Issue #8 shell tests

- `internal/ui/layout_test.go` — table-driven pure arithmetic over 80×24, 100×30, and 160×50 with minimal and growing builders: exactly one footer row; builder desired height inclusive of border+padding capped at floor(H/3); results equal every remaining row and greater than half-height; regions partitioning H exactly; `PageRows` subtracting results' owned fixed rows; viewport excluding builder border/padding. `TestViewRegionOwnership` renders full screens asserting exactly H rows with the footer on the last, both boxes occupying exactly their owned heights, exactly two top-border corner rows across the whole view (independent borders, none shared), and page area consistent with inner rows minus status/header.
- `internal/ui/suspension_test.go` — scripted `(model, msg) → (model, cmd)` behavior: focus scrolling with builders far beyond the cap keeping each complete multiline focused field inside the visible range on Tab/Shift+Tab/Up/Down; below-minimum sizes (both-below, width-only, height-only) render exactly `terminal too small`; ordinary keys produce no commands, leave the view exact, and mutate nothing hidden; resize-back restores context/focus/scroll exactly then applies normal layout; Ctrl+W returns non-nil routing into the generic cancellation flow only when hidden state owns cancellable work, invoking it without exposing or mutating hidden state, and returns nil otherwise.

## internal/ui Issue #10 tracer rendering tests

- `internal/ui/tracer_test.go` — scripted messages only (no database opened): start message returns exactly one command whose completion re-enters Update; success renders returned column headers and rows inside a bordered region at exactly H lines for 80×24 with the builder bar intact and no row data leaking into builder fields; failure renders the typed error text without crashing and without copy claiming paging/count/history/recovery; nil-executor zero-value messages never panic.
- `internal/ui/results_test.go` — focused layout assertions: tracer grid views still partition 80×24, 100×30, and 160×50 into exactly H rows, keeping Issue #8 region ownership intact.

Cross-references: [early-integration-tracer.md](early-integration-tracer.md).
