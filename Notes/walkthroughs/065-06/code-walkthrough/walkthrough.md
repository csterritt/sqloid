# Issue #065 Code Walkthrough: Stale SELECT Projection Gating Through the Runnable Report

*2026-09-01T17:43:36Z by Showboat 0.6.1*
<!-- showboat-id: 3c95225d-ef36-40fa-a3b8-18cad074b937 -->

Issue #65 (Notes/tasks/065-select-projection-staleness-gate.md, Notes/PRD-sqloid.md §Runnable-State Contract, §Query Grammar, §Testing Decisions; user stories 26, 32, and 81) gates stale SELECT projections through the authoritative runnable report. Before this issue, `QueryBuilder.RunnableReport()` checked only that a SELECT had a nonempty projection — every committed named `(column, aggregate)` entry was trusted to still reference a visible column in the refreshed schema. A schema refresh that dropped a projected column left the report runnable, so Enter could start schema validation and execution against a column that no longer existed. Issue #65 adds `reportStaleProjection` to `internal/querybuilder/runnable.go`: after the empty-projection check and before WHERE, every committed `ProjectionColumn` entry is validated against the selected object's current visible columns. A vanished projected column returns `RunnableReport{Field: RunFieldProjection, Reason: "the projected column no longer exists"}`. The synthetic `ProjectionWildcard` and `ProjectionCountStar` identities are exempt by `ProjectionKind` — never by display text — so a real column literally named `*` or `COUNT(*)` is still validated as a named identifier while the synthetic wildcard and sentinel survive any column drop. The UI's existing generic `RunFieldProjection` → `columnsFieldLabel` mapping and `showRunnableReason` rendering satisfy the Enter-gating contract with no production UI change: Enter on a stale projection starts no request, appends no history, and focuses Column(s) with the exact reason verbatim.

This walkthrough demonstrates the stale-projection report and Enter gating through the Issue #65 test suite, covering named Value and aggregate projections, sentinel-versus-named-identity distinction with literal `*` and `COUNT(*)` column names, and the valid wildcard, `COUNT(*)`, and current named-projection controls.

## The stale-projection validation seam

reportSelect now calls reportStaleProjection after the empty-projection check and before reportWhere. The method builds a visible-name set from selectedColumns and returns the first stale named ProjectionColumn entry; wildcard and COUNT(*) are skipped by Kind:

```bash
sed -n '148,205p' internal/querybuilder/runnable.go
```

```output
	return RunnableReport{Runnable: true}
}

// reportSelect evaluates a SELECT in visual order: projection, WHERE,
// grouping, ORDER BY, Limit — reusing the Issue #18 validators for the rules
// they already own. Every committed named projection entry is validated
// against the selected object's current visible columns before later SELECT
// fields; the synthetic wildcard and COUNT(*) sentinel identities are
// exempt by identity, never by display text.
func (q QueryBuilder) reportSelect() RunnableReport {
	if q.ProjectionEmpty() {
		return RunnableReport{Field: RunFieldProjection, Reason: ReasonNoProjection}
	}
	if issue, invalid := q.reportStaleProjection(); invalid {
		return issue
	}
	if r, invalid := q.reportWhere(); invalid {
		return r
	}
	if issue, invalid := validateGrouping(q); invalid {
		return RunnableReport{Field: RunFieldGroupBy, Reason: issue.Reason}
	}
	if issue, invalid := validateOrderBy(q); invalid {
		return RunnableReport{Field: RunFieldOrderBy, Reason: issue.Reason}
	}
	if issue, invalid := validateLimit(q); invalid {
		return RunnableReport{Field: RunFieldLimit, Reason: issue.Reason}
	}
	return RunnableReport{Runnable: true}
}

// reportStaleProjection validates every committed named projection entry
// against the selected object's current visible columns, returning the first
// stale entry as a RunFieldProjection report. The synthetic wildcard and
// COUNT(*) sentinel identities are exempt by identity (Kind), never by
// display text, so a real column literally named `*` or `COUNT(*)` is still
// validated as a named identifier. Reuses selectedColumns for the
// visibility/identity pattern shared by reportWhere and validateGrouping.
func (q QueryBuilder) reportStaleProjection() (RunnableReport, bool) {
	visible := make(map[string]bool, len(q.projection))
	for _, col := range q.selectedColumns() {
		visible[col.Name] = true
	}
	for _, e := range q.projection {
		if e.Kind != ProjectionColumn {
			continue
		}
		if !visible[e.Column] {
			return RunnableReport{Field: RunFieldProjection, Reason: ReasonStaleProjectionColumn}, true
		}
	}
	return RunnableReport{}, false
}

// reportUpdate evaluates an UPDATE in visual order: SET assignments, then the
// optional WHERE. Duplicate SET columns block; every assignment needs exactly
// one complete {Value, NULL} choice, and Value entries must be submitted.
func (q QueryBuilder) reportUpdate() RunnableReport {
```

