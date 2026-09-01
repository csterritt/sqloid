# Issue #067 Code Walkthrough: Reject Stale INSERT Prompts Before SQL Rendering

*2026-09-01T23:33:43Z by Showboat 0.6.1*
<!-- showboat-id: eff3d419-2de5-4b04-b361-bc055d71190c -->

Issue #67 (Notes/tasks/067-reject-stale-insert-prompts.md, Notes/PRD-sqloid.md §Runnable-State Contract, §INSERT Query Grammar, §INSERT handling, §schema metadata/revalidation, §QueryBuilder Module Design, §Testing Decisions) strengthens the authoritative INSERT runnable report so every stored prompt column must still exist in the current InsertableColumns set. Before this issue, reportInsert checked only that the table had at least one insertable column and that every current insertable column had a complete prompt — a stored prompt whose column was dropped, hidden, generated, or otherwise became non-insertable after a schema refresh would slip through, because the completeness check iterated current columns and never noticed the orphaned prompt. Issue #67 adds a stale-prompt validation pass before the completeness checks: reportInsert builds the current insertable-column identity set from InsertableColumns() and iterates q.inserts in seeded schema order; the first stored prompt whose column is absent from that set returns RunFieldInsertColumns with the exact reason "the insert column no longer exists", regardless of the stale prompt's former choice or submitted value. Because InsertSQL and InsertParams (Issue #39 Task 4) were already gated on RunnableReport().Runnable before traversing stored prompts, stale INSERT state now emits no SQL and no parameters — no stale identifier is quoted and no former bound value escapes. Rejection is all-or-nothing through RunnableReport; the renderer never silently filters a stale prompt, quotes before validation, or duplicates insertability checks. Accepted prompts preserve schema order, Value/NULL/Default choices, parameter order, omission, and all-omit DEFAULT VALUES unchanged. This walkthrough builds completed INSERT prompts, refreshes each fixture so a prompted column is dropped, hidden, generated, or made non-insertable, shows the exact non-runnable INSERT-field feedback plus empty SQL and parameters, proves no stale identifier is quoted or bound, then demonstrates unchanged mixed Value/NULL/Default order, typed parameters, quoted names, and all-omit DEFAULT VALUES for current prompts.

## The stale-prompt validation in reportInsert

The authoritative INSERT report (internal/querybuilder/runnable.go) evaluates in visual order: the zero-insertable-column block, then the new stale-prompt validation, then the existing per-column choice/submission completeness checks. The stale check builds the current insertable-column identity set from InsertableColumns() and iterates every stored prompt in q.inserts in seeded schema order; the first prompt whose column is absent from the current set returns RunFieldInsertColumns with the exact reason. The check reuses InsertableColumns, prompt identity, and report patterns already used for stale SET/WHERE state — it never infers insertability from declared types, defaults, names, or old prompt metadata.

```bash
sed -n '72,110p' internal/querybuilder/runnable.go
```

```output
const (
	// ReasonNoCommand reports a builder with no selected command.
	ReasonNoCommand = "select a command"
	// ReasonNoTable reports a selected command without a table.
	ReasonNoTable = "select a table"
	// ReasonStaleTable reports a selected table absent from the refreshed
	// catalog snapshot.
	ReasonStaleTable = "the selected table no longer exists"
	// ReasonNoProjection reports a SELECT with no committed projection entry.
	ReasonNoProjection = "select at least one column"
	// ReasonStaleProjectionColumn reports a committed named projection whose
	// declared column no longer exists among the selected object's visible
	// columns after a refresh.
	ReasonStaleProjectionColumn = "the projected column no longer exists"
	// ReasonIncompletePrompt reports any open value prompt or incomplete
	// guided state: the common no-incomplete-value-prompt gate.
	ReasonIncompletePrompt = "complete the open value prompt"
	// ReasonStaleWhereColumn reports a committed WHERE naming a column that no
	// longer exists among the selected object's visible columns.
	ReasonStaleWhereColumn = "the where column no longer exists"
	// ReasonNoSetAssignments reports an UPDATE without any SET assignment.
	ReasonNoSetAssignments = "add at least one SET assignment"
	// ReasonDuplicateSetColumns reports an UPDATE whose SET columns repeat.
	ReasonDuplicateSetColumns = "SET columns must be unique"
	// ReasonStaleSetColumn reports an UPDATE assignment whose column is absent
	// from the selected table's refreshed visible columns.
	ReasonStaleSetColumn = "the SET column no longer exists"
	// ReasonNoInsertableColumns reports an INSERT onto a table with zero
	// insertable columns; the exact PRD wording.
	ReasonNoInsertableColumns = "table has no insertable columns"
	// ReasonStaleInsertColumn reports a stored INSERT prompt whose column is
	// absent from the selected table's current InsertableColumns set —
	// dropped, hidden, generated, or otherwise no longer insertable — while
	// the table remains eligible.
	ReasonStaleInsertColumn = "the insert column no longer exists"
	// ReasonIncompleteChoiceFmt reports an UPDATE SET assignment or INSERT
	// column whose {Value, NULL[, Default/Omit]} choice is still pending; %s
	// is the declared column name.
	ReasonIncompleteChoiceFmt = "complete the choice for column %s"
```

