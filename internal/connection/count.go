// Independent complete-SELECT result count execution (Issue #24), per the
// Paging consistency decision in Notes/PRD-sqloid.md. Exactly one bound count
// statement — always the complete-SELECT subquery text and ordered parameters
// produced by internal/querybuilder — runs once as a complete RunRequest on
// its own dedicated leased connection, concurrent with (never serialized
// behind) the first page, as an independent autocommit read with no shared
// snapshot. No page behavior, later paging, or drift resolution lives here:
// an independently observed count may legitimately differ from page rows.

package connection

import (
	"context"
	"database/sql"
)

// RunCount executes one complete-SELECT result count and returns the total.
// The statement and parameters must come from the QueryBuilder rendering
// seam; this package neither rebuilds nor rewrites any SQL. Cancelling
// parent aborts execution through the leased connection. On success the
// returned RequestResult has OutcomeSuccess and the counted total; failures
// (query, scan, lease) return the total zero plus the failed RequestResult
// whose Err preserves the underlying cause and whose Health carries any
// deletion or replacement classification, exactly like any other database
// request. Health and cancellation classifications are never reinterpreted
// as page failures by callers.
func (db *DB) RunCount(parent context.Context, statement string, params []any) (int64, RequestResult) {
	var total int64

	res := db.RunRequest(parent, func(ctx context.Context, conn *sql.Conn) error {
		if db.beforeCount != nil {
			db.beforeCount(ctx, conn) // test-only barrier seam (see DB doc)
		}
		n, err := runCount(ctx, conn, statement, params)
		if err != nil {
			return err
		}
		total = n
		return nil
	})
	return total, res
}

// runCount performs the single count read on the already-leased connection:
// exactly the wrapped complete-SELECT COUNT(*) row, scanned once as int64.
func runCount(ctx context.Context, conn *sql.Conn, statement string, params []any) (int64, error) {
	var total int64
	if err := conn.QueryRowContext(ctx, statement, params...).Scan(&total); err != nil {
		return 0, wrapCount("execute "+statement, err)
	}
	return total, nil
}

// wrapCount adds operation context while preserving the driver cause via %w
// so callers can inspect it with errors.Is/errors.As; outcome classification
// and terminal wording stay owned by RunRequest and the UI.
func wrapCount(what string, cause error) error {
	return &countError{what: what, cause: cause}
}

// countError marks one failing step of a count execution without claiming
// any classification of its own.
type countError struct {
	what  string
	cause error
}

// Error returns lower-case diagnostic text naming the failed step, the
// statement, and its cause.
func (e *countError) Error() string {
	return "could not run count — " + e.what + ": " + e.cause.Error()
}

// Unwrap exposes the underlying cause for errors.Is/errors.As inspection.
func (e *countError) Unwrap() error { return e.cause }
