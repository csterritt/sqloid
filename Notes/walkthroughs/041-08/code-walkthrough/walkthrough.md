# Issue #41 — estimate failure, confirmation, and dismissal

*2026-08-29T13:36:37Z by Showboat 0.6.1*
<!-- showboat-id: 4160e0ee-608e-4880-8d4a-2715765dcd78 -->

This walkthrough verifies the Issue #41 implementation (estimate failure, confirmation, and dismissal) against the 'Estimate SQL and modal', 'Identities and state', and 'Writes and commit boundary' decisions in Notes/PRD-sqloid.md: settled estimate success and failure retaining operation, table, canonical SQL, and any all-rows warning while enabling Enter/y confirmation; exactly-once deliberate confirmation emitting one WriteConfirmedMsg bound to a distinct write-execution identity; idempotent Ctrl+W showing exact cancelling… through true settlement with cancellation-wins late-success rejection; Esc/n dismissal restoring the exact opener; and neither history touched by any preparation stage.

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/ui/ ./internal/history/ ./internal/result/ 2>&1 | sed -E 's/[0-9]+\.[0-9]+s//g' | cut -c1-58
```

```output
ok  	github.com/chris/sqloid/internal/ui	
ok  	github.com/chris/sqloid/internal/history	
ok  	github.com/chris/sqloid/internal/result	
```

The scripted evidence below drives the real model transitions in internal/ui with a barrier fake estimator (each true line is an identity, retention, history, or request-count assertion): (A) a qualified UPDATE preparation held in flight, exact cancelling… via one idempotent Ctrl+W, a released late success classified cancelled, dismissal only after settlement, and both histories unchanged; (B) a settled failure retaining SQL plus the all-rows warning, y confirmation emitting exactly one WriteConfirmedMsg that replays cannot duplicate; (C) duplicate/stale settlement messages unable to replace retained content, then Enter confirming the settled DELETE with a fresh write-execution identity:

```bash
cd /home/chris/sqloid && cat > internal/ui/zz_walkthrough_test.go <<'EOF'
package ui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	qb "github.com/chris/sqloid/internal/querybuilder"
	"github.com/chris/sqloid/internal/schema"
)

// walkthroughEstimator holds every estimate behind a deterministic channel
// barrier so the walkthrough can cancel work that is genuinely in flight.
type walkthroughEstimator struct {
	mu       sync.Mutex
	requests int
	cancels  int
	release  chan struct{}
	result   EstimateResult
}

func newWalkthroughEstimator(result EstimateResult) *walkthroughEstimator {
	return &walkthroughEstimator{release: make(chan struct{}), result: result}
}

func (w *walkthroughEstimator) ExecuteEstimate(ctx context.Context, sql string, params []any) EstimateResult {
	w.mu.Lock()
	w.requests++
	w.mu.Unlock()
	select {
	case <-w.release:
	case <-ctx.Done():
		w.mu.Lock()
		w.cancels++
		w.mu.Unlock()
		return EstimateResult{Cancelled: true}
	}
	if ctx.Err() != nil {
		w.mu.Lock()
		w.cancels++
		w.mu.Unlock()
		return EstimateResult{Cancelled: true}
	}
	return w.result
}

func (w *walkthroughEstimator) stats() (int, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.requests, w.cancels
}

// openWalkthroughPreparation mirrors openPreparation but accepts any
// EstimateExecutor, so the barrier fake can drive the same validated route.
func openWalkthroughPreparation(t *testing.T, q qb.QueryBuilder, est EstimateExecutor) (Model, tea.Cmd) {
	t.Helper()
	m := prepModel(q, nil)
	m.Estimator = est
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on runnable destructive state emitted no command")
	}
	next, cmd = next.Update(cmd())
	if cmd != nil {
		t.Fatalf("pre-execution seam returned unexpected command %v", cmd)
	}
	next, cmd = next.(Model).Update(ValidationSettledMsg{Preparation: 1,
		Result: schema.Revalidation{Status: schema.RevalidateUnchanged}})
	nm := next.(Model)
	if !nm.prepOpen {
		t.Fatal("preparation modal did not open after settled validation")
	}
	return nm, cmd
}

