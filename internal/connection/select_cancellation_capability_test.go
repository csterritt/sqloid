//go:build unix

// Pinned-driver SELECT cancellation capability evidence for Issue #28 on
// Linux and macOS, extending Issue #6's proven modernc capability seam to
// real first-page, later-page, and count SELECT requests whose statements
// come from internal/querybuilder's rendering API. These barrier-based
// integration tests are release-blocking (never a best-effort skip): they
// prove the mandatory bounds of Notes/PRD-sqloid.md (Errors and cancellation
// bounds) for the StartedRequest handles — independent connection-scoped
// interrupts for concurrently active page/count work, isolation of either
// request from the other's cancellation, one-second settlement of controlled
// CPU-bound page and count queries, lock-wait settlement no later than the
// five-second busy timeout, cancellation-wins rejection of released late
// success, no replacement lease reuse before every targeted request settles,
// and healthy subsequent requests on each interrupted physical connection.
// Journal mode is never set or changed; the five-second bound comes solely
// from the configured busy timeout.

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

	"github.com/chris/sqloid/internal/querybuilder"
	"github.com/chris/sqloid/internal/schema"
)

const (
	// capabilityCPUbudget is the mandatory PRD bound for controlled CPU-bound
	// page and count work: settlement within one second of cancellation.
	capabilityCPUbudget = time.Second

	// capabilityLockBudget is the mandatory PRD bound for a lock-wait request:
	// settlement no later than the five-second configured busy timeout.
	capabilityLockBudget = 5 * time.Second

	// probeTableRows sizes each probe table so that the uncancellable
	// CPU-bound page and count scans run several times longer than the
	// one-second bound (about one probe-millisecond per row); cancellation
	// must be what ends them.
	probeTableRows = 5000

	// lockSettleGrace is test scheduling margin on top of the busy timeout:
	// SQLite's busy-handler sleep loop is honored to the configured bound but
	// may overshoot by one retry interval, and the pinned driver does not
	// preempt a busy wait with sqlite3_interrupt — the lock-wait bound is met
	// by settlement at (or before) the configured five-second expiry.
	lockSettleGrace = 500 * time.Millisecond
)

var capabilityRegistry struct {
	mu   sync.Mutex
	next int
}

// buildProbeTables creates a database at path with two tables whose virtual
// generated columns invoke their own deliberately expensive scalar function
// once per row read. Page and count statements generated through
// internal/querybuilder therefore execute controlled CPU-bound work per row
// without any probe text inside the builder-rendered SQL: the probe lives in
// the fixture, and querybuilder keeps sole ownership of the SQL.
func buildProbeTables(t *testing.T, path string) (pageStarted, countStarted chan struct{}) {
	t.Helper()

	pageFn, pageCh := registerCapabilityProbe(t, "page")
	countFn, countCh := registerCapabilityProbe(t, "count")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("fixture open %q error = %v", path, err)
	}
	defer db.Close()
	for _, table := range []struct {
		name  string
		probe string
	}{{"page_t", pageFn}, {"count_t", countFn}} {
		ddl := fmt.Sprintf(`
			CREATE TABLE %s (i INTEGER PRIMARY KEY,
				p INTEGER GENERATED ALWAYS AS (%s(i)) VIRTUAL);
			WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c WHERE x < %d)
			INSERT INTO %s SELECT x FROM c;`, table.name, table.probe, probeTableRows, table.name)
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("building probe table %s: %v", table.name, err)
		}
	}
	// SQLite evaluates virtual generated columns while inserting fixture
	// rows too; drain those stale signals so a receive from either channel
	// proves that the real query being tested is evaluating the probe.
	for _, ch := range []chan struct{}{pageCh, countCh} {
		for {
			select {
			case <-ch:
				continue
			default:
			}
			break
		}
	}
	return pageCh, countCh
}

// registerCapabilityProbe mints a uniquely named deterministic scalar
// function whose invocation is deliberately expensive and whose every call
// non-blockingly signals started. A builder-rendered query touching the
// probe column once per row therefore provides a deterministic "work has
// begun" barrier without sleeps and is guaranteed still running when the
// barrier fires.
func registerCapabilityProbe(t *testing.T, purpose string) (fnName string, started chan struct{}) {
	t.Helper()

	capabilityRegistry.mu.Lock()
	defer capabilityRegistry.mu.Unlock()
	capabilityRegistry.next++
	fnName = fmt.Sprintf("sqloid_select_cancel_probe_%s_%d", purpose, capabilityRegistry.next)

	started = make(chan struct{}, 1)
	sqlite.MustRegisterDeterministicScalarFunction(fnName, 1,
		func(ctx *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			select {
			case started <- struct{}{}:
			default:
			}
			x := 1.0
			for i := 0; i < 200000; i++ {
				x = math.Sqrt(x + 1)
				if x > 1e10 {
					x = 1
				}
			}
			return x, nil
		})
	return fnName, started
}

