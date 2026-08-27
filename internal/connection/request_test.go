package connection

import (
	"context"
	"sync"
	"testing"
	"time"
)

// newFakeLease builds a lease-shaped handle carrying no physical connection.
// Fake-backed lifecycle tests drive Request semantics through it: the
// lifecycle contract is defined against the Connection abstraction alone and
// must not require a live SQLite driver.
func newFakeLease() *Lease { return &Lease{} }

// TestRequestIdentityIsUniqueAndContextOwned covers Issue #6's identity and
// context-ownership contracts: every BeginRequest hands out a process-unique
// increasing ID and each request owns its own derived context — cancelling
// one request never touches another's context, and cancelling the parent
// propagates into both.
func TestRequestIdentityIsUniqueAndContextOwned(t *testing.T) {
	lease := newFakeLease()
	parent, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()

	first := lease.BeginRequest(parent)
	defer first.Close()
	second := lease.BeginRequest(parent)
	defer second.Close()

	if first.ID() == 0 || second.ID() == 0 {
		t.Fatalf("request IDs must be non-zero: first = %d, second = %d", first.ID(), second.ID())
	}
	if first.ID() == second.ID() {
		t.Fatalf("distinct requests received the same identity %d", first.ID())
	}

	first.Cancel()
	select {
	case <-first.Context().Done():
	case <-time.After(time.Minute):
		t.Fatal("first request's context was not cancelled by Cancel within the explicit bound")
	}
	select {
	case <-second.Context().Done():
		t.Fatal("cancelling the first request cancelled the second request's context")
	default:
	}
	parentCancel()
	select {
	case <-second.Context().Done():
	case <-time.After(time.Minute):
		t.Fatal("parent cancellation did not propagate into the request's context")
	}
}

// TestCancelIsIdempotentAndInterruptDispatchedOnce covers the atomic
// cancellation-requested flag and the exactly-one connection-scoped
// interrupt contract: concurrent idempotent Cancel calls dispatch the
// interrupt hook exactly once, and late Cancell after settlement dispatch
// nothing further.
func TestCancelIsIdempotentAndInterruptDispatchedOnce(t *testing.T) {
	for _, mode := range []string{"concurrent", "late"} {
		t.Run(mode, func(t *testing.T) {
			lease := newFakeLease()
			dispatched := make(chan struct{}, 1)
			lease.interruptFn = func() { dispatched <- struct{}{} }

			req := lease.BeginRequest(context.Background())
			switch mode {
			case "concurrent":
				start := make(chan struct{})
				var wg sync.WaitGroup
				for i := 0; i < 8; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						<-start
						req.Cancel()
					}()
				}
				close(start)
				wg.Wait()
			case "late":
				req.Cancel()
				req.Settle(nil)
				req.Cancel()
				req.Cancel()
			}

			select {
			case <-dispatched:
			case <-time.After(time.Minute):
				t.Fatalf("mode %s: connection-scoped interrupt was not dispatched exactly once", mode)
			}
			select {
			case <-dispatched:
				t.Fatalf("mode %s: interrupt was dispatched more than once", mode)
			default:
			}
			req.Close()
		})
	}
}

// TestVisibleLifecycleStateTracksCancellationUntilSettlement covers the
// observable cancelling-versus-settled contract: Cancel flips Running to
// Cancelling immediately; Cancelling persists until Settle records
// settlement even though the work itself may have already finished; Settle
// moves the state to Settled once and forever.
func TestVisibleLifecycleStateTracksCancellationUntilSettlement(t *testing.T) {
	lease := newFakeLease()
	req := lease.BeginRequest(context.Background())
	if got := req.State(); got != StateRunning {
		t.Fatalf("fresh request State() = %v, want running", got)
	}

	workDone := make(chan struct{})
	go func() {
		<-req.Context().Done()
		close(workDone)
	}()
	req.Cancel()
	if got := req.State(); got != StateCancelling {
		t.Fatalf("after Cancel, State() = %v, want cancelling", got)
	}

	<-workDone // in-flight work has settled inside the driver…
	if got := req.State(); got != StateCancelling {
		t.Fatalf("work finished but before Settle, State() = %v, want still cancelling", got)
	}

	if outcome := req.Settle(nil); outcome != OutcomeCancelled {
		t.Fatalf("Settle outcome = %v, want cancelled", outcome)
	}
	if !req.Settled() {
		t.Fatal("after Settle, Settled() = false")
	}
	if got := req.State(); got != StateSettled {
		t.Fatalf("after Settle, State() = %v, want settled", got)
	}
	req.Close()
}

// TestLateSuccessIsDiscardedAsCancelled covers the cancellation-wins rule:
// success arriving after cancellation was requested is discarded and
// classified as cancelled, never adopted as success.
func TestLateSuccessIsDiscardedAsCancelled(t *testing.T) {
	lease := newFakeLease()
	req := lease.BeginRequest(context.Background())
	defer req.Close()

	releaseSuccess := make(chan struct{})
	settled := make(chan Outcome, 1)
	go func() {
		<-releaseSuccess
		settled <- req.Settle(nil) // deliberately released "success" result
	}()

	req.Cancel()
	close(releaseSuccess)

	select {
	case outcome := <-settled:
		if outcome != OutcomeCancelled {
			t.Fatalf("late-success outcome = %v, want cancelled (success after cancellation must be discarded)", outcome)
		}
	case <-time.After(time.Minute):
		t.Fatal("settling goroutine did not report within the explicit deadlock bound")
	}
}

