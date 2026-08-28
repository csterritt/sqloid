// Immutable transition tests for whole-value clearing (Issue #19 Task 3):
// completed WHERE values for every consumer, valid and invalid Limit text,
// and the forward-compatible UPDATE/INSERT Value fields. Both clearing paths
// must remove the entered representation, bound value, and submission marker
// atomically while preserving surrounding structural choices; already absent
// or empty whole fields are exact unchanged no-ops.

package querybuilder

import (
	"reflect"
	"testing"
)

// whereDeleteTable drives DELETE over items with a completed `=` predicate.
func whereDeleteTable(text string) QueryBuilder {
	return whereCompleteEq(buildDelete(), text)
}

// expectWhereDraftIntact asserts the cleared WHERE reopened its draft with the
// same column and operator but no submission.
func expectWhereDraftIntact(t *testing.T, name string, q QueryBuilder, colName string, op Operator) {
	t.Helper()
	if !q.WhereDrafting() {
		t.Fatalf("%s: clearing did not reopen the WHERE draft", name)
	}
	draft := q.WhereDraft()
	if draft.State() != WhereAwaitingValue {
		t.Errorf("%s: draft state = %v, want awaiting-value", name, draft.State())
	}
	if col, ok := draft.Column(); !ok || col.Name != colName {
		t.Errorf("%s: draft column = (%+v,%v), want %s preserved", name, col, ok, colName)
	}
	if got, ok := draft.ChosenOperator(); !ok || got != op {
		t.Errorf("%s: draft operator = (%v,%v), want %v preserved", name, got, ok, op)
	}
	if _, ok := draft.SubmittedValue(); ok {
		t.Errorf("%s: cleared draft still reports a submitted value", name)
	}
	if q.HasWhere() {
		t.Errorf("%s: cleared WHERE still committed", name)
	}
}

// TestClearWhereValueCoversEveryConsumer clears completed WHERE values for
// SELECT, UPDATE, and DELETE alike, keeping column/operator and reopening the
// incomplete draft.
func TestClearWhereValueCoversEveryConsumer(t *testing.T) {
	consumers := []struct {
		name  string
		build func() QueryBuilder
	}{
		{"select", func() QueryBuilder { return whereCompleteEq(selectWildcard(buildSelect()), "x") }},
		{"update", func() QueryBuilder { return whereCompleteEq(setSubmittedValue(buildUpdate(), "name", "x"), "x") }},
		{"delete", func() QueryBuilder { return whereDeleteTable("x") }},
	}
	for _, tc := range consumers {
		t.Run(tc.name, func(t *testing.T) {
			next := tc.build().ClearWhereValue()
			expectWhereDraftIntact(t, tc.name, next, "name", OpEq)
			if report := next.RunnableReport(); report.Runnable ||
				report.Field != RunFieldWhere || report.Reason != ReasonIncompletePrompt {
				t.Errorf("%s: cleared report = %+v, want blocked at WHERE with %q",
					tc.name, report, ReasonIncompletePrompt)
			}
		})
	}
}

// TestClearWhereValueNoOps pins every unchanged case: absent predicates,
// null-operator completions, and open drafts.
func TestClearWhereValueNoOps(t *testing.T) {
	cases := []struct {
		name  string
		build func() QueryBuilder
	}{
		{"absent committed predicate", buildDelete},
		{
			name: "committed null operator takes no value",
			build: func() QueryBuilder {
				// whereDeleteTable commits an `=` predicate; replace it with a
				// committed IS NULL through the draft flow.
				cleared := whereDeleteTable("x").ClearWhereValue()
				draft, ok := cleared.WhereDraft().ChooseOperator(OpIsNull)
				if !ok {
					panic("setup: ChooseOperator failed")
				}
				cleared = cleared.ApplyWhereDraft(draft)
				cleared, ok = cleared.CommitWhereDraft()
				if !ok {
					panic("setup: CommitWhereDraft failed")
				}
				return cleared
			},
		},
		{"open draft", func() QueryBuilder {
			next, _ := buildDelete().StartWhere("name")
			return next
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.build()
			after := before.ClearWhereValue()
			if !reflect.DeepEqual(before, after) {
				t.Errorf("%s: clearing changed state", tc.name)
			}
		})
	}
}

