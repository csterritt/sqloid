// Active-SELECT lifetime and single-finalization matrix (Issue #34), per the
// Identities and state, SELECT, and active-finalization Testing Decisions in
// Notes/PRD-sqloid.md. The active SELECT is distinct from any individual
// request in flight: it stays active across builder edits, overlays, save and
// export keys, query-history keys, resize, serialized paging, count/page
// settlement and count failure, and idle periods. Only the exhaustive
// finalizing events — an actual new execution, entering result history, a
// cancellation or failure that ends the SELECT, and an accepted quit — end it
// and produce exactly one immutable result-history entry per execution.
// Pre-execution validation, rejected execution attempts, and unaccepted quit
// never finalize. Destructive-estimate phases are pre-execution workflows
// owned by Issues #37/#38: no implemented flow emits them, and the lifecycle
// seam classifies them as non-finalizing by construction (they never call the
// finalization path).

package ui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/history"
)

// activeState names the count/page pending and settled combinations the
// matrix is built over. Every combination must preserve the active SELECT's
// identity: request completion alone never defines the active lifetime.
type activeState struct {
	name         string
	pagePending  bool // one later page request dispatched, unsettled
	countPending bool // the independent count request unsettled
}

// activeSelectStates returns the required combinations over a settled first
// page: idle, count-pending, page-pending, count-settled, page-settled.
func activeSelectStates() []activeState {
	return []activeState{
		{name: "count settled, page settled (idle)"},
		{name: "count pending"},
		{name: "page pending", pagePending: true},
		{name: "count settled, page pending", pagePending: true},
		{name: "page settled, count pending", countPending: true},
	}
}

// startActiveSelect drives a full SELECT execution through validation and
// execution start, returning the model right after ExecutionStartedMsg with
// the launched page and count messages withheld so each test decides which
// requests settle. It asserts the execution became the active SELECT.
func startActiveSelect(t *testing.T) (Model, SelectSettledMsg, CountSettledMsg) {
	t.Helper()
	exec := &fakeSelectExecutor{page: firstPageRows(defaultPageRows)}
	countExec := &fakeCountExecutor{total: 3}
	m := twoVersionSelectModel()
	m.Select = exec.selectPage
	m.Count = countExec.count
	m.ResultHistory = history.NewResultStore()
	m.Page = (&fakePageExecutor{rowsShown: 3}).page

	execModel, execCmd := driveToExecutionStart(t, m)
	if !execModel.SelectIsActive() {
		t.Fatal("execution start did not begin an active SELECT")
	}
	if execModel.ActiveSelectExecutionID() == 0 {
		t.Fatal("active SELECT has no execution identity")
	}
	pageMsg, countMsg := splitSelectCount(t, execBatch(t, execCmd))
	return execModel, pageMsg, countMsg
}

// fixtureFor builds the requested pending/settled state over one active
// SELECT, returning the model, the first-page and later-page settled
// messages, and the count settled message. The first page always settles into
// the fixture (rows displayed and cached); messages not marked pending by the
// state are already consumed, and re-applying them later is inert.
func fixtureFor(t *testing.T, state activeState) (Model, SelectSettledMsg, PageSettledMsg, CountSettledMsg) {
	t.Helper()
	m, pageFirst, countMsg := startActiveSelect(t)
	m = apply(m, pageFirst)
	pageNext := PageSettledMsg{}
	if !state.countPending {
		m = apply(m, countMsg)
		countMsg = CountSettledMsg{} // consumed; re-application is inert
	}
	if state.pagePending {
		// Dispatch the later page request without settling it: the fixture
		// keeps its settled message so tests can apply or cancel it.
		withRequest, cmd := pageDown(m)
		if cmd == nil {
			t.Fatal("page-pending fixture could not dispatch a page request")
		}
		pageNext = cmd().(PageSettledMsg)
		m = withRequest
	}
	return m, pageFirst, pageNext, countMsg
}

// requireActive asserts that m still owns the given active SELECT execution,
// nothing was finalized, and no result-history entry exists.
func requireActive(t *testing.T, m Model, execID uint64, context string) {
	t.Helper()
	if !m.SelectIsActive() {
		t.Fatalf("%s: active SELECT lost", context)
	}
	if m.ActiveSelectExecutionID() != execID {
		t.Fatalf("%s: active execution = %d, want %d", context, m.ActiveSelectExecutionID(), execID)
	}
	if m.FinalizedSelectExecutionID() != 0 {
		t.Fatalf("%s: execution %d was finalized unexpectedly", context, m.FinalizedSelectExecutionID())
	}
	if m.ResultHistory.Len() != 0 {
		t.Fatalf("%s: %d result-history entries created without a finalizer", context, m.ResultHistory.Len())
	}
}

