// UI-independent table tests for the authoritative runnable report (Issue #19
// Task 1): every SELECT, UPDATE, DELETE, and INSERT prerequisite, the common
// gates, first-invalid ordering in each command's visual builder order, and
// one specific reason per report. The tests construct states only through
// QueryBuilder transitions plus the forward-compatible write-state seam
// (WithSetAssignments for duplicate SET columns); no UI focus, request state,
// or rendering is imported.

package querybuilder

import (
	"reflect"
	"testing"

	"github.com/chris/sqloid/internal/schema"
)

// runnableCatalog returns a snapshot resembling a small main schema with
// typed columns: an insertable ordinary table, a zero-insertable-column
// ordinary table, and a SELECT-only view.
func runnableCatalog() *schema.Catalog {
	return &schema.Catalog{
		Version: 19,
		Objects: []*schema.Object{
			{
				Name: "items", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
				Columns: []schema.Column{
					{Name: "id", DeclaredType: "INTEGER", Insertable: true},
					{Name: "name", DeclaredType: "TEXT", Insertable: true},
					{Name: "score", DeclaredType: "REAL", Insertable: true},
				},
				InsertableCount: 3,
			},
			{
				Name: "blobs_only", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
				Columns: []schema.Column{{Name: "data", DeclaredType: "BLOB", Hidden: true}},
			},
			{
				Name: "vw", Kind: schema.KindView, Rowid: schema.RowidNotApplicable,
				Columns: []schema.Column{{Name: "line"}},
			},
		},
	}
}

// itemsCatalogDropsScore returns a refreshed snapshot whose items table no
// longer declares the score column, to force stale committed identifiers.
func itemsCatalogDropsScore() *schema.Catalog {
	c := runnableCatalog()
	kept := make([]*schema.Object, 0, len(c.Objects))
	for _, o := range c.Objects {
		if o.Name == "items" {
			o = &schema.Object{
				Name: "items", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
				Columns:         []schema.Column{{Name: "id", DeclaredType: "INTEGER", Insertable: true}},
				InsertableCount: 1,
			}
		}
		kept = append(kept, o)
	}
	return &schema.Catalog{Version: 20, Objects: kept}
}

// assertReport compares one report against the expected outcome.
func assertReport(t *testing.T, name string, got, want RunnableReport) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: report = %+v, want %+v", name, got, want)
	}
}

// buildSelect drives a fresh builder to SELECT over items.
func buildSelect() QueryBuilder {
	return NewQuery().RefreshSchema(runnableCatalog()).SelectCommand(CommandSelect).SelectTable("items")
}

// buildUpdate drives a fresh builder to UPDATE over items.
func buildUpdate() QueryBuilder {
	return NewQuery().RefreshSchema(runnableCatalog()).SelectCommand(CommandUpdate).SelectTable("items")
}

// buildDelete drives a fresh builder to DELETE over items.
func buildDelete() QueryBuilder {
	return NewQuery().RefreshSchema(runnableCatalog()).SelectCommand(CommandDelete).SelectTable("items")
}

// buildInsert drives a fresh builder to INSERT over items with prompts begun.
func buildInsert() QueryBuilder {
	return NewQuery().RefreshSchema(runnableCatalog()).SelectCommand(CommandInsert).SelectTable("items").
		BeginInsertPrompts()
}

// selectWildcard commits the sole wildcard projection entry.
func selectWildcard(q QueryBuilder) QueryBuilder {
	return q.AcceptProjection(ProjectionCandidate{Kind: ProjectionWildcard}).Builder
}

// buildPlainSelect drives a fresh builder to SELECT over items with the plain
// named column "name" committed (no aggregates), for stale-identifier tests.
func buildPlainSelect() QueryBuilder {
	out := buildSelect().AcceptProjection(ProjectionCandidate{Kind: ProjectionColumn, Column: "name"})
	return out.Builder.CompleteProjectionAggregate("name", AggregateValue).Builder
}

