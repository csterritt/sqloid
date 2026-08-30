// Independent destructive matching-target estimate execution (Issue #40),
// per the Estimate SQL and modal decision in Notes/PRD-sqloid.md. The
// statement is always the querybuilder rendering seam's exact
// `SELECT COUNT(*) FROM <quoted target> [WHERE <identical predicate>]` with
// its WHERE-only ordered parameters — this package neither rebuilds nor
// rewrites any SQL and never executes the destructive write itself. The
// estimate runs once as a complete RunRequest on its own leased connection
// as an independent autocommit read, cancellable through the caller's
// context, and returns the counted total with the same request-result
// classification rules as every other database request.

package connection

import (
	"context"
	"database/sql"
)

// ExecuteEstimate executes one matching-target estimate read and returns the
// total. The statement and parameters must come from the QueryBuilder
// estimate rendering seam. Cancelling parent aborts execution through the
// leased connection. Failures return the total zero plus the failed
// RequestResult whose Err preserves the underlying cause and whose Health
// carries any deletion or replacement classification, exactly like any other
// database request.
func (db *DB) ExecuteEstimate(parent context.Context, statement string, params []any) (int64, RequestResult) {
	var total int64

	res := db.RunRequest(parent, func(ctx context.Context, conn *sql.Conn) error {
		n, err := runCount(ctx, conn, statement, params)
		if err != nil {
			return err
		}
		total = n
		return nil
	})
	return total, res
}
