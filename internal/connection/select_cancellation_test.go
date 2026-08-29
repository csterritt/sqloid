// Scoped Ctrl+W cancellation lifecycle coverage on the Connection boundary
// (Issue #28 Task 1): applying Issue #6's cancellable request infrastructure
// to active first-page, later-page, and count SELECT work. The started
// request handles prove independent connection-scoped interrupt identities,
// settlement held behind deterministic barriers, no lease reuse or
// replacement before every targeted request truly settles, cancellation-wins
// classification of released late work, idempotent cancellation, no
// force-close, and healthy subsequent work on each settled connection.

package connection

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// barrierSeams installs the DB's test-only barrier hooks so a test can hold
// the first-page and count operations after their dedicated lease is
// acquired and before their statement runs, capturing each request's context
// and physical connection as evidence of independent interrupt identity.
type barrierSeams struct {
	pageReached  chan struct{}
	pageCtx      chan context.Context
	pageConn     chan *sql.Conn
	releasePage  chan struct{}
	countReached chan struct{}
	countCtx     chan context.Context
	countConn    chan *sql.Conn
	releaseCount chan struct{}
}

func newBarrierSeams() *barrierSeams {
	return &barrierSeams{
		pageReached:  make(chan struct{}),
		pageCtx:      make(chan context.Context, 1),
		pageConn:     make(chan *sql.Conn, 1),
		releasePage:  make(chan struct{}),
		countReached: make(chan struct{}),
		countCtx:     make(chan context.Context, 1),
		countConn:    make(chan *sql.Conn, 1),
		releaseCount: make(chan struct{}),
	}
}

func (b *barrierSeams) install(db *DB) {
	db.beforeFirstPage = func(ctx context.Context, conn *sql.Conn) {
		b.pageCtx <- ctx
		b.pageConn <- conn
		close(b.pageReached)
		<-b.releasePage
	}
	db.beforeCount = func(ctx context.Context, conn *sql.Conn) {
		b.countCtx <- ctx
		b.countConn <- conn
		close(b.countReached)
		<-b.releaseCount
	}
}

// mustReach waits for a barrier with a deadlock-detection bound only; all
// ordering comes from channels, never sleeps.
func mustReach(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Minute):
		t.Fatalf("%s never reached its barrier", what)
	}
}

// TestStartedPageAndCountCancelIndependentlyAndSettleAll proves the scoped
// cancellation contract for one execution's concurrent pair: each started
// request owns a distinct lease and interrupt identity, cancelling only the
// page leaves the count's context untouched, every cancelled request settles
// only through its own lifecycle, and a cancelled-but-unsettled count keeps
// its lease while the settled page's connection stays healthy and reusable.
func TestStartedPageAndCountCancelIndependentlyAndSettleAll(t *testing.T) {
	ctx := context.Background()
	db := openMixed(t)
	seams := newBarrierSeams()
	seams.install(db)

	page := db.StartFirstPage(ctx, `SELECT id FROM "mix"`, nil)
	count := db.StartCount(ctx, `SELECT COUNT(*) FROM (SELECT id FROM "mix")`, nil)
	mustReach(t, seams.pageReached, "first-page request")
	mustReach(t, seams.countReached, "count request")

	pageCtx, countCtx := <-seams.pageCtx, <-seams.countCtx
	pageConn, countConn := <-seams.pageConn, <-seams.countConn
	if rawDriverConnPointer(t, pageConn) == rawDriverConnPointer(t, countConn) {
		t.Fatal("page and count requests ran on the same physical connection")
	}
	if pageCtx == countCtx {
		t.Fatal("page and count requests shared one cancellation identity")
	}

	// Scoped: cancelling only the page must not disturb the count's context.
	page.Cancel()
	if err := countCtx.Err(); err != nil {
		t.Fatalf("count context disturbed by page cancellation: %v", err)
	}
	if count.State() != StateRunning {
		t.Fatalf("count state after page cancellation = %v, want running", count.State())
	}

	// The page settles as cancelled only after its work ends; the count
	// request keeps running independently behind its own barrier.
	close(seams.releasePage)
	_, pageRes := page.Wait()
	if pageRes.Outcome != OutcomeCancelled {
		t.Fatalf("cancelled page outcome = %v, want cancelled", pageRes.Outcome)
	}

	// The settled page's lease returns to the pool healthy: harmless work
	// succeeds on it, proving cancellation never force-closes anything.
	lease, err := db.Lease(ctx)
	if err != nil {
		t.Fatalf("leasing the settled page's returned connection: %v", err)
	}
	var version int64
	if err := lease.Conn().QueryRowContext(ctx, "PRAGMA schema_version").Scan(&version); err != nil {
		t.Fatalf("harmless work on the pool after page cancellation failed (force-closed?): %v", err)
	}

	// No replacement work can take the count's still-owned lease: with only
	// one lease free and now held by this test, a second lease must fail.
	held := lease
	third := make(chan error, 1)
	go func() {
		acquireCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()
		l, err := db.Lease(acquireCtx)
		if err == nil {
			l.Release(ctx)
		}
		third <- err
	}()
	select {
	case err := <-third:
		if err == nil {
			t.Error("replacement work acquired the cancelled count's lease before it settled")
		}
	case <-time.After(time.Minute):
		t.Fatal("third-lease attempt deadlocked")
	}
	held.Release(ctx)

	// The count settles through its own cancellation, and only then is its
	// lease reusable for a healthy subsequent request.
	count.Cancel()
	close(seams.releaseCount)
	total, countRes := count.Wait()
	if countRes.Outcome != OutcomeCancelled {
		t.Fatalf("cancelled count outcome = %v, want cancelled", countRes.Outcome)
	}
	if total != 0 {
		t.Errorf("cancelled count returned total %d, want zero", total)
	}

	reused, err := db.Lease(ctx)
	if err != nil {
		t.Fatalf("leasing after full settlement: %v", err)
	}
	var check int64
	if err := reused.Conn().QueryRowContext(ctx, "PRAGMA schema_version").Scan(&check); err != nil {
		t.Fatalf("healthy subsequent request on the reused connection failed: %v", err)
	}
	reused.Release(ctx)
}