// TestErrorsSettleNormally covers ordinary failure classification: an error
// from uncancelled work classifies as failed, and a settlement recorded
// before any later call is returned unchanged.
func TestErrorsSettleNormally(t *testing.T) {
	lease := newFakeLease()
	req := lease.BeginRequest(context.Background())

	failure := context.DeadlineExceeded
	if outcome := req.Settle(failure); outcome != OutcomeFailed {
		t.Fatalf("uncancelled error classified as %v, want failed", outcome)
	}
	if again := req.Settle(nil); again != OutcomeFailed {
		t.Fatalf("second Settle changed the outcome to %v, want the original failed classification", again)
	}
	if got := req.State(); got != StateSettled {
		t.Fatalf("State() after failed settlement = %v, want settled", got)
	}
	req.Close()
}

// TestCancellationErrorClassifiesAsCancelled covers work that fails through
// its own observation of context cancellation: such failures classify as
// cancelled even when only the parent context (not Cancel itself) ended the
// request.
func TestCancellationErrorClassifiesAsCancelled(t *testing.T) {
	lease := newFakeLease()
	parent, parentCancel := context.WithCancel(context.Background())
	req := lease.BeginRequest(parent)

	parentCancel()
	err := req.Context().Err()
	if outcome := req.Settle(err); outcome != OutcomeCancelled {
		t.Fatalf("context-cancellation failure classified as %v, want cancelled", outcome)
	}
	req.Close()
}

// TestLeaseHeldUntilSettlementThenReusable covers lease-ownership reuse on
// the real two-connection pool: while an unsettled request owns a lease no
// third lease can be acquired, proving no replacement work can start on the
// dedicated connection; after settlement and Close the same lease is safely
// reusable for subsequent work.
func TestLeaseHeldUntilSettlementThenReusable(t *testing.T) {
	ctx := context.Background()
	db, _ := setJournalAndOpen(t, "delete")

	held := holdConcurrentLeases(t, db)
	defer held[1].Release(ctx)

	requestLease := held[0]
	req := requestLease.BeginRequest(ctx)

	blocked := make(chan struct{})
	go func() {
		acquireCtx, acquireCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer acquireCancel()
		_, err := db.Lease(acquireCtx)
		if err == nil {
			t.Error("acquired a third lease while an unsettled request owned one of only two connections")
			return
		}
		close(blocked)
	}()
	select {
	case <-blocked:
	case <-time.After(time.Minute):
		t.Fatal("third-lease acquisition attempt deadlocked instead of failing fast")
	}

	req.Settle(nil)
	if err := req.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}

	reused, err := db.Lease(ctx)
	if err != nil {
		t.Fatalf("lease after settlement error = %v", err)
	}
	reused.Release(ctx)
}

// TestConnectionNotForceClosedByCancellationAndSafeForReuse covers the real
// connection through the lifecycle: cancellation settles work, leaves the
// physical connection usable on the very lease that was interrupted, release
// happens only after settlement, and harmless subsequent work succeeds on
// the same connection obtained through the pool afterwards.
func TestConnectionNotForceClosedByCancellationAndSafeForReuse(t *testing.T) {
	ctx := context.Background()
	db, _ := setJournalAndOpen(t, "delete")

	lease, err := db.Lease(ctx)
	if err != nil {
		t.Fatalf("Lease error = %v", err)
	}

	req := lease.BeginRequest(ctx)
	var version int64
	if err := lease.Conn().QueryRowContext(req.Context(), "PRAGMA schema_version").Scan(&version); err != nil {
		t.Fatalf("querying under the request context: %v", err)
	}
	req.Cancel()
	if outcome := req.Settle(nil); outcome != OutcomeCancelled {
		t.Fatalf("outcome = %v, want cancelled", outcome)
	}

	// The same lease still works after cancellation settled: the physical
	// connection was interrupted but never force-closed or replaced.
	var versionAgain int64
	if err := lease.Conn().QueryRowContext(ctx, "PRAGMA schema_version").Scan(&versionAgain); err != nil {
		t.Fatalf("harmless work on the interrupted leased connection failed (force-closed?): %v", err)
	}
	if err := lease.Release(ctx); err != nil {
		t.Fatalf("Release after settlement error = %v", err)
	}

	next, err := db.Lease(ctx)
	if err != nil {
		t.Fatalf("subsequent lease error = %v", err)
	}
	defer next.Release(ctx)
	var probe int64
	if err := next.Conn().QueryRowContext(ctx, "PRAGMA schema_version").Scan(&probe); err != nil {
		t.Fatalf("subsequent work after cancellation error = %v", err)
	}
}
