# Issue #060 Code Walkthrough: Classify Pre-Lease Cancellation Correctly

*2026-08-31T22:57:54Z by Showboat 0.6.1*
<!-- showboat-id: 2fc3df23-f93b-4e9a-80e5-8db54043878a -->

Issue #60 (Notes/tasks/060-classify-prelease-cancellation.md, Notes/PRD-sqloid.md §Identities and state, §Errors and cancellation bounds, §Connection/cancellation invariant; user stories 12, 14, 82) classifies cancellation that arrives while a database request is queued for a lease — before any connection is acquired, before any operation callback runs, before BEGIN, before any statement, and before any transaction or phase work starts. Before this issue, a wrapped context.Canceled from the database/sql pool's Conn(ctx) wait was classified as OutcomeFailed/WriteFailed at every entry point, contradicting the PRD's cancellation-wins contract. Issue #60 adds a cancellation-first check (errors.Is(err, context.Canceled)) before the typed HealthError check (errors.As) and the ordinary failure fallback in the lease-acquisition error branches of RunRequest (health.go), startRequest (started_request.go), and StartWrite (write.go). The existing OutcomeCancelled/WriteCancelled shape is returned with the cancellation cause preserved; genuine HealthError values stay classified as failed-with-health (not masked); and ordinary lease failures (e.g. context.DeadlineExceeded) are unchanged.

The fix lives in the lease-acquisition error branches of three entry points. Each follows the same ordering: cancellation first, health second, ordinary failure last. The error from DB.Lease wraps the database/sql pool error through fmt.Errorf("lease connection from pool: %w", err), so errors.Is(err, context.Canceled) traverses the wrapping chain. A *HealthError from VerifyHealth is returned directly (unwrapped), so errors.As(err, &he) matches it. The two branches are mutually exclusive — DB.Lease fails at either db.SQL.Conn (context/pool error) or VerifyHealth (health error), never both — but the explicit ordering guarantees the contract.

```bash
grep -n 'func (db \*DB) RunRequest' internal/connection/health.go
```

```output
122:func (db *DB) RunRequest(parent context.Context, op func(ctx context.Context, conn *sql.Conn) error) RequestResult {
```

```bash
sed -n '122,145p' internal/connection/health.go
```

```output
func (db *DB) RunRequest(parent context.Context, op func(ctx context.Context, conn *sql.Conn) error) RequestResult {
	lease, err := db.Lease(parent)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return RequestResult{Outcome: OutcomeCancelled, Err: err}
		}
		var he *HealthError
		if errors.As(err, &he) {
			return RequestResult{Outcome: OutcomeFailed, Health: he}
		}
		return RequestResult{Outcome: OutcomeFailed, Err: err}
	}

	request := lease.BeginRequest(parent)
	var opErr error
	outcome := request.Run(func(ctx context.Context) error {
		opErr = op(ctx, lease.Conn())
		return opErr
	})
	closeErr := request.Close()

	switch outcome {
	case OutcomeSuccess:
		return RequestResult{Outcome: OutcomeSuccess, Err: closeErr}
```

RunRequest is the synchronous general-purpose boundary. The cancellation check is the first branch in the lease-error block: errors.Is(err, context.Canceled) returns OutcomeCancelled with the cause preserved in Err. The health check follows, and the ordinary failure fallback is last. No Request is created, no op is invoked, and no lease is released (none was acquired).

```bash
sed -n '44,72p' internal/connection/started_request.go
```

```output
func (db *DB) startRequest(parent context.Context, op func(ctx context.Context, conn *sql.Conn) error) *StartedRequest {
	s := &StartedRequest{
		done:    make(chan RequestResult, 1),
		settled: make(chan struct{}),
	}

	lease, err := db.Lease(parent)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			s.done <- RequestResult{Outcome: OutcomeCancelled, Err: err}
		} else {
			var he *HealthError
			if errors.As(err, &he) {
				s.done <- RequestResult{Outcome: OutcomeFailed, Health: he}
			} else {
				s.done <- RequestResult{Outcome: OutcomeFailed, Err: err}
			}
		}
		close(s.settled)
		return s
	}

	request := lease.BeginRequest(parent)
	s.req = request
	go func() {
		opErr := op(request.Context(), lease.Conn())
		outcome := request.Settle(opErr)
		closeErr := request.Close()
		var res RequestResult
```

