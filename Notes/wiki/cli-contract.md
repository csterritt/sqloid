# CLI Contract

The complete command-line contract implemented by `internal/cli` on top of the PRD-mandated [mow.cli](https://github.com/jawher/mow.cli) command structure (Issue #1, `Notes/PRD-sqloid.md` §CLI behavior).

## Commands

| Command | Spec | Behavior |
| --- | --- | --- |
| `sqloid sqlite <file>` | `FILE` (exactly one string argument) | Opens the SQLite database at `<file>` through the production connection handler (wired in `cmd/sqloid/main.go`). The handler is invoked only after successful routing; startup validation/diagnostics are defined by Issue #2 ([sqlite-startup.md](sqlite-startup.md)). |
| `sqloid d1` | no arguments | Discovers and opens the local D1 database under `.wrangler/state/v3/d1/miniflare-D1DatabaseObject`; discovery rules in [d1-discovery.md](d1-discovery.md). |

## Flags

| Flag | Forms | Behavior |
| --- | --- | --- |
| Help | `--help`, `-h` | Prints the long usage message; exits 0. |
| Version | `--version`, `-v` | Prints exactly `sqloid <version>` plus a newline to **stdout**; exits 0. `<version>` is the build version string (currently `dev`, overridable at link time via `internal/cli.Version`). |

## Stream ownership

- **stdout** — only version output (`sqloid <version>\n`). Successful `sqlite`/`d1` dispatch adds nothing; the CLI has no success message.
- **stderr** — help/usage text, usage-error messages (`Error: incorrect usage` and the usage block), and each startup failure's exact one-line diagnostic printed verbatim from the handler error (`internal/cli` performs the rendering).

## Exit statuses

| Situation | Status |
| --- | --- |
| Help or version request | 0 |
| Successful `sqlite <file>` or `d1` dispatch | 0 |
| Usage failure: missing argument, unexpected argument, unknown command, illegal option | 2 |

Startup/database-validation failures exit **1** with exactly one stderr line per [sqlite-startup.md](sqlite-startup.md) (Issue #2); successful startup is silent. The sole documented exception is D1 zero-candidate discovery ([d1-discovery.md](d1-discovery.md)), which exits 1 with exactly two stderr lines; multiple-candidate D1 discovery exits 1 with exactly one stderr line and no hint.

## Roles of `internal/cli` and `cmd/sqloid`

- `internal/cli` owns the entire shell: it builds the mow.cli app (`sqlite` command with `FILE` argument, `d1` command, `--version`/`-v` flag, root action), sets `flag.ContinueOnError` so mow.cli never calls `os.Exit` directly, and exposes `Main(args, handlers) int` returning the process status. Database startup is behind the injectable `Handlers` struct so tests can inject fakes while the production binary wires `session.RunSQLite` (for `sqlite`, see [production-tui-composition.md](production-tui-composition.md)) and `cli.RunD1` (for `d1`, which maps [discovery failures](d1-discovery.md) to their exact diagnostics without touching the opener and passes a discovered sole candidate unchanged to `session.RunSQLite`). `cli.RunD1WithRunner(run)` is the testable D1 handler that injects a program runner so the D1 end-to-end test can exercise the lifecycle without a real TTY. A routed handler that returns an error has its `Error()` printed verbatim to stderr with status 1 — including the explicitly defined two-line zero-candidate D1 exception.
- `cmd/sqloid` is a thin process boundary: `os.Exit(cli.Main(os.Args, handlers))` where `handlers.SQLite = session.RunSQLite` and `handlers.D1 = cli.RunD1`; nothing else.

## Verification

`go build ./...` and `go vet ./...` must pass. Exact streams and exit statuses are asserted by re-executing the test binary as the CLI (see [unit-tests.md](unit-tests.md)).