## Stale named Value and aggregate projections block at RunFieldProjection

The QueryBuilder test suite commits a named Value projection on "score", refreshes against a catalog where score vanished, and asserts the report returns RunFieldProjection with the exact reason. Each named aggregate (Count, Min, Max, Avg, Sum) is covered the same way. Stale projection also blocks before later WHERE, GROUP BY, ORDER BY, and Limit failures in visual order. The full test output (timing stripped for reproducibility):

```bash
go test ./internal/querybuilder/ -run '^TestRunnableSelectGatesStaleNamedProjections$' -count=1 -v 2>&1 | grep -E 'RUN|PASS|FAIL' | sed 's/[0-9]\+\.[0-9]\+s/Ns/g'
```

```output
=== RUN   TestRunnableSelectGatesStaleNamedProjections
=== RUN   TestRunnableSelectGatesStaleNamedProjections/stale_named_Value_projection_blocks
=== RUN   TestRunnableSelectGatesStaleNamedProjections/stale_named_Count_projection_blocks
=== RUN   TestRunnableSelectGatesStaleNamedProjections/stale_named_Min_projection_blocks
=== RUN   TestRunnableSelectGatesStaleNamedProjections/stale_named_Max_projection_blocks
=== RUN   TestRunnableSelectGatesStaleNamedProjections/stale_named_Avg_projection_blocks
=== RUN   TestRunnableSelectGatesStaleNamedProjections/stale_named_Sum_projection_blocks
=== RUN   TestRunnableSelectGatesStaleNamedProjections/stale_projection_blocks_before_open_WHERE_draft
=== RUN   TestRunnableSelectGatesStaleNamedProjections/stale_projection_blocks_before_mixed_grouping_failure
=== RUN   TestRunnableSelectGatesStaleNamedProjections/stale_projection_blocks_before_stale_ORDER_BY
=== RUN   TestRunnableSelectGatesStaleNamedProjections/stale_projection_blocks_before_invalid_Limit
=== RUN   TestRunnableSelectGatesStaleNamedProjections/valid_named_Value_projection_remains_runnable
=== RUN   TestRunnableSelectGatesStaleNamedProjections/valid_named_Min_projection_remains_runnable
=== RUN   TestRunnableSelectGatesStaleNamedProjections/wildcard_remains_runnable_after_refresh
=== RUN   TestRunnableSelectGatesStaleNamedProjections/synthetic_COUNT(*)_sentinel_remains_runnable_after_refresh
=== RUN   TestRunnableSelectGatesStaleNamedProjections/literal_star_named_column_dropped_blocks
=== RUN   TestRunnableSelectGatesStaleNamedProjections/literal_COUNT_star_named_column_dropped_blocks
=== RUN   TestRunnableSelectGatesStaleNamedProjections/synthetic_wildcard_survives_when_literal_star_named_column_drops
=== RUN   TestRunnableSelectGatesStaleNamedProjections/synthetic_COUNT_star_sentinel_survives_when_literal_COUNT_star_named_column_drops
--- PASS: TestRunnableSelectGatesStaleNamedProjections (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/stale_named_Value_projection_blocks (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/stale_named_Count_projection_blocks (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/stale_named_Min_projection_blocks (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/stale_named_Max_projection_blocks (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/stale_named_Avg_projection_blocks (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/stale_named_Sum_projection_blocks (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/stale_projection_blocks_before_open_WHERE_draft (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/stale_projection_blocks_before_mixed_grouping_failure (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/stale_projection_blocks_before_stale_ORDER_BY (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/stale_projection_blocks_before_invalid_Limit (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/valid_named_Value_projection_remains_runnable (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/valid_named_Min_projection_remains_runnable (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/wildcard_remains_runnable_after_refresh (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/synthetic_COUNT(*)_sentinel_remains_runnable_after_refresh (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/literal_star_named_column_dropped_blocks (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/literal_COUNT_star_named_column_dropped_blocks (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/synthetic_wildcard_survives_when_literal_star_named_column_drops (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/synthetic_COUNT_star_sentinel_survives_when_literal_COUNT_star_named_column_drops (Ns)
PASS
```

