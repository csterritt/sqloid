// Production first-page SELECT execution (Issue #22), per the Execution and
// Result Lifecycle decisions in Notes/PRD-sqloid.md. Exactly one bound SELECT
// statement — always the safely quoted text and ordered parameters produced
// by internal/querybuilder — runs once as a complete RunRequest on its own
// dedicated leased connection, so the Issue #6 request lifecycle and Issue #7
// health classification apply unchanged. Rows are scanned eagerly with
// typed SQLite values and converted exactly once into the shared
// internal/result representation; BLOB bytes are preserved verbatim and
// invalid UTF-8 TEXT becomes one U+FFFD per maximal invalid sequence with
// warning metadata. No count request, later paging, cache, or finalization
// behavior lives here.

package connection

import (
	"context"
	"database/sql"

	"github.com/chris/sqloid/internal/result"
)

// RunFirstPage executes one first-page SELECT and returns the typed page.
// The statement and parameters must come from the QueryBuilder rendering
// seam; this package neither rebuilds nor rewrites any SQL. Cancelling
// parent aborts execution through the leased connection. On success the
// returned RequestResult has OutcomeSuccess and the populated page; failures
// (query, scan, lease) return a nil page plus the failed RequestResult whose
// Err preserves the underlying cause and whose Health carries any deletion
// or replacement classification, exactly like any other database request.
func (db *DB) RunFirstPage(parent context.Context, statement string, params []any) (*result.Page, RequestResult) {
	var page *result.Page

	res := db.RunRequest(parent, func(ctx context.Context, conn *sql.Conn) error {
		if db.beforeFirstPage != nil {
			db.beforeFirstPage(ctx, conn) // test-only barrier seam (see DB doc)
		}
		p, err := runFirstPage(ctx, conn, statement, params)
		if err != nil {
			return err
		}
		page = p
		return nil
	})
	return page, res
}

// runFirstPage performs the single read on the already-leased connection.
// Rows are scanned eagerly into owned copies because database/sql reuses its
// scan arguments, then converted once into the shared result representation.
func runFirstPage(ctx context.Context, conn *sql.Conn, statement string, params []any) (*result.Page, error) {
	rows, err := conn.QueryContext(ctx, statement, params...)
	if err != nil {
		return nil, wrapFirstPage("execute "+statement, err)
	}

	columns, err := rows.Columns()
	if err != nil {
		rows.Close()
		return nil, wrapFirstPage("read columns", err)
	}

	scan := make([]any, len(columns))
	dest := make([]any, len(columns))
	for i := range dest {
		dest[i] = &scan[i]
	}
	raw := make([][]any, 0, 64)
	for rows.Next() {
		if err := rows.Scan(dest...); err != nil {
			rows.Close()
			return nil, wrapFirstPage("scan row", err)
		}
		row := make([]any, len(scan))
		copy(row, scan)
		raw = append(raw, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, wrapFirstPage("iterate rows", err)
	}
	rows.Close()

	page := result.FromDriver(columns, raw)
	return &page, nil
}

// wrapFirstPage adds operation context while preserving the driver cause via
// %w so callers can inspect it with errors.Is/errors.As; outcome
// classification and terminal wording stay owned by RunRequest and the UI.
func wrapFirstPage(what string, cause error) error {
	return &firstPageError{what: what, cause: cause}
}

// firstPageError marks one failing step of a first-page execution without
// claiming any classification of its own.
type firstPageError struct {
	what  string
	cause error
}

// Error returns lower-case diagnostic text naming the failed step, the
// statement, and its cause.
func (e *firstPageError) Error() string {
	return "could not run select — " + e.what + ": " + e.cause.Error()
}

// Unwrap exposes the underlying cause for errors.Is/errors.As inspection.
func (e *firstPageError) Unwrap() error { return e.cause }
