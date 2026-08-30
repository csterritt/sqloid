# # Issue #7 — request-boundary database identity checks walkthrough

*2026-08-26T23:18:46Z by Showboat 0.6.1*
<!-- showboat-id: a8d92097-906e-4573-b141-ee0f291e6d85 -->

Sqloid Issue #7 adds request-boundary database identity checks per the 'Session health' section of Notes/PRD-sqloid.md: at startup the opener records the validated target's device and inode; immediately before every database request — and before any newly opened or replacement pooled connection is admitted for use — the original path is re-statted and compared. There is no watcher, polling loop, or UI dependency anywhere in this mechanism, and terminal copy stays owned by Issue #46.

The whole internal/connection suite passes with the new coverage:

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/connection/ | sed -E 's/[0-9]+\.[0-9]+s//g' | cut -c1-58
```

```output
ok  	github.com/chris/sqloid/internal/connection	
```

Startup identity recording: after the full ordered validation succeeds (existence → readability → header → writability proof → mode=rw open → probe), Open re-stats the path and records the device/inode reference on the DB; a stat failure in that instant classifies as a read-write startup failure rather than leaving a zero reference.

```bash
cd /home/chris/sqloid && sed -n '/Record the validated target/,/startIno }, nil/p' internal/connection/startup.go
```

```output
	// Record the validated target's filesystem identity after the full
	// ordered validation succeeded; this reference backs every later
	// request-boundary verification per Issue #7.
	startDev, startIno, statErr := statIdentity(path)
	if statErr != nil {
		// The file vanished or became unstatable in the instant between
		// validation and recording; classify the same way as open failures.
		return closeOnError(classifyOpenError(path, statErr))
	}
	return &DB{SQL: sqlDB, path: path, startDev: startDev, startIno: startIno}, nil
}

// probe issues the harmless schema probe required after opening.
func probe(sqlDB *sql.DB) error {
	var version int64
	return sqlDB.QueryRow("PRAGMA schema_version").Scan(&version)
}

// classifyOpenError turns a driver-level mode=rw failure into the documented
// StartupError. Readability was already proven before opening, so any failure
// reaching this point belongs to the read-write class; its cause is kept
// unwrappable so permission-denied, read-only-filesystem, and other raw
// OS/driver details all render through readWriteDetail.
func classifyOpenError(path string, err error) error {
	return &StartupError{Path: path, Kind: FailureReadWrite, Cause: err}
}

// openReadWrite attempts an OS-level O_RDWR open of path without creation,
// returning its *PathError so errno classification stays lossless.
func openReadWrite(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	return f.Close()
}

// Session is the CLI-facing sqlite command handler: it starts a session on
// path and keeps it open until the caller's deferred Close point. For now it
// closes when the handler returns because no TUI consumes the handle yet;
// Issue #2 only requires successful startup to be silent.
func Session(path string) error {
	db, err := Open(path)
	if err != nil {
		return err
	}
	defer db.Close()
	return nil
}
```

The test pins that the recorded reference equals what the kernel reports for the validated target:

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/connection/ -run 'TestOpenRecordsStartupDeviceAndInode' -v 2>&1 | grep -E '^(--- |ok|FAIL)' | sed -E 's/[0-9]+\.[0-9]+s//g' | cut -c1-70
```

```output
--- PASS: TestOpenRecordsStartupDeviceAndInode ()
ok  	github.com/chris/sqloid/internal/connection	
```

Request-boundary verification is typed and copy-free: VerifyHealth stats the recorded path once per call and returns nil, or a *HealthError{Path, Kind, Cause}. Any stat failure classifies as absence (rename-away is deletion of the path); same-path replacement with a different device OR inode classifies as replaced; in-place mutation retaining both identifiers returns nil — ordinary SQLite behavior continues.

```bash
cd /home/chris/sqloid && sed -n '/func (db \*DB) VerifyHealth/,/^}/p' internal/connection/health.go
```

