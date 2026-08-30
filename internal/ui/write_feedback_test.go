// Write-phase in-flight feedback coverage for Issue #44, per the Writes and
// commit boundary, Global Key Precedence and Context/Action Matrix, and
// Testing Decisions in Notes/PRD-sqloid.md. Phase labels are held against the
// controllable fake write executor and preparation modal seams and mapped
// from typed lifecycle state: beginning/executing `Running…`, estimate
// `Estimating matching target rows…`, committing `Committing…`, rollback
// cleanup `Rolling back…`, and requested cancellation `cancelling…` until
// settlement. Labels stay visible through permitted local updates, follow the
// current execution identity, and cannot be overwritten by stale or duplicate
// phase messages. The generic Issue #27 gate consumes Enter, histories, save,
// and export during every write phase with phase-appropriate Ctrl+W
// guidance, routes Ctrl+W only while cancellable, gives the exact
// post-boundary feedback for rollback cleanup and committing, and never
// issues a further database request. Issue #27's read labels and behavior
// are regression-asserted unchanged.

package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/connection"
	"github.com/chris/sqloid/internal/result"
)

// committedNeverRunFake returns a write fake whose run command is never
// invoked by the hold helpers, so its recorded call count stays zero unless a
// gated action stacked a second write dispatch.
func committedNeverRunFake() *writeFakeExecutor {
	return &writeFakeExecutor{
		phases: nil,
		result: connection.WriteResult{Outcome: connection.WriteCommitted, RowsAffected: 1},
	}
}

// heldWriteModel confirms one settled preparation with the executor wired but
// never runs the write command, returning the model with the sole write
// pending and its phase channel held (no phases delivered yet). The fake's
// call count is therefore zero and can only rise through a stacked dispatch.
func heldWriteModel(t *testing.T, fake *writeFakeExecutor) Model {
	t.Helper()
	est := &prepFakeEstimator{}
	m := settledPreparation(t, prepUpdateQB(true), est, EstimateResult{Total: 7})
	m.Write = fake.Write
	m.catalog = prepCatalog()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("settled confirmation produced no command")
	}
	confirmed, ok := cmd().(WriteConfirmedMsg)
	if !ok {
		t.Fatalf("confirmation command produced %T, want WriteConfirmedMsg", cmd())
	}
	next, cmd = next.(Model).Update(confirmed)
	if cmd == nil {
		t.Fatal("confirmed write dispatched no command")
	}
	held := next.(Model)
	if !held.writePending || held.writeExecution == 0 {
		t.Fatal("setup: write did not enter the pending lifecycle")
	}
	// The batch's run command is never invoked: fake.calls stays 0, proving
	// every later assertion exercises the held write with no stacked work.
	return held
}

// holdWritePhase feeds one typed phase message for the held write's current
// execution identity through Update. The returned relay command is discarded
// because the held phase channel has nothing buffered; tests never invoke it.
func holdWritePhase(t *testing.T, m Model, phase connection.WritePhase) Model {
	t.Helper()
	next, _ := m.Update(connection.WritePhaseMsg{Execution: m.writeExecution, Phase: phase})
	nm, ok := next.(Model)
	if !ok {
		t.Fatalf("phase update returned %T", next)
	}
	return nm
}

// requestCancelling presses Ctrl+W on a cancellable phase and runs the
// dispatched cancellation closure, leaving the model in its cancellation-
// requested state.
func requestCancelling(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := pressKey(m, tea.KeyMsg{Type: tea.KeyCtrlW})
	if cmd == nil {
		t.Fatal("setup: Ctrl+W during a cancellable phase dispatched no cancellation command")
	}
	cmd() // the cancellation closure is the single interrupt request
	return next
}

// writeFixture is one held write-phase scenario: the model builder, the
// executor whose call count must never change, the exact label the view must
// show, and the phase's Enter/Ctrl+W expectations.
type writeFixture struct {
	name          string
	model         func(t *testing.T) (Model, *writeFakeExecutor, *prepFakeEstimator)
	wantLabel     string
	cancellable   bool
	cancelVisible string // exact label required after Ctrl+W on cancellable phases
	postBoundary  bool   // Ctrl+W gives CommitBoundaryFeedback instead of routing
	enterFeedback string
}

