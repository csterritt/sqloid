# Issue #22 walkthrough: first end-to-end SELECT and result grid

*2026-08-27T23:08:18Z by Showboat 0.6.1*
<!-- showboat-id: 208311e3-8186-463d-9637-220a00dba8d0 -->

This walkthrough demonstrates Issue #22 (first end-to-end SELECT and result grid), per Notes/PRD-sqloid.md. Each step re-runs real repository tests as evidence.

**1. Runnable builder produces exact safe SQL and ordered parameters.** QueryBuilder is the sole source of the executed statement; these tests pin wildcard SELECT, duplicate-label aggregate projections, WHERE binding order, and GROUP BY/ORDER BY/LIMIT assembly.

```bash
go test ./internal/querybuilder -count=1 -run 'TestSelectSQL' -v 2>&1 | sed -E 's/(([0-9]+.[0-9]+s|cached))/(t)/'
```

```output
=== RUN   TestSelectSQLWildcard
--- PASS: TestSelectSQLWildcard ((t))
=== RUN   TestSelectSQLDuplicateLabelAggregates
--- PASS: TestSelectSQLDuplicateLabelAggregates ((t))
=== RUN   TestSelectSQLWhereParamsOrdered
--- PASS: TestSelectSQLWhereParamsOrdered ((t))
=== RUN   TestSelectSQLGroupOrderLimit
--- PASS: TestSelectSQLGroupOrderLimit ((t))
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	(t)
```


**2. Schema validation succeeds, history appends at the actual-execution boundary, and exactly one first-page execution runs with the builder's exact SQL/params.** The scripted tests drive the full model route: Enter on runnable data opens Issue #21 validation, the unchanged version settles it, `ExecutionStartedMsg` appends query history (with consecutive-identical suppression) and returns the executor command; failed validation runs no execution and appends nothing.

```bash
go test ./internal/ui -count=1 -run 'TestFirstSelect' -v 2>&1 | sed -E 's/(([0-9]+.[0-9]+s|cached))/(t)/'
```

```output
=== RUN   TestFirstSelectRunsOnePageAfterValidation
--- PASS: TestFirstSelectRunsOnePageAfterValidation ((t))
=== RUN   TestFirstSelectHistorySuppressionAtBoundary
--- PASS: TestFirstSelectHistorySuppressionAtBoundary ((t))
=== RUN   TestFirstSelectErrorFollowsOrdinaryBoundary
--- PASS: TestFirstSelectErrorFollowsOrdinaryBoundary ((t))
PASS
ok  	github.com/chris/sqloid/internal/ui	(t)
```


