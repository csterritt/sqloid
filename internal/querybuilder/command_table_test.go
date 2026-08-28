// Pure table-driven tests for the UI-independent command and table selection
// lifecycle, per Issue #11 Tasks 1–2 and the Builder lifecycle decision in
// Notes/PRD-sqloid.md: initial unselected state, one-key S/U/D/I selection and
// replacement, downstream clearing, Schema-owned eligibility with ordinary and
// virtual tables as write candidates plus SELECT-only views, and view-to-write
// clearing that retains the refreshed eligible write-table list.
//
// These tests use only internal/schema object kinds and eligibility metadata;
// there is no Bubble Tea dependency and no SQL construction here.

package querybuilder

import (
	"testing"

	"github.com/chris/sqloid/internal/schema"
)

// fixtureCatalog returns a fixed snapshot resembling a small main schema:
// an ordinary table, a virtual table, and a SELECT-only view.
func fixtureCatalog() *schema.Catalog {
	return &schema.Catalog{
		Version: 7,
		Objects: []*schema.Object{
			{Name: "logs_fts", Kind: schema.KindVirtualTable, WriteEligible: true, Rowid: schema.RowidNotApplicable},
			{Name: "users", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas},
			{Name: "vw_summary", Kind: schema.KindView, WriteEligible: false, Rowid: schema.RowidNotApplicable},
		},
	}
}

func names(q QueryBuilder) []string {
	var out []string
	for _, o := range q.EligibleTables() {
		out = append(out, o.Name)
	}
	return out
}

