// Scripted Bubble Tea coverage for Issue #41's estimate settlement and
// deliberate confirmation: settled success and settled failure retain the
// operation, table, standalone SQL, and any all-rows warning while showing
// the count or the error; Enter/y become equivalent exactly-once confirmation
// only after either settled outcome; preparation and write-execution
// identities stay distinct; stale and duplicate settlement messages never
// replace retained content, enable the wrong preparation, start an execution,
// or touch either history; and repeated Ctrl+W plus stale cancellation
// messages stay idempotent with exact opener restoration.
package ui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	qb "github.com/chris/sqloid/internal/querybuilder"
)

// settledPreparation opens a preparation on q and settles its estimate with
// the given result. It fails the test if any message between opening and
// settlement dispatches a command (no actual write may start during
// preparation).
func settledPreparation(t *testing.T, q qb.QueryBuilder, est *prepFakeEstimator, res EstimateResult) Model {
	t.Helper()
	m, cmd := openPreparation(t, q, est)
	if cmd == nil {
		t.Fatal("open preparation returned no estimate command")
	}
	msg := cmd()
	settled, ok := msg.(EstimateSettledMsg)
	if !ok {
		t.Fatalf("estimate command produced %T, want EstimateSettledMsg", msg)
	}
	if requests, _, _, _ := est.snapshot(); requests != 1 {
		t.Fatalf("estimate requests = %d, want exactly 1", requests)
	}
	next, cmd := m.Update(settled)
	if cmd != nil {
		t.Fatalf("settling the estimate dispatched %v; preparation must not start work", cmd)
	}
	nm := next.(Model)
	if !nm.prepOpen {
		t.Fatal("settled preparation closed before confirmation")
	}
	return nm
}

// confirmOnce drives one confirmation key on a freshly settled preparation
// and returns the emitted model plus the WriteConfirmedMsg carried by the
// single returned command.
func confirmOnce(t *testing.T, m Model, key tea.KeyMsg) (Model, WriteConfirmedMsg) {
	t.Helper()
	next, cmd := m.Update(key)
	if cmd == nil {
		t.Fatalf("%v on a settled preparation emitted no confirmation command", key)
	}
	msg := cmd()
	confirmed, ok := msg.(WriteConfirmedMsg)
	if !ok {
		t.Fatalf("confirmation command produced %T, want WriteConfirmedMsg", msg)
	}
	next, cmd = next.(Model).Update(confirmed)
	if cmd != nil {
		t.Fatalf("delivered WriteConfirmedMsg dispatched %v", cmd)
	}
	return next.(Model), confirmed
}