// TestClearLimitValueRestoresUnboundedValidity clears valid and invalid
// nonempty Limit text and verifies empty Limit is an exact no-op.
func TestClearLimitValueRestoresUnboundedValidity(t *testing.T) {
	for _, text := range []string{"5", "abc", "0"} {
		q := selectWildcard(buildSelect()).SetLimitInput(text)
		next := q.ClearLimitValue()
		if next.LimitInput() != "" {
			t.Errorf("clearing %q left entered text %q", text, next.LimitInput())
		}
		if _, ok := next.LimitValue(); ok {
			t.Errorf("clearing %q left an accepted value", text)
		}
		if report := next.RunnableReport(); !report.Runnable {
			t.Errorf("clearing %q report = %+v, want runnable unbounded", text, report)
		}
	}
	q := selectWildcard(buildSelect()).SetLimitInput("")
	if after := q.ClearLimitValue(); !reflect.DeepEqual(q, after) {
		t.Error("clearing an already empty Limit changed state")
	}
}

// TestClearSetValueKeepsChoiceButIncomplete clears a submitted UPDATE Value
// while keeping the Value choice; unsubmitted entries are no-ops.
func TestClearSetValueKeepsChoiceButIncomplete(t *testing.T) {
	q := setSubmittedValue(buildUpdate(), "name", "x")
	next := q.ClearSetValue("name")
	assignments := next.SetAssignments()
	if len(assignments) != 1 || assignments[0].Choice() != SetChoiceValue {
		t.Fatalf("cleared assignment = %+v, want Value choice preserved", assignments)
	}
	if _, ok := assignments[0].SubmittedValue(); ok {
		t.Error("cleared assignment still reports a submitted value")
	}
	if report := next.RunnableReport(); report.Runnable ||
		report.Reason != "submit a value for column name" {
		t.Errorf("cleared report = %+v, want unsubmitted Value block", report)
	}
	if _, ok := q.SetAssignments()[0].SubmittedValue(); !ok {
		t.Error("source snapshot mutated by clearing")
	}
	unchanged := setSubmittedValue(buildUpdate(), "name", "x").
		ClearSetValue("score") // no such assignment
	if !reflect.DeepEqual(unchanged, setSubmittedValue(buildUpdate(), "name", "x")) {
		t.Error("clearing an absent column changed state")
	}
	if empty := buildUpdate().ClearSetValue("name"); !reflect.DeepEqual(empty, buildUpdate()) {
		t.Error("clearing an unassigned column changed state")
	}
	if unsubmitted, ok := buildUpdate().AcceptSetColumn("name"); ok {
		if after := unsubmitted.ClearSetValue("name"); !reflect.DeepEqual(unsubmitted, after) {
			t.Error("clearing an unsubmitted Value changed state")
		}
	}
}

// TestClearInsertValueKeepsChoiceButIncomplete clears a submitted INSERT
// Value while keeping the Value choice; unsubmitted entries are no-ops.
func TestClearInsertValueKeepsChoiceButIncomplete(t *testing.T) {
	q, ok := buildInsert().ChooseInsertColumn("id", InsertChoiceValue)
	if !ok {
		panic("setup: ChooseInsertColumn failed")
	}
	q, ok = q.SubmitInsertValue("id", "1")
	if !ok {
		panic("setup: SubmitInsertValue failed")
	}
	next := q.ClearInsertValue("id")
	var col *InsertColumn
	for i := range next.InsertColumns() {
		if next.InsertColumns()[i].Column == "id" {
			col = &next.InsertColumns()[i]
		}
	}
	if col == nil || col.Choice() != InsertChoiceValue {
		t.Fatalf("cleared prompt = %+v, want Value choice preserved", col)
	}
	if _, has := col.SubmittedValue(); has {
		t.Error("cleared prompt still reports a submitted value")
	}
	if report := next.RunnableReport(); report.Runnable ||
		report.Reason != "submit a value for column id" {
		t.Errorf("cleared report = %+v, want unsubmitted Value block", report)
	}
	if after := buildInsert().ClearInsertValue("id"); !reflect.DeepEqual(after, buildInsert()) {
		t.Error("clearing an unsubmitted INSERT Value changed state")
	}
}