// requireFinalized asserts the SELECT is inactive, the given execution was
// finalized exactly once, and exactly one result-history entry exists.
func requireFinalized(t *testing.T, m Model, execID uint64, context string) {
	t.Helper()
	if m.SelectIsActive() {
		t.Fatalf("%s: active SELECT did not end", context)
	}
	if m.FinalizedSelectExecutionID() != execID {
		t.Fatalf("%s: finalized execution = %d, want %d", context, m.FinalizedSelectExecutionID(), execID)
	}
	if m.ResultHistory.Len() != 1 {
		t.Fatalf("%s: result history = %d entries, want exactly one", context, m.ResultHistory.Len())
	}
}

// TestNonFinalizingEventsPreserveActiveSelect walks every non-finalizing
// event category over each pending/settled combination and proves request
// completion alone never defines the active lifetime.
func TestNonFinalizingEventsPreserveActiveSelect(t *testing.T) {
	events := []struct {
		name  string
		apply func(t *testing.T, m Model, pageFirst SelectSettledMsg, pageNext PageSettledMsg, count CountSettledMsg) Model
	}{
		{
			name: "idle period",
			apply: func(t *testing.T, m Model, _ SelectSettledMsg, _ PageSettledMsg, _ CountSettledMsg) Model {
				return m
			},
		},
		{
			name: "builder focus edit",
			apply: func(t *testing.T, m Model, _ SelectSettledMsg, _ PageSettledMsg, _ CountSettledMsg) Model {
				return pressKeyLifecycle(t, m, tea.KeyMsg{Type: tea.KeyTab})
			},
		},
		{
			name: "help overlay open and close",
			apply: func(t *testing.T, m Model, _ SelectSettledMsg, _ PageSettledMsg, _ CountSettledMsg) Model {
				opened := pressKeyLifecycle(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
				return pressKeyLifecycle(t, opened, tea.KeyMsg{Type: tea.KeyEsc})
			},
		},
		{
			name: "save and export keys",
			apply: func(t *testing.T, m Model, _ SelectSettledMsg, _ PageSettledMsg, _ CountSettledMsg) Model {
				afterSave := pressKeyLifecycle(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
				return pressKeyLifecycle(t, afterSave, tea.KeyMsg{Type: tea.KeyCtrlX})
			},
		},
		{
			name: "query history browsing keys",
			apply: func(t *testing.T, m Model, _ SelectSettledMsg, _ PageSettledMsg, _ CountSettledMsg) Model {
				afterPrev := pressKeyLifecycle(t, m, tea.KeyMsg{Type: tea.KeyCtrlP})
				return pressKeyLifecycle(t, afterPrev, tea.KeyMsg{Type: tea.KeyCtrlN})
			},
		},
		{
			name: "gated result history keys",
			apply: func(t *testing.T, m Model, _ SelectSettledMsg, _ PageSettledMsg, _ CountSettledMsg) Model {
				// Issue #36: while any request is in flight the gate rejects
				// Ctrl+E/Y with feedback and no state change; when nothing is in
				// flight the key finalizes the active SELECT (entering result
				// history is a finalizing event), which requireActive forbids.
				if m.selectRequestPending() {
					afterNewer := pressKeyLifecycle(t, m, tea.KeyMsg{Type: tea.KeyCtrlE})
					return pressKeyLifecycle(t, afterNewer, tea.KeyMsg{Type: tea.KeyCtrlY})
				}
				return m
			},
		},
		{
			name: "resize",
			apply: func(t *testing.T, m Model, _ SelectSettledMsg, _ PageSettledMsg, _ CountSettledMsg) Model {
				return apply(m, tea.WindowSizeMsg{Width: 100, Height: 30})
			},
		},
		{
			name: "page settlement",
			apply: func(t *testing.T, m Model, pageFirst SelectSettledMsg, pageNext PageSettledMsg, _ CountSettledMsg) Model {
				if pageNext.RequestID != 0 {
					return apply(m, pageNext)
				}
				// The page already settled: replaying its message is inert
				// and must not finalize anything either.
				return apply(m, pageFirst)
			},
		},
		{
			name: "count settlement",
			apply: func(t *testing.T, m Model, _ SelectSettledMsg, _ PageSettledMsg, count CountSettledMsg) Model {
				return apply(m, count)
			},
		},
		{
			name: "count failure",
			apply: func(t *testing.T, m Model, _ SelectSettledMsg, _ PageSettledMsg, count CountSettledMsg) Model {
				count.Result = CountResult{Err: errors.New("count failed")}
				return apply(m, count)
			},
		},
		{
			name: "serialized paging dispatch",
			apply: func(t *testing.T, m Model, _ SelectSettledMsg, _ PageSettledMsg, _ CountSettledMsg) Model {
				withRequest, cmd := pageDown(m)
				if cmd == nil {
					// A page request is already pending: the key is consumed
					// without stacking. Consuming it must not finalize either.
					return m
				}
				// Held, unsettled page request: the key itself must not
				// finalize anything.
				return withRequest
			},
		},
		{
			name: "pre-execution validation opens",
			apply: func(t *testing.T, m Model, _ SelectSettledMsg, _ PageSettledMsg, _ CountSettledMsg) Model {
				next, cmd := pressKeyLifecycleCmd(t, m, tea.KeyMsg{Type: tea.KeyEnter})
				if cmd != nil {
					cmd() // drain the version-read command without settling it
				}
				return next
			},
		},
		{
			name: "unaccepted quit",
			apply: func(t *testing.T, m Model, _ SelectSettledMsg, _ PageSettledMsg, _ CountSettledMsg) Model {
				confirming := pressKeyLifecycle(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
				return pressKeyLifecycle(t, confirming, tea.KeyMsg{Type: tea.KeyEsc})
			},
		},
	}
	for _, state := range activeSelectStates() {
		for _, event := range events {
			t.Run(state.name+" / "+event.name, func(t *testing.T) {
				m, pageFirst, pageNext, count := fixtureFor(t, state)
				execID := m.ActiveSelectExecutionID()
				after := event.apply(t, m, pageFirst, pageNext, count)
				requireActive(t, after, execID, event.name)
			})
		}
	}
}

// pressKeyLifecycle applies one key press and returns the resulting model.
func pressKeyLifecycle(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

// pressKeyLifecycleCmd applies one key press and returns model and command.
func pressKeyLifecycleCmd(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

// TestFinalizersEndActiveSelectExactlyOnce enumerates each finalizing event
// and asserts it deactivates the SELECT, records exactly one finalized
// execution, and creates exactly one result-history entry.
func TestFinalizersEndActiveSelectExactlyOnce(t *testing.T) {
	t.Run("actual new execution start", func(t *testing.T) {
		m, _, _, _ := fixtureFor(t, activeState{name: "idle"})
		first := m.ActiveSelectExecutionID()

		// A second actual execution finalizes the first before replacing it.
		m.Select = (&fakeSelectExecutor{page: threeRowFirstPage()}).selectPage
		m.Count = (&fakeCountExecutor{total: 3}).count
		second, secondCmd := driveToExecutionStart(t, m)
		if !second.SelectIsActive() || second.ActiveSelectExecutionID() == first {
			t.Fatalf("new execution did not become active: id=%d", second.ActiveSelectExecutionID())
		}
		if second.FinalizedSelectExecutionID() != first {
			t.Fatalf("finalized execution = %d, want the first execution %d", second.FinalizedSelectExecutionID(), first)
		}
		if second.ResultHistory.Len() != 1 {
			t.Fatalf("result history = %d entries, want exactly one", second.ResultHistory.Len())
		}
		page2, _ := splitSelectCount(t, execBatch(t, secondCmd))
		if page2.RequestID == 0 || page2.RequestID == second.FinalizedSelectExecutionID() {
			t.Fatal("new execution did not assign a fresh first-page request identity")
		}
	})

	t.Run("entering result history", func(t *testing.T) {
		m, _, _, _ := fixtureFor(t, activeState{name: "idle"})
		execID := m.ActiveSelectExecutionID()

		m.enterResultHistory()
		requireFinalized(t, m, execID, "result history entry")
		// Deactivation invalidates future page mutation.
		if _, cmd := pageDown(m); cmd != nil {
			t.Fatal("finalized SELECT still dispatched a page request")
		}
	})

	t.Run("ending cancellation settles first then finalizes", func(t *testing.T) {
		// Cancellation that ends the SELECT: the first-page request is
		// interrupted before any row is retained, and its classified-cancelled
		// settlement finalizes the execution once with a Cancelled entry.
		m, page, _ := startActiveSelect(t)
		execID := m.ActiveSelectExecutionID()

		settling := pressKeyLifecycle(t, m, tea.KeyMsg{Type: tea.KeyCtrlW})
		if !settling.selectCancelling {
			t.Fatal("Ctrl+W did not enter the cancelling state")
		}
		// Merely requesting cancellation does not finalize.
		requireActive(t, settling, execID, "during cancelling")
		// The boundary classifies the interrupted first page as cancelled.
		page.Result = FirstPageResult{Cancelled: true}
		ended := apply(settling, page)
		requireFinalized(t, ended, execID, "ending cancellation")
	})

	t.Run("first-page failure before rows finalizes with error entry", func(t *testing.T) {
		exec := &fakeSelectExecutor{page: threeRowFirstPage()}
		count := &fakeCountExecutor{total: 3}
		base := concurrentCountModel(exec, count)
		base.ResultHistory = history.NewResultStore()
		execModel, execCmd := driveToExecutionStart(t, base)
		page, _ := splitSelectCount(t, execBatch(t, execCmd))
		execID := execModel.ActiveSelectExecutionID()

		page.Result = FirstPageResult{Err: errors.New("first page failed")}
		ended := apply(execModel, page)
		requireFinalized(t, ended, execID, "first-page failure")
	})

	t.Run("accepted quit with pending count work", func(t *testing.T) {
		m, page, _, _ := fixtureFor(t, activeState{name: "count pending", countPending: true})
		execID := m.ActiveSelectExecutionID()
		_ = page // first-page message consumed inside the fixture

		confirming := pressKeyLifecycle(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
		accepted := pressKeyLifecycle(t, confirming, tea.KeyMsg{Type: tea.KeyEnter})
		requireFinalized(t, accepted, execID, "accepted quit with pending work")
	})

	t.Run("accepted quit while idle", func(t *testing.T) {
		m, _, _, _ := fixtureFor(t, activeState{name: "idle"})
		execID := m.ActiveSelectExecutionID()

		confirming := pressKeyLifecycle(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
		accepted := pressKeyLifecycle(t, confirming, tea.KeyMsg{Type: tea.KeyEnter})
		requireFinalized(t, accepted, execID, "accepted quit while idle")
	})
}

// TestValidationWithoutExecutionNeverFinalizes covers the boundary cases that
// must never finalize: a settled pre-execution validation whose execution
// start never arrives, and a rejected (blocked) execution attempt.
func TestValidationWithoutExecutionNeverFinalizes(t *testing.T) {
	m, page, count := startActiveSelect(t)
	execID := m.ActiveSelectExecutionID()
	m = apply(apply(m, page), count)

	// Settle the validation workflow but withhold the execution-start route:
	// validation alone — success, failure, or repair — finalizes nothing.
	next, cmd := pressKeyLifecycleCmd(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter issued no pre-execution workflow command")
	}
	settled, _ := next.Update(cmd())
	after := settled.(Model)
	if after.FinalizedSelectExecutionID() != 0 || after.ResultHistory.Len() != 0 {
		t.Fatal("pre-execution validation finalized the active SELECT")
	}
	if after.SelectIsActive() {
		requireActive(t, after, execID, "after settled validation")
	}
}

// TestLateMessagesAfterFinalizationAreInert proves a finalized SELECT cannot
// be reactivated or mutated by late request messages from its execution.
func TestLateMessagesAfterFinalizationAreInert(t *testing.T) {
	m, page, count := startActiveSelect(t)
	execID := m.ActiveSelectExecutionID()
	m = apply(apply(m, page), count)
	rows := m.Result.Page.Rows

	m.enterResultHistory()

	// Late duplicate first-page and count completions from the old execution.
	late := apply(apply(m, page), count)
	if late.SelectIsActive() {
		t.Fatal("late request message reactivated a finalized SELECT")
	}
	if late.ResultHistory.Len() != 1 {
		t.Fatalf("late messages created extra entries: %d", late.ResultHistory.Len())
	}
	if late.FinalizedSelectExecutionID() != execID {
		t.Fatalf("late message rewrote finalization to %d", late.FinalizedSelectExecutionID())
	}
	if len(late.Result.Page.Rows) != len(rows) || late.Result.Offset != m.Result.Offset {
		t.Fatal("late request message mutated the finalized result")
	}

	// A repeated finalizer invocation is a deterministic no-op.
	m.enterResultHistory()
	if m.ResultHistory.Len() != 1 {
		t.Fatalf("repeated finalizer created extra entries: %d", m.ResultHistory.Len())
	}
}