func TestSettledEstimatesEnableConfirmation(t *testing.T) {
	tests := []struct {
		name      string
		qb        qb.QueryBuilder
		operation string
		res       EstimateResult
		wantView  string
		wantSQL   string
	}{
		{
			name:      "qualified update settles successfully",
			qb:        prepUpdateQB(true),
			operation: "UPDATE",
			res:       EstimateResult{Total: 7},
			wantView:  "Estimated matching target rows: 7",
			wantSQL:   `UPDATE "users" SET "email" = 'new' WHERE "id" = 5`,
		},
		{
			name:      "unqualified update settles with a failure",
			qb:        prepUpdateQB(false),
			operation: "UPDATE",
			res:       EstimateResult{Err: errors.New("disk I/O error")},
			wantView:  "Estimate failed: disk I/O error",
			wantSQL:   `UPDATE "users" SET "email" = 'new'`,
		},
		{
			name:      "qualified delete settles successfully",
			qb:        prepDeleteQB(true),
			operation: "DELETE",
			res:       EstimateResult{Total: 0},
			wantView:  "Estimated matching target rows: 0",
			wantSQL:   `DELETE FROM "users" WHERE "id" = 5`,
		},
		{
			name:      "unqualified delete settles with a failure",
			qb:        prepDeleteQB(false),
			operation: "DELETE",
			res:       EstimateResult{Err: errors.New("attempt to write a readonly database")},
			wantView:  "Estimate failed: attempt to write a readonly database",
			wantSQL:   `DELETE FROM "users"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			est := &prepFakeEstimator{result: tt.res}
			m := settledPreparation(t, tt.qb, est, tt.res)

			view := m.View()
			if !strings.Contains(view, tt.wantView) {
				t.Errorf("settled view missing %q:\n%s", tt.wantView, view)
			}
			if !strings.Contains(view, tt.wantSQL) || !strings.Contains(view, "users") {
				t.Errorf("settled view lost operation/table/SQL:\n%s", view)
			}
			if !strings.Contains(view, "Enter/y confirms the write") {
				t.Errorf("settled %s view does not enable confirmation:\n%s", tt.operation, view)
			}

			// Enter and y are equivalent: each on a fresh settled model
			// emits exactly one command carrying the confirmed operation,
			// the retained rendered statement, and a fresh actual-write
			// execution identity monotonic across confirmations.
			var firstExecution uint64
			for _, key := range []tea.KeyMsg{
				{Type: tea.KeyEnter},
				{Type: tea.KeyRunes, Runes: []rune{'y'}},
			} {
				nm, confirmed := confirmOnce(t, m, key)
				if confirmed.Preparation != m.prepAttempt {
					t.Errorf("%v confirmation preparation = %d, want %d", key, confirmed.Preparation, m.prepAttempt)
				}
				if confirmed.Execution == 0 {
					t.Errorf("%v confirmation carried no execution identity", key)
				}
				if firstExecution != 0 && confirmed.Execution <= firstExecution {
					t.Errorf("%v execution identity %d is not fresh beyond %d", key, confirmed.Execution, firstExecution)
				}
				firstExecution = confirmed.Execution
				if confirmed.Operation != tt.operation {
					t.Errorf("%v confirmation operation = %q, want %q", key, confirmed.Operation, tt.operation)
				}
				if confirmed.SQL != tt.wantSQL {
					t.Errorf("%v confirmation SQL = %q, want %q", key, confirmed.SQL, tt.wantSQL)
				}
				if nm.prepOpen {
					t.Errorf("%v confirmation left the preparation open", key)
				}
				if nm.writeAttempt != 1 {
					t.Errorf("%v writeAttempt = %d, want exactly 1", key, nm.writeAttempt)
				}
				if nm.History == nil || nm.History.Len() != 1 || nm.ResultHistory.Len() != 0 {
					t.Errorf("%v confirmation did not append exactly one query-history entry and no result entry (history=%d, results=%d)", key, nm.History.Len(), nm.ResultHistory.Len())
				}
				if nm.History != nil {
					entries := nm.History.Entries()
					if state := entries[len(entries)-1].State; !state.Equal(nm.QB.HistoryState()) {
						t.Errorf("%v confirmation appended an incomplete query state", key)
					}
				}
			}
		})
	}
}

// assertNoConfirmedWrite fails the test when cmd carries a WriteConfirmedMsg.
func assertNoConfirmedWrite(t *testing.T, what string, cmd tea.Cmd) {
	t.Helper()
	if cmd != nil {
		if _, ok := cmd().(WriteConfirmedMsg); ok {
			t.Errorf("%s started a confirmed write", what)
		}
	}
}

func TestConfirmationRequiresSettledOutcome(t *testing.T) {
	est := &prepFakeEstimator{result: EstimateResult{Total: 1}}

	t.Run("pending", func(t *testing.T) {
		m, cmd := openPreparation(t, prepUpdateQB(true), est)
		cmd()
		for _, key := range []tea.KeyMsg{
			{Type: tea.KeyEnter},
			{Type: tea.KeyRunes, Runes: []rune{'y'}},
		} {
			next, c := m.Update(key)
			if c != nil {
				t.Errorf("pending %v emitted a confirmation command %v", key, c)
			}
			nm := next.(Model)
			if !nm.prepOpen || !nm.prepPending || nm.writeAttempt != 0 {
				t.Errorf("pending %v mutated preparation state", key)
			}
		}
	})

	t.Run("cancelling", func(t *testing.T) {
		m, cmd := openPreparation(t, prepUpdateQB(true), est)
		cmd()
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
		nm := next.(Model)
		for _, key := range []tea.KeyMsg{
			{Type: tea.KeyEnter},
			{Type: tea.KeyRunes, Runes: []rune{'y'}},
		} {
			next, c := nm.Update(key)
			if c != nil {
				t.Errorf("cancelling %v emitted a confirmation command %v", key, c)
			}
			cm := next.(Model)
			if !cm.prepOpen || !cm.prepCancelling || cm.writeAttempt != 0 {
				t.Errorf("cancelling %v mutated preparation state", key)
			}
		}
	})

	t.Run("dismissed", func(t *testing.T) {
		m, cmd := openPreparation(t, prepUpdateQB(true), est)
		cmd()
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		dm := next.(Model)
		// The dismissed preparation cannot confirm: any command on the
		// restored builder is the ordinary validation reopen path, never a
		// write, and no execution identity is allocated.
		for _, key := range []tea.KeyMsg{
			{Type: tea.KeyEnter},
			{Type: tea.KeyRunes, Runes: []rune{'y'}},
		} {
			next, cmd := dm.Update(key)
			assertNoConfirmedWrite(t, key.String(), cmd)
			if next.(Model).writeAttempt != 0 {
				t.Errorf("dismissed %v allocated an execution identity", key)
			}
		}
	})
}

func TestConfirmationIsExactlyOnce(t *testing.T) {
	est := &prepFakeEstimator{result: EstimateResult{Total: 3}}
	m := settledPreparation(t, prepDeleteQB(true), est, est.result)

	nm, confirmed := confirmOnce(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if nm.confirmedExecution != confirmed.Execution {
		t.Fatalf("confirmedExecution = %d, want %d", nm.confirmedExecution, confirmed.Execution)
	}

	// Every replay path after the first accepted key — repeated Enter, y,
	// duplicate and stale estimate settlement, and re-delivery of the same
	// WriteConfirmedMsg — must never start a second confirmed write.
	replays := []struct {
		name string
		msg  tea.Msg
	}{
		{"repeat enter", tea.KeyMsg{Type: tea.KeyEnter}},
		{"repeat y", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}},
		{"duplicate settlement", EstimateSettledMsg{Preparation: confirmed.Preparation, Result: EstimateResult{Total: 99}}},
		{"stale settlement", EstimateSettledMsg{Preparation: confirmed.Preparation + 5, Result: EstimateResult{Err: errors.New("late")}}},
		{"re-delivered confirmation", confirmed},
	}
	for _, rp := range replays {
		next, cmd := nm.Update(rp.msg)
		assertNoConfirmedWrite(t, rp.name, cmd)
		next, _ = next.(Model).Update(confirmed) // re-delivery after each replay, post-delivery
		nm = next.(Model)
		if nm.writeAttempt != 1 || nm.confirmedExecution != confirmed.Execution {
			t.Errorf("replay %q mutated writeAttempt/confirmedExecution = %d/%d", rp.name, nm.writeAttempt, nm.confirmedExecution)
		}
	}
	if nm.History == nil || nm.History.Len() != 1 || nm.ResultHistory.Len() != 0 {
		t.Errorf("replays did not retain exactly the one execution-start query entry and no result entry (history=%d, results=%d)", nm.History.Len(), nm.ResultHistory.Len())
	}
}

func TestSettlementMessagesCannotReplaceRetainedContent(t *testing.T) {
	est := &prepFakeEstimator{result: EstimateResult{Total: 7}}
	m := settledPreparation(t, prepUpdateQB(false), est, est.result)
	identity := m.prepAttempt

	// A duplicate success with a different total must not replace the
	// retained estimate; a stale failure must not either.
	next, cmd := m.Update(EstimateSettledMsg{Preparation: identity, Result: EstimateResult{Total: 42}})
	if cmd != nil {
		t.Fatal("duplicate settlement dispatched a command")
	}
	next, cmd = next.(Model).Update(EstimateSettledMsg{Preparation: identity + 1, Result: EstimateResult{Err: errors.New("stale failure")}})
	if cmd != nil {
		t.Fatal("stale settlement dispatched a command")
	}
	nm := next.(Model)
	if view := nm.View(); !strings.Contains(view, "Estimated matching target rows: 7") || strings.Contains(view, "Estimate failed") {
		t.Errorf("retained estimate was replaced:\n%s", view)
	}
	if !strings.Contains(nm.View(), `UPDATE "users" SET "email" = 'new'`) || !strings.Contains(nm.View(), "every row") {
		t.Error("retained SQL or all-rows warning was lost")
	}
	if nm.History.Len() != 0 || nm.ResultHistory.Len() != 0 {
		t.Error("settlement messages mutated history")
	}

	// A settled failure is equally guarded: a late success cannot replace it.
	estFail := &prepFakeEstimator{result: EstimateResult{Err: errors.New("locked")}}
	fm := settledPreparation(t, prepDeleteQB(false), estFail, estFail.result)
	next, cmd = fm.Update(EstimateSettledMsg{Preparation: fm.prepAttempt, Result: EstimateResult{Total: 99}})
	if cmd != nil {
		t.Fatal("duplicate settlement dispatched a command")
	}
	if view := next.(Model).View(); !strings.Contains(view, "Estimate failed: locked") || strings.Contains(view, "Estimated matching target rows: 99") {
		t.Errorf("retained failure was replaced:\n%s", view)
	}
}

func TestRepeatedCtrlWIsIdempotentAndStaleCancellationsInert(t *testing.T) {
	est := &prepFakeEstimator{result: EstimateResult{Total: 4}}
	m, estimateCmd := openPreparation(t, prepUpdateQB(true), est)
	estimateCmd()

	next, cancelCmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	if cancelCmd == nil {
		t.Fatal("first Ctrl+W dispatched no cancellation command")
	}
	nm := next.(Model)
	if !nm.prepCancelling {
		t.Fatal("first Ctrl+W did not enter the cancelling state")
	}

	// Repeated Ctrl+W while cancelling is a no-op: no second cancellation.
	next, again := nm.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	if again != nil {
		t.Fatalf("repeated Ctrl+W dispatched a second cancellation command %v", again)
	}
	cm := next.(Model)
	if !cm.prepCancelling || !strings.Contains(cm.View(), "cancelling…") {
		t.Fatal("repeated Ctrl+W disturbed the cancelling state")
	}

	// A cancellation message replay, and a late estimate for a superseded
	// identity, mutate nothing while the current preparation still waits.
	next, _ = cm.Update(CancelEstimateMsg{})
	cm = next.(Model)
	if !cm.prepOpen || !cm.prepCancelling {
		t.Fatal("stale cancellation message closed the current preparation")
	}
	next, _ = cm.Update(EstimateSettledMsg{Preparation: cm.prepAttempt + 1, Result: EstimateResult{Total: 4}})
	cm = next.(Model)
	if !cm.prepOpen || !cm.prepCancelling {
		t.Fatal("late stale estimate mutated the current preparation")
	}

	// The single original request settles; preparation dismisses without
	// history, and the late estimate for the old identity cannot re-open it.
	cancelCmd() // release the context cancellation observed by the fake
	settledMsg := EstimateSettledMsg{Preparation: nm.prepAttempt, Result: EstimateResult{Cancelled: true}}
	next, _ = cm.Update(settledMsg)
	dm := next.(Model)
	if dm.prepOpen || dm.History.Len() != 0 || dm.ResultHistory.Len() != 0 {
		t.Fatal("cancelled settlement did not dismiss without history")
	}
	next, _ = dm.Update(EstimateSettledMsg{Preparation: nm.prepAttempt, Result: EstimateResult{Total: 4}})
	if next.(Model).prepOpen || next.(Model).writeAttempt != 0 {
		t.Fatal("late success for the cancelled preparation created an execution")
	}
}

func TestDismissalRestoresExactOpenerState(t *testing.T) {
	for _, tc := range []struct {
		name    string
		dismiss func(t *testing.T, m Model, estimateCmd tea.Cmd) Model
	}{
		{"esc", func(t *testing.T, m Model, _ tea.Cmd) Model {
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
			return next.(Model)
		}},
		{"n", func(t *testing.T, m Model, _ tea.Cmd) Model {
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
			return next.(Model)
		}},
		{"cancel then settle", func(t *testing.T, m Model, estimateCmd tea.Cmd) Model {
			next, cancelCmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
			if cancelCmd == nil {
				t.Fatal("Ctrl+W dispatched no cancellation command")
			}
			cancelCmd()
			msg := estimateCmd()
			settled, ok := msg.(EstimateSettledMsg)
			if !ok {
				t.Fatalf("late estimate produced %T", msg)
			}
			settled.Result = EstimateResult{Total: 5} // late success must lose to cancellation
			next, _ = next.(Model).Update(settled)
			return next.(Model)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			test := &prepFakeEstimator{result: EstimateResult{Total: 5}}
			// Capture the exact opener before the modal opens; the dismissal
			// must restore this captured state, not a reconstructed base.
			opener := prepModel(prepUpdateQB(true), test)
			openerFocus, openerScroll, openerQB := opener.Focus, opener.Scroll, opener.QB
			m, estimateCmd := openPreparation(t, prepUpdateQB(true), test)
			estimateCmd()

			dm := tc.dismiss(t, m, estimateCmd)
			if dm.prepOpen || dm.writeAttempt != 0 {
				t.Fatalf("%s dismissal left preparation open or allocated an execution", tc.name)
			}
			if dm.Focus != openerFocus {
				t.Errorf("%s dismissal focus = %d, want opener %d", tc.name, dm.Focus, openerFocus)
			}
			if dm.Scroll != openerScroll {
				t.Errorf("%s dismissal scroll = %d, want opener %d", tc.name, dm.Scroll, openerScroll)
			}
			if !reflect.DeepEqual(dm.QB, openerQB) {
				t.Errorf("%s dismissal did not restore the exact opener builder state", tc.name)
			}
			if dm.History.Len() != 0 || dm.ResultHistory.Len() != 0 {
				t.Errorf("%s dismissal appended query or result history", tc.name)
			}
		})
	}
}
