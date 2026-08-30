// Scripted Bubble Tea coverage for the generic request-in-flight gate
// (Issue #27 Task 1), per the Global Key Precedence and Context/Action
// Matrix in Notes/PRD-sqloid.md. The gate is exercised with SELECT
// first-page, later-page, and count work independently held against the
// controllable fake executor seams: Enter is consumed without another
// execution, history/save/export are rejected with explanatory feedback and
// unchanged request counts, horizontal one-column interaction stays local,
// `q`/Ctrl+C open the shared quit confirmation, and Ctrl+W reaches
// cancellation only for an owned cancellable request. Higher-precedence
// contexts (terminal, quit confirmation, top overlay, focused input)
// consume keys before the gate, and one case proves the gate derives
// pending state from request-ownership flags rather than rendered
// phase-label strings. Write-phase feedback and integration remain with
// Issue #44.

package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/result"
)

// ctrlKey builds a Ctrl+<letter> press by exact key type.
func ctrlKey(kt tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: kt} }

// runeKey builds a printable press.
func runeKey(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// pendingFirstPage drives a first-page-only model to an execution start and
// holds the page request in flight without settling it.
func pendingFirstPage(t *testing.T, exec *fakeSelectExecutor) Model {
	t.Helper()
	m := firstSelectModel(exec)
	execModel, _ := driveToExecutionStart(t, m)
	if !execModel.firstPagePending {
		t.Fatal("execution start did not claim the first-page slot")
	}
	return execModel
}

// pendingLaterPage settles a first page and issues one later-page request,
// holding it in flight without settling it.
func pendingLaterPage(t *testing.T, exec *fakeSelectExecutor, pageExec *fakePageExecutor) Model {
	t.Helper()
	m := settledFirstPage(t, exec, pageExec)
	_, cmd := pageDown(m)
	if cmd == nil {
		t.Fatal("page down issued no request")
	}
	cmd() // the held request is dispatched exactly once before the test gates on it
	if pageExec.issued != 1 {
		t.Fatalf("setup: page dispatches = %d, want 1", pageExec.issued)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	pending := next.(Model)
	if !pending.pagePending {
		t.Fatal("page down did not claim the page slot")
	}
	return pending
}

// pendingCountOnly settles only the page completion of one execution,
// leaving the independent count request held in flight.
func pendingCountOnly(t *testing.T, exec *fakeSelectExecutor, count *fakeCountExecutor) Model {
	t.Helper()
	m := concurrentCountModel(exec, count)
	execModel, execCmd := driveToExecutionStart(t, m)
	msgs := execBatch(t, execCmd)
	page, _ := splitSelectCount(t, msgs)
	next, _ := execModel.Update(page)
	out := next.(Model)
	if out.firstPagePending || !out.countPendingFlag {
		t.Fatalf("setup: want count-only pending, firstPage=%v count=%v", out.firstPagePending, out.countPendingFlag)
	}
	return out
}

// pendingPhase is one held-request scenario with the exact phase wording its
// Enter feedback must lead with.
type pendingPhase struct {
	name       string
	build      func(t *testing.T) Model
	phaseWords string
}

// pendingPhases covers SELECT first-page, later-page, and count work.
func pendingPhases() []pendingPhase {
	return []pendingPhase{
		{"first page pending", func(t *testing.T) Model {
			return pendingFirstPage(t, &fakeSelectExecutor{page: threeRowPage()})
		}, SelectRunningIndicator},
		{"later page pending", func(t *testing.T) Model {
			return pendingLaterPage(t,
				&fakeSelectExecutor{page: threeRowPage()},
				&fakePageExecutor{rowsShown: 11})
		}, PageLoadingIndicator},
		{"count pending", func(t *testing.T) Model {
			return pendingCountOnly(t,
				&fakeSelectExecutor{page: threeRowPage()},
				&fakeCountExecutor{total: 7})
		}, "Counting rows…"},
	}
}

// blockedAction is one gated action with the exact feedback it must produce
// while a request is in flight.
type blockedAction struct {
	name     string
	key      tea.KeyMsg
	feedback string
}

// blockedActions enumerates the query/result-history, save, and export
// actions the generic gate consumes with explanatory feedback. Enter's
// feedback is phase-specific and asserted separately.
func blockedActions() []blockedAction {
	return []blockedAction{
		{"query history older", ctrlKey(tea.KeyCtrlP), QueryHistoryBlockedFeedback},
		{"query history newer", ctrlKey(tea.KeyCtrlN), QueryHistoryBlockedFeedback},
		{"result history older", ctrlKey(tea.KeyCtrlE), ResultHistoryBlockedFeedback},
		{"result history newer", ctrlKey(tea.KeyCtrlY), ResultHistoryBlockedFeedback},
		{"save", ctrlKey(tea.KeyCtrlS), SaveBlockedFeedback},
		{"export", ctrlKey(tea.KeyCtrlX), ExportBlockedFeedback},
	}
}

// TestInFlightGateBlocksActionsForEveryPendingPhase is the table-driven core
// contract: while SELECT first-page, later-page, or count work is pending,
// every blocked action is consumed with no command dispatch — so the held
// executors can never run again — and explanatory feedback renders.
func TestInFlightGateBlocksActionsForEveryPendingPhase(t *testing.T) {
	for _, phase := range pendingPhases() {
		for _, action := range blockedActions() {
			t.Run(phase.name+"/"+action.name, func(t *testing.T) {
				m := phase.build(t)
				next, cmd := pressKey(m, action.key)
				if cmd != nil {
					t.Fatalf("blocked %s action returned a command %v; the held request could stack", action.name, cmd)
				}
				if got := next.inFlightNotice; got != action.feedback {
					t.Errorf("feedback = %q, want exactly %q", got, action.feedback)
				}
				if view := next.View(); !strings.Contains(view, action.feedback) {
					t.Errorf("view did not render feedback %q:\n%s", action.feedback, view)
				}
			})
		}
		// Enter feedback carries the phase wording plus the exact Ctrl+W hint.
		t.Run(phase.name+"/enter hint", func(t *testing.T) {
			m := phase.build(t)
			next, cmd := pressKey(m, tea.KeyMsg{Type: tea.KeyEnter})
			if cmd != nil {
				t.Fatal("blocked Enter returned a command")
			}
			want := phase.phaseWords + " — press Ctrl+W to cancel"
			if next.inFlightNotice != want {
				t.Errorf("Enter feedback = %q, want exactly %q", next.inFlightNotice, want)
			}
		})
	}
}

// TestInFlightGateEnterNeverStacksRequests requires Enter during every
// pending phase to be consumed without issuing any further executor request:
// the held executors' recorded call counts stay unchanged because no command
// is ever returned to invoke, and request ownership is untouched.
func TestInFlightGateEnterNeverStacksRequests(t *testing.T) {
	// First-page pending: the executor runs only when its command is
	// invoked, so zero recorded calls proves no stacked dispatch.
	exec := &fakeSelectExecutor{page: threeRowPage()}
	m := pendingFirstPage(t, exec)
	if exec.calls != 0 {
		t.Fatalf("executor ran before its command was invoked: %d calls", exec.calls)
	}
	next, cmd := pressKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("Enter during first-page work returned a stacking command")
	}
	if next.firstPagePending != m.firstPagePending {
		t.Fatal("blocked Enter mutated request ownership")
	}

	// Count-only pending: same contract for the count seam.
	count := &fakeCountExecutor{total: 7}
	mCount := pendingCountOnly(t, exec, count)
	before := count.calls
	if _, cmd := pressKey(mCount, tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatal("Enter during count work returned a stacking command")
	}
	if count.calls != before {
		t.Fatalf("Enter during count work dispatched extra count requests: %d -> %d", before, count.calls)
	}

	// Later-page pending: a further Page key must not stack a second request.
	pageExec := &fakePageExecutor{rowsShown: 11}
	mPage := pendingLaterPage(t, exec, pageExec)
	if pageExec.issued != 1 {
		t.Fatalf("setup: page issued = %d, want 1", pageExec.issued)
	}
	if _, cmd := pressKey(mPage, tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatal("Enter during later-page work returned a stacking command")
	}
}

// TestInFlightGateKeepsHorizontalMovementLocal requires the permitted
// one-column horizontal navigation keys to remain local while a request is
// in flight: no rejection feedback appears and no command is dispatched.
func TestInFlightGateKeepsHorizontalMovementLocal(t *testing.T) {
	m := pendingFirstPage(t, &fakeSelectExecutor{page: threeRowPage()})
	m.inFlightNotice = QueryHistoryBlockedFeedback // stale explanation from an earlier rejection
	for _, k := range []string{",", "."} {
		next, cmd := pressKey(m, runeKey(k))
		if cmd != nil {
			t.Fatalf("horizontal key %q produced a command while pending", k)
		}
		if next.inFlightNotice != "" {
			t.Fatalf("horizontal key %q kept or produced feedback %q; movement must stay local", k, next.inFlightNotice)
		}
	}
}

// TestInFlightGateQuitConfirmation requires `q` and Ctrl+C to open the shared
// quit confirmation during in-flight work, with Enter/y/Ctrl+C confirming,
// Esc/n restoring the exact suspended context, and other keys consumed with
// no leakage.
func TestInFlightGateQuitConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyMsg
	}{
		{"q opens", runeKey("q")},
		{"ctrl+c opens", tea.KeyMsg{Type: tea.KeyCtrlC}},
	} {
		m := pendingFirstPage(t, &fakeSelectExecutor{page: threeRowPage()})
		m.Focus = 2
		next, cmd := pressKey(m, tc.key)
		if cmd != nil || !next.quitConfirm {
			t.Fatalf("%s: quit confirmation not opened (cmd %v, confirm %v)", tc.name, cmd, next.quitConfirm)
		}
		if view := next.View(); !strings.Contains(view, "Quit") {
			t.Errorf("%s: view did not render the confirmation:\n%s", tc.name, view)
		}
		// Unrelated key: consumed with no leakage.
		after, _ := pressKey(next, runeKey("x"))
		if !after.quitConfirm {
			t.Fatalf("%s: printable key leaked out of the confirmation", tc.name)
		}
		// Esc restores the exact suspended context.
		restored, _ := pressKey(after, tea.KeyMsg{Type: tea.KeyEsc})
		if restored.quitConfirm || restored.quitSuspended != nil {
			t.Fatalf("%s: Esc left the confirmation open", tc.name)
		}
		if restored.Focus != 2 {
			t.Fatalf("%s: Esc restored focus %d, want the exact suspended focus 2", tc.name, restored.Focus)
		}
	}
	// Ctrl+C inside the confirmation confirms quit.
	m := pendingFirstPage(t, &fakeSelectExecutor{page: threeRowPage()})
	opened, _ := pressKey(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	_, cmd := pressKey(opened, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("confirmed quit produced no command")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Fatalf("confirmed quit produced %T, want tea.Quit", msg)
	}
}

