// Exact INSERT statement and parameter contract tests, per Issue #39 Task 3:
// Value columns appear in schema prompt order with `?` placeholders and exact
// universal bound types; NULL columns stay included with the SQL keyword and
// no parameter; Default/Omit columns are absent from both lists; all-omit
// emits exactly `INSERT INTO <quoted table> DEFAULT VALUES` with no
// parameters; unusual quoted names quote safely; parameter order follows the
// included Value choices only. SQL generation is pure over complete state —
// incomplete or zero-insertable-column state never renders.

package querybuilder

import (
	"github.com/chris/sqloid/internal/schema"
	"reflect"
	"testing"
)

// insertSQLCatalog backs the pure SQL tests: a mixed ordinary table with an
// INTEGER PRIMARY KEY, TEXT and REAL columns and unusual quoted names, plus
// a virtual table with a visible column.
func insertSQLCatalog() *schema.Catalog {
	return &schema.Catalog{
		Version: 11,
		Objects: []*schema.Object{
			{
				Name: "items", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
				Columns: []schema.Column{
					{Name: "id", DeclaredType: "INTEGER", Insertable: true, PrimaryKey: 1},
					{Name: "name", DeclaredType: "TEXT", Insertable: true},
					{Name: "score", DeclaredType: "REAL", Insertable: true},
				},
				InsertableCount: 3,
				PrimaryKeyCount: 1,
			},
			{
				Name: `tr "icky`, Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
				Columns: []schema.Column{
					{Name: `co "l`, Insertable: true},
					{Name: "plain", Insertable: true},
				},
				InsertableCount: 2,
			},
			{
				Name: "doc_fts", Kind: schema.KindVirtualTable, WriteEligible: true, Rowid: schema.RowidNotApplicable,
				Columns:         []schema.Column{{Name: "body", Insertable: true}},
				InsertableCount: 1,
			},
		},
	}
}

// insertSQLBuilder drives a fresh INSERT builder over items through the
// given per-column choices; valueText submits text for Value choices.
func insertSQLBuilder(t *testing.T, choices map[string]InsertChoice, values map[string]string) QueryBuilder {
	t.Helper()
	q := NewQuery().RefreshSchema(insertSQLCatalog()).
		SelectCommand(CommandInsert).SelectTable("items").BeginInsertPrompts()
	for _, c := range q.InsertColumns() {
		choice, ok := choices[c.Column]
		if !ok {
			// Prompt completeness requires every column chosen; unlisted
			// columns default to Default/Omit so each case tests only what it
			// names.
			choice = InsertChoiceOmit
		}
		next, ok := q.ChooseInsertColumn(c.Column, choice)
		if !ok {
			t.Fatalf("setup: choice %v on %q failed", choice, c.Column)
		}
		q = next
		if choice == InsertChoiceValue {
			text, ok := values[c.Column]
			if !ok {
				t.Fatalf("setup: no value text for %q", c.Column)
			}
			next, ok = q.SubmitInsertValue(c.Column, text)
			if !ok {
				t.Fatalf("setup: submit %q failed", c.Column)
			}
			q = next
		}
	}
	return q
}

// TestInsertSQLOrdersValuesNullsAndOmissions proves exact statement shape
// across single and mixed Value/NULL/omit choices, typed NULL TEXT versus
// SQL NULL, empty TEXT, and omissions from both lists in prompt order.
func TestInsertSQLOrdersValuesNullsAndOmissions(t *testing.T) {
	for _, tc := range []struct {
		name       string
		choices    map[string]InsertChoice
		values     map[string]string
		wantSQL    string
		wantParams []any
	}{
		{
			name:       "single Value",
			choices:    map[string]InsertChoice{"id": InsertChoiceValue},
			values:     map[string]string{"id": "42"},
			wantSQL:    `INSERT INTO "items" ("id") VALUES (?)`,
			wantParams: []any{int64(42)},
		},
		{
			name:       "mixed Value NULL omit keeps prompt order",
			choices:    map[string]InsertChoice{"score": InsertChoiceValue, "id": InsertChoiceNull, "name": InsertChoiceOmit},
			values:     map[string]string{"score": "9.5"},
			wantSQL:    `INSERT INTO "items" ("id", "score") VALUES (NULL, ?)`,
			wantParams: []any{9.5},
		},
		{
			name:       "empty Value is empty TEXT parameter",
			choices:    map[string]InsertChoice{"name": InsertChoiceValue},
			values:     map[string]string{"name": ""},
			wantSQL:    `INSERT INTO "items" ("name") VALUES (?)`,
			wantParams: []any{""},
		},
		{
			name:       "typed NULL stays bound TEXT",
			choices:    map[string]InsertChoice{"name": InsertChoiceValue},
			values:     map[string]string{"name": "NULL"},
			wantSQL:    `INSERT INTO "items" ("name") VALUES (?)`,
			wantParams: []any{"NULL"},
		},
		{
			name:       "explicit NULL binds nothing",
			choices:    map[string]InsertChoice{"name": InsertChoiceNull},
			wantSQL:    `INSERT INTO "items" ("name") VALUES (NULL)`,
			wantParams: nil,
		},
		{
			name:       "all omitted emits DEFAULT VALUES",
			choices:    map[string]InsertChoice{"id": InsertChoiceOmit, "name": InsertChoiceOmit, "score": InsertChoiceOmit},
			wantSQL:    `INSERT INTO "items" DEFAULT VALUES`,
			wantParams: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := insertSQLBuilder(t, tc.choices, tc.values)
			if got := q.InsertSQL(); got != tc.wantSQL {
				t.Fatalf("InsertSQL() = %q, want %q", got, tc.wantSQL)
			}
			got := q.InsertParams()
			if len(got) == 0 && len(tc.wantParams) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.wantParams) {
				t.Fatalf("InsertParams() = %#v, want %#v", got, tc.wantParams)
			}
		})
	}
}

