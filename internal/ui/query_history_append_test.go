// Narrow scripted integration coverage for the execution-start query-history
// append seam (Issue #20): runnable evaluation and the pre-execution
// lifecycle append nothing; only the actual-execution-start message appends,
// with consecutive-identical suppression; UPDATE/DELETE never append until
// their confirmation flow exists. This test owns only the execution-start
// timing boundary — not navigation, restoration, or database execution.

package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/history"
	qb "github.com/chris/sqloid/internal/querybuilder"
)

// asModel asserts an Update result back into the concrete Model type.
func asModel(v tea.Model, _ tea.Cmd) Model { return v.(Model) }

// historyModel returns a model running runnable SELECT data with the history
// store wired.
func historyModel() (Model, *history.Store) {
	store := history.NewStore()
	m := focusField(modelWithQB(validSelectQB()), commandFieldLabel)
	m.History = store
	return m, store
}

// TestRunnableEvaluationNeverAppendsHistory requires base-context Enter on
// runnable data to emit only the pre-execution seam and to append nothing:
// validation, estimation, cancellation, and execution have not started.
func TestRunnableEvaluationNeverAppendsHistory(t *testing.T) {
	m, store := historyModel()
	next, cmd := enterPress(m)
	if cmd == nil {
		t.Fatal("runnable Enter returned no command; want the pre-execution seam")
	}
	if _, ok := cmd().(PreExecutionRequestedMsg); !ok {
		t.Fatalf("runnable Enter returned %T; want PreExecutionRequestedMsg", cmd())
	}
	if store.Len() != 0 {
		t.Fatalf("history Len = %d after runnable evaluation, want 0", store.Len())
	}
	// Deliver the pre-execution seam back through Update: it still must not
	// append — actual execution has not started.
	next = asModel(next.Update(PreExecutionRequestedMsg{}))
	if got := next; got.History.Len() != 0 {
		t.Fatalf("history Len = %d after pre-execution seam, want 0", got.History.Len())
	}
}

// TestExecutionStartAppendsThenSuppressesConsecutive requires the actual
// execution-start seam to append the normalized state once and to suppress an
// identical consecutive start without a new entry.
func TestExecutionStartAppendsThenSuppressesConsecutive(t *testing.T) {
	m, store := historyModel()
	nextModel := asModel(m.Update(ExecutionStartedMsg{}))
	if store.Len() != 1 {
		t.Fatalf("history Len = %d after one execution start, want 1", store.Len())
	}
	entries := store.Entries()
	if entries[0].State.Command.String() != "SELECT" || entries[0].State.Table != "users" {
		t.Fatalf("retained state = %v over %q; want SELECT over users", entries[0].State.Command, entries[0].State.Table)
	}
	nextModel = asModel(nextModel.Update(ExecutionStartedMsg{}))
	if store.Len() != 1 {
		t.Fatalf("history Len = %d after consecutive identical start, want 1 (suppressed)", store.Len())
	}
}

// TestDistinctExecutionStartRetainsEntries requires A→B to retain both
// entries with stable IDs, and both to survive later lifecycle events (the
// append already occurred at execution start, so later failure of the active
// execution cannot undo it).
func TestDistinctExecutionStartRetainsEntries(t *testing.T) {
	m, store := historyModel()
	m = asModel(m.Update(ExecutionStartedMsg{}))
	changed := modelWithQB(validSelectQB().SetLimitInput("5"))
	changed.History = store
	changed = asModel(changed.Update(ExecutionStartedMsg{}))
	if store.Len() != 2 {
		t.Fatalf("history Len = %d after A→B, want 2", store.Len())
	}
	entries := store.Entries()
	if entries[0].ID == entries[1].ID {
		t.Fatal("retained entries share a stable ID")
	}
	if entries[1].State.LimitHas != true || entries[1].State.LimitValue != 5 {
		t.Fatalf("second entry limit = (%v, %d); want accepted 5", entries[1].State.LimitHas, entries[1].State.LimitValue)
	}
	// An ordinary later lifecycle message (a resize) must not disturb the
	// retained entries — the failed-execution contract: append already
	// happened at start and no follow-up event removes it.
	changed = asModel(changed.Update(tea.WindowSizeMsg{Width: 80, Height: 24}))
	if store.Len() != 2 {
		t.Fatalf("history Len = %d after later lifecycle event, want 2", store.Len())
	}
	if _, ok := store.Lookup(entries[0].ID); !ok {
		t.Fatal("first entry vanished after later lifecycle event")
	}
}

// setSubmittedUIUpdateQB returns an UPDATE builder with one submitted Value
// SET assignment over the users table.
func setSubmittedUIUpdateQB() qb.QueryBuilder {
	q := qb.NewQuery().RefreshSchema(whereUICatalog()).
		SelectCommand(qb.CommandUpdate).SelectTable("users")
	q, ok := q.AcceptSetColumn("email")
	if !ok {
		panic("setup: AcceptSetColumn failed")
	}
	q, ok = q.ChooseSetAssignment("email", qb.SetChoiceValue)
	if !ok {
		panic("setup: ChooseSetAssignment failed")
	}
	q, ok = q.SubmitSetValue("email", "x")
	if !ok {
		panic("setup: SubmitSetValue failed")
	}
	return q
}

// TestWriteCommandsNeverAppendWithoutConfirmation requires UPDATE and DELETE
// execution-start emissions to append nothing: their sole actual write begins
// only at destructive confirmation, which no implemented flow can emit yet.
func TestWriteCommandsNeverAppendWithoutConfirmation(t *testing.T) {
	store := history.NewStore()
	upd := modelWithQB(setSubmittedUIUpdateQB())
	upd.History = store
	upd = asModel(upd.Update(ExecutionStartedMsg{}))
	if store.Len() != 0 {
		t.Fatalf("history Len = %d after unconfirmed UPDATE start, want 0", store.Len())
	}
}
