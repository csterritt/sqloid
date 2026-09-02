// Pre-lease cancellation classification tests for Issue #60: when both
// connections from the exact-two pool are held, a third RunRequest,
// startRequest-backed first-page/page/count operation, or StartWrite that
// is queued for a lease and then cancelled before acquisition must settle
// exactly once as the existing cancelled outcome, start no operation
// callback, BEGIN, statement, transaction hook, or phase work, and leave
// no replacement work before settlement. Direct classification rows prove
// cancellation precedence without masking typed HealthError or changing
// non-cancellation lease failures. After the holders are released both
// pool connections and a subsequent request/write remain usable.
//
// Synchronization is channel-based throughout: a started signal proves the
// request goroutine is about to enter lease acquisition, and the both-held
// invariant guarantees the call blocks (or returns immediately with the
// already-cancelled context error) — never a sleep.

package connection

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// assertBothPoolConnectionsUsable releases both holders and requires both
// pool connections to answer harmless work, proving pre-lease cancellation
// never force-closed or corrupted the pool.
func assertBothPoolConnectionsUsable(t *testing.T, db *DB, held [2]*Lease) {
	t.Helper()
	ctx := context.Background()
	held[0].Release(ctx)
	held[1].Release(ctx)
	for i := 0; i < 2; i++ {
		lease, err := db.Lease(ctx)
		if err != nil {
			t.Fatalf("lease %d after pre-lease cancellation: %v", i+1, err)
		}
		var version int64
		if err := lease.Conn().QueryRowContext(ctx, "PRAGMA schema_version").Scan(&version); err != nil {
			t.Fatalf("harmless work on lease %d after pre-lease cancellation: %v", i+1, err)
		}
		lease.Release(ctx)
	}
}

// TestRunRequestCancelledBeforeLeaseAcquisition holds both pool
// connections, starts a third RunRequest, waits until it is queued for a
// lease through the started signal, cancels its context before releasing
// either holder, and requires the request to settle exactly once as
// OutcomeCancelled with no operation callback invoked and the cancellation
// cause preserved. After releasing the holders, both pool connections and a
// subsequent RunRequest remain usable.
func TestRunRequestCancelledBeforeLeaseAcquisition(t *testing.T) {
	ctx := context.Background()
	db, _ := setJournalAndOpen(t, "delete")
	held := holdConcurrentLeases(t, db)

	reqCtx, reqCancel := context.WithCancel(ctx)
	opCalled := make(chan struct{}, 1)
	started := make(chan struct{})
	result := make(chan RequestResult, 1)
	go func() {
		close(started) // synchronized: about to enter Lease
		res := db.RunRequest(reqCtx, func(ctx context.Context, conn *sql.Conn) error {
			close(opCalled)
			return nil
		})
		result <- res
	}()

	<-started
	reqCancel()

	var res RequestResult
	select {
	case res = <-result:
	case <-time.After(time.Minute):
		t.Fatal("RunRequest did not settle after pre-lease cancellation")
	}
	if res.Outcome != OutcomeCancelled {
		t.Fatalf("pre-lease cancellation outcome = %v, want cancelled", res.Outcome)
	}
	if res.Health != nil {
		t.Fatalf("pre-lease cancellation health = %v, want nil (no health failure occurred)", res.Health)
	}
	if res.Err == nil || !errors.Is(res.Err, context.Canceled) {
		t.Fatalf("pre-lease cancellation err = %v, want a preserved context.Canceled cause", res.Err)
	}
	select {
	case <-opCalled:
		t.Fatal("operation callback ran despite pre-lease cancellation; the lease was never acquired")
	default:
	}

	assertBothPoolConnectionsUsable(t, db, held)

	followUp := db.RunRequest(ctx, func(ctx context.Context, conn *sql.Conn) error {
		var version int64
		return conn.QueryRowContext(ctx, "PRAGMA schema_version").Scan(&version)
	})
	if followUp.Outcome != OutcomeSuccess {
		t.Fatalf("subsequent RunRequest outcome = %v, want success", followUp.Outcome)
	}
}