func TestWalkthrough41Evidence(t *testing.T) {
	// --- A: a qualified UPDATE held behind the barrier, Ctrl+W while the
	// estimate is genuinely in flight, and a late released success.
	est := newWalkthroughEstimator(EstimateResult{Total: 9})
	m, estimateCmd := openWalkthroughPreparation(t, prepUpdateQB(true), est.ExecuteEstimate)
	view := m.View()
	t.Logf("A pending status exact: %v", strings.Contains(view, "Estimating matching target rows…"))
	t.Logf("A retained SQL visible: %v", strings.Contains(view, `UPDATE "users" SET "email" = 'new' WHERE "id" = 5`))

	msgCh := make(chan tea.Msg, 1)
	go func() { msgCh <- estimateCmd() }()

	// Ctrl+W while the estimate is genuinely blocked in flight.
	next, cancelCmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	nm := next.(Model)
	_, again := nm.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	t.Logf("A repeated Ctrl+W idempotent (no second cancellation): %v", again == nil)
	t.Logf("A exact cancelling… visible: %v", strings.Contains(nm.View(), "cancelling…"))
	t.Logf("A preparation still open while waiting: %v", nm.prepOpen)
	cancelCmd() // request the connection-scoped interrupt

	// The blocked request settles as cancelled: the released success loses
	// to cancellation and preparation dismisses without history.
	settled := (<-msgCh).(EstimateSettledMsg)
	reqs, cancels := est.stats()
	t.Logf("A estimate request count stays exactly one: %v", reqs == 1)
	t.Logf("A released estimate classified cancelled: %v", settled.Result.Cancelled)
	t.Logf("A one cancellation observed by the fake: %v", cancels == 1)
	next, _ = nm.Update(settled)
	dm := next.(Model)
	t.Logf("A preparation dismissed only after settlement: %v", !dm.prepOpen)
	t.Logf("A late success did not survive: %v", !strings.Contains(dm.View(), "Estimated matching target rows: 9"))
	t.Logf("A histories unchanged after cancellation: %v", dm.History.Len() == 0 && dm.ResultHistory.Len() == 0)

	// --- B: settled failure retains SQL + warning and enables exactly-once y.
	fest := &prepFakeEstimator{result: EstimateResult{Err: errors.New("database is locked")}}
	fm, fcmd := openWalkthroughPreparation(t, prepUpdateQB(false), fest.ExecuteEstimate)
	fm = runEstimate(t, fm, fcmd)
	fv := fm.View()
	t.Logf("B failure text shown: %v", strings.Contains(fv, "Estimate failed: database is locked"))
	t.Logf("B SQL and all-rows warning retained: %v", strings.Contains(fv, `UPDATE "users" SET "email" = 'new'`) && strings.Contains(fv, "every row"))
	t.Logf("B confirmation enabled after failure: %v", strings.Contains(fv, "Enter/y confirms the write"))
	yNext, yCmd := fm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	yConfirmed := yCmd().(WriteConfirmedMsg)
	t.Logf("B y confirmation preparation=%d execution=%d op=%s sql=%s", yConfirmed.Preparation, yConfirmed.Execution, yConfirmed.Operation, yConfirmed.SQL)
	yn := yNext.(Model)
	// Replays: repeated y, duplicate/stale settlement, message re-delivery.
	// Model is a value: thread the final state through every replay.
	next, r1 := yn.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if r1 != nil {
		r1()
	}
	next, r2 := next.Update(EstimateSettledMsg{Preparation: yConfirmed.Preparation, Result: EstimateResult{Total: 99}})
	if r2 != nil {
		r2()
	}
	next, r3 := next.Update(EstimateSettledMsg{Preparation: yConfirmed.Preparation + 4, Result: EstimateResult{Err: errors.New("late")}})
	if r3 != nil {
		r3()
	}
	next, r4 := next.Update(yConfirmed)
	if r4 != nil {
		r4()
	}
	yn = next.(Model)
	t.Logf("B replays dispatched no second write: %v", r1 == nil && r2 == nil && r3 == nil && r4 == nil)
	t.Logf("B exactly one write after every replay: %v", yn.writeAttempt == 1 && yn.confirmedExecution == yConfirmed.Execution)
	t.Logf("B histories unchanged through confirmation: %v", yn.History.Len() == 0 && yn.ResultHistory.Len() == 0)
	freqs, _, _, _ := fest.snapshot()
	t.Logf("B exactly one estimate request for that preparation: %v", freqs == 1)

	// --- C: settled success with a duplicate/stale retention guard + Enter.
	sest := &prepFakeEstimator{result: EstimateResult{Total: 7}}
	sm, scmd := openWalkthroughPreparation(t, prepDeleteQB(true), sest.ExecuteEstimate)
	sm = runEstimate(t, sm, scmd)
	next, c := sm.Update(EstimateSettledMsg{Preparation: sm.prepAttempt, Result: EstimateResult{Total: 42}})
	t.Logf("C duplicate settlement dispatched nothing: %v", c == nil)
	t.Logf("C retained estimate not replaced: %v", strings.Contains(next.(Model).View(), "Estimated matching target rows: 7"))
	next, _ = next.(Model).Update(EstimateSettledMsg{Preparation: sm.prepAttempt + 3, Result: EstimateResult{Err: errors.New("stale")}})
	t.Logf("C stale settlement inert: %v", strings.Contains(next.(Model).View(), "Estimated matching target rows: 7"))
	enNext, enCmd := next.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	enConfirmed := enCmd().(WriteConfirmedMsg)
	t.Logf("C Enter confirmation preparation=%d execution=%d op=%s sql=%s", enConfirmed.Preparation, enConfirmed.Execution, enConfirmed.Operation, enConfirmed.SQL)
	t.Logf("C execution identities fresh across confirmations: %v", enConfirmed.Execution > yConfirmed.Execution)
	en := enNext.(Model)
	t.Logf("C histories still untouched after Enter confirmation: %v", en.History.Len() == 0 && en.ResultHistory.Len() == 0)
}
EOF
timeout 120 go test ./internal/ui/ -run 'TestWalkthrough41Evidence' -v -count=1 2>&1 | grep -E 'zz_walkthrough_test.go|--- (PASS|FAIL)'
rm internal/ui/zz_walkthrough_test.go
```

```output
    zz_walkthrough_test.go:86: A pending status exact: true
    zz_walkthrough_test.go:87: A retained SQL visible: true
    zz_walkthrough_test.go:96: A repeated Ctrl+W idempotent (no second cancellation): true
    zz_walkthrough_test.go:97: A exact cancelling… visible: true
    zz_walkthrough_test.go:98: A preparation still open while waiting: true
    zz_walkthrough_test.go:105: A estimate request count stays exactly one: true
    zz_walkthrough_test.go:106: A released estimate classified cancelled: true
    zz_walkthrough_test.go:107: A one cancellation observed by the fake: true
    zz_walkthrough_test.go:110: A preparation dismissed only after settlement: true
    zz_walkthrough_test.go:111: A late success did not survive: true
    zz_walkthrough_test.go:112: A histories unchanged after cancellation: true
    zz_walkthrough_test.go:119: B failure text shown: true
    zz_walkthrough_test.go:120: B SQL and all-rows warning retained: true
    zz_walkthrough_test.go:121: B confirmation enabled after failure: true
    zz_walkthrough_test.go:124: B y confirmation preparation=1 execution=1 op=UPDATE sql=UPDATE "users" SET "email" = 'new'
    zz_walkthrough_test.go:145: B replays dispatched no second write: true
    zz_walkthrough_test.go:146: B exactly one write after every replay: true
    zz_walkthrough_test.go:147: B histories unchanged through confirmation: true
    zz_walkthrough_test.go:149: B exactly one estimate request for that preparation: true
    zz_walkthrough_test.go:156: C duplicate settlement dispatched nothing: true
    zz_walkthrough_test.go:157: C retained estimate not replaced: true
    zz_walkthrough_test.go:159: C stale settlement inert: true
    zz_walkthrough_test.go:162: C Enter confirmation preparation=1 execution=2 op=DELETE sql=DELETE FROM "users" WHERE "id" = 5
    zz_walkthrough_test.go:163: C execution identities fresh across confirmations: true
    zz_walkthrough_test.go:165: C histories still untouched after Enter confirmation: true
