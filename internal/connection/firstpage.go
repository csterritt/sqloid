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
	"errors"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/chris/sqloid/internal/result"
)

// RunFirstPage executes one first-page SELECT and returns the typed page.
// The statement and parameters must come from the QueryBuilder rendering
// seam; this package neither rebuilds nor rewrites any SQL. Cancelling
// parent aborts execution through the leased connection. On success the
// returned RequestResult has OutcomeSuccess and the populated page; failures
// (query, scan, lease) return the failed RequestResult whose Err preserves
// the underlying cause and whose Health carries any deletion or replacement
// classification, exactly like any other database request.
//
// Issue #31: when a scanned value exceeds the connection-local 64 MiB
// SQLITE_LIMIT_LENGTH, the request fails typed with
// *result.LimitFailure{KindValue} naming the one-based absolute logical
// result position of that row. The oversized row is never exposed: every
// earlier complete row is returned as a partial page with exact bytes, and
// no bytes or fields of the failing row are retained.
func (db *DB) RunFirstPage(parent context.Context, statement string, params []any) (*result.Page, RequestResult) {
	var page *result.Page

	res := db.RunRequest(parent, func(ctx context.Context, conn *sql.Conn) error {
		if db.beforeFirstPage != nil {
			db.beforeFirstPage(ctx, conn) // test-only barrier seam (see DB doc)
		}
		p, err := runFirstPage(ctx, conn, statement, params, 0)
		// Issue #31: a typed value-limit failure still returns the complete
		// leading rows of this page alongside the failure, so the partial page
		// is kept even when the request failed.
		if p != nil {
			page = p
		}
		if err != nil {
			return err
		}
		return nil
	})
	return page, res
}

// runFirstPage performs the single read on the already-leased connection.
// Rows are scanned eagerly into owned copies because database/sql reuses its
// scan arguments, then converted once into the shared result representation.
// offset is the count of absolute logical result rows before this page (the
// paged-page OFFSET), so value-limit failures report the one-based absolute
// logical position. Issue #31: an oversized value stops the scan typed at the
// failing row; the partial page of earlier complete rows is still returned
// alongside the *result.LimitFailure, and the failing row's bytes are never
// exposed.
func runFirstPage(ctx context.Context, conn *sql.Conn, statement string, params []any, offset int64) (*result.Page, error) {
	rows, err := conn.QueryContext(ctx, statement, params...)
	if err != nil {
		// Issue #31: the driver enforces SQLITE_LIMIT_LENGTH during statement
		// execution, so an oversized value can surface here before any row is
		// scanned. The failing row is the page's first: position offset+1.
		if failure := valueLimitFailure(err, offset+1); failure != nil {
			return nil, failure
		}
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
	for rowIdx := int64(0); rows.Next(); rowIdx++ {
		if err := rows.Scan(dest...); err != nil {
			if failure := valueLimitFailure(err, offset+rowIdx+1); failure != nil {
				// Typed Issue #31 failure: return the complete leading rows
				// plus the typed error; the oversized row is not exposed.
				rows.Close()
				partial := result.FromDriver(columns, raw)
				return &partial, failure
			}
			rows.Close()
			return nil, wrapFirstPage("scan row", err)
		}
		row := make([]any, len(scan))
		copy(row, scan)
		if pos := oversizedValue(row); pos {
			rows.Close()
			partial := result.FromDriver(columns, raw)
			var failure error = &result.LimitFailure{Kind: result.KindValue, Position: offset + rowIdx + 1}
			return &partial, failure
		}
		raw = append(raw, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		// Issue #31: the driver can also enforce the length limit while the
		// iteration is stepping. The row that failed is the one after every
		// completely scanned row, so its one-based absolute logical position
		// is offset+len(complete rows)+1, and only the complete leading rows
		// come back — never a partial row.
		if failure := valueLimitFailure(err, offset+int64(len(raw))+1); failure != nil {
			partial := result.FromDriver(columns, raw)
			return &partial, failure
		}
		return nil, wrapFirstPage("iterate rows", err)
	}
	rows.Close()

	converted := result.FromDriver(columns, raw)
	return &converted, nil
}

// oversizedValue reports whether any raw scanned value in one row exceeds
// the connection-local 64 MiB SQLITE_LIMIT_LENGTH. Only TEXT and BLOB can
// grow that large; INTEGER and REAL are 8 bytes and NULL costs nothing.
func oversizedValue(row []any) bool {
	for _, v := range row {
		switch val := v.(type) {
		case string:
			if int64(len(val)) > sqlMaxLengthBytes {
				return true
			}
		case []byte:
			if int64(len(val)) > sqlMaxLengthBytes {
				return true
			}
		}
	}
	return false
}

// valueLimitFailure classifies a driver scan error as the typed Issue #31
// value-limit failure at the given one-based logical position when the cause
// is SQLITE_TOOBIG (the driver's own enforcement of SQLITE_LIMIT_LENGTH),
// and returns nil for every other error.
func valueLimitFailure(err error, position int64) error {
	var driverErr *sqlite.Error
	if !errors.As(err, &driverErr) || driverErr.Code() != sqlite3.SQLITE_TOOBIG {
		return nil
	}
	var failure error = &result.LimitFailure{Kind: result.KindValue, Position: position}
	return failure
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