// TestStartedFirstPageCancelledBeforeLeaseAcquisition holds both pool
// connections, starts a third StartFirstPage, waits until it is queued,
// cancels its context, and requires the started request to settle exactly
// once as OutcomeCancelled with a nil page and no beforeFirstPage hook
// invoked. After releasing the holders, both pool connections remain
// usable.
func TestStartedFirstPageCancelledBeforeLeaseAcquisition(t *testing.T) {
	ctx := context.Background()
	db := openMixed(t)
	held := holdConcurrentLeases(t, db)

	hookReached := make(chan struct{}, 1)
	db.beforeFirstPage = func(ctx context.Context, conn *sql.Conn) {
		close(hookReached)
	}
	t.Cleanup(func() { db.beforeFirstPage = nil })

	reqCtx, reqCancel := context.WithCancel(ctx)
	started := make(chan struct{})
	handleCh := make(chan *StartedPageRequest, 1)
	go func() {
		close(started)
		handleCh <- db.StartFirstPage(reqCtx, `SELECT id FROM "mix"`, nil)
	}()

	<-started
	reqCancel()

	var page *StartedPageRequest
	select {
	case page = <-handleCh:
	case <-time.After(time.Minute):
		t.Fatal("StartFirstPage did not return after pre-lease cancellation")
	}
	p, res := page.Wait()
	if p != nil {
		t.Fatalf("cancelled pre-lease first page returned a page %+v, want nil", p)
	}
	if res.Outcome != OutcomeCancelled {
		t.Fatalf("pre-lease cancellation outcome = %v, want cancelled", res.Outcome)
	}
	if res.Health != nil {
		t.Fatalf("pre-lease cancellation health = %v, want nil", res.Health)
	}
	if res.Err == nil || !errors.Is(res.Err, context.Canceled) {
		t.Fatalf("pre-lease cancellation err = %v, want a preserved context.Canceled cause", res.Err)
	}
	select {
	case <-hookReached:
		t.Fatal("beforeFirstPage hook ran despite pre-lease cancellation; the lease was never acquired")
	default:
	}

	assertBothPoolConnectionsUsable(t, db, held)
}

// TestStartedPageCancelledBeforeLeaseAcquisition holds both pool
// connections, starts a third StartPage (later-page SELECT), waits until
// it is queued, cancels its context, and requires the started request to
// settle exactly once as OutcomeCancelled with a nil page. After releasing
// the holders, both pool connections remain usable.
func TestStartedPageCancelledBeforeLeaseAcquisition(t *testing.T) {
	ctx := context.Background()
	db := openMixed(t)
	held := holdConcurrentLeases(t, db)

	reqCtx, reqCancel := context.WithCancel(ctx)
	started := make(chan struct{})
	handleCh := make(chan *StartedPageRequest, 1)
	go func() {
		close(started)
		handleCh <- db.StartPage(reqCtx, `SELECT id FROM "mix"`, nil, 0)
	}()

	<-started
	reqCancel()

	var page *StartedPageRequest
	select {
	case page = <-handleCh:
	case <-time.After(time.Minute):
		t.Fatal("StartPage did not return after pre-lease cancellation")
	}
	p, res := page.Wait()
	if p != nil {
		t.Fatalf("cancelled pre-lease page returned a page %+v, want nil", p)
	}
	if res.Outcome != OutcomeCancelled {
		t.Fatalf("pre-lease cancellation outcome = %v, want cancelled", res.Outcome)
	}
	if res.Health != nil {
		t.Fatalf("pre-lease cancellation health = %v, want nil", res.Health)
	}
	if res.Err == nil || !errors.Is(res.Err, context.Canceled) {
		t.Fatalf("pre-lease cancellation err = %v, want a preserved context.Canceled cause", res.Err)
	}

	assertBothPoolConnectionsUsable(t, db, held)
}

// TestStartedCountCancelledBeforeLeaseAcquisition holds both pool
// connections, starts a third StartCount, waits until it is queued,
// cancels its context, and requires the started request to settle exactly
// once as OutcomeCancelled with a zero total and no beforeCount hook
// invoked. After releasing the holders, both pool connections remain
// usable.
func TestStartedCountCancelledBeforeLeaseAcquisition(t *testing.T) {
	ctx := context.Background()
	db := openMixed(t)
	held := holdConcurrentLeases(t, db)

	hookReached := make(chan struct{}, 1)
	db.beforeCount = func(ctx context.Context, conn *sql.Conn) {
		close(hookReached)
	}
	t.Cleanup(func() { db.beforeCount = nil })

	reqCtx, reqCancel := context.WithCancel(ctx)
	started := make(chan struct{})
	handleCh := make(chan *StartedCountRequest, 1)
	go func() {
		close(started)
		handleCh <- db.StartCount(reqCtx, `SELECT COUNT(*) FROM (SELECT id FROM "mix")`, nil)
	}()

	<-started
	reqCancel()

	var count *StartedCountRequest
	select {
	case count = <-handleCh:
	case <-time.After(time.Minute):
		t.Fatal("StartCount did not return after pre-lease cancellation")
	}
	total, res := count.Wait()
	if total != 0 {
		t.Fatalf("cancelled pre-lease count returned total %d, want zero", total)
	}
	if res.Outcome != OutcomeCancelled {
		t.Fatalf("pre-lease cancellation outcome = %v, want cancelled", res.Outcome)
	}
	if res.Health != nil {
		t.Fatalf("pre-lease cancellation health = %v, want nil", res.Health)
	}
	if res.Err == nil || !errors.Is(res.Err, context.Canceled) {
		t.Fatalf("pre-lease cancellation err = %v, want a preserved context.Canceled cause", res.Err)
	}
	select {
	case <-hookReached:
		t.Fatal("beforeCount hook ran despite pre-lease cancellation; the lease was never acquired")
	default:
	}

	assertBothPoolConnectionsUsable(t, db, held)
}