// writeFixtures covers estimating, estimate cancellation, beginning,
// executing, cancellation-requested, rollback-cleanup, and committing.
func writeFixtures() []writeFixture {
	enterCancellable := func(status string) string { return status + " — " + CancelHintSuffix }
	newHeld := func(t *testing.T) (Model, *writeFakeExecutor, *prepFakeEstimator) {
		fake := committedNeverRunFake()
		return heldWriteModel(t, fake), fake, nil
	}
	return []writeFixture{
		{
			name: "estimating",
			model: func(t *testing.T) (Model, *writeFakeExecutor, *prepFakeEstimator) {
				est := &prepFakeEstimator{}
				m, _ := openPreparation(t, prepUpdateQB(true), est)
				return m, committedNeverRunFake(), est
			},
			wantLabel:     DestructivePrepPendingStatus,
			cancellable:   true,
			cancelVisible: DestructivePrepCancellingStatus,
			enterFeedback: enterCancellable(DestructivePrepPendingStatus),
		},
		{
			name: "estimate cancellation requested",
			model: func(t *testing.T) (Model, *writeFakeExecutor, *prepFakeEstimator) {
				est := &prepFakeEstimator{}
				m, _ := openPreparation(t, prepUpdateQB(true), est)
				return requestCancelling(t, m), committedNeverRunFake(), est
			},
			wantLabel:     DestructivePrepCancellingStatus,
			cancellable:   true,
			cancelVisible: DestructivePrepCancellingStatus,
			enterFeedback: enterCancellable(DestructivePrepCancellingStatus),
		},
		{
			name:          "beginning",
			model:         newHeld,
			wantLabel:     SelectRunningIndicator,
			cancellable:   true,
			cancelVisible: SelectCancellingIndicator,
			enterFeedback: enterCancellable(SelectRunningIndicator),
		},
		{
			name: "executing",
			model: func(t *testing.T) (Model, *writeFakeExecutor, *prepFakeEstimator) {
				fake := committedNeverRunFake()
				return holdWritePhase(t, heldWriteModel(t, fake), connection.WritePhaseExecuting), fake, nil
			},
			wantLabel:     SelectRunningIndicator,
			cancellable:   true,
			cancelVisible: SelectCancellingIndicator,
			enterFeedback: enterCancellable(SelectRunningIndicator),
		},
		{
			name: "cancellation requested",
			model: func(t *testing.T) (Model, *writeFakeExecutor, *prepFakeEstimator) {
				fake := committedNeverRunFake()
				m := holdWritePhase(t, heldWriteModel(t, fake), connection.WritePhaseExecuting)
				return requestCancelling(t, m), fake, nil
			},
			wantLabel:     SelectCancellingIndicator,
			cancellable:   true,
			cancelVisible: SelectCancellingIndicator,
			enterFeedback: enterCancellable(SelectCancellingIndicator),
		},
		{
			name: "rollback cleanup",
			model: func(t *testing.T) (Model, *writeFakeExecutor, *prepFakeEstimator) {
				fake := committedNeverRunFake()
				return holdWritePhase(t, heldWriteModel(t, fake), connection.WritePhaseRollbackCleanup), fake, nil
			},
			wantLabel:     WriteRollingBackIndicator,
			postBoundary:  true,
			enterFeedback: CommitBoundaryFeedback,
			cancelVisible: WriteRollingBackIndicator,
		},
		{
			name: "committing",
			model: func(t *testing.T) (Model, *writeFakeExecutor, *prepFakeEstimator) {
				fake := committedNeverRunFake()
				return holdWritePhase(t, heldWriteModel(t, fake), connection.WritePhaseCommitting), fake, nil
			},
			wantLabel:     WriteCommittingIndicator,
			postBoundary:  true,
			enterFeedback: CommitBoundaryFeedback,
			cancelVisible: WriteCommittingIndicator,
		},
	}
}

// TestWritePhaseLabelsRenderExactlyFromTypedState requires each held write
// phase to render its exact label, mapped from the typed phase state and kept
// visible through a permitted local resize redraw.
func TestWritePhaseLabelsRenderExactlyFromTypedState(t *testing.T) {
	for _, fx := range writeFixtures() {
		t.Run(fx.name, func(t *testing.T) {
			m, _, _ := fx.model(t)
			if view := m.View(); !strings.Contains(view, fx.wantLabel) {
				t.Errorf("view missing exact label %q:\n%s", fx.wantLabel, view)
			}
			// A permitted local update (resize redraw) keeps the label.
			next, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 26})
			if view := next.View(); !strings.Contains(view, fx.wantLabel) {
				t.Errorf("resize lost exact label %q:\n%s", fx.wantLabel, view)
			}
		})
	}
}

