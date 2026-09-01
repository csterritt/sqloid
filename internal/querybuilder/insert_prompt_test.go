// INSERT prompt-plan contract tests, per Issue #39 Tasks 1–2: one prompt per
// visible insertable column in schema order, hidden and generated columns
// excluded regardless of declared type or defaults, exactly the closed
// {Value, NULL, Default/Omit} choice set per prompt, the INTEGER PRIMARY KEY
// omission hint exactly where schema metadata says it applies, and exact
// zero-insertable-column blocking with no prompt plan. These tests stay
// UI-independent: they exercise only QueryBuilder state and the runnable
// report, never SQL generation or popups.

package querybuilder

import (
	"reflect"
	"testing"

	"github.com/chris/sqloid/internal/schema"
)

// insertPlanCatalog is the synthetic fixture: a mixed ordinary table with
// unusual quoted names, hidden and generated columns; an all-generated
// table with zero insertable columns; and a virtual table whose only
// declared column is a hidden module input.
func insertPlanCatalog() *schema.Catalog {
	return &schema.Catalog{
		Version: 4,
		Objects: []*schema.Object{
			{
				Name: `we "ird`, Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
				Columns: []schema.Column{
					{Name: `i ""d`, DeclaredType: "INTEGER", Insertable: true, PrimaryKey: 1},
					{Name: "note", DeclaredType: "TEXT", Insertable: true},
					{Name: "qty", DeclaredType: "INTEGER", Insertable: true},
					{Name: "gen", DeclaredType: "INTEGER", Hidden: true},
					{Name: "secret", DeclaredType: "TEXT DEFAULT 'x'", Hidden: true},
				},
				InsertableCount: 3,
			},
			{
				Name: "only_gen", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
				Columns:         []schema.Column{{Name: "g", DeclaredType: "INTEGER", Hidden: true}},
				InsertableCount: 0,
			},
			{
				Name: "doc_fts", Kind: schema.KindVirtualTable, WriteEligible: true, Rowid: schema.RowidNotApplicable,
				Columns:         []schema.Column{{Name: "body", Hidden: true}},
				InsertableCount: 0,
			},
		},
	}
}

func insertPlanBuilder() QueryBuilder {
	return NewQuery().RefreshSchema(insertPlanCatalog()).
		SelectCommand(CommandInsert).SelectTable(`we "ird`).BeginInsertPrompts()
}

// TestInsertPromptPlanFollowsSchemaOrderAndExclusion proves every visible
// insertable column prompts exactly once in schema order and no hidden or
// generated column ever gains a prompt — no AUTOINCREMENT-based, nullable-
// based, or default-based skip exists.
func TestInsertPromptPlanFollowsSchemaOrderAndExclusion(t *testing.T) {
	q := insertPlanBuilder()
	var got []string
	for _, c := range q.InsertColumns() {
		if c.Choice() != InsertChoiceNone {
			t.Fatalf("fresh prompt %q chose %v, want unchosen", c.Column, c.Choice())
		}
		got = append(got, c.Column)
	}
	want := []string{`i ""d`, "note", "qty"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prompt plan = %q, want %q", got, want)
	}

	// The hidden generated and hidden module-style columns are excluded
	// regardless of declared type or defaults.
	for _, excluded := range []string{"gen", "secret"} {
		if _, found := q.insertPrompt(excluded); found {
			t.Fatalf("excluded column %q gained a prompt", excluded)
		}
	}

	// Virtual-table hidden inputs prompt nothing.
	vq := NewQuery().RefreshSchema(insertPlanCatalog()).
		SelectCommand(CommandInsert).SelectTable("doc_fts").BeginInsertPrompts()
	if got := vq.InsertColumns(); len(got) != 0 {
		t.Fatalf("virtual-table prompts = %#v, want none", got)
	}
}

// TestInsertPromptChoicesAreExactlyTheClosedSet proves each prompt accepts
// exactly Value, NULL, and Default/Omit in deterministic order with no
// declared-type filtering: every choice is accepted on every column,
// including the INTEGER PRIMARY KEY and untyped columns.
func TestInsertPromptChoicesAreExactlyTheClosedSet(t *testing.T) {
	q := insertPlanBuilder()
	for _, column := range []string{`i ""d`, "note", "qty"} {
		for _, choice := range []InsertChoice{InsertChoiceValue, InsertChoiceNull, InsertChoiceOmit} {
			next, ok := insertPlanBuilder().ChooseInsertColumn(column, choice)
			if !ok {
				t.Fatalf("choice %v on column %q rejected", choice, column)
			}
			c, found := next.insertPrompt(column)
			if !found || c.Choice() != choice {
				t.Fatalf("column %q choice = %v (found %v), want %v", column, c.Choice(), found, choice)
			}
		}
		if _, ok := q.ChooseInsertColumn(column, InsertChoice(99)); ok {
			t.Fatalf("out-of-range choice accepted on column %q", column)
		}
	}
}

