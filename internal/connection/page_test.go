// Paged-page SELECT execution coverage (Issue #25 Tasks 4): ExecutePage runs
// exactly one bound page statement — already carrying its exact LIMIT/OFFSET
// range from the QueryBuilder page seam — on a dedicated leased connection
// and returns eagerly scanned typed rows through the same conversion rules
// as a first page. Adjacent offsets return disjoint, order-adjacent row
// slices; failures are ordinary typed requests.

package connection

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestExecutePageAdjacentRanges(t *testing.T) {
	db := openMixed(t)

	first, res := db.ExecutePage(context.Background(), `SELECT id FROM "mix" ORDER BY rowid LIMIT 1 OFFSET 0`, nil, 0)
	if res.Outcome != OutcomeSuccess {
		t.Fatalf("outcome = %v (err %v), want success", res.Outcome, res.Err)
	}
	if len(first.Rows) != 1 || first.Rows[0][0].Int != 1 {
		t.Fatalf("first page rows = %v, want exactly id 1", first.Rows)
	}

	second, res := db.ExecutePage(context.Background(), `SELECT id FROM "mix" ORDER BY rowid LIMIT 1 OFFSET 1`, nil, 1)
	if res.Outcome != OutcomeSuccess {
		t.Fatalf("outcome = %v (err %v), want success", res.Outcome, res.Err)
	}
	if len(second.Rows) != 1 || second.Rows[0][0].Int != 2 {
		t.Fatalf("second page rows = %v, want exactly id 2", second.Rows)
	}

	// An offset beyond the data returns a typed empty page, not an error.
	empty, res := db.ExecutePage(context.Background(), `SELECT id FROM "mix" ORDER BY rowid LIMIT 1 OFFSET 99`, nil, 99)
	if res.Outcome != OutcomeSuccess {
		t.Fatalf("outcome = %v (err %v), want success", res.Outcome, res.Err)
	}
	if len(empty.Rows) != 0 {
		t.Fatalf("empty page rows = %v, want none", empty.Rows)
	}
}

func TestExecutePageFailureIsOrdinaryRequest(t *testing.T) {
	db := openMixed(t)

	page, res := db.ExecutePage(context.Background(), `SELECT * FROM "mix" WHERE no_such_column = 1`, nil, 0)
	if res.Outcome != OutcomeFailed {
		t.Fatalf("outcome = %v, want failed", res.Outcome)
	}
	if page != nil {
		t.Fatal("failed request returned a page")
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "no such column") {
		t.Fatalf("err = %v, want the driver cause preserved", res.Err)
	}
	var wrapped *firstPageError
	if !errors.As(res.Err, &wrapped) {
		t.Fatalf("err = %T, want a firstPageError chain", res.Err)
	}
}