## Sentinel identity vs. named identifiers with literal unusual column names

The tricky catalog has a table with visible columns literally named `*` and `COUNT(*)` alongside id. Committing the named `*` column as a Value projection and refreshing against a catalog where `*` vanished returns RunFieldProjection — the named identifier is stale. But the synthetic wildcard committed on the same table survives the same refresh, because its identity is ProjectionWildcard, not ProjectionColumn. The same distinction holds for `COUNT(*)`: the named column is stale when dropped, while the synthetic COUNT(*) sentinel survives. The sentinel-identity subtests (timing stripped):

```bash
go test ./internal/querybuilder/ -run '^TestRunnableSelectGatesStaleNamedProjections/literal_star_named_column_dropped_blocks|TestRunnableSelectGatesStaleNamedProjections/synthetic_wildcard_survives_when_literal_star_named_column_drops|TestRunnableSelectGatesStaleNamedProjections/literal_COUNT_star_named_column_dropped_blocks|TestRunnableSelectGatesStaleNamedProjections/synthetic_COUNT_star_sentinel_survives_when_literal_COUNT_star_named_column_drops' -count=1 -v 2>&1 | grep -E 'RUN|PASS|FAIL' | sed 's/[0-9]\+\.[0-9]\+s/Ns/g'
```

```output
=== RUN   TestRunnableSelectGatesStaleNamedProjections
=== RUN   TestRunnableSelectGatesStaleNamedProjections/literal_star_named_column_dropped_blocks
=== RUN   TestRunnableSelectGatesStaleNamedProjections/literal_COUNT_star_named_column_dropped_blocks
=== RUN   TestRunnableSelectGatesStaleNamedProjections/synthetic_wildcard_survives_when_literal_star_named_column_drops
=== RUN   TestRunnableSelectGatesStaleNamedProjections/synthetic_COUNT_star_sentinel_survives_when_literal_COUNT_star_named_column_drops
--- PASS: TestRunnableSelectGatesStaleNamedProjections (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/literal_star_named_column_dropped_blocks (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/literal_COUNT_star_named_column_dropped_blocks (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/synthetic_wildcard_survives_when_literal_star_named_column_drops (Ns)
    --- PASS: TestRunnableSelectGatesStaleNamedProjections/synthetic_COUNT_star_sentinel_survives_when_literal_COUNT_star_named_column_drops (Ns)
PASS
```

## Enter gating: no request, no history, focus Column(s)