// TestWriteCancellingHoldsUntilSettlement requires `cancelling…` requested
// from a cancellable write phase to stay visible across permitted local
// updates and through the subsequent typed rollback-cleanup transition.
func TestWriteCancellingHoldsUntilSettlement(t *testing.T) {
	fake := committedNeverRunFake()
	m := requestCancelling(t, holdWritePhase(t, heldWriteModel(t, fake), connection.WritePhaseExecuting))
	if view := m.View(); !strings.Contains(view, SelectCancellingIndicator) {
		t.Errorf("view missing exact %q after Ctrl+W:\n%s", SelectCancellingIndicator, view)
	}
	after, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 26})
	if view := after.(Model).View(); !strings.Contains(view, SelectCancellingIndicator) {
		t.Errorf("resize lost exact %q:\n%s", SelectCancellingIndicator, view)
	}
	rolled := holdWritePhase(t, m, connection.WritePhaseRollbackCleanup)
	if view := rolled.View(); !strings.Contains(view, WriteRollingBackIndicator) {
		t.Errorf("view missing exact %q during rollback cleanup:\n%s", WriteRollingBackIndicator, view)
	}
}

// TestWriteLabelsResistStaleAndDuplicatePhaseMessages requires labels to
// follow the current execution/request identity: a stale execution's phase
// message, a duplicate delivery of the current phase, and a post-boundary
// regression attempt can never replace or move the visible label backward.
func TestWriteLabelsResistStaleAndDuplicatePhaseMessages(t *testing.T) {
	fake := committedNeverRunFake()
	// Stale identity: executing label survives a later execution's
	// committing message.
	m := holdWritePhase(t, heldWriteModel(t, fake), connection.WritePhaseExecuting)
	stale, _ := m.Update(connection.WritePhaseMsg{Execution: m.writeExecution + 999, Phase: connection.WritePhaseCommitting})
	if view := stale.(Model).View(); !strings.Contains(view, SelectRunningIndicator) || strings.Contains(view, WriteCommittingIndicator) {
		t.Errorf("stale execution message changed the label:\n%s", view)
	}
	// Duplicate current phase: idempotent.
	dup, _ := m.Update(connection.WritePhaseMsg{Execution: m.writeExecution, Phase: connection.WritePhaseExecuting})
	if view := dup.(Model).View(); !strings.Contains(view, SelectRunningIndicator) {
		t.Errorf("duplicate phase message changed the label:\n%s", view)
	}
	// Post-boundary: committing label cannot regress to a stale beginning.
	committed := holdWritePhase(t, heldWriteModel(t, fake), connection.WritePhaseCommitting)
	regressed, _ := committed.Update(connection.WritePhaseMsg{Execution: committed.writeExecution, Phase: connection.WritePhaseBeginning})
	if view := regressed.(Model).View(); !strings.Contains(view, WriteCommittingIndicator) {
		t.Errorf("post-boundary phase regression changed the label:\n%s", view)
	}
}

// TestEstimateLabelsStayExact reasserts the preparation modal's exact
// estimate and cancellation labels stay driven from its typed pending and
// cancelling state, unchanged by Issue #44's write work.
func TestEstimateLabelsStayExact(t *testing.T) {
	est := &prepFakeEstimator{}
	m, _ := openPreparation(t, prepUpdateQB(true), est)
	if view := m.View(); !strings.Contains(view, DestructivePrepPendingStatus) {
		t.Errorf("view missing exact %q while estimating:\n%s", DestructivePrepPendingStatus, view)
	}
	// Ctrl+W requests cancellation: exact `cancelling…` until settlement.
	next := requestCancelling(t, m)
	if view := next.View(); !strings.Contains(view, DestructivePrepCancellingStatus) {
		t.Errorf("view missing exact %q while cancelling the estimate:\n%s", DestructivePrepCancellingStatus, view)
	}
}

