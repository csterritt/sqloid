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
