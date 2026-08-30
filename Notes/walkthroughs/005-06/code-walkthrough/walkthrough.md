# Issue #5 — Exact-two SQLite pool and dedicated leasing

*2026-08-26T21:48:56Z by Showboat 0.6.1*
<!-- showboat-id: b019ba1e-b73b-4a5b-97ea-b4ea48de1396 -->

Verification of Issue #5 (two-connection SQLite pool and limits) against the 'Connection pool, limits, and busy handling' decision in Notes/PRD-sqloid.md, run from the repository root at /home/chris/sqloid (test durations stripped for reproducibility).

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/connection/ | sed -E 's/[0-9]+\.[0-9]+s//g' | cut -c1-58
```

```output
ok  	github.com/chris/sqloid/internal/connection	
```

Pool and per-connection configuration: the exact-two pool contract, the five-second busy timeout and exact 64 MiB connection-local SQLITE_LIMIT_LENGTH on every inspected physical connection.

```bash
cd /home/chris/sqloid && go test -count=1 -v ./internal/connection/ -run '^(TestPoolHoldsExactlyTwoUsableConnections|TestEveryConnectionHasFiveSecondBusyTimeout|TestEveryConnectionHasExactLengthLimit)$' 2>&1 | grep -E '^(=== RUN|--- (PASS|FAIL)|ok|FAIL)' | sed -E 's/[0-9]+\.[0-9]+s//g'
```

```output
=== RUN   TestPoolHoldsExactlyTwoUsableConnections
--- PASS: TestPoolHoldsExactlyTwoUsableConnections ()
=== RUN   TestEveryConnectionHasFiveSecondBusyTimeout
--- PASS: TestEveryConnectionHasFiveSecondBusyTimeout ()
=== RUN   TestEveryConnectionHasExactLengthLimit
--- PASS: TestEveryConnectionHasExactLengthLimit ()
ok  	github.com/chris/sqloid/internal/connection	
```

Dedicated leasing in WAL and rollback-journal fixtures: concurrent callers hold both leases at once, receive distinct physical connections each carrying the five-second busy timeout and 64 MiB limit, and journal mode is unchanged by open-and-lease use.

```bash
cd /home/chris/sqloid && go test -count=1 -v ./internal/connection/ -run '^(TestConcurrentLeasesAreDistinctConnections|TestLeaseReleaseIsSafeAndRefusesReuse)$' 2>&1 | grep -E '^(=== RUN|--- (PASS|FAIL)|    ---|ok|FAIL)' | sed -E 's/[0-9]+\.[0-9]+s//g' | cut -c1-60
```

```output
=== RUN   TestConcurrentLeasesAreDistinctConnections
=== RUN   TestConcurrentLeasesAreDistinctConnections/delete
=== RUN   TestConcurrentLeasesAreDistinctConnections/wal
--- PASS: TestConcurrentLeasesAreDistinctConnections ()
    --- PASS: TestConcurrentLeasesAreDistinctConnections/del
    --- PASS: TestConcurrentLeasesAreDistinctConnections/wal
=== RUN   TestLeaseReleaseIsSafeAndRefusesReuse
--- PASS: TestLeaseReleaseIsSafeAndRefusesReuse ()
ok  	github.com/chris/sqloid/internal/connection	
```

Race-detector verification of the concurrency changes (cgo-enabled build solely for the race detector; production builds remain pure Go/no-cgo).

```bash
cd /home/chris/sqloid && CGO_ENABLED=1 go test -race -count=1 ./internal/connection/ | sed -E 's/[0-9]+\.[0-9]+s//g' | cut -c1-58
```

```output
ok  	github.com/chris/sqloid/internal/connection	
```

References: Issue #5 (exact-two pool, dedicated leasing, five-second busy handling, exact 64 MiB SQLITE_LIMIT_LENGTH, journal-mode invariants) and Notes/PRD-sqloid.md — 'Connection pool, limits, and busy handling' plus high-risk coverage item 2. Implementation: internal/connection/startup.go; tests: pool_config_test.go, lease_test.go; wiki: Notes/wiki/connection-pool.md.