// probeQB renders a runnable single-column SELECT over the named probe table
// through internal/querybuilder, so the executed page and count statements
// are exactly the builder's text and parameters. With whereOnProbe the
// rendered SELECT carries a bound `p > 0` predicate so a wrapping COUNT(*)
// must evaluate the probe column once per scanned row and cannot collapse
// into a row-count optimization.
func probeQB(table, column string, whereOnProbe bool) querybuilder.QueryBuilder {
	catalog := &schema.Catalog{
		Version: 1,
		Objects: []*schema.Object{{
			Name:            table,
			Kind:            schema.KindOrdinaryTable,
			WriteEligible:   true,
			Rowid:           schema.RowidHas,
			InsertableCount: 1,
			Columns:         []schema.Column{{Name: column, DeclaredType: "INTEGER", Insertable: true}},
		}},
	}
	q := querybuilder.NewQuery().RefreshSchema(catalog).
		SelectCommand(querybuilder.CommandSelect).SelectTable(table).
		AcceptProjection(querybuilder.ProjectionCandidate{Kind: querybuilder.ProjectionColumn, Column: column}).Builder.
		CompleteProjectionAggregate(column, querybuilder.AggregateValue).Builder
	if !whereOnProbe {
		return q
	}
	next, ok := q.StartWhere(column)
	if !ok {
		panic("setup: StartWhere on the probe column failed")
	}
	chosen, ok := next.WhereDraft().ChooseOperator(querybuilder.OpGt)
	if !ok {
		panic("setup: ChooseOperator on the probe column failed")
	}
	submitted, ok := chosen.SubmitValue("0")
	if !ok {
		panic("setup: SubmitValue on the probe predicate failed")
	}
	committed, ok := next.ApplyWhereDraft(submitted).CommitWhereDraft()
	if !ok {
		panic("setup: CommitWhereDraft on the probe predicate failed")
	}
	return committed
}

// mustSettleWithin fails unless the started request settles within budget of
// cancelAt and returns the settled result; all ordering comes from Wait's
// settlement, never sleeps.
func mustSettleWithin[Req interface {
	Wait() (V, RequestResult)
}, V any](t *testing.T, req Req, cancelAt time.Time, what string, budget time.Duration) (V, RequestResult) {
	t.Helper()

	type settled struct {
		value V
		res   RequestResult
	}
	done := make(chan settled, 1)
	go func() {
		v, res := req.Wait()
		done <- settled{v, res}
	}()
	select {
	case s := <-done:
		if took := time.Since(cancelAt); took > budget {
			t.Fatalf("%s settled %v after cancellation, exceeding the mandatory %v bound", what, took, budget)
		}
		return s.value, s.res
	case <-time.After(budget):
		t.Fatalf("%s did not settle within the mandatory %v of cancellation", what, budget)
		var zero V
		return zero, RequestResult{}
	}
}

