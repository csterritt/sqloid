# Issue #2 — SQLite startup validation and read-write errors

*2026-08-26T20:50:43Z by Showboat 0.6.1*
<!-- showboat-id: 3e866203-137a-40f8-8b88-278f3857b335 -->

Verification of Issue #2 (SQLite startup validation and read-write errors) against the 'Startup validation and errors' section of Notes/PRD-sqloid.md, run from the repository root at /home/chris/sqloid (test durations stripped for reproducibility).

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/connection/ ./internal/cli/ ./cmd/sqloid/ | sed -E 's/[0-9]+\.[0-9]+s//g' | cut -c1-58
```

```output
ok  	github.com/chris/sqloid/internal/connection	
ok  	github.com/chris/sqloid/internal/cli	
ok  	github.com/chris/sqloid/cmd/sqloid	
```

The opener owns the mandated order — existence → readability → exact 16-byte 'SQLite format 3\0' header → read-write open → probe — failing fast without ever creating or modifying the target:

```bash
cd /home/chris/sqloid && sed -n '/Open validates/,/^func Open/p' internal/connection/startup.go
```

```output
// Open validates the database at path without creating or modifying it and,
// when valid, opens it read-write through the pinned driver and probes the
// schema with a harmless `PRAGMA schema_version`. Journal mode is never set
// or changed, and there is deliberately no read-only fallback.
//
// Validation runs in the mandated order — existence → readability → exact
// 16-byte SQLite header → read-write open → probe — and fails fast at the
// first failing step with a *StartupError whose Error() is the exact one-line
// diagnostic required by Issue #2.
func Open(path string) (*DB, error) {
```

Pre-open validation: table-driven unit tests cover missing files, directories, invalid/corrupt/short headers, the mandated readability-before-header ordering (unreadable invalid-header file classifies as unreadable), and non-creation/non-modification via before/after snapshots:

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/connection/ -run TestPreOpenValidation -v 2>&1 | grep -E '^(=== RUN|--- (PASS|FAIL)|ok|FAIL)' | sed -E 's/[0-9]+\.[0-9]+s//g'
```

```output
=== RUN   TestPreOpenValidation
=== RUN   TestPreOpenValidation/missing_file_fails_existence_check_without_creating_it
=== RUN   TestPreOpenValidation/directory_is_rejected_as_not_a_database
=== RUN   TestPreOpenValidation/invalid_header_text_file_is_rejected_as_not_a_database
=== RUN   TestPreOpenValidation/short_file_that_cannot_hold_a_header_is_rejected_as_not_a_database
=== RUN   TestPreOpenValidation/header_corrupted_in_final_bytes_is_rejected
=== RUN   TestPreOpenValidation/unreadable_invalid-header_file_reports_readability_before_header
=== RUN   TestPreOpenValidation/unreadable_valid-header_file_still_fails_readability_first
--- PASS: TestPreOpenValidation ()
ok  	github.com/chris/sqloid/internal/connection	
```

Read-write opening with mode=rw: valid databases open and answer queries; journal mode ('wal' and 'delete' fixtures) is unchanged by opening; a readable-but-non-writable database fails with exactly 'cannot open database read-write: <path>: permission denied':

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/connection/ -run 'TestOpenPreservesJournalMode|TestOpenValidDatabaseReadWrite|TestOpenNonWritableDatabaseIsPermissionDenied' -v 2>&1 | grep -E '^(=== RUN|--- (PASS|FAIL)|ok|FAIL)' | sed -E 's/[0-9]+\.[0-9]+s//g'
```

```output
=== RUN   TestOpenValidDatabaseReadWrite
--- PASS: TestOpenValidDatabaseReadWrite ()
=== RUN   TestOpenPreservesJournalMode
=== RUN   TestOpenPreservesJournalMode/wal
=== RUN   TestOpenPreservesJournalMode/delete
--- PASS: TestOpenPreservesJournalMode ()
=== RUN   TestOpenNonWritableDatabaseIsPermissionDenied
--- PASS: TestOpenNonWritableDatabaseIsPermissionDenied ()
ok  	github.com/chris/sqloid/internal/connection	
```

Failure classification preserves structured causes: EACCES and EPERM render as 'permission denied', EROFS as 'read-only file system', and other raw OS/driver causes verbatim — always after the mandated prefix, always on a single line:

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/connection/ -run TestReadWriteDetailClassification -v 2>&1 | grep -E '^(=== RUN|--- (PASS|FAIL)|ok|FAIL)' | sed -E 's/[0-9]+\.[0-9]+s//g'
```

```output
=== RUN   TestReadWriteDetailClassification
=== RUN   TestReadWriteDetailClassification/EACCES_renders_permission_denied
=== RUN   TestReadWriteDetailClassification/EPERM_renders_permission_denied
=== RUN   TestReadWriteDetailClassification/EROFS_renders_read-only_file_system
=== RUN   TestReadWriteDetailClassification/raw_driver_causes_are_preserved_verbatim
--- PASS: TestReadWriteDetailClassification ()
ok  	github.com/chris/sqloid/internal/connection	
```

Process-level behavior through cmd/sqloid with the production connection handler wired from main.go: failing fixtures exit 1 with exactly one stderr line (asserted byte-for-byte in TestSQLiteStartupProcessBehavior), and a valid database exits 0 silently:

```bash
cd /home/chris/sqloid && go test -count=1 ./cmd/sqloid/ -run TestSQLiteStartupProcessBehavior -v 2>&1 | grep -E '^(=== RUN|--- (PASS|FAIL)|ok|FAIL)' | sed -E 's/[0-9]+\.[0-9]+s//g'
```

```output
=== RUN   TestSQLiteStartupProcessBehavior
=== RUN   TestSQLiteStartupProcessBehavior/valid_database_opens_silently
=== RUN   TestSQLiteStartupProcessBehavior/missing_file
=== RUN   TestSQLiteStartupProcessBehavior/invalid_header
=== RUN   TestSQLiteStartupProcessBehavior/non-writable_database
--- PASS: TestSQLiteStartupProcessBehavior ()
ok  	github.com/chris/sqloid/cmd/sqloid	
```

CLI rendering belongs to internal/cli: the handler error's Error() is printed verbatim by one Fprintln on stderr with exit status 1 (TestStartupFailuresRenderOneLineOnStderr covers missing, unreadable, directory, invalid-header, and non-writable fixtures end-to-end through Main), while successful startup is completely silent (TestSuccessfulStartupIsSilent).

Full verification: gofmt clean, go vet, go build.

```bash
cd /home/chris/sqloid && test -z "$(gofmt -l cmd internal)" && go vet ./... && go build ./... && echo 'gofmt + vet + build OK'
```

```output
gofmt + vet + build OK
```