// whereCompleteEq completes a WHERE draft with an `=` submission.
func whereCompleteEq(q QueryBuilder, text string) QueryBuilder {
	next, ok := q.StartWhere("name")
	if !ok {
		panic("setup: StartWhere failed")
	}
	draft, ok := next.WhereDraft().ChooseOperator(OpEq)
	if !ok {
		panic("setup: ChooseOperator failed")
	}
	next = next.ApplyWhereDraft(draft)
	draft, ok = draft.SubmitValue(text)
	if !ok {
		panic("setup: SubmitValue failed")
	}
	next = next.ApplyWhereDraft(draft)
	next, ok = next.CommitWhereDraft()
	if !ok {
		panic("setup: CommitWhereDraft failed")
	}
	return next
}

// setSubmittedValue completes one SET assignment as a submitted Value.
func setSubmittedValue(q QueryBuilder, column, text string) QueryBuilder {
	next, ok := q.AcceptSetColumn(column)
	if !ok {
		panic("setup: AcceptSetColumn failed")
	}
	next, ok = next.ChooseSetAssignment(column, SetChoiceValue)
	if !ok {
		panic("setup: ChooseSetAssignment failed")
	}
	next, ok = next.SubmitSetValue(column, text)
	if !ok {
		panic("setup: SubmitSetValue failed")
	}
	return next
}

// insertChoiceAllOmit installs the Default/Omit choice on every prompt.
func insertChoiceAllOmit(q QueryBuilder) QueryBuilder {
	next := q
	for _, c := range q.InsertColumns() {
		var ok bool
		next, ok = next.ChooseInsertColumn(c.Column, InsertChoiceOmit)
		if !ok {
			panic("setup: ChooseInsertColumn failed")
		}
	}
	return next
}

