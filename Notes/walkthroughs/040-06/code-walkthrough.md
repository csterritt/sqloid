# Issue #40 — destructive-write estimate presentation and count

*2026-08-29T13:06:04Z by Showboat 0.6.1*
<!-- showboat-id: 50c8f79b-a044-4124-9270-1fa7bf9b5c0b -->

This walkthrough verifies the Issue #40 implementation (destructive-write estimate presentation and count) against the 'Estimate SQL and modal' decision in Notes/PRD-sqloid.md: opening preparation for qualified and unqualified UPDATE and DELETE, the canonical rendered write SQL produced solely by Issue #14's shared identifier/literal atoms, the prominent all-rows warning only for no-WHERE statements, the exact 'SELECT COUNT(*) FROM <quoted target> [WHERE <identical predicate>]' estimate with WHERE-only parameters, the exact pending text with disabled confirmation, current/stale success and failure handling, and dismissal/cancellation without execution or history.

Full package suites for the three touched modules (test durations stripped for reproducibility):

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/querybuilder/ ./internal/connection/ ./internal/ui/ | sed -E 's/[0-9]+\.[0-9]+s//g' | cut -c1-58
```

```output
ok  	github.com/chris/sqloid/internal/querybuilder	
ok  	github.com/chris/sqloid/internal/connection	
ok  	github.com/chris/sqloid/internal/ui	
```

The canonical standalone rendered write SQL derives from the same structured state as the executable statements and inlines every literal through the shared atoms. A temporary evidence test prints the exact artifacts for the UPDATE with mixed Value/NULL SET assignments and a bound-value WHERE, the unqualified DELETE, and their estimate requests:

```bash
cd /home/chris/sqloid && cat > internal/querybuilder/zz_walkthrough_test.go <<'EOF'
package querybuilder

import (
	"testing"
)

func TestWalkthroughEvidence(t *testing.T) {
	// Qualified UPDATE: mixed Value/NULL SET plus a bound-value WHERE.
	q := updateBuilder()
	q = addSetValue(t, q, "score", "7")
	q = addSetNull(t, q, "note")
	q = whereCompleteEqOn(t, q, `first"name`, "alice")
	t.Logf("write SQL      = %s", q.UpdateRenderedSQL())
	t.Logf("estimate SQL   = %s", q.EstimateSQL())
	t.Logf("estimate params= %#v", q.EstimateParams())

	// Unqualified DELETE: no WHERE — the all-rows form.
	d := deleteBuilder()
	t.Logf("write SQL      = %s", d.DeleteRenderedSQL())
	t.Logf("estimate SQL   = %s", d.EstimateSQL())
	t.Logf("estimate params= %#v", d.EstimateParams())
}
EOF
go test ./internal/querybuilder/ -run TestWalkthroughEvidence -v 2>&1 | grep -E 'write SQL|estimate|PASS|ok'; rm internal/querybuilder/zz_walkthrough_test.go
```

```output
    zz_walkthrough_test.go:13: write SQL      = UPDATE "order""items" SET "score" = 7, "note" = NULL WHERE "first""name" = 'alice'
    zz_walkthrough_test.go:14: estimate SQL   = SELECT COUNT(*) FROM "order""items" WHERE "first""name" = ?
    zz_walkthrough_test.go:15: estimate params= []interface {}{"alice"}
    zz_walkthrough_test.go:19: write SQL      = DELETE FROM "order""items"
    zz_walkthrough_test.go:20: estimate SQL   = SELECT COUNT(*) FROM "order""items"
    zz_walkthrough_test.go:21: estimate params= []interface {}(nil)
