//go:build unix

// Pinned-driver interruption capability evidence for Issue #6 on Linux and
// macOS: these barrier-based integration tests prove the mandatory bounds in
// Notes/PRD-sqloid.md (Errors and cancellation bounds) against the exact
// modernc.org/sqlite version pinned by go.mod. They are release- and
// dependency-upgrade-blocking, and they exercise connection-scoped interrupt
// semantics — context cancellation on a request whose work runs through
// QueryContext/ExecContext makes the driver issue sqlite3_interrupt against
// exactly the leased physical connection.

package connection

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	sqlite "modernc.org/sqlite"
)

const (
	// cpuBoundBudget is the mandatory PRD bound: controlled CPU-bound work
	// settles within one second of cancellation.
	cpuBoundBudget = time.Second
	// lockWaitBudget is the mandatory PRD bound: a lock wait settles no
	// later than the five-second busy timeout.
	lockWaitBudget = 5 * time.Second
	// fixtureRows sizes the scan feeding the CPU-bound probe so that the
	// uncancellable workload would run orders of magnitude longer than any
	// bound under test; cancellation must be what ends it.
	fixtureRows = 200000
)

var probeRegistry struct {
	mu   sync.Mutex
	next int
}

// buildRowsFixture creates a database at path holding fixtureRows rows in
// table t, through a session separate from any pool under test.
func buildRowsFixture(t *testing.T, path string) {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("fixture open %q error = %v", path, err)
	}
	defer db.Close()
	stmt := fmt.Sprintf(`
		CREATE TABLE t (i INTEGER);
		WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c WHERE x < %d)
		INSERT INTO t SELECT x FROM c;`, fixtureRows)
	if _, err := db.Exec(stmt); err != nil {
		t.Fatalf("building %d-row fixture: %v", fixtureRows, err)
	}
}

// registerCPUProbe mints a uniquely named scalar function whose invocation is
// deliberately expensive and whose first call signals started. A query calling
// it once per row provides a deterministic "work has begun" barrier without
// sleeps and is guaranteed still running when the barrier fires.
func registerCPUProbe(t *testing.T, purpose string) (fnName string, started <-chan struct{}) {
	t.Helper()

	probeRegistry.mu.Lock()
	defer probeRegistry.mu.Unlock()
	probeRegistry.next++
	fnName = fmt.Sprintf("sqloid_interrupt_probe_%s_%d", purpose, probeRegistry.next)

	first := make(chan struct{}, 1)
	var once sync.Once
	sqlite.MustRegisterDeterministicScalarFunction(fnName, 1,
		func(ctx *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			once.Do(func() { first <- struct{}{} })
			x := 1.0
			for i := 0; i < 200000; i++ {
				x = math.Sqrt(x + 1)
				if x > 1e10 {
					x = 1
				}
			}
			return x, nil
		})
	return fnName, first
}

// mustSettleByCancellation waits for the interrupted work to finish after
// cancelAt and fails unless settlement lands within budget. The returned
// outcome reflects how the caller should have classified err via Settle.
func mustSettleByCancellation(t *testing.T, req *Request, workErr <-chan error, cancelAt time.Time, scenario string, budget time.Duration) Outcome {
	t.Helper()

	select {
	case err := <-workErr:
		took := time.Since(cancelAt)
		if took > budget {
			t.Fatalf("%s settled %v after cancellation, exceeding the mandatory %v bound (error = %v)", scenario, took, budget, err)
		}
		return req.Settle(err)
	case <-time.After(budget):
		t.Fatalf("%s did not settle within the mandatory %v of cancellation", scenario, budget)
		return OutcomeFailed
	}
}