// TestRunnableSelectCoversEveryPrerequisite walks the SELECT prerequisite
// matrix in visual order: missing command, missing table, empty projection,
// incomplete WHERE drafts, every grouping rule, stale identifiers, and every
// Limit state including the exact invalid reason.
func TestRunnableSelectCoversEveryPrerequisite(t *testing.T) {
	groupedPlain := selectWildcard(buildSelect()) // placeholder replaced below
	_ = groupedPlain

	cases := []struct {
		name  string
		build func() QueryBuilder
		want  RunnableReport
	}{
		{
			name:  "missing command",
			build: func() QueryBuilder { return NewQuery() },
			want:  RunnableReport{Field: RunFieldCommand, Reason: ReasonNoCommand},
		},
		{
			name: "command chosen but no table",
			build: func() QueryBuilder {
				return NewQuery().RefreshSchema(runnableCatalog()).SelectCommand(CommandSelect)
			},
			want: RunnableReport{Field: RunFieldTable, Reason: ReasonNoTable},
		},
		{
			name:  "empty projection blocks",
			build: buildSelect,
			want:  RunnableReport{Field: RunFieldProjection, Reason: ReasonNoProjection},
		},
		{
			name: "open WHERE draft at column choice blocks",
			build: func() QueryBuilder {
				next, _ := selectWildcard(buildSelect()).StartWhere("name")
				return next
			},
			want: RunnableReport{Field: RunFieldWhere, Reason: ReasonIncompletePrompt},
		},
		{
			name: "open WHERE draft awaiting value blocks",
			build: func() QueryBuilder {
				next, _ := selectWildcard(buildSelect()).StartWhere("name")
				draft, _ := next.WhereDraft().ChooseOperator(OpEq)
				return next.ApplyWhereDraft(draft)
			},
			want: RunnableReport{Field: RunFieldWhere, Reason: ReasonIncompletePrompt},
		},
		{
			name: "committed WHERE with submitted empty TEXT is complete",
			build: func() QueryBuilder {
				return whereCompleteEq(selectWildcard(buildSelect()), "")
			},
			want: RunnableReport{Runnable: true},
		},
		{
			name: "mixed aggregate/nonaggregate projection without GROUP BY blocks",
			build: func() QueryBuilder {
				q := buildSelect()
				out := q.AcceptProjection(ProjectionCandidate{Kind: ProjectionColumn, Column: "name"})
				out = out.Builder.CompleteProjectionAggregate("name", AggregateValue)
				out = out.Builder.AcceptProjection(ProjectionCandidate{Kind: ProjectionColumn, Column: "score"})
				out = out.Builder.CompleteProjectionAggregate("score", AggMin)
				return out.Builder
			},
			want: RunnableReport{Field: RunFieldGroupBy, Reason: MixedAggregationNeedsGroupReason},
		},
		{
			name: "wildcard beside GROUP BY blocks",
			build: func() QueryBuilder {
				next, ok := selectWildcard(buildSelect()).AcceptGroupColumn("name")
				if !ok {
					panic("setup: AcceptGroupColumn failed")
				}
				return next
			},
			want: RunnableReport{Field: RunFieldGroupBy, Reason: WildcardGroupedByReason},
		},
		{
			name: "stale grouped column blocks after refresh",
			build: func() QueryBuilder {
				next, ok := buildPlainSelect().AcceptGroupColumn("name")
				if !ok {
					panic("setup: AcceptGroupColumn failed")
				}
				return next.RefreshSchema(itemsCatalogDropsScore())
			},
			want: RunnableReport{Field: RunFieldGroupBy, Reason: StaleGroupColumnReason},
		},
		{
			name: "stale ORDER BY expression blocks after refresh",
			build: func() QueryBuilder {
				next, ok := buildPlainSelect().AcceptOrderBy("order-column:name")
				if !ok {
					panic("setup: AcceptOrderBy failed")
				}
				return next.RefreshSchema(itemsCatalogDropsScore())
			},
			want: RunnableReport{Field: RunFieldOrderBy, Reason: StaleOrderByExpressionReason},
		},
		{
			name: "empty Limit is the valid unbounded result",
			build: func() QueryBuilder {
				return selectWildcard(buildSelect()).SetLimitInput("")
			},
			want: RunnableReport{Runnable: true},
		},
		{
			name: "valid Limit is runnable",
			build: func() QueryBuilder {
				return selectWildcard(buildSelect()).SetLimitInput("5")
			},
			want: RunnableReport{Runnable: true},
		},
		{
			name: "invalid Limit reports the exact reason",
			build: func() QueryBuilder {
				return selectWildcard(buildSelect()).SetLimitInput("abc")
			},
			want: RunnableReport{Field: RunFieldLimit, Reason: LimitInvalidReason},
		},
		{
			name: "zero Limit reports the exact reason",
			build: func() QueryBuilder {
				return selectWildcard(buildSelect()).SetLimitInput("0")
			},
			want: RunnableReport{Field: RunFieldLimit, Reason: LimitInvalidReason},
		},
		{
			name: "overflow Limit reports the exact reason",
			build: func() QueryBuilder {
				return selectWildcard(buildSelect()).SetLimitInput("9223372036854775808")
			},
			want: RunnableReport{Field: RunFieldLimit, Reason: LimitInvalidReason},
		},
		{
			name: "fully valid SELECT is runnable",
			build: func() QueryBuilder {
				return whereCompleteEq(selectWildcard(buildSelect()).SetLimitInput("10"), "x")
			},
			want: RunnableReport{Runnable: true},
		},
		{
			name: "multiple simultaneous failures return only the first visual field",
			build: func() QueryBuilder {
				next, _ := buildSelect().StartWhere("name")
				return next.SetLimitInput("abc") // empty projection + draft + invalid limit
			},
			want: RunnableReport{Field: RunFieldProjection, Reason: ReasonNoProjection},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertReport(t, tc.name, tc.build().RunnableReport(), tc.want)
		})
	}
}