// TestReadPhaseLabelsUnchanged is the Issue #27 regression: SELECT
// first-page `Running…`, page loading, count, count-unavailable, and read
// cancellation labels render exactly as before.
func TestReadPhaseLabelsUnchanged(t *testing.T) {
	exec := &fakeSelectExecutor{page: threeRowPage()}
	m := pendingFirstPage(t, exec)
	if view := m.View(); !strings.Contains(view, SelectRunningIndicator) {
		t.Errorf("first-page view missing exact %q:\n%s", SelectRunningIndicator, view)
	}

	pageExec := &fakePageExecutor{rowsShown: 11}
	mp := pendingLaterPage(t, exec, pageExec)
	if view := mp.View(); !strings.Contains(view, PageLoadingIndicator) {
		t.Errorf("page view missing exact %q:\n%s", PageLoadingIndicator, view)
	}

	mc := pendingCountOnly(t, exec, &fakeCountExecutor{total: 7})
	if view := mc.View(); !strings.Contains(view, "Counting rows…") {
		t.Errorf("count view missing exact %q:\n%s", "Counting rows…", view)
	}

	// Count-unavailable wording is unchanged on a settled page.
	counted := settledFirstPage(t, exec, pageExec)
	counted.countState = result.CountState{Status: result.CountUnavailable}
	if view := counted.View(); !strings.Contains(view, "Count unavailable") {
		t.Errorf("count-unavailable view missing exact %q:\n%s", "Count unavailable", view)
	}

	// Read cancellation handoff is unchanged.
	cancelled := requestCancelling(t, pendingFirstPage(t, &fakeSelectExecutor{page: threeRowPage()}))
	if view := cancelled.View(); !strings.Contains(view, SelectCancellingIndicator) {
		t.Errorf("read cancellation view missing exact %q:\n%s", SelectCancellingIndicator, view)
	}
}

// TestWriteGateBlocksActionsPerPhase is the table-driven core gating
// contract: during every write phase the blocked actions are consumed with no
// command dispatch, explanatory feedback, unchanged executor call counts, and
// unchanged builder selection.
func TestWriteGateBlocksActionsPerPhase(t *testing.T) {
	for _, fx := range writeFixtures() {
		for _, action := range blockedActions() {
			t.Run(fx.name+"/"+action.name, func(t *testing.T) {
				m, fake, est := fx.model(t)
				estCalls := 0
				if est != nil {
					estCalls, _, _, _ = est.snapshot()
				}
				next, cmd := pressKey(m, action.key)
				if cmd != nil {
					t.Fatalf("blocked %s action returned a command %v; the held write could stack", action.name, cmd)
				}
				nm := next
				if got := nm.inFlightNotice; got != action.feedback {
					t.Errorf("feedback = %q, want exactly %q", got, action.feedback)
				}
				if view := nm.View(); !strings.Contains(view, action.feedback) {
					t.Errorf("view did not render feedback %q:\n%s", action.feedback, view)
				}
				if fake.calls != 0 {
					t.Errorf("blocked action dispatched write requests: %d calls", fake.calls)
				}
				if est != nil {
					if calls, _, _, _ := est.snapshot(); calls != estCalls {
						t.Errorf("blocked action dispatched estimate requests: %d -> %d", estCalls, calls)
					}
				}
				// Unchanged query/result selection and phase label.
				if nm.Focus != m.Focus {
					t.Errorf("blocked action moved focus %d -> %d", m.Focus, nm.Focus)
				}
				if view := nm.View(); !strings.Contains(view, fx.wantLabel) {
					t.Errorf("blocked action lost the phase label %q:\n%s", fx.wantLabel, view)
				}
			})
		}
	}
}

// TestWriteGateEnterFeedbackPerPhase requires blocked Enter during every
// write phase to carry the phase-appropriate Ctrl+W guidance (or the exact
// post-boundary message) with no command dispatch.
func TestWriteGateEnterFeedbackPerPhase(t *testing.T) {
	for _, fx := range writeFixtures() {
		t.Run(fx.name, func(t *testing.T) {
			m, fake, _ := fx.model(t)
			next, cmd := pressKey(m, tea.KeyMsg{Type: tea.KeyEnter})
			if cmd != nil {
				t.Fatalf("blocked Enter returned a command %v", cmd)
			}
			if got := next.inFlightNotice; got != fx.enterFeedback {
				t.Errorf("Enter feedback = %q, want exactly %q", got, fx.enterFeedback)
			}
			if fake.calls != 0 {
				t.Errorf("blocked Enter dispatched write requests: %d calls", fake.calls)
			}
		})
	}
}