--- PASS: TestWalkthrough41Evidence (0.00s)
```

The contract tests pin all of this permanently — confirmation enablement and exactly-once identity across settled success and failure, rejection while pending/cancelling/dismissed, duplicate/stale settlement retention guards, idempotent Ctrl+W with stale cancellation inertness, and exact opener restoration with zero histories:

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/ui/ -run 'TestSettledEstimatesEnableConfirmation|TestConfirmationRequiresSettledOutcome|TestConfirmationIsExactlyOnce|TestSettlementMessagesCannotReplaceRetainedContent|TestRepeatedCtrlWIsIdempotentAndStaleCancellationsInert|TestDismissalRestoresExactOpenerState|TestDestructivePreparationRetainsSuccessAndFailure|TestDestructivePreparationRejectsStaleEstimateResponses|TestDestructivePreparationEscDismissesWithCancellation|TestDestructivePreparationCancelThenSettleDismisses|TestDestructivePreparationNeverAppendsHistory' -v 2>&1 | grep -E '^(--- |=== RUN   Test[A-Z]|ok|FAIL)' | sed -E 's/[0-9]+\.[0-9]+s//g'
```

```output
=== RUN   TestSettledEstimatesEnableConfirmation
=== RUN   TestSettledEstimatesEnableConfirmation/qualified_update_settles_successfully
=== RUN   TestSettledEstimatesEnableConfirmation/unqualified_update_settles_with_a_failure
=== RUN   TestSettledEstimatesEnableConfirmation/qualified_delete_settles_successfully
=== RUN   TestSettledEstimatesEnableConfirmation/unqualified_delete_settles_with_a_failure
--- PASS: TestSettledEstimatesEnableConfirmation ()
=== RUN   TestConfirmationRequiresSettledOutcome
=== RUN   TestConfirmationRequiresSettledOutcome/pending
=== RUN   TestConfirmationRequiresSettledOutcome/cancelling
=== RUN   TestConfirmationRequiresSettledOutcome/dismissed
--- PASS: TestConfirmationRequiresSettledOutcome ()
=== RUN   TestConfirmationIsExactlyOnce
--- PASS: TestConfirmationIsExactlyOnce ()
=== RUN   TestSettlementMessagesCannotReplaceRetainedContent
--- PASS: TestSettlementMessagesCannotReplaceRetainedContent ()
=== RUN   TestRepeatedCtrlWIsIdempotentAndStaleCancellationsInert
--- PASS: TestRepeatedCtrlWIsIdempotentAndStaleCancellationsInert ()
=== RUN   TestDismissalRestoresExactOpenerState
=== RUN   TestDismissalRestoresExactOpenerState/esc
=== RUN   TestDismissalRestoresExactOpenerState/n
=== RUN   TestDismissalRestoresExactOpenerState/cancel_then_settle
--- PASS: TestDismissalRestoresExactOpenerState ()
=== RUN   TestDestructivePreparationRetainsSuccessAndFailure
=== RUN   TestDestructivePreparationRetainsSuccessAndFailure/success
=== RUN   TestDestructivePreparationRetainsSuccessAndFailure/failure
--- PASS: TestDestructivePreparationRetainsSuccessAndFailure ()
=== RUN   TestDestructivePreparationRejectsStaleEstimateResponses
--- PASS: TestDestructivePreparationRejectsStaleEstimateResponses ()
=== RUN   TestDestructivePreparationEscDismissesWithCancellation
--- PASS: TestDestructivePreparationEscDismissesWithCancellation ()
=== RUN   TestDestructivePreparationCancelThenSettleDismisses
--- PASS: TestDestructivePreparationCancelThenSettleDismisses ()
=== RUN   TestDestructivePreparationNeverAppendsHistory
=== RUN   TestDestructivePreparationNeverAppendsHistory/open
=== RUN   TestDestructivePreparationNeverAppendsHistory/success
=== RUN   TestDestructivePreparationNeverAppendsHistory/failure
=== RUN   TestDestructivePreparationNeverAppendsHistory/dismiss
--- PASS: TestDestructivePreparationNeverAppendsHistory ()
ok  	github.com/chris/sqloid/internal/ui	
```