// TestCPUBoundWorkInterruptsWithinOneSecond proves the pinned driver settles
// a long-running CPU-bound query within one second of cancellation; while it
// runs, an independent request on the other dedicated lease completes in
// isolation; afterwards the interrupted physical connection accepts harmless
// subsequent work and returns to the pool for reuse rather than being closed.
func TestCPUBoundWorkInterruptsWithinOneSecond(t *testing.T) {
	path := t.TempDir() + "/cpu.db"
	buildRowsFixture(t, path)
	fnName, started := registerCPUProbe(t, "cpu")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open error = %v", err)
	}
	defer db.Close()

	target, err := db.Lease(context.Background())
	if err != nil {
		t.Fatalf("leasing target connection: %v", err)
	}
	independent, err := db.Lease(context.Background())
	if err != nil {
		t.Fatalf("leasing independent connection: %v", err)
	}

	req := target.BeginRequest(context.Background())
	workErr := make(chan error, 1)
	go func() {
		var sum int64
		workErr <- target.Conn().QueryRowContext(req.Context(),
			fmt.Sprintf("SELECT sum(%s(i)) FROM t", fnName)).Scan(&sum)
	}()

	// Barrier: the CPU-bound statement is verifiably running before anything
	// else happens; on its heels, independent work proves the other lease is
	// untouched by the soon-to-be-interrupted request.
	select {
	case <-started:
	case <-time.After(cpuBoundBudget):
		t.Fatal("controlled CPU-bound work never signalled that it had started")
	}
	isolatedDone := make(chan error, 1)
	go func() {
		var version int64
		isolatedDone <- independent.Conn().QueryRowContext(context.Background(), "PRAGMA schema_version").Scan(&version)
	}()
	select {
	case err := <-isolatedDone:
		if err != nil {
			t.Fatalf("independent lease could not run its own request concurrently: %v", err)
		}
	case <-time.After(cpuBoundBudget):
		t.Fatal("independent work on the second lease did not complete within the explicit bound")
	}

	cancelAt := time.Now()
	req.Cancel()
	if outcome := mustSettleByCancellation(t, req, workErr, cancelAt, "CPU-bound work", cpuBoundBudget); outcome != OutcomeCancelled {
		t.Fatalf("interrupted CPU-bound work classified as %v, want cancelled", outcome)
	}

	// No force-close: the same physical connection immediately accepts new
	// harmless work on its lease.
	var version int64
	if err := target.Conn().QueryRowContext(context.Background(), "PRAGMA schema_version").Scan(&version); err != nil {
		t.Fatalf("subsequent work on the interrupted leased connection failed (force-closed?): %v", err)
	}
	if err := req.Close(); err != nil {
		t.Fatalf("closing settled request error = %v", err)
	}

	// Safe reuse: the connection comes back through the pool and serves a
	// fresh request without special handling.
	reused, err := db.Lease(context.Background())
	if err != nil {
		t.Fatalf("re-leasing after interruption error = %v", err)
	}
	defer reused.Release(context.Background())
	var check int64
	if err := reused.Conn().QueryRowContext(context.Background(), "PRAGMA schema_version").Scan(&check); err != nil {
		t.Fatalf("harmless subsequent request on the reused connection failed: %v", err)
	}
}