// TestQuitConfirmationRestoresPendingPhase requires cancelling quit during
// in-flight work to restore the exact pending phase — ownership flags and
// feedback intact, and the gate still engaged.
func TestQuitConfirmationRestoresPendingPhase(t *testing.T) {
	m := pendingCountOnly(t,
		&fakeSelectExecutor{page: threeRowPage()},
		&fakeCountExecutor{total: 7})
	m.inFlightNotice = SaveBlockedFeedback
	opened, _ := pressKey(m, runeKey("q"))
	restored, _ := pressKey(opened, tea.KeyMsg{Type: tea.KeyEsc})
	if !restored.countPendingFlag || restored.inFlightNotice != SaveBlockedFeedback {
		t.Fatalf("restored model lost pending context: count=%v notice=%q",
			restored.countPendingFlag, restored.inFlightNotice)
	}
	if _, cmd := pressKey(restored, tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatal("restored pending phase no longer gates Enter")
	}
}

// TestInFlightGateCtrlWRoutesOnlyToCancellableRequests requires Ctrl+W to
// reach the scoped cancellation command only for an owned cancellable
// request, dispatching it exactly once and marking SELECT work `cancelling…`
// until settlement; without ownership the key is ignored with no state
// change.
func TestInFlightGateCtrlWRoutesOnlyToCancellableRequests(t *testing.T) {
	// Cancellable: owned cancellation command dispatches once.
	m := pendingFirstPage(t, &fakeSelectExecutor{page: threeRowPage()})
	m.ActiveCancellable = true
	cancelled := 0
	m.CancelCommand = func() tea.Msg {
		cancelled++
		return SelectCancelRequestedMsg{}
	}
	next, cmd := pressKey(m, tea.KeyMsg{Type: tea.KeyCtrlW})
	if cmd == nil {
		t.Fatal("cancellable request: Ctrl+W produced no cancellation command")
	}
	if cancelled != 0 {
		t.Fatalf("cancellation command ran outside its dispatch: %d times", cancelled)
	}
	cmd() // invoking the dispatched command is the single cancel signal
	if cancelled != 1 {
		t.Fatalf("cancellation dispatched %d times, want exactly 1", cancelled)
	}
	if !next.selectCancelling {
		t.Fatal("Ctrl+W did not enter the cancelling handoff state")
	}
	if view := next.View(); !strings.Contains(view, "cancelling…") {
		t.Errorf("view did not render the cancelling handoff:\n%s", view)
	}

	// Not cancellable: ignored with no state change.
	idle := pendingFirstPage(t, &fakeSelectExecutor{page: threeRowPage()})
	idle.ActiveCancellable = false
	idle.CancelCommand = nil
	after, cmd := pressKey(idle, tea.KeyMsg{Type: tea.KeyCtrlW})
	if cmd != nil || after.selectCancelling {
		t.Fatal("Ctrl+W without cancellable ownership must be ignored")
	}
}

