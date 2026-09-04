# Production TUI composition

Issue #57 the production application-composition path between
`cmd/sqloid/main.go`, `internal/connection`, and `internal/ui`: the
`internal/session` package owns the one composition root that opens a
database, loads the initial schema catalog synchronously, constructs the
fully wired `ui.Model` with thin adapters over the real `*connection.DB` and
real filesystem implementations, and runs the Bubble Tea program until it
quits. Both `sqloid sqlite <file>` and D1-discovered paths flow through this
single composition layer; no second startup or database-opening path exists.

## Why this issue exists

Before Issue #57 the CLI handlers opened the database and returned silently
after validation — `connection.Session` deferred `db.Close()` and exited
without constructing any UI, wiring any executor, or running any Bubble Tea
program. Every module test passed because it tested disconnected components
with fake seams, so the missing composition root was invisible at the
package level. Issue #57 lands the production path that makes the v1 TUI,
execution, history, and export stories reachable from the shipped binary.

## The composition root

`internal/session.Compose(db *connection.DB) (*Session, error)` is the one
composition constructor:

1. **Synchronous catalog load** — `db.ReadCatalog(context.Background())`
   runs before any model construction. A failure returns the wrapped cause
   (preserving the typed `*connection.HealthError` chain) so the CLI can
   render the exact one-line diagnostic and stop before any Bubble Tea
   program starts. The caller retains ownership of `db` until `Close`;
   `Compose` never closes `db` itself, even on a catalog-load failure.
2. **Schema installation** — the loaded catalog is installed through the
   established `SchemaRefreshedMsg` seam so the `QueryBuilder`'s
   eligible-object list and the field bar reflect the real database from the
   first frame. This is the same transition a successful Table-popup refresh
   takes, applied once at startup before any user input.
3. **History stores** — `history.NewStore()` and `history.NewResultStore()`
   are installed so query and result history are session-scoped and
   in-memory.
4. **Database seam adapters** — every database seam is wired to a thin
   adapter that calls the matching `*connection.DB` method and maps the
   typed `connection.RequestResult` onto the established UI typed result
   values. No driver type ever leaks into `internal/ui`.
5. **Filesystem seams left nil** — `PickerFS` and `SaveFS` stay nil so the
   model uses the real `filepicker.OSFS` and `export.OSSaveFS`
   implementations in production.

The returned `*Session` owns the `*connection.DB` for the session lifetime.
`Catalog()` and `Model()` stay readable after `Close` for inspection by
tests and post-run diagnostics; `Close()` releases the database pool exactly
once and is a safe no-op on a second call.

## The database seam adapters

Each adapter is a thin closure over `db` that maps the connection layer's
typed outcomes onto the UI's typed values:

| UI seam | Adapter | Connection call | Mapping |
|---|---|---|---|
| `Select` | `selectAdapter` | `RunFirstPage` | `RequestResult` → `FirstPageResult`; `*result.LimitFailure` preserved via `errors.As`; `*connection.HealthError` surfaced as `Err` when `Err` is nil but `Health` is set so `healthTerminalFor` classifies it; `errors.Is(err, context.Canceled)` classified `Cancelled` |
| `Page` | `pageAdapter` | `ExecutePage` | same as `Select` |
| `Count` | `countAdapter` | `RunCount` | `RequestResult` → `CountResult`; same cancellation and health mapping |
| `VersionReader` | `versionAdapter` | `ReadSchemaVersion` | `RequestResult` → `schema.VersionAttempt`; `HealthDeleted`/`HealthReplaced` → `NewVersionDeleted`/`NewVersionReplaced` |
| `Refresher` | `refresherAdapter` | `ReadCatalog` | `RequestResult` → `schema.Attempt`; `HealthDeleted`/`HealthReplaced` → `NewTerminal(RefreshDeleted)`/`NewTerminal(RefreshReplaced)` |
| `Estimator` | `estimateAdapter` | `ExecuteEstimate` | `RequestResult` → `EstimateResult`; same cancellation and health mapping |
| `Write` | `writeAdapter` | `StartWrite` | drains the phase stream through the callback, then `Wait()` returns the typed `connection.WriteResult` unchanged |

The cancellation mapping is necessary because a lease acquisition failure on
a cancelled context returns `OutcomeFailed` with `Err` wrapping
`context.Canceled` (the lease never opened, so the request lifecycle never
classified it). The adapter detects this with `errors.Is(res.Err,
context.Canceled)` so the UI's cancellation settlement stays inert.

The health surfacing is necessary because a lease-boundary health failure
(the file vanishing after open but before the request body runs) returns
`OutcomeFailed` with `Err` nil and `Health` set. The adapter surfaces
`Health` as `Err` so the UI's `errors.As` mapping in `healthTerminalFor`
classifies it without parsing driver text.

## The CLI lifecycle

`session.RunSQLite(path string) error` is the CLI-facing handler. It
delegates to `RunSQLiteWith(path, defaultRunner, nil)`, the testable
lifecycle:

1. `connection.Open(path)` — startup validation, read-write open, schema
   probe. A failure returns the connection layer's already-prepared
   one-line diagnostic.
2. `Compose(db)` — synchronous catalog load and model wiring. A failure
   closes the database pool directly so no handle leaks, then returns the
   wrapped cause.