```output
func (db *DB) VerifyHealth() error {
	db.identityChecks.Add(1)
	if !statIdentitySupported() {
		return nil
	}
	dev, ino, err := statIdentity(db.path)
	if err != nil {
		return &HealthError{Path: db.path, Kind: HealthDeleted, Cause: err}
	}
	if dev != db.startDev || ino != db.startIno {
		return &HealthError{Path: db.path, Kind: HealthReplaced}
	}
	return nil
}
```

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/connection/ -run 'TestVerifyHealthClassifications' -v 2>&1 | grep -E '^(    --- |--- |ok|FAIL)' | sed -E 's/[0-9]+\.[0-9]+s//g' | cut -c1-70
```

```output
--- PASS: TestVerifyHealthClassifications ()
    --- PASS: TestVerifyHealthClassifications/unchanged_target_passes 
    --- PASS: TestVerifyHealthClassifications/deletion_reports_typed_a
    --- PASS: TestVerifyHealthClassifications/rename-away_reports_type
    --- PASS: TestVerifyHealthClassifications/same-path_replacement_wi
    --- PASS: TestVerifyHealthClassifications/in-place_same-inode_muta
ok  	github.com/chris/sqloid/internal/connection	
```

The boundary sits inside DB.Lease, which is acquired before any database work and before configuring/admitting a newly opened or replacement physical connection: no pooled connection is handed over unverified.

```bash
cd /home/chris/sqloid && sed -n '/func (db \*DB) Lease/,/^}/p' internal/connection/startup.go
```

```output
func (db *DB) Lease(ctx context.Context) (*Lease, error) {
	conn, err := db.SQL.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("lease connection from pool: %w", err)
	}
	// Request-boundary health check (Issue #7): no physical connection — a
	// retained pooled member or one newly opened/reopened for this lease — is
	// admitted for use until the original path still carries its startup
	// device and inode. This check is the pre-request check itself: work can
	// only follow after Lease returns, so every request begins verified.
	if err := db.VerifyHealth(); err != nil {
		conn.Close()
		return nil, err
	}
	if _, err := sqlite.Limit(conn, sqlite3.SQLITE_LIMIT_LENGTH, sqlMaxLengthBytes); err != nil {
		conn.Close()
		return nil, fmt.Errorf("configure leased connection length limit: %w", err)
	}
	return &Lease{conn: conn}, nil
}
```

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/connection/ -run 'TestLeaseVerifiesIdentityBeforeConnectionIsUsed' -v 2>&1 | grep -E '^(    --- |--- |ok|FAIL)' | sed -E 's/[0-9]+\.[0-9]+s//g' | cut -c1-70
```

```output
--- PASS: TestLeaseVerifiesIdentityBeforeConnectionIsUsed ()
    --- PASS: TestLeaseVerifiesIdentityBeforeConnectionIsUsed/deleted_
    --- PASS: TestLeaseVerifiesIdentityBeforeConnectionIsUsed/replacem
ok  	github.com/chris/sqloid/internal/connection	
```