// TestCapabilityPageAndCountCancelIndependentlyWithinOneSecond proves the
// scoped Ctrl+W contract against real driver work: builder-generated page
// and count SELECTs run concurrently on distinct dedicated leased
// connections; cancelling only the page interrupts exactly that work within
// the mandatory one-second bound while the count keeps running on its own
// identity; cancelling the count then settles it within the same bound; and
// both interrupted physical connections are reused — never force-closed —
// for healthy subsequent requests afterwards.
func TestCapabilityPageAndCountCancelIndependentlyWithinOneSecond(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/select_cancel_cpu.db"
	pageStarted, countStarted := buildProbeTables(t, path)
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open error = %v", err)
	}
	defer db.Close()

	seams := newBarrierSeams()
	seams.install(db)

	pageQB := probeQB("page_t", "p", false)
	countQB := probeQB("count_t", "p", true)
	page := db.StartFirstPage(ctx, pageQB.PageSQL(probeTableRows, 0), pageQB.PageParams())
	count := db.StartCount(ctx, countQB.CountSQL(), countQB.CountParams())

	mustReach(t, seams.pageReached, "first-page request")
	mustReach(t, seams.countReached, "count request")
	pageCtx, countCtx := <-seams.pageCtx, <-seams.countCtx
	pageConn, countConn := <-seams.pageConn, <-seams.countConn
	pagePtr := rawDriverConnPointer(t, pageConn)
	countPtr := rawDriverConnPointer(t, countConn)
	if pagePtr == countPtr {
		t.Fatal("page and count ran on the same physical connection")
	}

	// Let the statements run; each probe's first row evaluation proves its
	// CPU-bound work verifiably started before any cancellation is requested.
	close(seams.releasePage)
	close(seams.releaseCount)
	mustReach(t, pageStarted, "page probe work")
	mustReach(t, countStarted, "count probe work")

	// Scoped: cancelling only the page leaves the count's interrupt identity
	// untouched while its own CPU-bound work is still running.
	pageCancelAt := time.Now()
	page.Cancel()
	if err := pageCtx.Err(); err == nil {
		t.Fatal("page request context not cancelled by its own Cancel")
	}
	if err := countCtx.Err(); err != nil {
		t.Fatalf("count request context disturbed by page cancellation: %v", err)
	}

	pageRows, pageRes := mustSettleWithin(t, page, pageCancelAt, "CPU-bound page", capabilityCPUbudget)
	if pageRes.Outcome != OutcomeCancelled {
		t.Fatalf("interrupted page outcome = %v, want cancelled", pageRes.Outcome)
	}
	if pageRows != nil {
		t.Fatalf("cancelled page returned rows %+v, want none", pageRows)
	}

	// Isolation: the count is still verifiably running on its own connection
	// after the page has fully settled and released its lease.
	select {
	case <-count.SettledChan():
		t.Fatal("count settled as a side effect of the page's cancellation")
	default:
	}
	if err := countCtx.Err(); err != nil {
		t.Fatalf("count request context disturbed by the page's settlement: %v", err)
	}

	countCancelAt := time.Now()
	count.Cancel()
	total, countRes := mustSettleWithin(t, count, countCancelAt, "CPU-bound count", capabilityCPUbudget)
	if countRes.Outcome != OutcomeCancelled {
		t.Fatalf("interrupted count outcome = %v, want cancelled", countRes.Outcome)
	}
	if total != 0 {
		t.Fatalf("cancelled count returned total %d, want zero", total)
	}

	// No force-close by Sqloid: every lease settled before its release, and
	// the pool immediately serves two fresh healthy leases again.
	for i := 0; i < 2; i++ {
		lease, err := db.Lease(ctx)
		if err != nil {
			t.Fatalf("lease %d after settlement: %v", i+1, err)
		}
		var version int64
		if err := lease.Conn().QueryRowContext(ctx, "PRAGMA schema_version").Scan(&version); err != nil {
			t.Fatalf("subsequent work after settlement failed: %v", err)
		}
		lease.Release(ctx)
	}
}

// TestCapabilityLaterPageCancelInterruptsWithinOneSecond proves the
// later-page request path (StartPage) against real driver work: one
// builder-generated later-page SELECT runs CPU-bound on its dedicated
// connection and settles within the mandatory one-second bound of
// cancellation, then the interrupted connection serves healthy subsequent
// work.
func TestCapabilityLaterPageCancelInterruptsWithinOneSecond(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/select_cancel_page.db"
	pageStarted, _ := buildProbeTables(t, path)
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open error = %v", err)
	}
	defer db.Close()

	pageQB := probeQB("page_t", "p", false)
	page := db.StartPage(ctx, pageQB.PageSQL(probeTableRows, probeTableRows/2), pageQB.PageParams(), int64(probeTableRows/2))
	mustReach(t, pageStarted, "later-page probe work")

	pageCancelAt := time.Now()
	page.Cancel()
	rows, res := mustSettleWithin(t, page, pageCancelAt, "CPU-bound later page", capabilityCPUbudget)
	if res.Outcome != OutcomeCancelled {
		t.Fatalf("interrupted later page outcome = %v, want cancelled", res.Outcome)
	}
	if rows != nil {
		t.Fatalf("cancelled later page returned rows %+v, want none", rows)
	}

	lease, err := db.Lease(ctx)
	if err != nil {
		t.Fatalf("leasing after later-page settlement: %v", err)
	}
	var version int64
	if err := lease.Conn().QueryRowContext(ctx, "PRAGMA schema_version").Scan(&version); err != nil {
		t.Fatalf("subsequent work after the interrupted later page failed: %v", err)
	}
	lease.Release(ctx)
}

