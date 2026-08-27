package connection

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// acquireConfiguredLeases opens path through the shared opener and acquires
// exactly n simultaneous dedicated leases, proving the pool can serve them
// concurrently without blocking on an explicit bound. The returned function
// releases all held leases and closes the pool.
func acquireConfiguredLeases(t *testing.T, path string, n int) ([]*Lease, func()) {
	t.Helper()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	t.Cleanup(func() { db.Close() })

	type leaseResult struct {
		lease *Lease
		err   error
	}
	results := make(chan leaseResult, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		go func() {
			<-start
			lease, err := db.Lease(context.Background())
			results <- leaseResult{lease: lease, err: err}
		}()
	}
	close(start)

	leases := make([]*Lease, 0, n)
	for i := 0; i < n; i++ {
		select {
		case r := <-results:
			if r.err != nil {
				t.Fatalf("concurrent lease %d/%d failed: %v", i+1, n, r.err)
			}
			leases = append(leases, r.lease)
		case <-time.After(time.Minute):
			t.Fatalf("acquiring concurrent lease %d/%d exceeded the explicit bound; the pool cannot serve %d simultaneous connections or serialization is blocking acquisition", i+1, n, n)
		}
	}
	release := func() {
		for _, l := range leases {
			l.Release(context.Background())
		}
		db.Close()
	}
	t.Cleanup(release)
	return leases, release
}

// TestPoolHoldsExactlyTwoUsableConnections pins the Issue #5 pool contract:
// database/sql maintains its minimum and maximum at exactly two connections,
// both are usable simultaneously, and they remain open as the retained floor.
func TestPoolHoldsExactlyTwoUsableConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pool.db")
	createDatabase(t, path)

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	defer db.Close()

	if got := db.SQL.Stats().MaxOpenConnections; got != poolSize {
		t.Fatalf("pool configuration: maximum pool size = %d; the exact-two pool requires %d", got, poolSize)
	}

	ctx := context.Background()
	type leaseResult struct {
		lease *Lease
		err   error
	}
	results := make(chan leaseResult, poolSize)
	start := make(chan struct{})
	for i := 0; i < poolSize; i++ {
		go func() {
			<-start
			lease, err := db.Lease(ctx)
			results <- leaseResult{lease: lease, err: err}
		}()
	}
	close(start)

	var leases []*Lease
	for i := 0; i < poolSize; i++ {
		select {
		case r := <-results:
			if r.err != nil {
				t.Fatalf("concurrent lease %d/%d failed: %v", i+1, poolSize, r.err)
			}
			leases = append(leases, r.lease)
		case <-time.After(time.Minute):
			t.Fatalf("acquiring concurrent lease %d/%d exceeded the explicit bound; leases are being serialized rather than served by distinct pool members", i+1, poolSize)
		}
	}
	defer func() {
		for _, l := range leases {
			l.Release(ctx)
		}
	}()

	stats := db.SQL.Stats()
	if stats.InUse != poolSize || stats.OpenConnections != poolSize {
		t.Errorf("while holding %d concurrent leases: InUse = %d, OpenConnections = %d; both pooled connections must be usable at once",
			poolSize, stats.InUse, stats.OpenConnections)
	}
	var raw [poolSize]any
	for i, lease := range leases {
		conn := lease.Conn()
		var version int64
		if err := conn.QueryRowContext(ctx, "PRAGMA schema_version").Scan(&version); err != nil {
			t.Errorf("connection %d unusable under load: %v", i, err)
		}
		raw[i] = rawDriverConnPointer(t, conn)
	}

	for _, l := range leases {
		if err := l.Release(ctx); err != nil {
			t.Errorf("releasing lease: %v", err)
		}
	}

	// After release both connections stay pooled as idle: the pool's effective
	// minimum and its maximum are both exactly two.
	stats = db.SQL.Stats()
	if stats.OpenConnections != poolSize || stats.Idle != poolSize {
		t.Errorf("after releasing all leases: OpenConnections = %d, Idle = %d; the pool must maintain a floor of exactly %d retained connections",
			stats.OpenConnections, stats.Idle, poolSize)
	}
}

// rawDriverConnPointer captures the underlying driver connection so tests can
// compare physical identity rather than pooling bookkeeping.
func rawDriverConnPointer(t *testing.T, conn *sql.Conn) any {
	t.Helper()
	var p any
	err := conn.Raw(func(driverConn any) error {
		p = driverConn
		return nil
	})
	if err != nil {
		t.Fatalf("reaching underlying driver connection: %v", err)
	}
	return p
}

// TestEveryConnectionHasFiveSecondBusyTimeout proves each pooled physical
// connection receives the five-second busy timeout required by Issue #5 and
// the Connection pool decision in Notes/PRD-sqloid.md, by inspecting every
// connection individually rather than the pool as a whole.
func TestEveryConnectionHasFiveSecondBusyTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "busy.db")
	createDatabase(t, path)

	for i, lease := range mustSingleLease(t, path) {
		var millis int64
		if err := lease.Conn().QueryRowContext(context.Background(), "PRAGMA busy_timeout").Scan(&millis); err != nil {
			t.Fatalf("connection %d: reading busy timeout: %v", i, err)
		}
		if millis != busyTimeoutMillis {
			t.Errorf("connection %d lacks the five-second busy timeout: got %d ms, want %d ms", i, millis, busyTimeoutMillis)
		}
	}
}

// TestEveryConnectionHasExactLengthLimit proves each pooled physical
// connection carries the exact 64 MiB connection-local SQLITE_LIMIT_LENGTH
// mandated by Issue #5, queried back through sqlite3_limit(-1) which reports
// the current limit without changing it.
func TestEveryConnectionHasExactLengthLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "limit.db")
	createDatabase(t, path)

	for i, lease := range mustSingleLease(t, path) {
		current, err := sqlLengthLimit(t, lease.Conn())
		if err != nil {
			t.Fatalf("connection %d: reading SQLITE_LIMIT_LENGTH: %v", i, err)
		}
		if current != sqlMaxLengthBytes {
			t.Errorf("connection %d lacks the exact 64 MiB length limit: got SQLITE_LIMIT_LENGTH = %d bytes, want %d bytes (64 MiB)", i, current, sqlMaxLengthBytes)
		}
	}
}

// mustSingleLease acquires exactly one lease for per-connection invariant
// checks that do not need simultaneous holding.
func mustSingleLease(t *testing.T, path string) []*Lease {
	t.Helper()
	leases, release := acquireConfiguredLeases(t, path, 1)
	t.Cleanup(release)
	return leases
}

// sqlLengthLimit reports the connection's current SQLITE_LIMIT_LENGTH without
// changing it (sqlite3_limit with -1 queries only).
func sqlLengthLimit(t *testing.T, conn *sql.Conn) (int, error) {
	t.Helper()
	return sqlite.Limit(conn, sqlite3.SQLITE_LIMIT_LENGTH, -1)
}