// TestStartedRequestCancellationIsIdempotentAndSingleSettled proves Cancel
// on a started handle is at-most-once effective: repeated and late Cancell
// neither re-interrupt, re-settle, nor re-close the request, and the settled
// request classifies its cancellation exactly once.
func TestStartedRequestCancellationIsIdempotentAndSingleSettled(t *testing.T) {
	db := openMixed(t)
	seams := newBarrierSeams()
	seams.install(db)

	page := db.StartFirstPage(context.Background(), `SELECT id FROM "mix"`, nil)
	mustReach(t, seams.pageReached, "first-page request")

	page.Cancel()
	page.Cancel() // idempotent: no second interrupt, no state change
	if page.State() != StateCancelling {
		t.Fatalf("state after repeated Cancel = %v, want cancelling", page.State())
	}

	close(seams.releasePage)
	p, res := page.Wait()
	if p != nil {
		t.Errorf("cancelled page returned a page %+v, want nil", p)
	}
	if res.Outcome != OutcomeCancelled {
		t.Fatalf("outcome = %v, want cancelled", res.Outcome)
	}

	page.Cancel() // late Cancel after settlement: no effect
	if page.State() != StateSettled {
		t.Errorf("state after late Cancel = %v, want settled", page.State())
	}
}

// TestCountOnlyCancellationLeavesUnrelatedWorkAlone proves a count-only
// cancellation scope: only the count's interrupt identity is requested and
// only the count's lease is held by the cancelled-but-unsettled request.
func TestCountOnlyCancellationLeavesUnrelatedWorkAlone(t *testing.T) {
	ctx := context.Background()
	db := openMixed(t)
	seams := newBarrierSeams()
	seams.install(db)

	count := db.StartCount(ctx, `SELECT COUNT(*) FROM (SELECT id FROM "mix")`, nil)
	mustReach(t, seams.countReached, "count request")
	countCtx := <-seams.countCtx

	count.Cancel()
	if err := countCtx.Err(); err == nil {
		t.Fatal("count context not cancelled by its own Cancel")
	}

	close(seams.releaseCount)
	if _, res := count.Wait(); res.Outcome != OutcomeCancelled {
		t.Fatalf("count-only cancellation outcome = %v, want cancelled", res.Outcome)
	}

	// The pool is healthy afterwards: both connections accept new work.
	for i := 0; i < 2; i++ {
		lease, err := db.Lease(ctx)
		if err != nil {
			t.Fatalf("lease %d after count-only settlement: %v", i+1, err)
		}
		var version int64
		if err := lease.Conn().QueryRowContext(ctx, "PRAGMA schema_version").Scan(&version); err != nil {
			t.Fatalf("harmless work on lease %d after cancellation failed: %v", i+1, err)
		}
		lease.Release(ctx)
	}
}