RunRequest is the reusable request boundary: one identity check inside Lease → cancellable op on the dedicated lease (Issue #6 lifecycle) → settlement → post-error reclassification → release. Because Lease performs the pre-request check itself, RunRequest never checks twice at the same boundary. The next-request behavior for changed targets — blocked before work with exactly one check per boundary, operation never started:

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/connection/ -run 'TestRunRequestBlocksChangedTargetBeforeWork' -v 2>&1 | grep -E '^(    --- |--- |ok|FAIL)' | sed -E 's/[0-9]+\.[0-9]+s//g' | cut -c1-70
```

```output
--- PASS: TestRunRequestBlocksChangedTargetBeforeWork ()
    --- PASS: TestRunRequestBlocksChangedTargetBeforeWork/deleted ()
    --- PASS: TestRunRequestBlocksChangedTargetBeforeWork/replaced ()
ok  	github.com/chris/sqloid/internal/connection	
```

Races, driven by barriers (never sleeps): the target is replaced strictly after a request's successful precheck — proven by the operation signalling 'started' before the mutation happens. A request that then fails is reclassified immediately: deletion/replacement takes precedence over the preserved ordinary SQLite error. A successful result stands; the NEXT boundary then detects the replacement before its operation starts.

```bash
cd /home/chris/sqloid && sed -n '/A raced deletion or replacement wins/,/return RequestResult{Outcome: OutcomeFailed, Err: cause}/p' internal/connection/health.go
```

```output
			// A raced deletion or replacement wins the required race: the
			// typed classification takes precedence over ordinary handling,
			// while the request's own cause stays preserved alongside it.
			return RequestResult{Outcome: OutcomeFailed, Err: cause, Health: he}
		}
		return RequestResult{Outcome: OutcomeFailed, Err: cause}
```

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/connection/ -run 'TestPostErrorReclassificationPrecedence|TestRacedReplacementThenSuccessStands' -v 2>&1 | grep -E '^(    --- |--- |ok|FAIL)' | sed -E 's/[0-9]+\.[0-9]+s//g' | cut -c1-70
```

```output
--- PASS: TestPostErrorReclassificationPrecedence ()
    --- PASS: TestPostErrorReclassificationPrecedence/raced_deletion_b
    --- PASS: TestPostErrorReclassificationPrecedence/raced_replacemen
    --- PASS: TestPostErrorReclassificationPrecedence/ordinary_failure
--- PASS: TestRacedReplacementThenSuccessStands ()
ok  	github.com/chris/sqloid/internal/connection	
```

Newly opened pooled connections: two barrier-rendezvoused RunRequests hold the pool's exact two connections simultaneously and each proves its own boundary check and distinct physical connection (Issue #5 pool).

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/connection/ -run 'TestRunRequestExercisesReusableBoundaries/concurrent_pooled_requests_use_distinct_checked_connections' -v 2>&1 | grep -E '^(    --- |--- |ok|FAIL)' | sed -E 's/[0-9]+\.[0-9]+s//g' | cut -c1-70
```

```output
--- PASS: TestRunRequestExercisesReusableBoundaries ()
    --- PASS: TestRunRequestExercisesReusableBoundaries/concurrent_poo
ok  	github.com/chris/sqloid/internal/connection	
```

Finally the phased write boundary: an entire write transaction is ONE request, so it receives exactly one pre-BEGIN check (the Lease check) with none between statement execution and COMMIT — pinned for both committed and rolled-back phases by sampling the instrumented counter inside the transaction.

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/connection/ -run 'TestPhasedWriteReceivesExactlyOnePreBEGINCheck' -v 2>&1 | grep -E '^(    --- |--- |ok|FAIL)' | sed -E 's/[0-9]+\.[0-9]+s//g' | cut -c1-70
```

```output
--- PASS: TestPhasedWriteReceivesExactlyOnePreBEGINCheck ()
    --- PASS: TestPhasedWriteReceivesExactlyOnePreBEGINCheck/committed
    --- PASS: TestPhasedWriteReceivesExactlyOnePreBEGINCheck/rolled-ba
ok  	github.com/chris/sqloid/internal/connection	
```

Race-detector verification of the whole package (concurrency changes), per the project's verification standard, plus full-suite green:

```bash
cd /home/chris/sqloid && CGO_ENABLED=1 go test -race -count=1 ./internal/connection/ | sed -E 's/[0-9]+\.[0-9]+s//g' | cut -c1-58
```

```output
ok  	github.com/chris/sqloid/internal/connection	
```

The semantics documented here are ingested into the wiki at Notes/wiki/session-health.md (Issue #7 and the Session health section of Notes/PRD-sqloid.md); terminal UI wording remains owned by Issue #46.