```bash
sed -n '237,275p' internal/querybuilder/runnable.go
```

```output
	}
	r, _ := q.reportWhere()
	return r
}

// reportInsert evaluates an INSERT in visual order: the zero-insertable-column
// block, then every stored prompt validated against the current
// InsertableColumns set, then every per-column completeness check. A stored
// prompt whose column is dropped, hidden, generated, or otherwise no longer
// insertable blocks with the specific stale-column reason before any
// completeness check on current prompts, regardless of its former choice or
// submitted value. All-omit is valid; a missing prompt state is an incomplete
// choice, so prompts must be begun for runnable data.
func (q QueryBuilder) reportInsert() RunnableReport {
	insertable := q.InsertableColumns()
	if len(insertable) == 0 {
		return RunnableReport{Field: RunFieldInsertColumns, Reason: ReasonNoInsertableColumns}
	}
	current := make(map[string]bool, len(insertable))
	for _, col := range insertable {
		current[col.Name] = true
	}
	for _, c := range q.inserts {
		if !current[c.Column] {
			return RunnableReport{Field: RunFieldInsertColumns, Reason: ReasonStaleInsertColumn}
		}
	}
	for _, col := range insertable {
		c, found := q.insertPrompt(col.Name)
		if !found || c.choice == InsertChoiceNone {
			return RunnableReport{Field: RunFieldInsertColumns,
				Reason: fmt.Sprintf(ReasonIncompleteChoiceFmt, col.Name)}
		}
		if c.choice == InsertChoiceValue && !c.submitted {
			return RunnableReport{Field: RunFieldInsertColumns,
				Reason: fmt.Sprintf(ReasonUnsubmittedValueFmt, col.Name)}
		}
	}
	return RunnableReport{Runnable: true}
```

## Stale INSERT prompts: non-runnable report with the exact reason

The Task 1 tests (internal/querybuilder/runnable_test.go TestRunnableInsertGatesStalePrompts) build completed INSERT prompts over the items fixture (id as submitted Value, name as NULL, score as Default/Omit), then refresh to catalogs where a prompted column is dropped, hidden, generated, or made non-insertable while the table remains eligible. Every stale case requires Runnable=false, RunFieldInsertColumns, and the exact reason "the insert column no longer exists" — regardless of whether the stale prompt's former choice was Value, NULL, or Default/Omit. Multiple stale prompts report the first in schema order deterministically, and the stale check fires before completeness checks on current prompts. Controls prove every current prompt set remains runnable.

```bash
go test ./internal/querybuilder/ -run 'TestRunnableInsertGatesStalePrompts' -count=1 -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL|ok)' | sed 's/[0-9]\+\.[0-9]\+s/Ns/g'
```

```output
=== RUN   TestRunnableInsertGatesStalePrompts
=== RUN   TestRunnableInsertGatesStalePrompts/dropped_name_column_blocks_stale_NULL_prompt
=== RUN   TestRunnableInsertGatesStalePrompts/dropped_score_column_blocks_stale_Omit_prompt
=== RUN   TestRunnableInsertGatesStalePrompts/dropped_id_column_blocks_stale_Value_prompt
=== RUN   TestRunnableInsertGatesStalePrompts/hidden_name_column_blocks_stale_prompt
=== RUN   TestRunnableInsertGatesStalePrompts/generated_name_column_blocks_stale_prompt
=== RUN   TestRunnableInsertGatesStalePrompts/non-insertable_name_column_blocks_stale_prompt
=== RUN   TestRunnableInsertGatesStalePrompts/multiple_stale_prompts_report_the_first_in_schema_order
=== RUN   TestRunnableInsertGatesStalePrompts/stale_prompt_blocks_before_incomplete_current_prompt
=== RUN   TestRunnableInsertGatesStalePrompts/all_current_prompts_remain_runnable_after_same_refresh
=== RUN   TestRunnableInsertGatesStalePrompts/all-omit_current_prompts_remain_runnable_after_same_refresh
=== RUN   TestRunnableInsertGatesStalePrompts/prompts_over_a_surviving_subset_remain_runnable
--- PASS: TestRunnableInsertGatesStalePrompts (Ns)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	Ns
```