--- PASS: TestWalkthroughEvidence (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	0.002s
```

The evidence shows the shared-atom guarantees directly: the table and column identifiers are quote-doubled atoms, INTEGER 7 and the SQL-NULL keyword render inline in SET, the typed TEXT value 'alice' is a quote-doubled TEXT literal in the WHERE, the estimate is exactly SELECT COUNT(*) FROM <quoted target> [WHERE <identical predicate>] with the predicate's '?' placeholder reused verbatim, the parameter list carries only the WHERE value (no SET values, and none at all for the no-WHERE delete), and SET fragments never appear in estimate SQL. The contract tests pin these shapes:

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/querybuilder/ -run 'TestUpdateRenderedSQL|TestDeleteRenderedSQL|TestEstimateSQL|TestEstimateParams' -v 2>&1 | grep -E '^(--- |ok|FAIL)' | sed -E 's/[0-9]+\.[0-9]+s//g'
```

```output
--- PASS: TestUpdateRenderedSQLInlinesSharedLiterals ()
--- PASS: TestUpdateRenderedSQLRequiresRunnableUpdate ()
--- PASS: TestDeleteRenderedSQLInlinesSharedLiterals ()
--- PASS: TestDeleteRenderedSQLRequiresRunnableDelete ()
--- PASS: TestEstimateSQLIsExactlyCountOverTargetAndPredicate ()
--- PASS: TestEstimateSQLRequiresDestructiveRunnableState ()
--- PASS: TestEstimateParamsBindOnlyWhereValues ()
ok  	github.com/chris/sqloid/internal/querybuilder	
```

The connection seam executes the estimate as a cancellable independent read and proves the write never runs: a SQLite-backed fixture counts matching targets, binds nothing for the unqualified form, leaves the seeded rows byte-for-byte intact, surfaces query failures, and classifies cancellation:

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/connection/ -run 'TestExecuteEstimate' -v 2>&1 | grep -E '^(--- |ok|FAIL)' | sed -E 's/[0-9]+\.[0-9]+s//g'
```

```output
--- PASS: TestExecuteEstimateCountsMatchingTargets ()
--- PASS: TestExecuteEstimateNeverWrites ()
--- PASS: TestExecuteEstimateSurfacesQueryFailures ()
--- PASS: TestExecuteEstimateHonoursCancellation ()
ok  	github.com/chris/sqloid/internal/connection	
```

The UI modal opens immediately from runnable validated UPDATE and DELETE with a unique preparation identity and exactly one estimate request carrying the exact estimate SQL and WHERE-only parameters; while pending it renders exactly 'Estimating matching target rows…' with the continuously visible operation, table, canonical SQL, and the prominent all-rows warning only for no-WHERE statements:

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/ui/ -run 'TestDestructivePreparationOpensFromValidatedWrites|TestDestructivePreparationShowsOperationTableSQLAndStatus|TestDestructivePreparationEstimateExcludesSetValues' -v 2>&1 | grep -E '^(--- |=== RUN   Test[A-Z]|ok|FAIL)' | sed -E 's/[0-9]+\.[0-9]+s//g'
```

```output
=== RUN   TestDestructivePreparationOpensFromValidatedWrites
=== RUN   TestDestructivePreparationOpensFromValidatedWrites/unqualified_update
=== RUN   TestDestructivePreparationOpensFromValidatedWrites/qualified_update
=== RUN   TestDestructivePreparationOpensFromValidatedWrites/unqualified_delete
=== RUN   TestDestructivePreparationOpensFromValidatedWrites/qualified_delete
--- PASS: TestDestructivePreparationOpensFromValidatedWrites ()
=== RUN   TestDestructivePreparationEstimateExcludesSetValues
--- PASS: TestDestructivePreparationEstimateExcludesSetValues ()
=== RUN   TestDestructivePreparationShowsOperationTableSQLAndStatus
=== RUN   TestDestructivePreparationShowsOperationTableSQLAndStatus/qualified_update
=== RUN   TestDestructivePreparationShowsOperationTableSQLAndStatus/unqualified_update
=== RUN   TestDestructivePreparationShowsOperationTableSQLAndStatus/qualified_delete
=== RUN   TestDestructivePreparationShowsOperationTableSQLAndStatus/unqualified_delete
--- PASS: TestDestructivePreparationShowsOperationTableSQLAndStatus ()
ok  	github.com/chris/sqloid/internal/ui	
```

Settlement is identity-guarded: current success retains 'Estimated matching target rows: N' and current failure retains 'Estimate failed: <cause>' while preserving SQL and the warning; a stale identity mutates nothing; Enter/y remain disabled no-ops throughout; Esc dismisses cancelling the in-flight request and restoring the opener; Ctrl+W shows exact 'cancelling…' until the cancelled settlement dismisses; and every stage appends zero query/result history:

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/ui/ -run 'TestDestructivePreparationBlocksConfirmationAndHistoryWhilePending|TestDestructivePreparationRetainsSuccessAndFailure|TestDestructivePreparationRejectsStaleEstimateResponses|TestDestructivePreparationEscDismissesWithCancellation|TestDestructivePreparationCancelThenSettleDismisses|TestDestructivePreparationNeverAppendsHistory' -v 2>&1 | grep -E '^(--- |ok|FAIL)' | sed -E 's/[0-9]+\.[0-9]+s//g'
```

```output
--- PASS: TestDestructivePreparationBlocksConfirmationAndHistoryWhilePending ()
--- PASS: TestDestructivePreparationRetainsSuccessAndFailure ()
--- PASS: TestDestructivePreparationRejectsStaleEstimateResponses ()
--- PASS: TestDestructivePreparationEscDismissesWithCancellation ()
--- PASS: TestDestructivePreparationCancelThenSettleDismisses ()
--- PASS: TestDestructivePreparationNeverAppendsHistory ()
ok  	github.com/chris/sqloid/internal/ui	
```

Every stage above keeps the modal's sole-serializer architecture intact: the QueryBuilder literal renderer is the only SQL serialization path and the modal owns none, Enter/y confirmation stays a no-op (Issue #41 owns enabling it), and no actual write, history append, or transaction is ever started. Cross-references: Notes/issues/040-destructive-write-estimate-presentation-and-count.md, Notes/PRD-sqloid.md (Estimate SQL and modal, Writes lifecycle, Testing Decisions), and Notes/wiki/destructive-preparation.md. Final verification of the whole module:

```bash
cd /home/chris/sqloid && go vet ./... && go test -count=1 ./... 2>&1 | sed -E 's/[0-9]+\.[0-9]+s//g' | cut -c1-58
```

```output
ok  	github.com/chris/sqloid/cmd/sqloid	
ok  	github.com/chris/sqloid/internal/cli	
ok  	github.com/chris/sqloid/internal/connection	
ok  	github.com/chris/sqloid/internal/d1	
ok  	github.com/chris/sqloid/internal/history	
ok  	github.com/chris/sqloid/internal/querybuilder	
ok  	github.com/chris/sqloid/internal/result	
ok  	github.com/chris/sqloid/internal/resultcache	
ok  	github.com/chris/sqloid/internal/schema	
ok  	github.com/chris/sqloid/internal/ui	
```
