// Production INSERT execution (Issue #39), per the INSERT handling decision
// in Notes/PRD-sqloid.md. The statement is always the complete INSERT SQL —
// ordered columns, `?` placeholders, NULL keywords, or the all-omit
// DEFAULT VALUES form — produced by internal/querybuilder's rendering seam,
// so this package neither rebuilds nor rewrites any SQL and never synthesizes
// hidden virtual-module arguments: modules requiring hidden inputs surface
// ordinary database errors, unchanged. Each INSERT runs once as one complete
// RunRequest on its own dedicated leased connection, so a failed statement's
// constraint or module error rolls back atomically and the typed
// classification follows the same race rules as every other request.

package connection

import (
	"context"
	"database/sql"
)

// ExecuteInsert executes one complete INSERT statement and returns the
// number of rows inserted. The statement and parameters must come from the
// QueryBuilder INSERT rendering seam. Cancelling parent aborts execution
// through the leased connection. Failures return the failed RequestResult
// whose Err preserves the underlying cause — including virtual-table module
// errors — and whose Health carries any deletion or replacement
// classification, exactly like any other database request.
func (db *DB) ExecuteInsert(parent context.Context, statement string, params []any) (int64, RequestResult) {
	var affected int64

	res := db.RunRequest(parent, func(ctx context.Context, conn *sql.Conn) error {
		result, err := conn.ExecContext(ctx, statement, params...)
		if err != nil {
			return err
		}
		affected, err = result.RowsAffected()
		if err != nil {
			return err
		}
		return nil
	})
	return affected, res
}