func has(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// TestInitialQueryRequiresCommand pins the startup builder: no command is
// selected yet and the next required field is Command.
func TestInitialQueryRequiresCommand(t *testing.T) {
	q := NewQuery()
	if q.Command() != CommandUnselected {
		t.Errorf("initial command = %v, want %v", q.Command(), CommandUnselected)
	}
	if _, ok := q.SelectedTable(); ok {
		t.Error("initial state already has a selected table")
	}
	if q.Focus() != FieldCommand {
		t.Errorf("initial focus = %v, want %v", q.Focus(), FieldCommand)
	}
	if got := names(q); len(got) != 0 {
		t.Errorf("initial eligible objects = %v, want empty", got)
	}
}

// TestOneKeyCommandSelection covers each plain S/U/D/I selection from the
// initial state: the command becomes selected, focus advances to Table, and
// the source builder is never mutated.
func TestOneKeyCommandSelection(t *testing.T) {
	cases := []struct {
		cmd     Command
		display string
	}{
		{CommandSelect, "SELECT"},
		{CommandUpdate, "UPDATE"},
		{CommandDelete, "DELETE"},
		{CommandInsert, "INSERT"},
	}
	for _, tc := range cases {
		q := NewQuery().RefreshSchema(fixtureCatalog())
		next := q.SelectCommand(tc.cmd)
		if next.Command() != tc.cmd || next.Command().String() != tc.display {
			t.Errorf("after %v command = %v/%v, want %s", tc.cmd, next.Command(), next.Command().String(), tc.display)
		}
		if next.Focus() != FieldTable {
			t.Errorf("after %v focus = %v, want %v", tc.cmd, next.Focus(), FieldTable)
		}
		if q.Command() != CommandUnselected || q.Focus() != FieldCommand {
			t.Errorf("source mutated by %v: command=%v focus=%v", tc.cmd, q.Command(), q.Focus())
		}
	}
}

// TestReplacementRetainsOnlyEligibleTables verifies every one-key replacement:
// downstream state clears on each command change, and a selected object is
// retained only when it remains eligible for the new command under Schema
// metadata — views only survive SELECT, ordinary and virtual tables survive
// every command, and a name absent from the latest refresh never survives.
func TestReplacementRetainsOnlyEligibleTables(t *testing.T) {
	all := []Command{CommandSelect, CommandUpdate, CommandDelete, CommandInsert}
	eligibility := func(c Command) bool { return c == CommandSelect }
	for _, sel := range all {
		for _, rep := range all {
			if sel == rep {
				continue // plain first-time selection, covered above
			}
			viewSelected := eligibility(sel)
			next := NewQuery().
				RefreshSchema(fixtureCatalog()).
				SelectCommand(sel).
				SelectTable("vw_summary").
				SelectCommand(rep)
			wantRetained := viewSelected && eligibility(rep)
			name, ok := next.SelectedTable()
			if ok != wantRetained {
				t.Errorf("%v→%v retained (%q,%v), want retained=%v",
					sel, rep, name, ok, wantRetained)
			}
			if wantRetained && name != "vw_summary" {
				t.Errorf("%v→%v retained %q, want vw_summary", sel, rep, name)
			}
			if next.Focus() != FieldTable {
				t.Errorf("%v→%v focus = %v, want %v", sel, rep, next.Focus(), FieldTable)
			}
			wantViewInList := rep == CommandSelect
			if got := has(names(next), "vw_summary"); got != wantViewInList {
				t.Errorf("%v→%v eligible list contains view=%v, want %v",
					sel, rep, got, wantViewInList)
			}
		}
	}

	// A stale name missing from the refreshed catalog clears even though its
	// kind would otherwise remain eligible for every command.
	stale := &schema.Catalog{
		Version: 8,
		Objects: []*schema.Object{
			{Name: "other", Kind: schema.KindOrdinaryTable, WriteEligible: true},
		},
	}
	dropped := NewQuery().RefreshSchema(fixtureCatalog()).
		SelectCommand(CommandDelete).
		SelectTable("users").
		RefreshSchema(stale).
		SelectCommand(CommandUpdate)
	if _, ok := dropped.SelectedTable(); ok {
		t.Error("stale table survived refresh+replacement, want cleared")
	}
}

// TestReplacementClearsDownstreamState pins that choosing any command key
// discards all downstream command-specific state represented by the builder's
// downstream generation, even when the table itself is retained.
func TestReplacementClearsDownstreamState(t *testing.T) {
	before := NewQuery().
		RefreshSchema(fixtureCatalog()).
		SelectCommand(CommandUpdate).
		SelectTable("users")
	after := before.SelectCommand(CommandDelete)
	if after.DownstreamGeneration() != before.DownstreamGeneration()+1 {
		t.Errorf("replacement left downstream generation at %d (was %d): downstream state not cleared",
			after.DownstreamGeneration(), before.DownstreamGeneration())
	}
	unchanged := after.SelectTable("users")
	if unchanged.DownstreamGeneration() != after.DownstreamGeneration() {
		t.Error("table selection must not disturb downstream state")
	}
}

// TestViewToWriteClearingKeepsEligibleWriteTables pins the special transition:
// selecting a view then moving to UPDATE, DELETE, or INSERT clears Table,
// focuses Table, and still reports a populated eligible list containing the
// ordinary and virtual tables but never the view.
func TestViewToWriteClearingKeepsEligibleWriteTables(t *testing.T) {
	for _, cmd := range []Command{CommandUpdate, CommandDelete, CommandInsert} {
		next := NewQuery().
			RefreshSchema(fixtureCatalog()).
			SelectCommand(CommandSelect).
			SelectTable("vw_summary").
			SelectCommand(cmd)

		if _, ok := next.SelectedTable(); ok {
			t.Errorf("%s kept view selection, want cleared", cmd)
		}
		if next.Focus() != FieldTable {
			t.Errorf("%s focused %v, want %v", cmd, next.Focus(), FieldTable)
		}
		got := names(next)
		if len(got) != 2 || !has(got, "users") || !has(got, "logs_fts") {
			t.Errorf("%s eligible = %v, want exactly the ordinary+virtual write tables", cmd, got)
		}
	}
}

// TestEligibleTablesFollowSchemaMetadata checks the refreshed eligible-object
// list itself: SELECT admits every cataloged object including views, write
// commands admit only Schema-declared write-eligible kinds, and eligibility
// follows the latest refresh rather than stale metadata.
func TestEligibleTablesFollowSchemaMetadata(t *testing.T) {
	sel := NewQuery().RefreshSchema(fixtureCatalog()).SelectCommand(CommandSelect)
	gotSel := names(sel)
	for _, want := range []string{"vw_summary", "users", "logs_fts"} {
		if !has(gotSel, want) {
			t.Errorf("SELECT eligible = %v, misses %q", gotSel, want)
		}
	}

	for _, cmd := range []Command{CommandUpdate, CommandDelete, CommandInsert} {
		got := names(NewQuery().RefreshSchema(fixtureCatalog()).SelectCommand(cmd))
		if len(got) != 2 || has(got, "vw_summary") {
			t.Errorf("%s eligible = %v, want ordinary+virtual without view", cmd, got)
		}
	}

	// Refreshing replaces metadata wholesale: a non-write-eligible entry is
	// offered to SELECT but disappears from every write list.
	some := &schema.Catalog{
		Version: 9,
		Objects: []*schema.Object{
			{Name: "gone", Kind: schema.KindOrdinaryTable, WriteEligible: false},
		},
	}
	after := NewQuery().RefreshSchema(fixtureCatalog()).SelectCommand(CommandInsert).RefreshSchema(some)
	if got := names(after); len(got) != 0 {
		t.Errorf("write eligibility after refresh = %v, want empty", got)
	}
}

// TestSelectTableRejectsIneligibleNames guards the pure setter: only a name
// present in the current eligible list may be selected; anything else leaves
// the selection untouched.
func TestSelectTableRejectsIneligibleNames(t *testing.T) {
	q := NewQuery().RefreshSchema(fixtureCatalog()).SelectCommand(CommandInsert)

	rejected := q.SelectTable("vw_summary")
	if _, ok := rejected.SelectedTable(); ok {
		t.Error("view accepted as INSERT target")
	}
	rejected = q.SelectTable("missing")
	if _, ok := rejected.SelectedTable(); ok {
		t.Error("unknown name accepted")
	}
	if _, ok := q.SelectTable("").SelectedTable(); ok {
		t.Error("empty name accepted")
	}

	accepted := q.SelectTable("logs_fts")
	name, ok := accepted.SelectedTable()
	if !ok || name != "logs_fts" {
		t.Errorf("eligible select gave (%q,%v), want (logs_fts,true)", name, ok)
	}
	if accepted.Focus() != FieldTable {
		t.Errorf("table selection focused %v, want %v", accepted.Focus(), FieldTable)
	}
}