The Bubble Tea test suite in internal/ui/runnable_feedback_test.go drives Enter through Update on a model seeded with a stale named Value or aggregate projection. Enter starts no request (cmd is nil), opens no popup or value prompt, appends no query or result history, and focuses Column(s) with the authoritative reason verbatim in both the inline field content and the rendered view. The same holds when Enter is pressed from the Limit field with invalid nonempty Limit text — the earlier-field bypass moves focus to Column(s) instead of reopening the Limit value prompt. The control cases prove the established pre-execution handoff is unchanged: wildcard, synthetic COUNT(*), current named Value, and current named Min aggregate projections all emit PreExecutionRequestedMsg with no overlay opened (timing stripped):

```bash
go test ./internal/ui/ -run '^TestEnterOnStaleProjectionFocusesColumns$' -count=1 -v 2>&1 | grep -E 'RUN|PASS|FAIL' | sed 's/[0-9]\+\.[0-9]\+s/Ns/g'
```

```output
=== RUN   TestEnterOnStaleProjectionFocusesColumns
=== RUN   TestEnterOnStaleProjectionFocusesColumns/stale_named_Value_projection
=== RUN   TestEnterOnStaleProjectionFocusesColumns/stale_named_Min_aggregate_projection
=== RUN   TestEnterOnStaleProjectionFocusesColumns/stale_projection_from_Limit_field_focuses_Column(s)
=== RUN   TestEnterOnStaleProjectionFocusesColumns/wildcard_projection
=== RUN   TestEnterOnStaleProjectionFocusesColumns/COUNT(*)_sentinel_projection
=== RUN   TestEnterOnStaleProjectionFocusesColumns/current_named_Value_projection
=== RUN   TestEnterOnStaleProjectionFocusesColumns/current_named_Min_aggregate_projection
--- PASS: TestEnterOnStaleProjectionFocusesColumns (Ns)
    --- PASS: TestEnterOnStaleProjectionFocusesColumns/stale_named_Value_projection (Ns)
    --- PASS: TestEnterOnStaleProjectionFocusesColumns/stale_named_Min_aggregate_projection (Ns)
    --- PASS: TestEnterOnStaleProjectionFocusesColumns/stale_projection_from_Limit_field_focuses_Column(s) (Ns)
    --- PASS: TestEnterOnStaleProjectionFocusesColumns/wildcard_projection (Ns)
    --- PASS: TestEnterOnStaleProjectionFocusesColumns/COUNT(*)_sentinel_projection (Ns)
    --- PASS: TestEnterOnStaleProjectionFocusesColumns/current_named_Value_projection (Ns)
    --- PASS: TestEnterOnStaleProjectionFocusesColumns/current_named_Min_aggregate_projection (Ns)
PASS
```

## Full verification

The complete project verification — go vet, go test, go build — passes with no failures (timing stripped):

```bash
go vet ./... && go test ./... 2>&1 | sed 's/[0-9]\+\.[0-9]\+s/Ns/g' && go build ./... && echo ALL GREEN
```

```output
?   	github.com/chris/sqloid/Notes/walkthroughs/063-04/code-walkthrough	[no test files]
ok  	github.com/chris/sqloid/cmd/sqloid	(cached)
ok  	github.com/chris/sqloid/internal/cli	(cached)
ok  	github.com/chris/sqloid/internal/connection	(cached)
ok  	github.com/chris/sqloid/internal/d1	(cached)
ok  	github.com/chris/sqloid/internal/export	(cached)
ok  	github.com/chris/sqloid/internal/filepicker	(cached)
ok  	github.com/chris/sqloid/internal/history	(cached)
ok  	github.com/chris/sqloid/internal/querybuilder	(cached)
ok  	github.com/chris/sqloid/internal/result	(cached)
ok  	github.com/chris/sqloid/internal/resultcache	(cached)
ok  	github.com/chris/sqloid/internal/schema	(cached)
ok  	github.com/chris/sqloid/internal/session	(cached)
ok  	github.com/chris/sqloid/internal/ui	(cached)
ALL GREEN
```