// TestInsertPromptHintExactScope proves the exact hint
// "(auto-assigned if omitted)" attaches only to the single-column INTEGER
// PRIMARY KEY rowid alias prompt; similar declared types, non-primary
// INTEGER columns, and hidden columns never receive it, and the hint never
// pre-selects omission.
func TestInsertPromptHintExactScope(t *testing.T) {
	q := NewQuery().RefreshSchema(schema.BuildCatalog(schema.Input{
		Version: 1,
		Master: []schema.MasterRow{
			{Name: "plain", Type: "table", SQL: "CREATE TABLE plain (id INTEGER PRIMARY KEY, n INTEGER, s INT)"},
		},
		Columns: map[string][]schema.ColumnRow{
			"plain": {
				{Name: "id", DeclaredType: "INTEGER", PrimaryKey: 1},
				{Name: "n", DeclaredType: "INTEGER"},
				{Name: "s", DeclaredType: "INT"},
			},
		},
	})).SelectCommand(CommandInsert).SelectTable("plain").BeginInsertPrompts()

	if got, ok := q.InsertPromptHint("id"); !ok || got != schema.InsertOmissionHint {
		t.Fatalf("id hint = (%q, %v), want exactly %q", got, ok, schema.InsertOmissionHint)
	}
	for _, column := range []string{"n", "s"} {
		if got, ok := q.InsertPromptHint(column); ok {
			t.Fatalf("column %q hint = %q, want none", column, got)
		}
	}
	if q.InsertColumns()[0].Choice() != InsertChoiceNone {
		t.Fatalf("hint changed the unchosen state: %#v", q.InsertColumns()[0])
	}
}

// insertPlanCatalogDropsNote returns a refreshed insertPlanCatalog whose
// `we "ird` table drops the note column, keeping `i ""d` and qty insertable,
// so a stored prompt naming note goes stale while the table stays eligible.
func insertPlanCatalogDropsNote() *schema.Catalog {
	return &schema.Catalog{
		Version: 5,
		Objects: []*schema.Object{
			{
				Name: `we "ird`, Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
				Columns: []schema.Column{
					{Name: `i ""d`, DeclaredType: "INTEGER", Insertable: true, PrimaryKey: 1},
					{Name: "qty", DeclaredType: "INTEGER", Insertable: true},
					{Name: "gen", DeclaredType: "INTEGER", Hidden: true},
					{Name: "secret", DeclaredType: "TEXT DEFAULT 'x'", Hidden: true},
				},
				InsertableCount: 2,
			},
			{
				Name: "only_gen", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
				Columns:         []schema.Column{{Name: "g", DeclaredType: "INTEGER", Hidden: true}},
				InsertableCount: 0,
			},
			{
				Name: "doc_fts", Kind: schema.KindVirtualTable, WriteEligible: true, Rowid: schema.RowidNotApplicable,
				Columns:         []schema.Column{{Name: "body", Hidden: true}},
				InsertableCount: 0,
			},
		},
	}
}

// insertPlanCatalogHidesNote returns a refreshed insertPlanCatalog whose
// `we "ird` table marks note as a hidden module-style column (Hidden=true),
// so it remains declared but is no longer insertable while `i ""d` and qty
// stay insertable.
func insertPlanCatalogHidesNote() *schema.Catalog {
	return &schema.Catalog{
		Version: 5,
		Objects: []*schema.Object{
			{
				Name: `we "ird`, Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
				Columns: []schema.Column{
					{Name: `i ""d`, DeclaredType: "INTEGER", Insertable: true, PrimaryKey: 1},
					{Name: "note", DeclaredType: "TEXT", Hidden: true},
					{Name: "qty", DeclaredType: "INTEGER", Insertable: true},
					{Name: "gen", DeclaredType: "INTEGER", Hidden: true},
					{Name: "secret", DeclaredType: "TEXT DEFAULT 'x'", Hidden: true},
				},
				InsertableCount: 2,
			},
			{
				Name: "only_gen", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
				Columns:         []schema.Column{{Name: "g", DeclaredType: "INTEGER", Hidden: true}},
				InsertableCount: 0,
			},
			{
				Name: "doc_fts", Kind: schema.KindVirtualTable, WriteEligible: true, Rowid: schema.RowidNotApplicable,
				Columns:         []schema.Column{{Name: "body", Hidden: true}},
				InsertableCount: 0,
			},
		},
	}
}