// TestRunnableUpdateCoversEveryPrerequisite walks the forward-compatible
// UPDATE contract: no assignments, duplicate SET columns, incomplete
// Value/NULL choices, unsubmitted Value entries, and the optional WHERE.
func TestRunnableUpdateCoversEveryPrerequisite(t *testing.T) {
	cases := []struct {
		name  string
		build func() QueryBuilder
		want  RunnableReport
	}{
		{
			name:  "no SET assignments blocks",
			build: buildUpdate,
			want:  RunnableReport{Field: RunFieldSetAssignments, Reason: ReasonNoSetAssignments},
		},
		{
			name: "duplicate SET columns block",
			build: func() QueryBuilder {
				dup := []SetAssignment{
					{Column: "name", choice: SetChoiceValue, value: ParseValue("x"), input: "x", submitted: true},
					{Column: "name", choice: SetChoiceValue, value: ParseValue("y"), input: "y", submitted: true},
				}
				return buildUpdate().WithSetAssignments(dup)
			},
			want: RunnableReport{Field: RunFieldSetAssignments, Reason: ReasonDuplicateSetColumns},
		},
		{
			name: "incomplete Value/NULL choice blocks with the column name",
			build: func() QueryBuilder {
				next, _ := buildUpdate().AcceptSetColumn("name")
				return next
			},
			want: RunnableReport{Field: RunFieldSetAssignments, Reason: "complete the choice for column name"},
		},
		{
			name: "unsubmitted Value entry blocks with the column name",
			build: func() QueryBuilder {
				next, ok := buildUpdate().AcceptSetColumn("name")
				if !ok {
					panic("setup: AcceptSetColumn failed")
				}
				next, _ = next.ChooseSetAssignment("name", SetChoiceValue)
				return next
			},
			want: RunnableReport{Field: RunFieldSetAssignments, Reason: "submit a value for column name"},
		},
		{
			name: "NULL choice is complete and distinct from a typed TEXT NULL",
			build: func() QueryBuilder {
				next, ok := buildUpdate().AcceptSetColumn("name")
				if !ok {
					panic("setup: AcceptSetColumn failed")
				}
				next, _ = next.ChooseSetAssignment("name", SetChoiceNull)
				return next
			},
			want: RunnableReport{Runnable: true},
		},
		{
			name: "typed TEXT NULL submission stays TEXT, not the SQL-NULL choice",
			build: func() QueryBuilder {
				q := setSubmittedValue(buildUpdate(), "name", "NULL")
				if _, ok := q.SetAssignments()[0].SubmittedValue(); !ok {
					panic("setup: submitted TEXT NULL vanished")
				}
				return q
			},
			want: RunnableReport{Runnable: true},
		},
		{
			name: "open WHERE draft blocks behind complete assignments",
			build: func() QueryBuilder {
				next, _ := setSubmittedValue(buildUpdate(), "name", "x").StartWhere("name")
				return next
			},
			want: RunnableReport{Field: RunFieldWhere, Reason: ReasonIncompletePrompt},
		},
		{
			name: "multiple failures return the first visual field (SET before WHERE)",
			build: func() QueryBuilder {
				next, _ := buildUpdate().StartWhere("name")
				return next
			},
			want: RunnableReport{Field: RunFieldSetAssignments, Reason: ReasonNoSetAssignments},
		},
		{
			name: "fully valid UPDATE with submitted Value is runnable",
			build: func() QueryBuilder {
				return whereCompleteEq(setSubmittedValue(buildUpdate(), "name", "x"), "y")
			},
			want: RunnableReport{Runnable: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertReport(t, tc.name, tc.build().RunnableReport(), tc.want)
		})
	}
}

// TestRunnableDeleteCoversEveryPrerequisite walks the DELETE contract: an
// eligible table with absent, complete, and incomplete WHERE states.
func TestRunnableDeleteCoversEveryPrerequisite(t *testing.T) {
	cases := []struct {
		name  string
		build func() QueryBuilder
		want  RunnableReport
	}{
		{
			name: "command without table blocks",
			build: func() QueryBuilder {
				return NewQuery().RefreshSchema(runnableCatalog()).SelectCommand(CommandDelete)
			},
			want: RunnableReport{Field: RunFieldTable, Reason: ReasonNoTable},
		},
		{
			name:  "eligible table without WHERE is runnable",
			build: buildDelete,
			want:  RunnableReport{Runnable: true},
		},
		{
			name:  "complete WHERE is runnable",
			build: func() QueryBuilder { return whereCompleteEq(buildDelete(), "x") },
			want:  RunnableReport{Runnable: true},
		},
		{
			name: "incomplete WHERE draft blocks",
			build: func() QueryBuilder {
				next, _ := buildDelete().StartWhere("name")
				return next
			},
			want: RunnableReport{Field: RunFieldWhere, Reason: ReasonIncompletePrompt},
		},
		{
			name: "IS NOT NULL completes without a value prompt",
			build: func() QueryBuilder {
				next, _ := buildDelete().StartWhere("name")
				draft, ok := next.WhereDraft().ChooseOperator(OpIsNotNull)
				if !ok {
					panic("setup: ChooseOperator failed")
				}
				next = next.ApplyWhereDraft(draft)
				next, ok = next.CommitWhereDraft()
				if !ok {
					panic("setup: CommitWhereDraft failed")
				}
				return next
			},
			want: RunnableReport{Runnable: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertReport(t, tc.name, tc.build().RunnableReport(), tc.want)
		})
	}
}