// TestStartWriteCancelledBeforeLeaseAcquisition holds both pool
// connections, starts a third StartWrite, waits until it is queued,
// cancels its context, and requires the write to settle exactly once as
// WriteCancelled with no phases emitted, no writeLeaseHook or
// beforeWriteBegin invoked, and the cancellation cause preserved. After
// releasing the holders, both pool connections and a subsequent write
// remain usable.
func TestStartWriteCancelledBeforeLeaseAcquisition(t *testing.T) {
	ctx := context.Background()
	db := openJournalFixture(t, "delete")
	held := holdConcurrentLeases(t, db)

	leaseHookReached := make(chan struct{}, 1)
	db.writeLeaseHook = func(l *Lease) {
		close(leaseHookReached)
	}
	beginHookReached := make(chan struct{}, 1)
	db.beforeWriteBegin = func(ctx context.Context, conn *sql.Conn) {
		close(beginHookReached)
	}
	t.Cleanup(func() {
		db.writeLeaseHook = nil
		db.beforeWriteBegin = nil
	})

	reqCtx, reqCancel := context.WithCancel(ctx)
	started := make(chan struct{})
	handleCh := make(chan *StartedWriteRequest, 1)
	go func() {
		close(started)
		handleCh <- db.StartWrite(reqCtx, writeExecSeq.Add(1), `UPDATE "users" SET "email" = 'new'`, nil)
	}()

	<-started
	reqCancel()

	var w *StartedWriteRequest
	select {
	case w = <-handleCh:
	case <-time.After(time.Minute):
		t.Fatal("StartWrite did not return after pre-lease cancellation")
	}
	res := w.Wait()
	if res.Outcome != WriteCancelled {
		t.Fatalf("pre-lease cancellation write outcome = %v, want cancelled", res.Outcome)
	}
	if res.Health != nil {
		t.Fatalf("pre-lease cancellation write health = %v, want nil", res.Health)
	}
	if res.Err == nil || !errors.Is(res.Err, context.Canceled) {
		t.Fatalf("pre-lease cancellation write err = %v, want a preserved context.Canceled cause", res.Err)
	}
	if res.RollbackConfirmed {
		t.Fatal("pre-lease cancellation reported rollback confirmed; no transaction was ever begun")
	}
	phases := collectPhases(t, w)
	if len(phases) != 0 {
		t.Fatalf("pre-lease cancellation emitted phases %v, want none (no lease was acquired)", phases)
	}
	select {
	case <-leaseHookReached:
		t.Fatal("writeLeaseHook ran despite pre-lease cancellation; the lease was never acquired")
	default:
	}
	select {
	case <-beginHookReached:
		t.Fatal("beforeWriteBegin hook ran despite pre-lease cancellation; no transaction was begun")
	default:
	}

	assertBothPoolConnectionsUsable(t, db, held)
	assertLeaseReusable(t, db, `UPDATE "users" SET "email" = 'reused'`)
}