// TestInsertSQLQuotesUnusualNames proves embedded double quotes in table and
// column identifiers quote safely atom-by-atom without altering parameters.
func TestInsertSQLQuotesUnusualNames(t *testing.T) {
	q := NewQuery().RefreshSchema(insertSQLCatalog()).
		SelectCommand(CommandInsert).SelectTable(`tr "icky`).BeginInsertPrompts()
	q, _ = q.ChooseInsertColumn(`co "l`, InsertChoiceValue)
	q, _ = q.SubmitInsertValue(`co "l`, "NULL")
	q, _ = q.ChooseInsertColumn("plain", InsertChoiceOmit)
	if got, want := q.InsertSQL(), `INSERT INTO "tr ""icky" ("co ""l") VALUES (?)`; got != want {
		t.Fatalf("InsertSQL() = %q, want %q", got, want)
	}
	if got := q.InsertParams(); !reflect.DeepEqual(got, []any{"NULL"}) {
		t.Fatalf("InsertParams() = %#v, want one bound TEXT NULL", got)
	}
}

// TestInsertSQLVirtualTableBestEffort proves a virtual table renders its
// visible insertable columns through the same normal path with no builder
// fabrication about module internals.
func TestInsertSQLVirtualTableBestEffort(t *testing.T) {
	q := NewQuery().RefreshSchema(insertSQLCatalog()).
		SelectCommand(CommandInsert).SelectTable("doc_fts").BeginInsertPrompts()
	q, _ = q.ChooseInsertColumn("body", InsertChoiceValue)
	q, _ = q.SubmitInsertValue("body", "hello")
	if got, want := q.InsertSQL(), `INSERT INTO "doc_fts" ("body") VALUES (?)`; got != want {
		t.Fatalf("InsertSQL() = %q, want %q", got, want)
	}
	if got := q.InsertParams(); !reflect.DeepEqual(got, []any{"hello"}) {
		t.Fatalf("InsertParams() = %#v", got)
	}
}

// TestInsertSQLRejectsIncompleteState proves incomplete choices, unsubmitted
// Value entries, and zero-insertable-column tables render nothing instead of
// partial SQL, per the authoritative runnable report.
func TestInsertSQLRejectsIncompleteState(t *testing.T) {
	incomplete := NewQuery().RefreshSchema(insertSQLCatalog()).
		SelectCommand(CommandInsert).SelectTable("items").BeginInsertPrompts()
	incomplete, _ = incomplete.ChooseInsertColumn("id", InsertChoiceValue)
	incomplete, _ = incomplete.SubmitInsertValue("id", "1")
	if got := incomplete.InsertSQL(); got != "" {
		t.Fatalf("incomplete state rendered %q", got)
	}
	zero := NewQuery().RefreshSchema(runnableCatalog()).
		SelectCommand(CommandInsert).SelectTable("blobs_only")
	if got := zero.InsertSQL(); got != "" {
		t.Fatalf("zero-insertable table rendered %q", got)
	}
	if got := zero.InsertParams(); got != nil {
		t.Fatalf("zero-insertable table params = %#v", got)
	}
}

// TestInsertOmissionHintDoesNotForceOmission proves the INTEGER PRIMARY KEY
// prompt retains all three choices: omission, Value, and NULL are all
// accepted with the hint present, so the hint never changes behavior.
func TestInsertOmissionHintDoesNotForceOmission(t *testing.T) {
	for _, choice := range []InsertChoice{InsertChoiceOmit, InsertChoiceValue, InsertChoiceNull} {
		q := NewQuery().RefreshSchema(insertSQLCatalog()).
			SelectCommand(CommandInsert).SelectTable("items").BeginInsertPrompts()
		if got, ok := q.InsertPromptHint("id"); !ok || got != schema.InsertOmissionHint {
			t.Fatalf("setup: id hint = (%q, %v)", got, ok)
		}
		next, ok := q.ChooseInsertColumn("id", choice)
		if !ok {
			t.Fatalf("hinted INTEGER PRIMARY KEY rejected choice %v", choice)
		}
		q = next
		if choice == InsertChoiceValue {
			q, _ = q.SubmitInsertValue("id", "7")
		}
		// Every prompted column must be complete; the companions stay omitted.
		q, _ = q.ChooseInsertColumn("name", InsertChoiceOmit)
		q, _ = q.ChooseInsertColumn("score", InsertChoiceOmit)
		if !q.RunnableReport().Runnable {
			t.Fatalf("choice %v on hinted column blocked runnable report", choice)
		}
	}
}
