// Focused lifecycle coverage for cancellation and replacement settlement
// ordering on the Connection boundary (Issue #26 Task 3): a cancelled
// request holds its dedicated lease until it actually settles, so no
// replacement work can begin or reuse that lease early, and page and count
// requests settle independently in either order across distinct leases.
// Deterministic barriers coordinate the requests — no timing sleeps.

package connection

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestCancelledRequestHoldsLeaseUntilSettlement proves that replacement work
// cannot acquire the cancelled request's dedicated lease — or any third
// lease from the exact-two pool — until the cancelled request actually
// settles. Cancellation is only a request: the lease stays owned.
func TestCancelledRequestHoldsLeaseUntilSettlement(t *testing.T) {
	ctx := context.Background()
	db, _ := setJournalAndOpen(t, "delete")

	held := holdConcurrentLeases(t, db)
	defer held[1].Release(ctx)

	requestLease := held[0]
	req := requestLease.BeginRequest(ctx)
	req.Cancel()
	if req.State() != StateCancelling {
		t.Fatalf("state after Cancel = %v, want cancelling", req.State())
	}

	acquired := make(chan error, 1)
	go func() {
		acquireCtx, acquireCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer acquireCancel()
		lease, err := db.Lease(acquireCtx)
		if err == nil {
			lease.Release(ctx)
		}
		acquired <- err
	}()
	select {
	case err := <-acquired:
		if err == nil {
			t.Error("replacement work acquired a lease while an unsettled cancelled request owned one of only two connections")
		}
	case <-time.After(time.Minute):
		t.Fatal("third-lease acquisition attempt deadlocked instead of failing fast")
	}

	// True settlement releases the lease; the outcome classifies the
	// post-cancellation nil success as cancelled, not success.
	if outcome := req.Settle(nil); outcome != OutcomeCancelled {
		t.Errorf("late nil success after Cancel classified %v, want cancelled", outcome)
	}
	if err := req.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}

	reused, err := db.Lease(ctx)
	if err != nil {
		t.Fatalf("lease after settlement error = %v", err)
	}
	reused.Release(ctx)
}

// TestPageAndCountSettleIndependentlyInEitherOrder proves the first-page and
// count requests of one execution settle in either order on distinct leases:
// neither waits for the other, and a still-running predecessor does not
// delay or block the other role's settlement.
func TestPageAndCountSettleIndependentlyInEitherOrder(t *testing.T) {
	for _, first := range []string{"page", "count"} {
		t.Run(first, func(t *testing.T) {
			db := openMixed(t)

			pageReached := make(chan struct{})
			releasePage := make(chan struct{})
			countReached := make(chan struct{})
			releaseCount := make(chan struct{})
			db.beforeFirstPage = func(ctx context.Context, conn *sql.Conn) {
				close(pageReached)
				<-releasePage
			}
			db.beforeCount = func(ctx context.Context, conn *sql.Conn) {
				close(countReached)
				<-releaseCount
			}

			pageDone := make(chan RequestResult, 1)
			countDone := make(chan RequestResult, 1)
			go func() {
				_, res := db.RunFirstPage(context.Background(), `SELECT id FROM "mix"`, nil)
				pageDone <- res
			}()
			go func() {
				_, res := db.RunCount(context.Background(), `SELECT COUNT(*) FROM (SELECT id FROM "mix")`, nil)
				countDone <- res
			}()
			<-pageReached
			<-countReached

			if first == "count" {
				close(releaseCount)
				select {
				case <-countDone:
				case <-time.After(time.Minute):
					t.Fatal("count did not settle while the page predecessor was still running")
				}
				select {
				case <-pageDone:
					t.Fatal("page settled without its release barrier")
				default:
				}
				close(releasePage)
				<-pageDone
			} else {
				close(releasePage)
				select {
				case <-pageDone:
				case <-time.After(time.Minute):
					t.Fatal("page did not settle while the count predecessor was still running")
				}
				select {
				case <-countDone:
					t.Fatal("count settled without its release barrier")
				default:
				}
				close(releaseCount)
				<-countDone
			}
		})
	}
}