```bash
go test ./internal/querybuilder/ -run 'TestInsertPromptStaleColumnBlocksReport' -count=1 -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL|ok)' | sed 's/[0-9]\+\.[0-9]\+s/Ns/g'
```

```output
=== RUN   TestInsertPromptStaleColumnBlocksReport
=== RUN   TestInsertPromptStaleColumnBlocksReport/dropped_note_column_blocks_stale_NULL_prompt
=== RUN   TestInsertPromptStaleColumnBlocksReport/hidden_note_column_blocks_stale_prompt
=== RUN   TestInsertPromptStaleColumnBlocksReport/generated_note_column_blocks_stale_prompt
=== RUN   TestInsertPromptStaleColumnBlocksReport/all_current_prompts_remain_runnable_after_same_refresh
--- PASS: TestInsertPromptStaleColumnBlocksReport (Ns)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	Ns
```

The focused prompt tests (internal/querybuilder/insert_prompt_test.go TestInsertPromptStaleColumnBlocksReport) exercise the same stale conditions through the `we \"ird` quoting fixture — a stored prompt on the note column (chosen NULL) goes stale when note is dropped, hidden, or generated, while the unusual quoted identifier `i \"\"d` and qty stay current. The control proves all current prompts remain runnable after a same-catalog refresh.

## Report-gated INSERT rendering: no stale identifier quoted or bound

InsertSQL and InsertParams (internal/querybuilder/insert_sql.go) were already gated on RunnableReport().Runnable before traversing stored prompts — the same all-or-nothing pattern Issue #66 applied to the SELECT renderer family. The strengthened report makes stale INSERT state emit nothing: no stale identifier is quoted and no former bound value escapes. The renderer never silently filters a stale prompt and renders the remainder, never quotes before validation, and never duplicates insertability checks.

```bash
sed -n '1,60p' internal/querybuilder/insert_sql.go
```

```output
// Pure INSERT statement generation (Issue #39 Task 4), per the INSERT
// handling decision in Notes/PRD-sqloid.md. The complete prompt state is
// traversed in authoritative schema prompt order: Value columns contribute a
// `?` placeholder and one bound parameter each; NULL columns stay in both
// lists as the SQL keyword NULL with no parameter; Default/Omit columns are
// absent from both lists. When every prompted column is omitted the exact
// `INSERT INTO <quoted table> DEFAULT VALUES` form is emitted with no
// parameters and no empty parentheses. Incomplete, zero-insertable-column,
// or stale-prompt state (Issue #67: a stored prompt whose column is dropped,
// hidden, generated, or otherwise no longer insertable) renders nothing: the
// authoritative runnable report gates both functions before any stored prompt
// is traversed, so no partial SQL, stale identifier, or former bound value
// ever escapes. Identifier quoting reuses Issue #14's atom-by-atom quoting
// and parameters reuse the universal Value binding; no hidden-input
// synthesis, default inference, or AUTOINCREMENT special-casing exists.

package querybuilder

// InsertSQL renders the complete INSERT statement with safely quoted table
// and column atoms, ordered columns, and per-choice values. It returns empty
// unless the authoritative runnable report accepts the state.
func (q QueryBuilder) InsertSQL() string {
	if q.command != CommandInsert || !q.RunnableReport().Runnable {
		return ""
	}
	columns := make([]string, 0, len(q.inserts))
	values := make([]string, 0, len(q.inserts))
	for _, c := range q.inserts {
		switch c.choice {
		case InsertChoiceValue:
			columns = append(columns, quoteIdentifierAtom(c.Column))
			values = append(values, "?")
		case InsertChoiceNull:
			columns = append(columns, quoteIdentifierAtom(c.Column))
			values = append(values, "NULL")
		default:
			// Default/Omit: absent from both lists entirely.
		}
	}
	if len(columns) == 0 {
		return "INSERT INTO " + quoteIdentifierAtom(q.table) + " DEFAULT VALUES"
	}
	return "INSERT INTO " + quoteIdentifierAtom(q.table) +
		" (" + joinSQLList(columns) + ") VALUES (" + joinSQLList(values) + ")"
}

// InsertParams returns fresh bound parameters in placeholder order: the
// submitted Value choices in schema prompt order, skipping SQL NULL and
// Default/Omit columns. It returns nil when nothing binds or the INSERT
// state is not runnable.
func (q QueryBuilder) InsertParams() []any {
	if q.command != CommandInsert || !q.RunnableReport().Runnable {
		return nil
	}
	var params []any
	for _, c := range q.inserts {
		if value, ok := c.SubmittedValue(); ok {
			params = append(params, value.ParamValue())
		}
	}
```