// TestRunnableInsertCoversEveryPrerequisite walks the INSERT contract: zero
// insertable columns, incomplete per-column choices, unsubmitted Value
// entries, and the valid all-omit state.
func TestRunnableInsertCoversEveryPrerequisite(t *testing.T) {
	cases := []struct {
		name  string
		build func() QueryBuilder
		want  RunnableReport
	}{
		{
			name: "zero insertable columns block with the exact wording",
			build: func() QueryBuilder {
				return NewQuery().RefreshSchema(runnableCatalog()).
					SelectCommand(CommandInsert).SelectTable("blobs_only")
			},
			want: RunnableReport{Field: RunFieldInsertColumns, Reason: ReasonNoInsertableColumns},
		},
		{
			name: "view is never an INSERT target",
			build: func() QueryBuilder {
				return NewQuery().RefreshSchema(runnableCatalog()).
					SelectCommand(CommandInsert).SelectTable("vw")
			},
			want: RunnableReport{Field: RunFieldTable, Reason: ReasonNoTable},
		},
		{
			name: "unbegun prompts block on the first insertable column",
			build: func() QueryBuilder {
				return NewQuery().RefreshSchema(runnableCatalog()).SelectCommand(CommandInsert).SelectTable("items")
			},
			want: RunnableReport{Field: RunFieldInsertColumns, Reason: "complete the choice for column id"},
		},
		{
			name: "incomplete choice on a later column blocks with that column",
			build: func() QueryBuilder {
				next, ok := buildInsert().ChooseInsertColumn("id", InsertChoiceNull)
				if !ok {
					panic("setup: ChooseInsertColumn failed")
				}
				return next
			},
			want: RunnableReport{Field: RunFieldInsertColumns, Reason: "complete the choice for column name"},
		},
		{
			name: "unsubmitted Value entry blocks with the column name",
			build: func() QueryBuilder {
				next, _ := buildInsert().ChooseInsertColumn("id", InsertChoiceValue)
				return next
			},
			want: RunnableReport{Field: RunFieldInsertColumns, Reason: "submit a value for column id"},
		},
		{
			name: "submitted empty TEXT Value completes",
			build: func() QueryBuilder {
				next, ok := buildInsert().ChooseInsertColumn("id", InsertChoiceValue)
				if !ok {
					panic("setup: ChooseInsertColumn failed")
				}
				next, _ = next.SubmitInsertValue("id", "")
				next, ok = next.ChooseInsertColumn("name", InsertChoiceNull)
				if !ok {
					panic("setup: ChooseInsertColumn failed")
				}
				next, ok = next.ChooseInsertColumn("score", InsertChoiceOmit)
				if !ok {
					panic("setup: ChooseInsertColumn failed")
				}
				return next
			},
			want: RunnableReport{Runnable: true},
		},
		{
			name: "typed TEXT NULL submission stays TEXT, not the SQL-NULL choice",
			build: func() QueryBuilder {
				next, _ := buildInsert().ChooseInsertColumn("id", InsertChoiceValue)
				next, _ = next.SubmitInsertValue("id", "NULL")
				next, ok := next.ChooseInsertColumn("name", InsertChoiceNull)
				if !ok {
					panic("setup: ChooseInsertColumn failed")
				}
				next, ok = next.ChooseInsertColumn("score", InsertChoiceOmit)
				if !ok {
					panic("setup: ChooseInsertColumn failed")
				}
				for _, c := range next.InsertColumns() {
					if c.Column == "id" {
						if _, ok := c.SubmittedValue(); !ok {
							panic("setup: typed TEXT NULL vanished")
						}
					} else if c.Choice() != InsertChoiceNull && c.Choice() != InsertChoiceOmit {
						panic("setup: unexpected choice")
					}
				}
				return next
			},
			want: RunnableReport{Runnable: true},
		},
		{
			name:  "valid all-omit state is runnable",
			build: func() QueryBuilder { return insertChoiceAllOmit(buildInsert()) },
			want:  RunnableReport{Runnable: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertReport(t, tc.name, tc.build().RunnableReport(), tc.want)
		})
	}
}