// TestPreLeaseCancellationClassificationRunRequest proves the classification
// precedence at the RunRequest entry point through direct rows: a wrapped
// context.Canceled classifies as OutcomeCancelled (cancellation precedence),
// a typed HealthError classifies as OutcomeFailed with Health set (health
// not masked), and an ordinary lease failure (context.DeadlineExceeded)
// classifies as OutcomeFailed unchanged.
func TestPreLeaseCancellationClassificationRunRequest(t *testing.T) {
	ctx := context.Background()
	db, path := setJournalAndOpen(t, "delete")

	// Pool one idle connection so the HealthError row reaches VerifyHealth
	// rather than failing at db.SQL.Conn.
	healthy, err := db.Lease(ctx)
	if err != nil {
		t.Fatalf("initial lease: %v", err)
	}
	healthy.Release(ctx)

	for _, tc := range []struct {
		name        string
		setupCtx    func() (context.Context, context.CancelFunc)
		mutate      func(t *testing.T)
		wantOutcome Outcome
		wantHealth  bool
		wantErrIs   error
	}{
		{
			name: "wrapped context.Canceled classifies cancelled",
			setupCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(ctx)
				cancel()
				return ctx, cancel
			},
			wantOutcome: OutcomeCancelled,
			wantErrIs:   context.Canceled,
		},
		{
			name:        "typed HealthError classifies failed with health",
			setupCtx:    func() (context.Context, context.CancelFunc) { return ctx, func() {} },
			mutate:      func(t *testing.T) { os.Remove(path) },
			wantOutcome: OutcomeFailed,
			wantHealth:  true,
		},
		{
			name: "ordinary lease failure classifies failed",
			setupCtx: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(ctx, time.Now().Add(-time.Second))
			},
			wantOutcome: OutcomeFailed,
			wantErrIs:   context.DeadlineExceeded,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.mutate != nil && !statIdentitySupported() {
				t.Skip("health classification requires device/inode support (linux or darwin)")
			}
			if tc.mutate != nil {
				tc.mutate(t)
			}
			reqCtx, cancel := tc.setupCtx()
			defer cancel()

			opCalled := make(chan struct{}, 1)
			res := db.RunRequest(reqCtx, func(ctx context.Context, conn *sql.Conn) error {
				close(opCalled)
				return nil
			})
			if res.Outcome != tc.wantOutcome {
				t.Fatalf("outcome = %v, want %v", res.Outcome, tc.wantOutcome)
			}
			if tc.wantHealth {
				if res.Health == nil {
					t.Fatal("want typed HealthError, got nil health")
				}
			} else if res.Health != nil {
				t.Fatalf("want no health, got %v", res.Health)
			}
			if tc.wantErrIs != nil && (res.Err == nil || !errors.Is(res.Err, tc.wantErrIs)) {
				t.Fatalf("err = %v, want errors.Is(_, %v)", res.Err, tc.wantErrIs)
			}
			select {
			case <-opCalled:
				t.Fatal("operation callback ran; the lease should never have been acquired")
			default:
			}
		})
	}
}

// TestPreLeaseCancellationClassificationStartedRequest proves the
// classification precedence at the startRequest entry point (via
// StartFirstPage) through direct rows: a wrapped context.Canceled classifies
// as OutcomeCancelled, a typed HealthError classifies as OutcomeFailed with
// Health set, and an ordinary lease failure classifies as OutcomeFailed
// unchanged.
func TestPreLeaseCancellationClassificationStartedRequest(t *testing.T) {
	ctx := context.Background()
	db, path := setJournalAndOpen(t, "delete")

	healthy, err := db.Lease(ctx)
	if err != nil {
		t.Fatalf("initial lease: %v", err)
	}
	healthy.Release(ctx)

	for _, tc := range []struct {
		name        string
		setupCtx    func() (context.Context, context.CancelFunc)
		mutate      func(t *testing.T)
		wantOutcome Outcome
		wantHealth  bool
		wantErrIs   error
	}{
		{
			name: "wrapped context.Canceled classifies cancelled",
			setupCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(ctx)
				cancel()
				return ctx, cancel
			},
			wantOutcome: OutcomeCancelled,
			wantErrIs:   context.Canceled,
		},
		{
			name:        "typed HealthError classifies failed with health",
			setupCtx:    func() (context.Context, context.CancelFunc) { return ctx, func() {} },
			mutate:      func(t *testing.T) { os.Remove(path) },
			wantOutcome: OutcomeFailed,
			wantHealth:  true,
		},
		{
			name: "ordinary lease failure classifies failed",
			setupCtx: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(ctx, time.Now().Add(-time.Second))
			},
			wantOutcome: OutcomeFailed,
			wantErrIs:   context.DeadlineExceeded,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.mutate != nil && !statIdentitySupported() {
				t.Skip("health classification requires device/inode support (linux or darwin)")
			}
			if tc.mutate != nil {
				tc.mutate(t)
			}
			reqCtx, cancel := tc.setupCtx()
			defer cancel()

			hookReached := make(chan struct{}, 1)
			db.beforeFirstPage = func(ctx context.Context, conn *sql.Conn) { close(hookReached) }
			t.Cleanup(func() { db.beforeFirstPage = nil })

			page := db.StartFirstPage(reqCtx, `SELECT id FROM "mix"`, nil)
			p, res := page.Wait()
			if p != nil {
				t.Fatalf("returned a page %+v, want nil (no work should have run)", p)
			}
			if res.Outcome != tc.wantOutcome {
				t.Fatalf("outcome = %v, want %v", res.Outcome, tc.wantOutcome)
			}
			if tc.wantHealth {
				if res.Health == nil {
					t.Fatal("want typed HealthError, got nil health")
				}
			} else if res.Health != nil {
				t.Fatalf("want no health, got %v", res.Health)
			}
			if tc.wantErrIs != nil && (res.Err == nil || !errors.Is(res.Err, tc.wantErrIs)) {
				t.Fatalf("err = %v, want errors.Is(_, %v)", res.Err, tc.wantErrIs)
			}
			select {
			case <-hookReached:
				t.Fatal("beforeFirstPage hook ran; the lease should never have been acquired")
			default:
			}
		})
	}
}