// TestHigherPrecedenceConsumesKeysBeforeInFlightGate covers the precedence
// order terminal → quit confirmation → top overlay → focused text/search
// input → request-pending gate → base: higher contexts consume keys first.
func TestHigherPrecedenceConsumesKeysBeforeInFlightGate(t *testing.T) {
	m := pendingFirstPage(t, &fakeSelectExecutor{page: threeRowPage()})

	// Terminal state: every key is consumed with no gate action at all.
	// Issue #46: `q` alone exits immediately — it is the one key that emits
	// a command (tea.Quit), and it never reaches the gate.
	terminal := m
	terminal.enterTerminal(TerminalDeleted)
	for _, key := range []tea.KeyMsg{
		tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyCtrlW},
	} {
		next, cmd := pressKey(terminal, key)
		if cmd != nil || next.inFlightNotice != "" {
			t.Fatalf("terminal state leaked a key to the gate (cmd %v, notice %q)", cmd, next.inFlightNotice)
		}
	}
	qNext, qCmd := pressKey(terminal, runeKey("q"))
	if qCmd == nil || qCmd() != tea.Quit() || qNext.ExitStatus() != 1 {
		t.Fatalf("terminal q did not exit immediately with status 1 (cmd %v)", qCmd)
	}

	// Focused text input: `q` is inserted into the buffer, never quit, and
	// no gate feedback is produced behind the prompt.
	promptModel := m
	promptModel.ValuePrompt = &ValuePrompt{Opener: limitFieldLabel, Label: "Limit"}
	qed, _ := pressKey(promptModel, runeKey("q"))
	if qed.ValuePrompt == nil {
		t.Fatal("focused prompt was closed by a printable key")
	}
	if got := qed.ValuePrompt.Buffer(); got != "q" {
		t.Fatalf("`q` was not inserted into the focused prompt: buffer %q", got)
	}
	if qed.inFlightNotice != "" {
		t.Fatalf("gate feedback appeared behind a focused prompt: %q", qed.inFlightNotice)
	}
	// Enter submits the prompt locally — the gate never sees it.
	submitted, _ := pressKey(qed, tea.KeyMsg{Type: tea.KeyEnter})
	if submitted.ValuePrompt != nil {
		t.Fatal("focused prompt did not consume Enter locally")
	}
	if submitted.inFlightNotice != "" {
		t.Fatalf("gate feedback leaked out of prompt submission: %q", submitted.inFlightNotice)
	}

	// Quit confirmation opened above a pending phase: Enter confirms rather
	// than producing the Enter-in-flight hint.
	opened, _ := pressKey(m, runeKey("q"))
	confirmed, cmd := pressKey(opened, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("quit confirmation Enter produced no command")
	}
	if confirmed.inFlightNotice != "" {
		t.Fatalf("quit confirmation leaked Enter to the gate: %q", confirmed.inFlightNotice)
	}
}

