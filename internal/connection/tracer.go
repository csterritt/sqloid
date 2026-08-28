// Disposable tracer execution path, per Issue #10: executes one catalog-
// chosen object's hardcoded SELECT * through the established RunRequest
// boundary (health checks, leasing, typed outcomes) and returns typed column
// names and row values for composition. This path exists only to de-risk the
// Bubble Tea ↔ Connection ↔ Schema stack; Issue #22 must replace it rather
// than extend it. No builder, schema revalidation, paging, count, history,
// recovery, cancellation, or write behavior lives here.

package connection

import (
	"context"
	"database/sql"

	"github.com/chris/sqloid/internal/schema"
)

// TracerResult is the typed transport of one successful trace execution.
// Columns holds returned column names in result order; Rows holds every row's
// values in column order using the driver/database/sql value types — nil for
// SQL NULL, int64, float64, string, or []byte — so downstream composition can
// render deterministically without re-parsing.
type TracerResult struct {
	Columns []string
	Rows    [][]any
}

// RunTraceSelectAll executes the disposable hardcoded SELECT * for target,
// which must already have been validated against a fresh Catalog via
// schema.ChooseTracerTarget: this package neither revalidates the object nor
// rebuilds any query beyond schema.SelectAllSQL. Execution is one complete
// RunRequest, so request-boundary identity checks and typed outcome
// classification apply exactly as to any other database request. On success
// it returns the populated TracerResult with an OutcomeSuccess result; on
// failure it returns nil plus the failed RequestResult whose Err preserves
// the underlying cause through wrapping. Cancelling parent aborts execution
// through the leased connection.
func (db *DB) RunTraceSelectAll(parent context.Context, target *schema.Object) (*TracerResult, RequestResult) {
	var out *TracerResult

	res := db.RunRequest(parent, func(ctx context.Context, conn *sql.Conn) error {
		t, err := runTraceSelectAll(ctx, conn, target)
		if err != nil {
			return err
		}
		out = t
		return nil
	})
	return out, res
}

// runTraceSelectAll performs the single read on the already-leased connection,
// retaining copied row slices because database/sql reuses its scan arguments.
func runTraceSelectAll(ctx context.Context, conn *sql.Conn, target *schema.Object) (*TracerResult, error) {
	rows, err := conn.QueryContext(ctx, schema.SelectAllSQL(target))
	if err != nil {
		return nil, wrapTrace("execute "+schema.SelectAllSQL(target), err)
	}

	columns, err := rows.Columns()
	if err != nil {
		rows.Close()
		return nil, wrapTrace("read columns", err)
	}

	out := &TracerResult{Columns: append([]string(nil), columns...)}
	scan := make([]any, len(columns))
	dest := make([]any, len(columns))
	for i := range dest {
		dest[i] = &scan[i]
	}
	for rows.Next() {
		if err := rows.Scan(dest...); err != nil {
			rows.Close()
			return nil, wrapTrace("scan row", err)
		}
		row := make([]any, len(scan))
		copy(row, scan)
		out.Rows = append(out.Rows, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, wrapTrace("iterate rows", err)
	}
	rows.Close()
	return out, nil
}

// wrapTrace adds operation context while preserving the driver cause via %w
// so callers can inspect it with errors.Is/errors.As; terminal wording stays
// owned by later issues.
func wrapTrace(what string, cause error) error {
	return &tracerError{what: what, cause: cause}
}

// tracerError marks one failing step of a trace execution without claiming
// any terminal classification of its own.
type tracerError struct {
	what  string
	cause error
}

// Error returns lower-case diagnostic text naming the failed step, the traced
// statement, and its cause.
func (e *tracerError) Error() string {
	return "could not trace " + e.what + ": " + e.cause.Error()
}

// Unwrap exposes the underlying cause for errors.Is/errors.As inspection.
func (e *tracerError) Unwrap() error { return e.cause }