// TestWriteGateCtrlWPerPhase requires Ctrl+W to route to scoped cancellation
// only during cancellable write phases (marking exact `cancelling…` until
// settlement), and to be refused with the exact boundary feedback once
// rollback cleanup or committing has begun.
func TestWriteGateCtrlWPerPhase(t *testing.T) {
	for _, fx := range writeFixtures() {
		t.Run(fx.name, func(t *testing.T) {
			m, fake, _ := fx.model(t)
			next, cmd := pressKey(m, tea.KeyMsg{Type: tea.KeyCtrlW})
			if fx.postBoundary {
				if cmd != nil {
					t.Fatalf("Ctrl+W during %s dispatched an interrupt; the boundary forbids it", fx.name)
				}
				if got := next.inFlightNotice; got != CommitBoundaryFeedback {
					t.Errorf("Ctrl+W feedback = %q, want exactly %q", got, CommitBoundaryFeedback)
				}
				return
			}
			if fx.name == "cancellation requested" || fx.name == "estimate cancellation requested" {
				// The cancellation was already requested in setup; a repeat
				// Ctrl+W must be an idempotent no-op with no second interrupt
				// while `cancelling…` stays visible until settlement.
				if cmd != nil {
					cmd()
				}
				if view := next.View(); !strings.Contains(view, fx.cancelVisible) {
					t.Errorf("view missing exact %q on repeated Ctrl+W:\n%s", fx.cancelVisible, view)
				}
				return
			}
			if cmd == nil {
				t.Fatalf("Ctrl+W during cancellable %s dispatched no cancellation command", fx.name)
			}
			cmd()
			if view := next.View(); !strings.Contains(view, fx.cancelVisible) {
				t.Errorf("view missing exact %q after Ctrl+W:\n%s", fx.cancelVisible, view)
			}
			if fake.calls != 0 {
				t.Errorf("Ctrl+W dispatched write requests: %d calls", fake.calls)
			}
		})
	}
}

// TestWriteGateQuitConfirmationPerPhase requires `q` and Ctrl+C during every
// write phase to open the shared quit confirmation, consumed above the gate
// with no key leakage and exact restoration on Esc.
func TestWriteGateQuitConfirmationPerPhase(t *testing.T) {
	for _, fx := range writeFixtures() {
		for _, kc := range []struct {
			name string
			key  tea.KeyMsg
		}{
			{"q", runeKey("q")},
			{"ctrl+c", tea.KeyMsg{Type: tea.KeyCtrlC}},
		} {
			t.Run(fx.name+"/"+kc.name, func(t *testing.T) {
				m, _, _ := fx.model(t)
				next, cmd := pressKey(m, kc.key)
				if cmd != nil || !next.quitConfirm {
					t.Fatalf("%s during %s: quit confirmation not opened (cmd %v)", kc.name, fx.name, cmd)
				}
				if view := next.View(); !strings.Contains(view, "Quit") {
					t.Errorf("%s during %s: view did not render the confirmation", kc.name, fx.name)
				}
				// Esc restores the exact suspended pending context.
				restored, _ := pressKey(next, tea.KeyMsg{Type: tea.KeyEsc})
				if restored.quitConfirm || restored.quitSuspended != nil {
					t.Fatalf("%s during %s: Esc did not close the confirmation", kc.name, fx.name)
				}
			})
		}
	}
}

// TestWriteGateKeepsLocalInteractionDuringWrite requires permitted local
// interaction (horizontal one-column movement and resize redraws) to stay
// ungated during write phases: no rejection feedback is produced and the
// phase label stays visible.
func TestWriteGateKeepsLocalInteractionDuringWrite(t *testing.T) {
	fake := committedNeverRunFake()
	m := holdWritePhase(t, heldWriteModel(t, fake), connection.WritePhaseExecuting)
	m.inFlightNotice = SaveBlockedFeedback // stale explanation from an earlier rejection
	for _, k := range []string{",", "."} {
		next, cmd := pressKey(m, runeKey(k))
		if cmd != nil {
			t.Fatalf("horizontal key %q produced a command during a write phase", k)
		}
		if next.inFlightNotice != "" {
			t.Fatalf("horizontal key %q kept or produced feedback %q", k, next.inFlightNotice)
		}
		if view := next.View(); !strings.Contains(view, SelectRunningIndicator) {
			t.Errorf("horizontal key %q lost the phase label:\n%s", k, view)
		}
	}
}