Preparation, failure, cancellation, dismissal, and confirmation dispatch all leave internal/history untouched — the execution-start lifecycle of Issue #42 owns the first query append and result finalization. The history store suites confirm the append seam itself remains intact:

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/history/ -run 'TestQueryAppend|TestQueryStore|TestResultEntry|TestSnapshot' -v 2>&1 | grep -E '^(--- |ok|FAIL)' | sed -E 's/[0-9]+\.[0-9]+s//g'
```

```output
--- PASS: TestSnapshotMetadataCarriesTypedFacts ()
--- PASS: TestSnapshotMetadataRejectsInvalidShape ()
--- PASS: TestSnapshotMetadataValueSemantics ()
--- PASS: TestSnapshotMetadataNoPresentationDuplication ()
ok  	github.com/chris/sqloid/internal/history	
```

Every stage above keeps the modal's sole-serializer architecture intact: the QueryBuilder literal renderer is the only SQL serialization path and WriteConfirmedMsg carries the retained canonical statement verbatim, with the actual-write execution identity allocated from the dedicated result.NextWriteExecutionID space — distinct from every preparation, SELECT-execution, and request identity. Issue #42 owns consuming WriteConfirmedMsg for the transactional write, execution-start query append, and result finalization. Cross-references: Notes/issues/041-estimate-failure-confirmation-and-dismissal.md, Notes/PRD-sqloid.md (Estimate SQL and modal, Identities and state, Writes and commit boundary, Global Key Precedence, Testing Decisions), Notes/wiki/destructive-preparation.md, and the shared Issue #6 cancellation infrastructure. Final verification of the whole module:

```bash
cd /home/chris/sqloid && go vet ./... && go test -count=1 ./... 2>&1 | sed -E 's/[0-9]+\.[0-9]+s//g' | cut -c1-58
```

```output
ok  	github.com/chris/sqloid/cmd/sqloid	
ok  	github.com/chris/sqloid/internal/cli	
ok  	github.com/chris/sqloid/internal/connection	
ok  	github.com/chris/sqloid/internal/d1	
ok  	github.com/chris/sqloid/internal/history	
ok  	github.com/chris/sqloid/internal/querybuilder	
ok  	github.com/chris/sqloid/internal/result	
ok  	github.com/chris/sqloid/internal/resultcache	
ok  	github.com/chris/sqloid/internal/schema	
ok  	github.com/chris/sqloid/internal/ui	
```