startRequest backs StartFirstPage, StartPage, and StartCount. The same cancellation-first ordering pre-settles the StartedRequest.done channel (buffered 1) and closes the settled channel before returning the handle. Wait receives the pre-settled result exactly once. No Request is created and no work goroutine is started.

```bash
sed -n '178,210p' internal/connection/write.go
```

```output
func (db *DB) StartWrite(parent context.Context, execution uint64, statement string, params []any) *StartedWriteRequest {
	w := &StartedWriteRequest{
		execution: execution,
		owner:     db,
		phases:    make(chan WritePhaseMsg, 4),
		settled:   make(chan struct{}),
	}

	lease, err := db.Lease(parent)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			w.deliver(WriteResult{Outcome: WriteCancelled, Err: err})
		} else {
			var he *HealthError
			if errors.As(err, &he) {
				w.deliver(WriteResult{Outcome: WriteFailed, Err: err, Health: he})
			} else {
				w.deliver(WriteResult{Outcome: WriteFailed, Err: err})
			}
		}
		return w
	}
	if db.writeLeaseHook != nil {
		db.writeLeaseHook(lease)
	}

	request := lease.BeginRequest(parent)
	w.req = request
	w.lease = lease
	conn := lease.Conn()
	go func() {
		res := w.run(conn, statement, params)
		outcome := request.Settle(res.Err)
```

StartWrite follows the same pattern: the cancellation branch delivers WriteCancelled through the deliver method (which closes the phase channel and the settled channel). No writeLeaseHook is invoked, no beforeWriteBegin hook fires, no phases are emitted, and no work goroutine starts. The phase channel is closed empty, so collectPhases returns an empty slice.

The synchronized tests in internal/connection/prelease_cancellation_test.go prove the contract at every entry point. Each hold-and-cancel test saturates both pool connections with holdConcurrentLeases, starts a third request through a goroutine that signals on a channel before entering lease acquisition, waits for that signal, cancels the context, and asserts the cancelled outcome with no work started. Synchronization is channel-based throughout — no sleeps.

```bash
go test -count=1 ./internal/connection -run 'TestRunRequestCancelledBeforeLeaseAcquisition|TestStartedFirstPageCancelledBeforeLeaseAcquisition|TestStartedPageCancelledBeforeLeaseAcquisition|TestStartedCountCancelledBeforeLeaseAcquisition|TestStartWriteCancelledBeforeLeaseAcquisition' -v 2>&1
```

```output
=== RUN   TestRunRequestCancelledBeforeLeaseAcquisition
--- PASS: TestRunRequestCancelledBeforeLeaseAcquisition (0.01s)
=== RUN   TestStartedFirstPageCancelledBeforeLeaseAcquisition
--- PASS: TestStartedFirstPageCancelledBeforeLeaseAcquisition (0.01s)
=== RUN   TestStartedPageCancelledBeforeLeaseAcquisition
--- PASS: TestStartedPageCancelledBeforeLeaseAcquisition (0.01s)
=== RUN   TestStartedCountCancelledBeforeLeaseAcquisition
--- PASS: TestStartedCountCancelledBeforeLeaseAcquisition (0.01s)
=== RUN   TestStartWriteCancelledBeforeLeaseAcquisition
--- PASS: TestStartWriteCancelledBeforeLeaseAcquisition (0.01s)
PASS
ok  	github.com/chris/sqloid/internal/connection	0.053s
```

All five hold-and-cancel tests pass: RunRequest settles as OutcomeCancelled, started first-page/page/count settle as OutcomeCancelled with nil page/zero total, and StartWrite settles as WriteCancelled with no phases. Each test verifies no operation callback or hook ran and both pool connections are usable after the holders are released.

The direct classification rows contrast the three failure shapes at each entry point. A wrapped context.Canceled classifies as cancelled (cancellation precedence). A typed *HealthError from VerifyHealth classifies as failed with Health set (not masked by cancellation). An ordinary context.DeadlineExceeded classifies as failed (unchanged). The health row requires device/inode support (linux/darwin) and is skipped on other platforms.

```bash
go test -count=1 ./internal/connection -run 'TestPreLeaseCancellationClassification' -v 2>&1
```

