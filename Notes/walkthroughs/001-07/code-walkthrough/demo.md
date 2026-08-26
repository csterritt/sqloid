# Issue #1 Code Walkthrough: CLI Shell and Usage Contracts

*2026-08-26T18:50:57Z by Showboat 0.6.1*
<!-- showboat-id: 095e0a61-b783-4a90-8bf9-53361ef23e5f -->

This walkthrough demonstrates the Issue #1 CLI shell (internal/cli, cmd/sqloid) against Notes/PRD-sqloid.md and Notes/issues/001-cli-shell-and-usage-contracts.md: routing, help, exact version output, usage failures with status 2, stream selection, and silent successful dispatch.

```bash
find cmd internal -name '*.go' | sort
```

```output
cmd/sqloid/main.go
cmd/sqloid/main_test.go
internal/cli/cli.go
internal/cli/cli_test.go
```

## Test suites

```bash
go test ./... 2>&1 | tail -3
```

```output
ok  	github.com/chris/sqloid/cmd/sqloid	(cached)
ok  	github.com/chris/sqloid/internal/cli	(cached)
```

The internal/cli tests are table-driven in-process routing tests with injectable handlers. The cmd/sqloid tests re-execute the test binary as the real CLI to assert exact streams and exit statuses.

## Routing: in-process table-driven tests

```bash
go test -count=1 ./... 2>&1 | grep -E '^(ok|FAIL)' | sed 's/\t[0-9.]*s$/ [ok]/'
```

```output
ok  	github.com/chris/sqloid/cmd/sqloid [ok]
ok  	github.com/chris/sqloid/internal/cli [ok]
```

The internal/cli tests are table-driven in-process routing tests with injectable handlers. The cmd/sqloid tests re-execute the test binary as the real CLI to assert exact streams and exit statuses.

```bash
go test -count=1 ./internal/cli -run '^TestMainRouting$' -v 2>&1 | grep -E '^(=== RUN|--- PASS)'
```

```output
=== RUN   TestMainRouting
=== RUN   TestMainRouting/sqlite_routes_the_file_argument
=== RUN   TestMainRouting/d1_routes_with_no_arguments
=== RUN   TestMainRouting/missing_sqlite_argument_is_a_usage_failure
=== RUN   TestMainRouting/unexpected_sqlite_argument_is_a_usage_failure
=== RUN   TestMainRouting/unexpected_d1_argument_is_a_usage_failure
=== RUN   TestMainRouting/unknown_command_is_a_usage_failure
=== RUN   TestMainRouting/help_flag_succeeds
=== RUN   TestMainRouting/short_help_flag_succeeds
=== RUN   TestMainRouting/version_flag_succeeds
=== RUN   TestMainRouting/short_version_flag_succeeds
--- PASS: TestMainRouting (0.00s)
```

## Exact version output via the real binary

```bash
go build -o /tmp/sqloid-walk ./cmd/sqloid && /tmp/sqloid-walk --version; echo "exit=$?"
```

```output
sqloid dev
exit=0
```

Version output is exactly `sqloid <version>` on stdout with exit 0; `-v` behaves identically.

## Stream selection: help goes to stderr, stdout stays empty

```bash
/tmp/sqloid-walk --help 1>/tmp/help-out 2>/tmp/help-err; echo "exit=$?"; echo "stdout bytes: $(wc -c </tmp/help-out)"; head -3 /tmp/help-err
```

```output
exit=0
stdout bytes: 0

Usage: sqloid [OPTIONS] COMMAND [arg...]

```

## Usage failures: status 2, error plus usage on stderr

```bash
/tmp/sqloid-walk sqlite 1>/tmp/u-out 2>/tmp/u-err; echo "exit=$?"; echo "stdout bytes: $(wc -c </tmp/u-out)"; head -2 /tmp/u-err
```

```output
exit=2
stdout bytes: 0
Error: incorrect usage

```

```bash
/tmp/sqloid-walk sqlite one.db two.db >/dev/null 2>&1; echo "unexpected-arg exit=$?"; /tmp/sqloid-walk bogus >/dev/null 2>&1; echo "unknown-command exit=$?"
```

```output
unexpected-arg exit=2
unknown-command exit=2
```

## Silent successful dispatch

```bash
touch /tmp/walk-fixture.db && /tmp/sqloid-walk sqlite /tmp/walk-fixture.db 1>/tmp/s-out 2>/tmp/s-err; echo "sqlite exit=$? stdout bytes: $(wc -c </tmp/s-out) stderr bytes: $(wc -c </tmp/s-err)"; /tmp/sqloid-walk d1 1>/tmp/d-out 2>/tmp/d-err; echo "d1 exit=$? stdout bytes: $(wc -c </tmp/d-out) stderr bytes: $(wc -c </tmp/d-err)"
```

```output
sqlite exit=0 stdout bytes: 0 stderr bytes: 0
d1 exit=0 stdout bytes: 0 stderr bytes: 0
```

## Final verification

```bash
gofmt -l cmd internal; go vet ./... && echo 'vet clean'; go build ./... && echo 'build ok'; rm -f /tmp/sqloid-walk /tmp/walk-fixture.db /tmp/*-out /tmp/*-err /tmp/help-* /tmp/u-* /tmp/s-* /tmp/d-*
```

```output
vet clean
build ok
```