**3. SQLite first-page execution through the Connection boundary.** `RunFirstPage` runs one bound SELECT as a complete `RunRequest` on a dedicated lease (Issue #6 lifecycle, Issue #7 health classification), eagerly scans typed rows with original driver labels, preserves BLOB bytes, and converts once into `internal/result`.

```bash
go test ./internal/connection -count=1 -run 'TestRunFirstPage' -v 2>&1 | sed -E 's/(([0-9]+.[0-9]+s|cached))/(t)/'
```

```output
=== RUN   TestRunFirstPageTypedRowsAndLabels
--- PASS: TestRunFirstPageTypedRowsAndLabels ((t))
=== RUN   TestRunFirstPageNullsTyped
--- PASS: TestRunFirstPageNullsTyped ((t))
=== RUN   TestRunFirstPageZeroRows
--- PASS: TestRunFirstPageZeroRows ((t))
=== RUN   TestRunFirstPageQueryErrorOrdinaryFailure
--- PASS: TestRunFirstPageQueryErrorOrdinaryFailure ((t))
=== RUN   TestRunFirstPageBindsParamsInOrder
--- PASS: TestRunFirstPageBindsParamsInOrder ((t))
=== RUN   TestRunFirstPagePrecancelledContext
--- PASS: TestRunFirstPagePrecancelledContext ((t))
=== RUN   TestRunFirstPageScanTypedWithoutStringCoercion
--- PASS: TestRunFirstPageScanTypedWithoutStringCoercion ((t))
PASS
ok  	github.com/chris/sqloid/internal/connection	(t)
```


**4. The frozen result grid: deduplicated headers, absolute range, typed cells, and the executed-empty state.** These tests consume only `internal/result` fixtures: full-set duplicate labels like repeated `COUNT(*)` collide safely, the status line shows the absolute inclusive `rows 1-N` range, numeric-looking TEXT never coerces, tabs/newlines render as visible symbols, an invalid-UTF-8 warning persists without adding rows, and an executed empty SELECT shows exactly `No rows`.

```bash
go test ./internal/ui -count=1 -run 'TestResultGrid' -v 2>&1 | sed -E 's/(([0-9]+.[0-9]+s|cached))/(t)/'
```

```output
=== RUN   TestResultGridFrozenDeduplicatedHeader
--- PASS: TestResultGridFrozenDeduplicatedHeader ((t))
=== RUN   TestResultGridAbsoluteRangeStatus
--- PASS: TestResultGridAbsoluteRangeStatus ((t))
=== RUN   TestResultGridTypedCellDistinctions
--- PASS: TestResultGridTypedCellDistinctions ((t))
=== RUN   TestResultGridVisibleControlCharacters
--- PASS: TestResultGridVisibleControlCharacters ((t))
=== RUN   TestResultGridInvalidUTFWarningPersistent
--- PASS: TestResultGridInvalidUTFWarningPersistent ((t))
=== RUN   TestResultGridExactNoRows
--- PASS: TestResultGridExactNoRows ((t))
PASS
ok  	github.com/chris/sqloid/internal/ui	(t)
```


**5. The rendered grid itself.** A temporary demo test (created and removed inside the block, so it re-runs cleanly) drives the production route — runnable builder → validation → execution start → settled page — through a fake executor and prints the real `View()` output. Note the deduplicated `COUNT(*)_2` header, absolute `rows 1-3` status with the persistent invalid-UTF warning, visible `⇥` tab symbol, U+FFFD replacements, exact `[BLOB 3 bytes]`, typed NULL, REAL tokens `1.5`, `0.0`, and `1e+20`, and TEXT `42` left as text.

```bash
cat > internal/ui/demo_grid_test.go <<'EOF'
package ui

import (
	"fmt"
	"testing"

	"github.com/chris/sqloid/internal/result"
)

// Demo fixtures exercised through the production renderResultPage seam.
func demoPage() *result.Page {
	decoded, _ := result.DecodeText("ab\xff\xfex")
	return &result.Page{
		Columns: []string{"COUNT(*)", "COUNT(*)", "note", "data"},
		Rows: [][]result.Value{
			{result.NewInteger(3), result.NewInteger(5), result.NewText("line1\tline2"), result.NewReal(1.5)},
			{result.NewNull(), result.NewText("42"), result.NewText("42"), result.NewReal(-0.0)},
			{result.NewText(decoded), result.NewBlob([]byte{0x00, 0xff, 0xfe}), result.NewText(""), result.NewReal(1e20)},
		},
		InvalidUTF: true,
	}
}

func TestDemoGridRender(t *testing.T) {
	exec := &fakeSelectExecutor{page: demoPage()}
	m := firstSelectModel(exec)
	execModel, execCmd := driveToExecutionStart(t, m)
	final := asModel(execModel.Update(execCmd()))
	fmt.Println(final.View())
}
EOF
go test ./internal/ui -count=1 -run TestDemoGridRender -v 2>&1 | sed -E 's/(([0-9]+.[0-9]+s|cached))/(t)/'
rc=$?
rm internal/ui/demo_grid_test.go
exit $rc
```

```output
=== RUN   TestDemoGridRender
╭──────────────────────────────────────────────────────────────────────────────╮
│rows 1-3 — invalid UTF-8 replaced with U+FFFD                                 │
│COUNT(*) | COUNT(*)_2     | note        | data                                │
│3        | 5              | line1⇥line2 | 1.5                                 │
│(NULL)   | 42             | 42          | 0.0                                 │
│ab��x    | [BLOB 3 bytes] |             | 1e+20                               │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
╰──────────────────────────────────────────────────────────────────────────────╯
╭──────────────────────────────────────────────────────────────────────────────╮
│                                                                              │
│> Command: SELECT                                                             │
│  Table: users                                                                │
│  Column(s): *                                                                │
│  Where: "email" = ?                                                          │
│                                                                              │
╰──────────────────────────────────────────────────────────────────────────────╯
 q quit   ? help                                                                
--- PASS: TestDemoGridRender ((t))
PASS
ok  	github.com/chris/sqloid/internal/ui	(t)
```


**6. Shared result-seam contracts.** `internal/result` owns every representation policy: full-set name deduplication (empty labels, pre-suffixed labels, repeated `COUNT(*)`, collision chains), exact finite REAL tokens, maximal invalid-UTF-8 replacement, and exact BLOB retention with `[BLOB n bytes]` display.

```bash
go test ./internal/result -count=1 -run 'TestDeduplicateNames|TestRealToken|TestDecodeText|TestBlobBytes|TestDisplayTyped|TestTypeIdentity' -v 2>&1 | sed -E 's/(([0-9]+.[0-9]+s|cached))/(t)/'
```

```output
=== RUN   TestDeduplicateNames
=== RUN   TestDeduplicateNames/no_duplicates_unchanged
=== RUN   TestDeduplicateNames/empty_set_stays_empty
=== RUN   TestDeduplicateNames/empty_label_first_occurrence_unchanged
=== RUN   TestDeduplicateNames/duplicate_empty_labels_get_suffixes
=== RUN   TestDeduplicateNames/simple_duplicate_gets__2
=== RUN   TestDeduplicateNames/triple_duplicate_counts_up
=== RUN   TestDeduplicateNames/repeated_computed_labels_deduplicate
=== RUN   TestDeduplicateNames/pre-suffixed_original_name_blocks_colliding_suffix
=== RUN   TestDeduplicateNames/later_original_name_blocks_earlier_duplicate_suffix
=== RUN   TestDeduplicateNames/collision_chain_across_the_full_set
=== RUN   TestDeduplicateNames/duplicate_collides_with_original_even_when_far_later
=== RUN   TestDeduplicateNames/two_pairs_deduplicate_independently
--- PASS: TestDeduplicateNames ((t))
    --- PASS: TestDeduplicateNames/no_duplicates_unchanged ((t))
    --- PASS: TestDeduplicateNames/empty_set_stays_empty ((t))
    --- PASS: TestDeduplicateNames/empty_label_first_occurrence_unchanged ((t))
    --- PASS: TestDeduplicateNames/duplicate_empty_labels_get_suffixes ((t))
    --- PASS: TestDeduplicateNames/simple_duplicate_gets__2 ((t))
    --- PASS: TestDeduplicateNames/triple_duplicate_counts_up ((t))
    --- PASS: TestDeduplicateNames/repeated_computed_labels_deduplicate ((t))
    --- PASS: TestDeduplicateNames/pre-suffixed_original_name_blocks_colliding_suffix ((t))
    --- PASS: TestDeduplicateNames/later_original_name_blocks_earlier_duplicate_suffix ((t))
    --- PASS: TestDeduplicateNames/collision_chain_across_the_full_set ((t))
    --- PASS: TestDeduplicateNames/duplicate_collides_with_original_even_when_far_later ((t))
    --- PASS: TestDeduplicateNames/two_pairs_deduplicate_independently ((t))
=== RUN   TestDeduplicateNamesLeavesOriginalSliceUnchanged
--- PASS: TestDeduplicateNamesLeavesOriginalSliceUnchanged ((t))
=== RUN   TestRealToken
=== RUN   TestRealToken/integral_value_gets_.0
=== RUN   TestRealToken/negative_integral_value_gets_.0
=== RUN   TestRealToken/zero_gets_.0
=== RUN   TestRealToken/negative_zero_gets_.0
=== RUN   TestRealToken/fractional
=== RUN   TestRealToken/negative_fractional
=== RUN   TestRealToken/large_exponent
=== RUN   TestRealToken/small_exponent
=== RUN   TestRealToken/precision_edge_shortest_round_trip
=== RUN   TestRealToken/max_finite
=== RUN   TestRealToken/smallest_subnormal
=== RUN   TestRealToken/integer-like_with_exponent_token_kept
--- PASS: TestRealToken ((t))
    --- PASS: TestRealToken/integral_value_gets_.0 ((t))
    --- PASS: TestRealToken/negative_integral_value_gets_.0 ((t))
    --- PASS: TestRealToken/zero_gets_.0 ((t))
    --- PASS: TestRealToken/negative_zero_gets_.0 ((t))
    --- PASS: TestRealToken/fractional ((t))
    --- PASS: TestRealToken/negative_fractional ((t))
    --- PASS: TestRealToken/large_exponent ((t))
    --- PASS: TestRealToken/small_exponent ((t))
    --- PASS: TestRealToken/precision_edge_shortest_round_trip ((t))
    --- PASS: TestRealToken/max_finite ((t))
    --- PASS: TestRealToken/smallest_subnormal ((t))
    --- PASS: TestRealToken/integer-like_with_exponent_token_kept ((t))
=== RUN   TestRealTokenLocaleIndependent
--- PASS: TestRealTokenLocaleIndependent ((t))
=== RUN   TestRealTokenRoundTrips
--- PASS: TestRealTokenRoundTrips ((t))
=== RUN   TestDecodeTextMaximalInvalidSequences
=== RUN   TestDecodeTextMaximalInvalidSequences/valid_text_passes_through_unchanged
=== RUN   TestDecodeTextMaximalInvalidSequences/lone_continuation_byte_is_one_maximal_invalid_sequence
=== RUN   TestDecodeTextMaximalInvalidSequences/truncated_two-byte_sequence_is_one_maximal_invalid_sequence
=== RUN   TestDecodeTextMaximalInvalidSequences/bad_first_continuation_is_one_sequence_per_subpart
=== RUN   TestDecodeTextMaximalInvalidSequences/overlong_pair_replaced_per_subpart
=== RUN   TestDecodeTextMaximalInvalidSequences/four-byte_lead_with_bad_first_continuation
=== RUN   TestDecodeTextMaximalInvalidSequences/maximal_subpart_consumed_before_valid_tail
--- PASS: TestDecodeTextMaximalInvalidSequences ((t))
    --- PASS: TestDecodeTextMaximalInvalidSequences/valid_text_passes_through_unchanged ((t))
    --- PASS: TestDecodeTextMaximalInvalidSequences/lone_continuation_byte_is_one_maximal_invalid_sequence ((t))
    --- PASS: TestDecodeTextMaximalInvalidSequences/truncated_two-byte_sequence_is_one_maximal_invalid_sequence ((t))
    --- PASS: TestDecodeTextMaximalInvalidSequences/bad_first_continuation_is_one_sequence_per_subpart ((t))
    --- PASS: TestDecodeTextMaximalInvalidSequences/overlong_pair_replaced_per_subpart ((t))
    --- PASS: TestDecodeTextMaximalInvalidSequences/four-byte_lead_with_bad_first_continuation ((t))
    --- PASS: TestDecodeTextMaximalInvalidSequences/maximal_subpart_consumed_before_valid_tail ((t))
=== RUN   TestDisplayTypedCellValues
=== RUN   TestDisplayTypedCellValues/null
=== RUN   TestDisplayTypedCellValues/integer
=== RUN   TestDisplayTypedCellValues/negative_integer
=== RUN   TestDisplayTypedCellValues/real_uses_exact_token
=== RUN   TestDisplayTypedCellValues/real_negative_zero
=== RUN   TestDisplayTypedCellValues/real_exponent
=== RUN   TestDisplayTypedCellValues/text_verbatim_after_transformation
=== RUN   TestDisplayTypedCellValues/text_that_looks_numeric_stays_text_verbatim
=== RUN   TestDisplayTypedCellValues/blob_placeholder
=== RUN   TestDisplayTypedCellValues/empty_blob_placeholder
--- PASS: TestDisplayTypedCellValues ((t))
    --- PASS: TestDisplayTypedCellValues/null ((t))
    --- PASS: TestDisplayTypedCellValues/integer ((t))
    --- PASS: TestDisplayTypedCellValues/negative_integer ((t))
    --- PASS: TestDisplayTypedCellValues/real_uses_exact_token ((t))
    --- PASS: TestDisplayTypedCellValues/real_negative_zero ((t))
    --- PASS: TestDisplayTypedCellValues/real_exponent ((t))
    --- PASS: TestDisplayTypedCellValues/text_verbatim_after_transformation ((t))
    --- PASS: TestDisplayTypedCellValues/text_that_looks_numeric_stays_text_verbatim ((t))
    --- PASS: TestDisplayTypedCellValues/blob_placeholder ((t))
    --- PASS: TestDisplayTypedCellValues/empty_blob_placeholder ((t))
=== RUN   TestTypeIdentityPreserved
--- PASS: TestTypeIdentityPreserved ((t))
=== RUN   TestBlobBytesRetainedExactly
--- PASS: TestBlobBytesRetainedExactly ((t))
PASS
ok  	github.com/chris/sqloid/internal/result	(t)
```


**7. Architecture evidence.** `internal/result/architecture_test.go` pins that the result package imports neither Bubble Tea nor any driver, that `internal/ui` owns no private REAL/BLOB/UTF-8/deduplication formatting, and that no tracer execution route survives. A source grep independently confirms every tracer symbol is gone from production code.

```bash
go test ./internal/result -count=1 -run 'TestResultPackageStaysUIIndependent|TestNoUIPrivateResultRepresentation|TestSingleProductionExecutionRoute' -v 2>&1 | sed -E "s/\(([0-9]+\.[0-9]+s|cached)\)/(t)/; s/([0-9]+\.[0-9]+s)$/(t)/" && echo '--- tracer symbols in production source:' && grep -rliE 'tracer|StartTraceMsg|TraceResult|TraceGrid' internal/ cmd/ --include='*.go' | grep -v _test.go | wc -l
```

```output
=== RUN   TestResultPackageStaysUIIndependent
--- PASS: TestResultPackageStaysUIIndependent (t)
=== RUN   TestNoUIPrivateResultRepresentation
--- PASS: TestNoUIPrivateResultRepresentation (t)
=== RUN   TestSingleProductionExecutionRoute
--- PASS: TestSingleProductionExecutionRoute (t)
PASS
ok  	github.com/chris/sqloid/internal/result	(t)
--- tracer symbols in production source:
0
```


**8. Full verification.** The ordinary execution-error boundary and full suite: errors settle like any other first-page outcome (see step 2), and the whole repository builds, vets, and passes.

```bash
go vet ./... && go build ./... && go test -count=1 ./... 2>&1 | sed -E "s/\(([0-9]+\.[0-9]+s|cached)\)/(t)/; s/([0-9]+\.[0-9]+s)$/(t)/"
```

```output
ok  	github.com/chris/sqloid/cmd/sqloid	(t)
ok  	github.com/chris/sqloid/internal/cli	(t)
ok  	github.com/chris/sqloid/internal/connection	(t)
ok  	github.com/chris/sqloid/internal/d1	(t)
ok  	github.com/chris/sqloid/internal/history	(t)
ok  	github.com/chris/sqloid/internal/querybuilder	(t)
ok  	github.com/chris/sqloid/internal/result	(t)
ok  	github.com/chris/sqloid/internal/schema	(t)
ok  	github.com/chris/sqloid/internal/ui	(t)
```

Every code block re-runs cleanly (showboat verify confirms). References: Issue #22, Notes/PRD-sqloid.md, Notes/wiki/first-select-result-grid.md. All artifacts for this walkthrough live under Notes/walkthroughs/022-09/.
