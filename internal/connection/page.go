// Production paged-page SELECT execution (Issue #25), per the Paging
// consistency decision in Notes/PRD-sqloid.md. The statement is always the
// complete page SQL — base SELECT, any explicit user ORDER BY, the single
// eligible `ORDER BY rowid` fallback, and the exact LIMIT/OFFSET range —
// produced by internal/querybuilder's page API, so this package neither
// rebuilds nor rewrites any SQL. Each page runs once as a complete
// RunRequest on its own dedicated leased connection, exactly like a first
// page: typed rows are scanned eagerly and converted once into the shared
// internal/result representation, and failures return the failed
// RequestResult with the cause preserved. No adjacent-offset arithmetic,
// serialization, or caching lives here: the UI owns the page range.

package connection

import (
	"context"
	"database/sql"

	"github.com/chris/sqloid/internal/result"
)

// ExecutePage executes one paged page SELECT and returns the typed page.
// The statement and parameters must come from the QueryBuilder page
// rendering seam; the exact LIMIT/OFFSET range is already inside the
// statement text. Cancelling parent aborts execution through the leased
// connection. On success the returned RequestResult has OutcomeSuccess and
// the populated page; failures return a nil page plus the failed
// RequestResult whose Err preserves the underlying cause and whose Health
// carries any deletion or replacement classification, exactly like any
// other database request.
func (db *DB) ExecutePage(parent context.Context, statement string, params []any) (*result.Page, RequestResult) {
	var page *result.Page

	res := db.RunRequest(parent, func(ctx context.Context, conn *sql.Conn) error {
		p, err := runFirstPage(ctx, conn, statement, params)
		if err != nil {
			return err
		}
		page = p
		return nil
	})
	return page, res
}