// TestLockWaitInterruptsWithinFiveSeconds proves the pinned driver settles a
// write blocked behind another connection's shared read lock no later than
// the five-second busy timeout when its request is cancelled.
func TestLockWaitInterruptsWithinFiveSeconds(t *testing.T) {
	path := t.TempDir() + "/lock.db"
	buildRowsFixture(t, path)
	// The blocking statement uses a scalar probe so we know it began executing —
	// and therefore that its commit must contend for EXCLUSIVE against the
	// holder's persistent SHARED lock — before cancellation is requested.
	// Ordering is enforced by the probe's started channel, not sleeps; the probe
	// must be registered before the pool opens its physical connections, so it
	// is minted before Open below.
	fnName, writeStarted := registerCPUProbe(t, "lock")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open error = %v", err)
	}
	defer db.Close()

	holderLease, err := db.Lease(context.Background())
	if err != nil {
		t.Fatalf("leasing lock-holder connection: %v", err)
	}
	target, err := db.Lease(context.Background())
	if err != nil {
		t.Fatalf("leasing blocked-writer connection: %v", err)
	}

	// Holder keeps a streaming read transaction alive with an unconsumed
	// Rows cursor, so its SHARED lock persists until we close the rows.
	rows, err := holderLease.Conn().QueryContext(context.Background(), "SELECT i FROM t")
	if err != nil {
		t.Fatalf("holder read query error = %v", err)
	}
	if !rows.Next() {
		t.Fatalf("holder read produced no rows: %v", rows.Err())
	}

	// The blocking statement uses a scalar probe so we know it began
	// executing — and therefore that its commit must contend for EXCLUSIVE
	// against the holder's persistent SHARED lock — before cancellation is
	// requested. Ordering is enforced by the probe's started channel, not
	// by sleeps.
	req := target.BeginRequest(context.Background())
	workErr := make(chan error, 1)
	go func() {
		_, err := target.Conn().ExecContext(req.Context(),
			fmt.Sprintf("INSERT INTO t SELECT %s(i) FROM t WHERE i = 1", fnName))
		workErr <- err
	}()
	select {
	case <-writeStarted:
	case <-time.After(time.Minute):
		t.Fatal("blocked write never signalled that it had begun executing")
	}

	cancelAt := time.Now()
	req.Cancel()
	if outcome := mustSettleByCancellation(t, req, workErr, cancelAt, "lock wait", lockWaitBudget); outcome != OutcomeCancelled {
		t.Fatalf("interrupted lock wait classified as %v, want cancelled", outcome)
	}

	rows.Close() // release the holder's lock before pool teardown

	var version int64
	if err := target.Conn().QueryRowContext(context.Background(), "PRAGMA schema_version").Scan(&version); err != nil {
		t.Fatalf("subsequent work on the interrupted leased connection failed: %v", err)
	}
	if err := req.Close(); err != nil {
		t.Fatalf("closing settled request error = %v", err)
	}
}

// TestLateSuccessAfterCancellationIsDiscardedAndConnectionReusable covers a
// deliberately released late success against the real driver-backed lease:
// the successful statement ran before cancellation but its result is released
// to the lifecycle only after Cancel, so cancellation wins, the result is
// discarded as cancelled, and the connection stays usable.
func TestLateSuccessAfterCancellationIsDiscardedAndConnectionReusable(t *testing.T) {
	path := t.TempDir() + "/late.db"
	buildRowsFixture(t, path)
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open error = %v", err)
	}
	defer db.Close()

	lease, err := db.Lease(context.Background())
	if err != nil {
		t.Fatalf("leasing connection: %v", err)
	}
	req := lease.BeginRequest(context.Background())

	executed := make(chan struct{})
	results := make(chan error, 1)
	go func() {
		<-executed // result deliberately held back until after Cancel below
		results <- nil
	}()

	var sum int64
	queryErr := make(chan error, 1)
	go func() {
		queryErr <- lease.Conn().QueryRowContext(context.Background(),
			"SELECT sum(i) FROM (SELECT i FROM t LIMIT 3)").Scan(&sum)
	}()
	select {
	case err := <-queryErr:
		if err != nil {
			t.Fatalf("pre-cancellation statement failed unexpectedly: %v", err)
		}
	case <-time.After(time.Minute):
		t.Fatal("pre-cancellation statement deadlocked")
	}

	req.Cancel()
	close(executed)

	select {
	case lateErr := <-results:
		if outcome := req.Settle(lateErr); outcome != OutcomeCancelled {
			t.Fatalf("late-success outcome = %v, want cancelled (success arriving after cancellation must be discarded)", outcome)
		}
	case <-time.After(time.Minute):
		t.Fatal("deliberately released late success never reached Settle")
	}

	var version int64
	if err := lease.Conn().QueryRowContext(context.Background(), "PRAGMA schema_version").Scan(&version); err != nil {
		t.Fatalf("connection unusable after cancelled late success: %v", err)
	}
	if err := req.Close(); err != nil {
		t.Fatalf("closing settled request error = %v", err)
	}
}