```output
=== RUN   TestPreLeaseCancellationClassificationRunRequest
=== RUN   TestPreLeaseCancellationClassificationRunRequest/wrapped_context.Canceled_classifies_cancelled
=== RUN   TestPreLeaseCancellationClassificationRunRequest/typed_HealthError_classifies_failed_with_health
=== RUN   TestPreLeaseCancellationClassificationRunRequest/ordinary_lease_failure_classifies_failed
--- PASS: TestPreLeaseCancellationClassificationRunRequest (0.01s)
    --- PASS: TestPreLeaseCancellationClassificationRunRequest/wrapped_context.Canceled_classifies_cancelled (0.00s)
    --- PASS: TestPreLeaseCancellationClassificationRunRequest/typed_HealthError_classifies_failed_with_health (0.00s)
    --- PASS: TestPreLeaseCancellationClassificationRunRequest/ordinary_lease_failure_classifies_failed (0.00s)
=== RUN   TestPreLeaseCancellationClassificationStartedRequest
=== RUN   TestPreLeaseCancellationClassificationStartedRequest/wrapped_context.Canceled_classifies_cancelled
=== RUN   TestPreLeaseCancellationClassificationStartedRequest/typed_HealthError_classifies_failed_with_health
=== RUN   TestPreLeaseCancellationClassificationStartedRequest/ordinary_lease_failure_classifies_failed
--- PASS: TestPreLeaseCancellationClassificationStartedRequest (0.01s)
    --- PASS: TestPreLeaseCancellationClassificationStartedRequest/wrapped_context.Canceled_classifies_cancelled (0.00s)
    --- PASS: TestPreLeaseCancellationClassificationStartedRequest/typed_HealthError_classifies_failed_with_health (0.00s)
    --- PASS: TestPreLeaseCancellationClassificationStartedRequest/ordinary_lease_failure_classifies_failed (0.00s)
=== RUN   TestPreLeaseCancellationClassificationWrite
=== RUN   TestPreLeaseCancellationClassificationWrite/wrapped_context.Canceled_classifies_cancelled
=== RUN   TestPreLeaseCancellationClassificationWrite/typed_HealthError_classifies_failed_with_health
=== RUN   TestPreLeaseCancellationClassificationWrite/ordinary_lease_failure_classifies_failed
--- PASS: TestPreLeaseCancellationClassificationWrite (0.01s)
    --- PASS: TestPreLeaseCancellationClassificationWrite/wrapped_context.Canceled_classifies_cancelled (0.00s)
    --- PASS: TestPreLeaseCancellationClassificationWrite/typed_HealthError_classifies_failed_with_health (0.00s)
    --- PASS: TestPreLeaseCancellationClassificationWrite/ordinary_lease_failure_classifies_failed (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/connection	0.022s
```

The race detector passes over the connection package, proving the channel-based synchronization has no data races in the hold-and-cancel or classification tests.

```bash
CGO_ENABLED=1 go test -race ./internal/connection -run 'TestPreLeaseCancellation|TestRunRequestCancelledBeforeLease|TestStartedFirstPageCancelledBeforeLease|TestStartedPageCancelledBeforeLease|TestStartedCountCancelledBeforeLease|TestStartWriteCancelledBeforeLease' -count=1 2>&1
```

```output
ok  	github.com/chris/sqloid/internal/connection	1.141s
```

After Issue #57's production TUI composition (internal/session), the shipped application routes Ctrl+W through the UI's cancellation seam to the connection-layer handles. When a queued read or write is cancelled before lease acquisition, the UI observes the cancelled outcome through the same adapter that maps connection.RequestResult onto typed UI results (cancellation via errors.Is(err, context.Canceled)). The cancelling... feedback clears at settlement, no replacement dispatch occurs before settlement, and a follow-up request succeeds on the reusable pool. Issue #57's composition root wires every database seam (Select, Count, Page, Write) through thin adapters over the real *connection.DB, so the Issue #60 classification reaches the UI unchanged: a pre-lease cancelled request presents as cancelled, not failed, and the user's follow-up Enter after settlement launches a healthy new request. See Notes/PRD-sqloid.md §Identities and state, §Errors and cancellation bounds, and §Global Key Precedence and Context/Action Matrix for the Ctrl+W contract (user stories 12, 14, 82).
