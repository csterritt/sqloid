package connection

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// holdConcurrentLeases acquires exactly two leases from two goroutines, so
// acquisition is genuinely attempted concurrently. Waiting uses channel
// barriers; the explicit one-minute bound exists only to detect a deadlock in
// a silently serializing implementation.
func holdConcurrentLeases(t *testing.T, db *DB) [2]*Lease {
	t.Helper()

	type leaseResult struct {
		lease *Lease
		err   error
	}
	results := make(chan leaseResult, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			lease, err := db.Lease(context.Background())
			results <- leaseResult{lease: lease, err: err}
		}()
	}
	close(start)

	var held [2]*Lease
	for i := 0; i < 2; i++ {
		select {
		case r := <-results:
			if r.err != nil {
				t.Fatalf("concurrent lease %d/2 failed: %v", i+1, r.err)
			}
			held[i] = r.lease
		case <-time.After(time.Minute):
			t.Fatalf("acquiring concurrent lease %d/2 exceeded the explicit bound; callers are silently serialized instead of receiving distinct dedicated connections", i+1)
		}
	}
	return held
}

// setJournalAndOpen builds a fixture already in mode (wal or rollback-journal
// "delete"), records its journal mode before opening, opens it through the
// shared opener, and returns the opened DB plus the recorded path and mode.
func setJournalAndOpen(t *testing.T, mode string) (*DB, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "leased.db")
	createDatabase(t, path)
	if mode != "delete" {
		setJournalMode(t, path, mode)
	}
	if before := journalMode(t, path); before != mode {
		t.Fatalf("fixture %q has journal mode %q, want %q", path, before, mode)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	t.Cleanup(func() { db.Close() })
	return db, path
}

// TestConcurrentLeasesAreDistinctConnections covers the Issue #5 dedicated-
// leasing contract against the PRD's high-risk scenario: harmless concurrent
// requests on fixtures already in WAL and rollback-journal modes must receive
// distinct physical connections, each carrying the five-second busy timeout
// and exact 64 MiB length limit, without Connection mutating journal mode.
func TestConcurrentLeasesAreDistinctConnections(t *testing.T) {
	ctx := context.Background()

	for _, mode := range []string{"delete", "wal"} {
		t.Run(mode, func(t *testing.T) {
			db, path := setJournalAndOpen(t, mode)

			held := holdConcurrentLeases(t, db)
			defer func() {
				for _, l := range held {
					l.Release(ctx)
				}
			}()

			first := rawDriverConnPointer(t, held[0].Conn())
			second := rawDriverConnPointer(t, held[1].Conn())
			if first == second {
				t.Errorf("both concurrent leases resolved to the same physical connection; callers must receive distinct dedicated connections")
			}

			for i, lease := range held {
				var version int64
				if err := lease.Conn().QueryRowContext(ctx, "PRAGMA schema_version").Scan(&version); err != nil {
					t.Errorf("lease %d cannot run a harmless request while the other is held: %v", i, err)
				}
				var busy int64
				if err := lease.Conn().QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busy); err != nil {
					t.Errorf("lease %d: reading busy timeout: %v", i, err)
				} else if busy != busyTimeoutMillis {
					t.Errorf("lease %d lacks the five-second busy timeout: got %d ms, want %d ms", i, busy, busyTimeoutMillis)
				}
				limit, err := sqlLengthLimit(t, lease.Conn())
				if err != nil {
					t.Errorf("lease %d: reading SQLITE_LIMIT_LENGTH: %v", i, err)
				} else if limit != sqlMaxLengthBytes {
					t.Errorf("lease %d lacks the exact 64 MiB length limit: got SQLITE_LIMIT_LENGTH = %d bytes, want %d bytes", i, limit, sqlMaxLengthBytes)
				}
			}

			// Journal mode recorded before Open is unchanged after lease use;
			// Connection must neither set nor mutate it.
			if after := journalMode(t, path); after != mode {
				t.Errorf("journal mode changed across open-and-lease use: got %q, want unchanged %q", after, mode)
			}
		})
	}
}

// TestLeaseReleaseIsSafeAndRefusesReuse proves release cleanup is safe after
// success and that a released lease cannot be reused to run work against a
// connection the caller no longer owns.
func TestLeaseReleaseIsSafeAndRefusesReuse(t *testing.T) {
	ctx := context.Background()
	db, _ := setJournalAndOpen(t, "delete")

	lease, err := db.Lease(ctx)
	if err != nil {
		t.Fatalf("Lease error = %v", err)
	}
	var version int64
	if err := lease.Conn().QueryRowContext(ctx, "PRAGMA schema_version").Scan(&version); err != nil {
		t.Fatalf("querying leased connection: %v", err)
	}

	if err := lease.Release(ctx); err != nil {
		t.Fatalf("Release error = %v", err)
	}
	// Repeated release is safe cleanup, including on the error paths callers
	// may take.
	if err := lease.Release(ctx); err != nil {
		t.Fatalf("second Release error = %v", err)
	}

	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("Conn() on a released lease did not panic; reuse of released leases must be refused")
			}
			if r != "connection: use of released lease" {
				t.Fatalf("Conn() panic = %v, want the released-lease misuse message", r)
			}
		}()
		lease.Conn()
	}()
}
