// Request-boundary tests for schema catalog plumbing, per Issue #1 (Task 1)
// and the Connection patterns from Issues #2, #5, and #7: a catalog read is
// one RunRequest, so it verifies request-boundary identity before work,
// leases the pooled connections exclusively while running, propagates typed
// health classification on failure, and honours caller cancellation. The
// catalog contents themselves are exercised exhaustively in
// internal/schema; only Connection behavior is proven here.

package connection

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// TestReadCatalogReturnsSuccessResult pins that one successful catalog read
// settles with OutcomeSuccess and returns a populated catalog.
func TestReadCatalogReturnsSuccessResult(t *testing.T) {
	path := t.TempDir() + "/catalog.db"
	createDatabase(t, path)

	db := mustOpen(t, path)
	cat, res := db.ReadCatalog(context.Background())
	if res.Outcome != OutcomeSuccess || res.Err != nil {
		t.Fatalf("ReadCatalog result = %+v, want success without error", res)
	}
	if cat == nil {
		t.Fatal("successful ReadCatalog returned nil catalog")
	}
	if cat.Version <= 0 {
		t.Errorf("catalog version = %d, want positive PRAGMA schema_version", cat.Version)
	}
	if len(cat.Objects) != 1 {
		t.Fatalf("catalog object count = %d, want exactly the fixture table", len(cat.Objects))
	}
	if got := cat.Objects[0].Name; got != "t" {
		t.Errorf("object name = %q, want %q", got, "t")
	}
}

// TestReadCatalogConcurrentRequestsLeaseDistinctConnections proves two
// concurrent catalog reads can both run: each acquires its own lease from the
// exact-two pool rather than serializing or failing. A lease regression would
// make one call block forever here.
func TestReadCatalogConcurrentRequestsLeaseDistinctConnections(t *testing.T) {
	path := t.TempDir() + "/concurrent.db"
	createDatabase(t, path)

	db := mustOpen(t, path)
	var wg sync.WaitGroup
	results := make([]RequestResult, 2)
	for i := range results {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			_, results[slot] = db.ReadCatalog(context.Background())
		}(i)
	}
	wg.Wait()
	for slot, res := range results {
		if res.Outcome != OutcomeSuccess {
			t.Errorf("concurrent ReadCatalog %d = %+v, want success", slot, res)
		}
	}
}

// TestReadCatalogCancelledContextFailsWithCancellation proves the catalog
// read honours its context: an already-cancelled context yields a cancelled
// outcome (Issue #60 pre-lease cancellation) whose cause unwraps to
// context.Canceled through the refresh wrapper.
func TestReadCatalogCancelledContextFailsWithCancellation(t *testing.T) {
	path := t.TempDir() + "/cancelled.db"
	createDatabase(t, path)

	db := mustOpen(t, path)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cat, res := db.ReadCatalog(ctx)
	if res.Outcome != OutcomeCancelled {
		t.Errorf("cancelled ReadCatalog outcome = %s, want cancelled", res.Outcome)
	}
	if !errors.Is(res.Err, context.Canceled) {
		t.Errorf("cancelled ReadCatalog error = %v, want context.Canceled cause preserved unwrappable", res.Err)
	}
	if cat != nil {
		t.Errorf("cancelled ReadCatalog returned catalog %+v, want zero value", cat)
	}
}

// TestReadCatalogClassifiesReplacementAtBoundary pins Issue #7 precedence:
// after a same-path replacement the next catalog read fails with the typed
// replaced classification instead of ordinary SQLite handling.
func TestReadCatalogClassifiesReplacementAtBoundary(t *testing.T) {
	path := t.TempDir() + "/replaced.db"
	createDatabase(t, path)

	db := mustOpen(t, path)
	replacePath(t, path, db.startIno)

	_, res := db.ReadCatalog(context.Background())
	if res.Health == nil {
		t.Fatalf("replacement ReadCatalog result = %+v, want HealthError", res)
	}
	if res.Health.Kind != HealthReplaced {
		t.Errorf("health kind = %s, want %s", res.Health.Kind, HealthReplaced)
	}
}