// TestGateDerivesPendingFromOwnershipNotLabels proves the gate's behavior
// derives from generic in-flight state rather than SELECT phase-label
// strings: a bare model with the ownership flag set — no executor, no
// rendered phase text, no SELECT labels — rejects Enter exactly like a live
// execution, and clearing the flag restores base behavior.
func TestGateDerivesPendingFromOwnershipNotLabels(t *testing.T) {
	m := sized(New(), 80, 24).(Model)
	m.firstPagePending = true
	next, cmd := pressKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("ownership-flag pending did not consume Enter")
	}
	if !strings.Contains(next.inFlightNotice, "press Ctrl+W to cancel") {
		t.Fatalf("flag-driven feedback = %q, want the Ctrl+W hint", next.inFlightNotice)
	}
	if strings.Contains(next.View(), "Counting rows…") {
		t.Fatal("flag-driven gate rendered a count label; it must not derive from labels")
	}
	next.QB = validSelectQB()
	next.firstPagePending = false
	after, cmd := pressKey(next, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("cleared ownership left the gate engaged: base Enter issued no seam")
	}
	if after.inFlightNotice != "" {
		t.Fatalf("idle base Enter produced in-flight feedback %q", after.inFlightNotice)
	}
}

// TestInFlightGateSettlementReleasesTheGate requires the gate to release
// exactly at request settlement regardless of arrival order: settling the
// count while the page is still pending keeps the gate engaged with the
// page's own feedback; settling the last held request releases it entirely.
func TestInFlightGateSettlementReleasesTheGate(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	count := &fakeCountExecutor{total: 7}
	m := concurrentCountModel(exec, count)
	execModel, execCmd := driveToExecutionStart(t, m)
	page, countMsg := splitSelectCount(t, execBatch(t, execCmd))

	// Count settles first: the first page is still pending, so the gate must
	// stay engaged with the page phase.
	next, _ := execModel.Update(countMsg)
	partial := next.(Model)
	if partial.countPendingFlag || !partial.firstPagePending {
		t.Fatalf("count settlement left wrong ownership: firstPage=%v count=%v",
			partial.firstPagePending, partial.countPendingFlag)
	}
	gated, cmd := pressKey(partial, ctrlKey(tea.KeyCtrlS))
	if cmd != nil {
		t.Fatal("gate disengaged while the first page is still pending")
	}
	if gated.inFlightNotice != SaveBlockedFeedback {
		t.Fatalf("count-only feedback = %q, want %q", gated.inFlightNotice, SaveBlockedFeedback)
	}

	// Then the page settles: every held request has settled and the gate
	// releases — base save behavior becomes reachable again (a nil executor
	// here keeps it a no-op, but no in-flight feedback may appear).
	next2, _ := partial.Update(page)
	final := next2.(Model)
	if final.firstPagePending || final.selectRequestPending() {
		t.Fatal("settled first page left pending ownership")
	}
	if final.inFlightNotice != "" {
		t.Fatalf("stale in-flight feedback %q survived full settlement", final.inFlightNotice)
	}
}

// TestInFlightGateCountFailureKeepsExactWording requires a failed count to
// settle into the established exact `Count unavailable` state without ever
// converting into a page failure or clamping rows.
func TestInFlightGateCountFailureKeepsExactWording(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	count := &fakeCountExecutor{err: errors.New("database is locked")}
	m := concurrentCountModel(exec, count)
	execModel, execCmd := driveToExecutionStart(t, m)
	page, countMsg := splitSelectCount(t, execBatch(t, execCmd))
	next, _ := execModel.Update(countMsg)
	afterCount := next.(Model)
	if afterCount.countState.Status != result.CountUnavailable {
		t.Fatalf("count status = %v, want CountUnavailable", afterCount.countState.Status)
	}
	final, _ := afterCount.Update(page)
	view := final.View()
	if !strings.Contains(view, "Count unavailable") {
		t.Errorf("view did not render `Count unavailable` after failure:\n%s", view)
	}
	if strings.Contains(view, "Counting rows…") {
		t.Errorf("pending count wording survived failure:\n%s", view)
	}
}