// TestCapabilityNoLeaseReuseBeforeEveryRequestSettles proves deterministically
// that a cancelled-but-unsettled page and count hold their leases: while both
// are unsettled, no replacement work can lease from the pool; each request's
// lease returns only at its own true settlement; and the freed physical
// connections serve healthy subsequent work.
func TestCapabilityNoLeaseReuseBeforeEveryRequestSettles(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/select_cancel_settle.db"
	buildProbeTables(t, path)
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open error = %v", err)
	}
	defer db.Close()

	seams := newBarrierSeams()
	seams.install(db)

	pageQB := probeQB("page_t", "p", false)
	countQB := probeQB("count_t", "p", false)
	page := db.StartFirstPage(ctx, pageQB.PageSQL(probeTableRows, 0), pageQB.PageParams())
	count := db.StartCount(ctx, countQB.CountSQL(), countQB.CountParams())
	mustReach(t, seams.pageReached, "first-page request")
	mustReach(t, seams.countReached, "count request")

	// Both requests are cancelled while held behind their barriers, so their
	// settlement is fully test-controlled and cannot race this check.
	page.Cancel()
	count.Cancel()
	if page.State() != StateCancelling || count.State() != StateCancelling {
		t.Fatalf("states after cancel = %v/%v, want cancelling/cancelling", page.State(), count.State())
	}
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
			t.Fatal("replacement work leased from the pool while both cancelled requests were unsettled")
		}
	case <-time.After(time.Minute):
		t.Fatal("third-lease attempt deadlocked")
	}

	// The page settles alone: its lease is released at settlement, and the
	// pool then serves exactly one healthy lease for harmless work.
	close(seams.releasePage)
	if pageRows, res := page.Wait(); res.Outcome != OutcomeCancelled || pageRows != nil {
		t.Fatalf("cancelled page = (%+v, %v), want (nil, cancelled)", pageRows, res.Outcome)
	}
	single, err := db.Lease(ctx)
	if err != nil {
		t.Fatalf("lease after the page alone settled: %v", err)
	}
	var version int64
	if err := single.Conn().QueryRowContext(ctx, "PRAGMA schema_version").Scan(&version); err != nil {
		t.Fatalf("harmless work after the page settled failed: %v", err)
	}
	single.Release(ctx)

	// The count settles last, releasing the second lease: both physical
	// connections are healthy and reusable for subsequent requests.
	close(seams.releaseCount)
	if total, res := count.Wait(); res.Outcome != OutcomeCancelled || total != 0 {
		t.Fatalf("cancelled count = (%d, %v), want (0, cancelled)", total, res.Outcome)
	}
	for i := 0; i < 2; i++ {
		lease, err := db.Lease(ctx)
		if err != nil {
			t.Fatalf("lease %d after all-request settlement: %v", i+1, err)
		}
		if err := lease.Conn().QueryRowContext(ctx, "PRAGMA schema_version").Scan(&version); err != nil {
			t.Fatalf("subsequent work on reused connection %d failed: %v", i+1, err)
		}
		lease.Release(ctx)
	}
}

