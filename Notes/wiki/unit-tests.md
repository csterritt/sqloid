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