// TestPreLeaseCancellationClassificationWrite proves the classification
// precedence at the StartWrite entry point through direct rows: a wrapped
// context.Canceled classifies as WriteCancelled, a typed HealthError
// classifies as WriteFailed with Health set, and an ordinary lease failure
// classifies as WriteFailed unchanged.
func TestPreLeaseCancellationClassificationWrite(t *testing.T) {
	ctx := context.Background()
	db, path := setJournalAndOpen(t, "delete")

	healthy, err := db.Lease(ctx)
	if err != nil {
		t.Fatalf("initial lease: %v", err)
	}
	healthy.Release(ctx)

	for _, tc := range []struct {
		name        string
		setupCtx    func() (context.Context, context.CancelFunc)
		mutate      func(t *testing.T)
		wantOutcome WriteOutcome
		wantHealth  bool
		wantErrIs   error
	}{
		{
			name: "wrapped context.Canceled classifies cancelled",
			setupCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(ctx)
				cancel()
				return ctx, cancel
			},
			wantOutcome: WriteCancelled,
			wantErrIs:   context.Canceled,
		},
		{
			name:        "typed HealthError classifies failed with health",
			setupCtx:    func() (context.Context, context.CancelFunc) { return ctx, func() {} },
			mutate:      func(t *testing.T) { os.Remove(path) },
			wantOutcome: WriteFailed,
			wantHealth:  true,
		},
		{
			name: "ordinary lease failure classifies failed",
			setupCtx: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(ctx, time.Now().Add(-time.Second))
			},
			wantOutcome: WriteFailed,
			wantErrIs:   context.DeadlineExceeded,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.mutate != nil && !statIdentitySupported() {
				t.Skip("health classification requires device/inode support (linux or darwin)")
			}
			if tc.mutate != nil {
				tc.mutate(t)
			}
			reqCtx, cancel := tc.setupCtx()
			defer cancel()

			leaseHookReached := make(chan struct{}, 1)
			db.writeLeaseHook = func(l *Lease) { close(leaseHookReached) }
			t.Cleanup(func() { db.writeLeaseHook = nil })

			w := db.StartWrite(reqCtx, writeExecSeq.Add(1), `UPDATE "users" SET "email" = 'new'`, nil)
			res := w.Wait()
			if res.Outcome != tc.wantOutcome {
				t.Fatalf("outcome = %v, want %v", res.Outcome, tc.wantOutcome)
			}
			if tc.wantHealth {
				if res.Health == nil {
					t.Fatal("want typed HealthError, got nil health")
				}
			} else if res.Health != nil {
				t.Fatalf("want no health, got %v", res.Health)
			}
			if tc.wantErrIs != nil && (res.Err == nil || !errors.Is(res.Err, tc.wantErrIs)) {
				t.Fatalf("err = %v, want errors.Is(_, %v)", res.Err, tc.wantErrIs)
			}
			if res.RollbackConfirmed {
				t.Fatal("reported rollback confirmed; no transaction was ever begun")
			}
			phases := collectPhases(t, w)
			if len(phases) != 0 {
				t.Fatalf("emitted phases %v, want none (no lease was acquired)", phases)
			}
			select {
			case <-leaseHookReached:
				t.Fatal("writeLeaseHook ran; the lease should never have been acquired")
			default:
			}
		})
	}
}