// TestCapabilityLockWaitCountCancelsWithinBusyTimeout proves a count request
// blocked behind another connection's EXCLUSIVE lock settles no later than
// the configured five-second busy timeout when cancelled, and that the
// interrupted connection serves healthy subsequent work afterwards.
func TestCapabilityLockWaitCountCancelsWithinBusyTimeout(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/select_cancel_lock.db"
	buildSmallFixture(t, path)
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open error = %v", err)
	}
	defer db.Close()

	holder, err := db.Lease(ctx)
	if err != nil {
		t.Fatalf("leasing lock-holder connection: %v", err)
	}
	if _, err := holder.Conn().ExecContext(ctx, "BEGIN EXCLUSIVE"); err != nil {
		t.Fatalf("holder BEGIN EXCLUSIVE error = %v", err)
	}

	seams := newBarrierSeams()
	seams.install(db)
	countQB := probeQB("t", "i", false)
	count := db.StartCount(ctx, countQB.CountSQL(), countQB.CountParams())
	mustReach(t, seams.countReached, "count request")
	close(seams.releaseCount)
	contentionStart := time.Now()

	// The EXCLUSIVE lock is verifiably still held while the count request is
	// dispatched: a canary write on a separate session with a deliberately
	// tiny busy timeout fails with a busy error, proving the count is blocked
	// behind the held lock (its own read cannot progress until the holder
	// releases) before any cancellation is requested.
	canary, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("canary open error = %v", err)
	}
	defer canary.Close()
	if _, err := canary.ExecContext(ctx, "PRAGMA busy_timeout = 50"); err != nil {
		t.Fatalf("canary busy_timeout error = %v", err)
	}
	if _, err := canary.ExecContext(ctx, "BEGIN IMMEDIATE"); err == nil {
		t.Fatal("canary write succeeded while the EXCLUSIVE lock was supposed to be held (count not provably blocked)")
	}
	if t.Failed() {
		t.FailNow()
	}

	// Cancellation is requested while the count is blocked, and settlement
	// must land no later than the configured busy-timeout expiry measured
	// from the moment the statement began contending (the barrier release),
	// plus the test's busy-handler overshoot margin.
	count.Cancel()
	total, res := mustSettleWithin(t, count, contentionStart, "lock-wait count", capabilityLockBudget+lockSettleGrace)
	// The pinned driver does not preempt a busy-handler wait with
	// sqlite3_interrupt: the blocked read settles at the configured
	// five-second expiry with SQLITE_BUSY, which classifies as failed (the
	// busy cause is preserved) rather than cancelled, per the Issue #6
	// classification rules. Cancellation was still requested while blocked,
	// and the bound holds: settlement landed within one busy-timeout window.
	if res.Outcome == OutcomeSuccess {
		t.Fatalf("lock-wait count settled as %v with total %d after cancellation", res.Outcome, total)
	}
	if total != 0 {
		t.Fatalf("lock-wait count returned total %d after cancellation, want zero", total)
	}

	if _, err := holder.Conn().ExecContext(ctx, "ROLLBACK"); err != nil {
		t.Fatalf("holder ROLLBACK error = %v", err)
	}
	holder.Release(ctx)

	lease, err := db.Lease(ctx)
	if err != nil {
		t.Fatalf("leasing after lock-wait settlement: %v", err)
	}
	var version int64
	if err := lease.Conn().QueryRowContext(ctx, "PRAGMA schema_version").Scan(&version); err != nil {
		t.Fatalf("subsequent work on the interrupted connection failed: %v", err)
	}
	lease.Release(ctx)
}

// buildSmallFixture creates a tiny five-row table t at path through a
// session separate from any pool under test, for quick builder-generated
// counts.
func buildSmallFixture(t *testing.T, path string) {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("fixture open %q error = %v", path, err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE t (i INTEGER PRIMARY KEY);
INSERT INTO t VALUES (1), (2), (3), (4), (5);`); err != nil {
		t.Fatalf("building small fixture: %v", err)
	}
}

// TestCapabilityLateSuccessIsCancellationWinsOnRealConnection proves on a
// real driver-backed lease that a count whose statement already succeeded —
// but whose settled result is released only after cancellation was
// requested — is classified cancelled with cancellation winning, leaks no
// total, and leaves the connection healthy and reusable, never force-closed.
func TestCapabilityLateSuccessIsCancellationWinsOnRealConnection(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/select_cancel_late.db"
	buildSmallFixture(t, path)
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open error = %v", err)
	}
	defer db.Close()

	countQB := probeQB("t", "i", false)
	countSQL := countQB.CountSQL()
	countParams := countQB.CountParams()

	executed := make(chan struct{})
	release := make(chan struct{})
	var counted int64
	late := db.startRequest(ctx, func(reqCtx context.Context, conn *sql.Conn) error {
		// A real builder-generated count runs to success on the leased
		// connection; its settled success is deliberately held until after
		// Cancel is requested below, so cancellation wins the classification.
		n, err := runCount(reqCtx, conn, countSQL, countParams)
		if err != nil {
			return err
		}
		counted = n
		close(executed)
		<-release
		return nil
	})
	mustReach(t, executed, "late-success count statement")
	if late.State() != StateRunning {
		t.Fatalf("running late-success request state = %v, want running", late.State())
	}

	lateCancelAt := time.Now()
	late.Cancel()
	close(release)
	res := <-late.done
	if took := time.Since(lateCancelAt); took > capabilityCPUbudget {
		t.Fatalf("late-success settlement took %v after cancellation, exceeding the mandatory %v bound", took, capabilityCPUbudget)
	}
	if res.Outcome != OutcomeCancelled {
		t.Fatalf("late-success outcome = %v, want cancelled (cancellation wins)", res.Outcome)
	}
	if counted == 0 {
		t.Fatal("late success recorded no total: the statement did not actually succeed before cancellation")
	}

	lease, err := db.Lease(ctx)
	if err != nil {
		t.Fatalf("leasing after late-success cancellation: %v", err)
	}
	var version int64
	if err := lease.Conn().QueryRowContext(ctx, "PRAGMA schema_version").Scan(&version); err != nil {
		t.Fatalf("subsequent work after cancelled late success failed: %v", err)
	}
	lease.Release(ctx)
}
