//go:build unix

// Independent matching-target estimate execution through the Connection
// boundary (Issue #40): ExecuteEstimate runs exactly the querybuilder-supplied
// `SELECT COUNT(*) FROM <target> [WHERE …]` statement once as an independent
// cancellable read and returns the counted total, without ever executing the
// destructive write itself. WHERE-only parameters bind in predicate order; a
// statement without placeholders binds nothing.

package connection_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/chris/sqloid/internal/connection"
)

// openEstimateFixture opens a real database through connection.Open holding
// one small fixture table seeded with a fixed number of rows.
func openEstimateFixture(t *testing.T) (*connection.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "estimate-fixture.db")
	seed, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("seeding sql.Open: %v", err)
	}
	if _, err := seed.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT NOT NULL);
INSERT INTO users (email) VALUES ('a'), ('b'), ('c')`); err != nil {
		t.Fatalf("seeding fixture database: %v", err)
	}
	seed.Close()
	db, err := connection.Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	t.Cleanup(func() { db.Close() })
	return db, path
}

func TestExecuteEstimateCountsMatchingTargets(t *testing.T) {
	db, _ := openEstimateFixture(t)

	total, res := db.ExecuteEstimate(context.Background(),
		`SELECT COUNT(*) FROM "users" WHERE "id" >= ?`, []any{int64(2)})
	if res.Outcome != connection.OutcomeSuccess || res.Err != nil {
		t.Fatalf("ExecuteEstimate outcome = %v err = %v", res.Outcome, res.Err)
	}
	if total != 2 {
		t.Errorf("ExecuteEstimate total = %d, want 2", total)
	}

	// The unqualified estimate form binds no parameters and counts every row.
	total, res = db.ExecuteEstimate(context.Background(),
		`SELECT COUNT(*) FROM "users"`, nil)
	if res.Outcome != connection.OutcomeSuccess || res.Err != nil {
		t.Fatalf("unqualified ExecuteEstimate outcome = %v err = %v", res.Outcome, res.Err)
	}
	if total != 3 {
		t.Errorf("unqualified ExecuteEstimate total = %d, want 3", total)
	}
}

func TestExecuteEstimateNeverWrites(t *testing.T) {
	db, path := openEstimateFixture(t)
	seed, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("verification sql.Open: %v", err)
	}
	defer seed.Close()

	if _, res := db.ExecuteEstimate(context.Background(),
		`SELECT COUNT(*) FROM "users" WHERE "id" = ?`, []any{int64(1)}); res.Outcome != connection.OutcomeSuccess {
		t.Fatalf("ExecuteEstimate outcome = %v err = %v", res.Outcome, res.Err)
	}

	// The estimate is a read: the fixture rows survive byte-for-byte.
	var rows int
	if err := seed.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&rows); err != nil {
		t.Fatalf("verification count: %v", err)
	}
	if rows != 3 {
		t.Errorf("estimate executed a write: users rows = %d, want 3", rows)
	}
}

func TestExecuteEstimateSurfacesQueryFailures(t *testing.T) {
	db, _ := openEstimateFixture(t)

	_, res := db.ExecuteEstimate(context.Background(),
		`SELECT COUNT(*) FROM "missing"`, nil)
	if res.Outcome == connection.OutcomeSuccess || res.Err == nil {
		t.Fatalf("ExecuteEstimate on a missing table succeeded: outcome = %v err = %v", res.Outcome, res.Err)
	}
}

func TestExecuteEstimateHonoursCancellation(t *testing.T) {
	db, _ := openEstimateFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, res := db.ExecuteEstimate(ctx, `SELECT COUNT(*) FROM "users"`, nil)
	if res.Outcome == connection.OutcomeSuccess {
		t.Fatalf("cancelled ExecuteEstimate succeeded: outcome = %v err = %v", res.Outcome, res.Err)
	}
}