The Task 3 renderer tests (internal/querybuilder/insert_sql_test.go) assert the report rejects the stale state first, then require InsertSQL to return an empty string and InsertParams to return nil — so no stale identifier or former bound value escapes. The cases cover dropped, hidden, generated, non-insertable, multiple stale prompts, and a stale quoted identifier (`co \"l` on the `tr \"icky` table).

```bash
go test ./internal/querybuilder/ -run 'TestInsertSQLRejectsStalePrompts' -count=1 -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL|ok)' | sed 's/[0-9]\+\.[0-9]\+s/Ns/g'
```

```output
=== RUN   TestInsertSQLRejectsStalePrompts
=== RUN   TestInsertSQLRejectsStalePrompts/dropped_name_column_renders_nothing
=== RUN   TestInsertSQLRejectsStalePrompts/dropped_score_column_renders_nothing
=== RUN   TestInsertSQLRejectsStalePrompts/hidden_name_column_renders_nothing
=== RUN   TestInsertSQLRejectsStalePrompts/generated_name_column_renders_nothing
=== RUN   TestInsertSQLRejectsStalePrompts/non-insertable_name_column_renders_nothing
=== RUN   TestInsertSQLRejectsStalePrompts/multiple_stale_prompts_render_nothing
=== RUN   TestInsertSQLRejectsStalePrompts/stale_quoted_identifier_renders_nothing
--- PASS: TestInsertSQLRejectsStalePrompts (Ns)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	Ns
```

## Current prompts: unchanged Value/NULL/Default order, typed parameters, quoting, and DEFAULT VALUES

When every stored prompt corresponds to a current insertable column and is complete, the renderer is unchanged. The Task 3 current-state controls (TestInsertSQLCurrentPromptsRenderUnchanged) lock ordered Value bindings with exact typed parameters (int64 INTEGER, string TEXT, float64 REAL in schema order), SQL NULL included with no parameter, Default/Omit excluded from both lists, unusual quoted identifiers (`tr \"icky`/`co \"l` with embedded doubled quotes), submitted empty TEXT binding the empty string, typed `NULL` staying bound TEXT (distinct from the SQL-NULL choice), and all-omit emitting exactly `INSERT INTO \"items\" DEFAULT VALUES` with no parameters.

```bash
go test ./internal/querybuilder/ -run 'TestInsertSQLCurrentPromptsRenderUnchanged' -count=1 -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL|ok)' | sed 's/[0-9]\+\.[0-9]\+s/Ns/g'
```

```output
=== RUN   TestInsertSQLCurrentPromptsRenderUnchanged
=== RUN   TestInsertSQLCurrentPromptsRenderUnchanged/ordered_Value_bindings_keep_schema_prompt_order
=== RUN   TestInsertSQLCurrentPromptsRenderUnchanged/SQL_NULL_stays_included_with_no_parameter
=== RUN   TestInsertSQLCurrentPromptsRenderUnchanged/Default/Omit_excludes_from_both_lists
=== RUN   TestInsertSQLCurrentPromptsRenderUnchanged/unusual_quoted_identifiers_quote_safely
=== RUN   TestInsertSQLCurrentPromptsRenderUnchanged/submitted_empty_TEXT_binds_empty_string
=== RUN   TestInsertSQLCurrentPromptsRenderUnchanged/typed_NULL_stays_bound_TEXT
=== RUN   TestInsertSQLCurrentPromptsRenderUnchanged/all-omit_emits_DEFAULT_VALUES_with_no_parameters
--- PASS: TestInsertSQLCurrentPromptsRenderUnchanged (Ns)
PASS
ok  	github.com/chris/sqloid/internal/querybuilder	Ns
```

## Full suite verification

The complete querybuilder test suite and the full module build pass after the Issue #67 changes, confirming no regression in the existing INSERT, SELECT, UPDATE, DELETE, grouping, ordering, limit, history, restoration, or renderer behavior.

```bash
go test ./internal/querybuilder/ -count=1 2>&1 && go build ./... 2>&1 && echo 'BUILD OK'
```

```output
ok  	github.com/chris/sqloid/internal/querybuilder	0.007s
BUILD OK
```

```bash
go test ./internal/querybuilder/ -count=1 2>&1 | sed 's/[0-9]\+\.[0-9]\+s/Ns/g' && go build ./... 2>&1 && echo 'BUILD OK'
```

```output
ok  	github.com/chris/sqloid/internal/querybuilder	Ns
BUILD OK
```