3. `run(s.Model())` — the injected program runner. In production
   `defaultRunner` constructs `tea.NewProgram` over the model and runs it
   until it quits. In tests a no-op runner returns immediately without
   touching the TTY.
4. `s.closeWith(closeHook)` — the single close boundary. A non-nil
   `closeHook` is the observable close hook for tests; nil means the
   session's own `Close()`, which releases the database pool.

A startup failure never invokes `run` or constructs a session. A runner
error is returned to the caller after the session is closed. The close order
is program teardown before database pool release, matching the PRD's
"cleanup is never abandoned" rule.

`cmd/sqloid/main.go` wires `Handlers{SQLite: session.RunSQLite, D1:
cli.RunD1}`. `cli.RunD1` delegates to `runD1With(d1.Discover,
session.RunSQLite)` so both startup modes flow through the one composition
root. `cli.RunD1WithRunner(run)` is the testable D1 handler that injects a
program runner so the D1 end-to-end test can exercise the discovery → open →
compose → run → close lifecycle without a real TTY.

## The PTY integration test

`cmd/sqloid/pty_integration_test.go` is the production-level integration
test. It builds the real `sqloid` binary from `cmd/sqloid`, creates a real
SQLite database fixture, spawns the binary under
`github.com/creack/pty.StartWithSize` with a 100×30 terminal, responds to
Bubble Tea's `\x1b[6n` cursor-position-report request so the UI renders,
waits for the builder's `Command` field to appear in the PTY output, sends
`q` then `Enter` to confirm the universal quit, and asserts the process
exits with status 0. No injected runners or fakes are used: this is the
shipped binary through the shipped composition root through a real terminal.

The cursor-position-report response is required because Bubble Tea sends
`\x1b[6n` on startup and blocks until a response arrives; without it the UI
never renders and the test times out. The response `\x1b[1;1R` (cursor at
row 1, col 1) is the standard reply that lets rendering proceed.

### CI integration (Issue #88)

Issue #88's expanded release gate runs this test unattended on both Linux
and macOS through `go test -count=1 -timeout 20m ./...`, which includes
`cmd/sqloid`. The test's `t.TempDir()` fixtures (built binary and SQLite
database) are cleaned up automatically by Go's test framework, and
`t.Cleanup` closes the PTY master and kills any lingering process. The
10-second deadlines for builder rendering and process exit are deterministic
— no arbitrary sleeps. Captured PTY output appears in `t.Fatalf` messages
on failure, so CI logs show the rendered TUI state at the point of failure.
The test fails nonzero if the program launch, any real adapter, or the TUI
run is bypassed, so a regression that replaces production composition with
package-local fakes cannot merge behind a green partial workflow. See
[release-capability-gate.md](release-capability-gate.md) for the full gate.

## Test coverage

Three test files cover the composition root:

- `internal/session/compose_test.go` — test-only composition coverage that
  opens a real temporary SQLite database, reads its initial catalog through
  the production seam, constructs the model, and inspects or exercises every
  database/filesystem seam. Proves the retained catalog matches the real
  schema, every seam is wired to a real adapter, the filesystem seams are
  nil so the real implementations are used, connection outcomes map to the
  existing typed UI results (cancellation, health-terminal, write-phase),
  no driver type leaks into `internal/ui`, and an initial catalog failure
  stops before Bubble Tea starts.
- `internal/session/lifecycle_test.go` — deterministic lifecycle tests at
  the composition/CLI boundaries with an injected program runner and
  observable session close hooks. Proves the runner is invoked exactly once
  with the wired model, the session is closed in the reverse order after
  the runner returns, an Open failure never invokes the runner or
  constructs a session, a catalog failure closes the database pool and
  never invokes the runner, a runner error is returned after the session is
  closed, the close hook is called exactly once per session, and the model
  handed to the runner carries the real catalog and every wired seam.
- `cmd/sqloid/pty_integration_test.go` — the production-level PTY
  integration test described above.

The existing `internal/cli/startup_test.go`, `internal/cli/d1_test.go`, and
`cmd/sqloid/main_test.go` were updated to inject a no-op program runner so
they exercise the open → compose → close lifecycle without a real TTY; the
real TUI is covered by the PTY integration test.

## Cross-references

- [source-code.md](source-code.md) — `internal/session` catalog entry.
- [cli-contract.md](cli-contract.md) — the CLI shell and handler wiring.
- [sqlite-startup.md](sqlite-startup.md) — the startup pipeline that
  precedes composition.
- [d1-discovery.md](d1-discovery.md) — the D1 handoff to the shared
  composition root.
- [session-health.md](session-health.md) — the typed health classifications
  the adapters surface.
- [transactional-writes.md](transactional-writes.md) — the phased write
  lifecycle the `Write` adapter relays.
- [schema-catalog.md](schema-catalog.md) — the catalog the composition loads.
- [responsive-tui-shell.md](responsive-tui-shell.md) — the shell the
  composed model renders into.
- `Notes/PRD-sqloid.md` — Module Design (UI composes all other modules
  without embedding database behavior), Testing Decisions (CLI/integration
  matrix and the manual rendering matrix), and the Execution and Result
  Lifecycle section.