// insertPlanCatalogGeneratedNote returns a refreshed insertPlanCatalog whose
// `we "ird` table marks note as a generated column (Hidden=true with a
// GENERATED declared type), so it remains declared but is no longer
// insertable while `i ""d` and qty stay insertable.
func insertPlanCatalogGeneratedNote() *schema.Catalog {
	return &schema.Catalog{
		Version: 5,
		Objects: []*schema.Object{
			{
				Name: `we "ird`, Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
				Columns: []schema.Column{
					{Name: `i ""d`, DeclaredType: "INTEGER", Insertable: true, PrimaryKey: 1},
					{Name: "note", DeclaredType: "TEXT GENERATED ALWAYS AS (`i \"\"d` || 'x') STORED", Hidden: true},
					{Name: "qty", DeclaredType: "INTEGER", Insertable: true},
					{Name: "gen", DeclaredType: "INTEGER", Hidden: true},
					{Name: "secret", DeclaredType: "TEXT DEFAULT 'x'", Hidden: true},
				},
				InsertableCount: 2,
			},
			{
				Name: "only_gen", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
				Columns:         []schema.Column{{Name: "g", DeclaredType: "INTEGER", Hidden: true}},
				InsertableCount: 0,
			},
			{
				Name: "doc_fts", Kind: schema.KindVirtualTable, WriteEligible: true, Rowid: schema.RowidNotApplicable,
				Columns:         []schema.Column{{Name: "body", Hidden: true}},
				InsertableCount: 0,
			},
		},
	}
}

// insertPlanCompleteMixed completes every `we "ird` prompt with a mixed
// choice set: `i ""d` as a submitted Value, note as NULL, and qty as
// Default/Omit, driven only through the immutable INSERT transitions.
func insertPlanCompleteMixed(q QueryBuilder) QueryBuilder {
	next, ok := q.ChooseInsertColumn(`i ""d`, InsertChoiceValue)
	if !ok {
		panic("setup: ChooseInsertColumn failed")
	}
	next, _ = next.SubmitInsertValue(`i ""d`, "7")
	next, ok = next.ChooseInsertColumn("note", InsertChoiceNull)
	if !ok {
		panic("setup: ChooseInsertColumn failed")
	}
	next, ok = next.ChooseInsertColumn("qty", InsertChoiceOmit)
	if !ok {
		panic("setup: ChooseInsertColumn failed")
	}
	return next
}

// TestInsertPromptStaleColumnBlocksReport covers Issue #67 Task 1: a stored
// prompt whose column is dropped, hidden, generated, or otherwise no longer
// insertable blocks at RunFieldInsertColumns with the specific stale-column
// reason before completeness checks, regardless of the former choice. The
// `we "ird` fixture exercises unusual quoted identifiers alongside the
// stale conditions, and a control proves current prompts stay runnable.
func TestInsertPromptStaleColumnBlocksReport(t *testing.T) {
	cases := []struct {
		name  string
		build func() QueryBuilder
		want  RunnableReport
	}{
		{
			name: "dropped note column blocks stale NULL prompt",
			build: func() QueryBuilder {
				return insertPlanCompleteMixed(insertPlanBuilder()).RefreshSchema(insertPlanCatalogDropsNote())
			},
			want: RunnableReport{Field: RunFieldInsertColumns, Reason: ReasonStaleInsertColumn},
		},
		{
			name: "hidden note column blocks stale prompt",
			build: func() QueryBuilder {
				return insertPlanCompleteMixed(insertPlanBuilder()).RefreshSchema(insertPlanCatalogHidesNote())
			},
			want: RunnableReport{Field: RunFieldInsertColumns, Reason: ReasonStaleInsertColumn},
		},
		{
			name: "generated note column blocks stale prompt",
			build: func() QueryBuilder {
				return insertPlanCompleteMixed(insertPlanBuilder()).RefreshSchema(insertPlanCatalogGeneratedNote())
			},
			want: RunnableReport{Field: RunFieldInsertColumns, Reason: ReasonStaleInsertColumn},
		},
		{
			name: "all current prompts remain runnable after same refresh",
			build: func() QueryBuilder {
				return insertPlanCompleteMixed(insertPlanBuilder()).RefreshSchema(insertPlanCatalog())
			},
			want: RunnableReport{Runnable: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.build().RunnableReport()
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("%s: report = %+v, want %+v", tc.name, got, tc.want)
			}
		})
	}
}

// TestZeroInsertableColumnsBlockExactly proves a zero-insertable-column
// table yields the exact blocking reason "table has no insertable columns",
// no prompt plan, and a non-runnable report.
func TestZeroInsertableColumnsBlockExactly(t *testing.T) {
	for _, table := range []string{"only_gen", "doc_fts"} {
		q := NewQuery().RefreshSchema(insertPlanCatalog()).
			SelectCommand(CommandInsert).SelectTable(table)
		if got := q.InsertableColumns(); len(got) != 0 {
			t.Fatalf("%s insertable = %#v, want none", table, got)
		}
		q = q.BeginInsertPrompts()
		if got := q.InsertColumns(); len(got) != 0 {
			t.Fatalf("%s prompts = %#v, want no prompt plan", table, got)
		}
		report := q.RunnableReport()
		if report.Runnable {
			t.Fatalf("%s reported runnable", table)
		}
		if report.Field != RunFieldInsertColumns || report.Reason != ReasonNoInsertableColumns {
			t.Fatalf("%s report = (%v, %q), want (insert columns, %q)", table, report.Field, report.Reason, ReasonNoInsertableColumns)
		}
	}
}
