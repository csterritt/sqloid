# Issue #27 — In-flight feedback and execution gating (SELECT/page/count) code walkthrough

*2026-08-28T11:25:41Z by Showboat 0.6.1*
<!-- showboat-id: fc9cc9a9-fd9c-4ec4-8758-5760ca4dc48b -->

Issue #27 wires the generic request-in-flight action gate and the read-request phase feedback of Notes/PRD-sqloid.md (Global Key Precedence and Context/Action Matrix; User Stories 13–14) into internal/ui. This walkthrough holds SELECT first-page, count, and later-page requests independently against the fake executor seams and demonstrates the exact contracts.

1. The generic gate: Enter and the history/save/export actions are consumed with no command dispatch while any read request is pending.

```bash
cd /home/chris/sqloid && go test ./internal/ui/ -run 'TestInFlightGateBlocksActionsForEveryPendingPhase' -v 2>&1 | tail -30
```

```output
=== RUN   TestInFlightGateBlocksActionsForEveryPendingPhase/count_pending/query_history_newer
=== RUN   TestInFlightGateBlocksActionsForEveryPendingPhase/count_pending/result_history_older
=== RUN   TestInFlightGateBlocksActionsForEveryPendingPhase/count_pending/result_history_newer
=== RUN   TestInFlightGateBlocksActionsForEveryPendingPhase/count_pending/save
=== RUN   TestInFlightGateBlocksActionsForEveryPendingPhase/count_pending/export
=== RUN   TestInFlightGateBlocksActionsForEveryPendingPhase/count_pending/enter_hint
--- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase (0.00s)
    --- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase/first_page_pending/query_history_older (0.00s)
    --- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase/first_page_pending/query_history_newer (0.00s)
    --- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase/first_page_pending/result_history_older (0.00s)
    --- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase/first_page_pending/result_history_newer (0.00s)
    --- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase/first_page_pending/save (0.00s)
    --- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase/first_page_pending/export (0.00s)
    --- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase/first_page_pending/enter_hint (0.00s)
    --- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase/later_page_pending/query_history_older (0.00s)
    --- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase/later_page_pending/query_history_newer (0.00s)
    --- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase/later_page_pending/result_history_older (0.00s)
    --- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase/later_page_pending/result_history_newer (0.00s)
    --- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase/later_page_pending/save (0.00s)
    --- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase/later_page_pending/export (0.00s)
    --- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase/later_page_pending/enter_hint (0.00s)
    --- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase/count_pending/query_history_older (0.00s)
    --- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase/count_pending/query_history_newer (0.00s)
    --- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase/count_pending/result_history_older (0.00s)
    --- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase/count_pending/result_history_newer (0.00s)
    --- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase/count_pending/save (0.00s)
    --- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase/count_pending/export (0.00s)
    --- PASS: TestInFlightGateBlocksActionsForEveryPendingPhase/count_pending/enter_hint (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.005s
```

Each phase (first page, later page, count pending) rejects every blocked action with its exact explanatory string and no command, and Enter feedback carries the phase wording plus the exact 'press Ctrl+W to cancel' hint.

2. No request stacking, permitted local interaction, quit confirmation, and scoped cancellation.

```bash
cd /home/chris/sqloid && go test ./internal/ui/ -run 'TestInFlightGateEnterNeverStacksRequests|TestInFlightGateKeepsHorizontalMovementLocal|TestInFlightGateQuitConfirmation|TestQuitConfirmationRestoresPendingPhase|TestInFlightGateCtrlWRoutesOnlyToCancellableRequests' -v 2>&1 | grep -E '^(=== RUN|--- PASS|--- FAIL|PASS|FAIL|ok)'
```

```output
=== RUN   TestInFlightGateEnterNeverStacksRequests
--- PASS: TestInFlightGateEnterNeverStacksRequests (0.00s)
=== RUN   TestInFlightGateKeepsHorizontalMovementLocal
--- PASS: TestInFlightGateKeepsHorizontalMovementLocal (0.00s)
=== RUN   TestInFlightGateQuitConfirmation
--- PASS: TestInFlightGateQuitConfirmation (0.00s)
=== RUN   TestQuitConfirmationRestoresPendingPhase
--- PASS: TestQuitConfirmationRestoresPendingPhase (0.00s)
=== RUN   TestInFlightGateCtrlWRoutesOnlyToCancellableRequests
--- PASS: TestInFlightGateCtrlWRoutesOnlyToCancellableRequests (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/ui	0.007s
```

3. Precedence: terminal, quit-confirmation, top-overlay, and focused-input contexts consume keys before the pending gate; and the gate derives its behavior from request-ownership flags, not phase-label strings.

```bash
cd /home/chris/sqloid && go test ./internal/ui/ -run 'TestHigherPrecedenceConsumesKeysBeforeInFlightGate|TestGateDerivesPendingFromOwnershipNotLabels' -v 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)'
```

```output
--- PASS: TestHigherPrecedenceConsumesKeysBeforeInFlightGate (0.00s)
--- PASS: TestGateDerivesPendingFromOwnershipNotLabels (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.005s
```

4. Read-phase labels: exact 'Running…', 'Counting rows…', the distinct page-loading state, settlement in either order, the Ctrl+W 'cancelling…' handoff, and count failure landing on the established 'Count unavailable'.

```bash
cd /home/chris/sqloid && go test ./internal/ui/ -run 'TestRunningFeedbackForInitialSelectPage|TestCountingFeedbackWhileCountPending|TestPageLoadingFeedbackIsDistinctFromCounting|TestFeedbackLabelsUpdateAsRequestsSettleInEitherOrder|TestFeedbackSurvivesPermittedLocalInteraction|TestCancellingHandoffRendersUntilSettlement|TestInFlightGateCountFailureKeepsExactWording' -v 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)'
```

```output
--- PASS: TestInFlightGateCountFailureKeepsExactWording (0.00s)
--- PASS: TestRunningFeedbackForInitialSelectPage (0.00s)
--- PASS: TestCountingFeedbackWhileCountPending (0.00s)
--- PASS: TestPageLoadingFeedbackIsDistinctFromCounting (0.00s)
--- PASS: TestFeedbackLabelsUpdateAsRequestsSettleInEitherOrder (0.00s)
--- PASS: TestCancellingHandoffRendersUntilSettlement (0.00s)
ok  	github.com/chris/sqloid/internal/ui	0.009s
```

5. Action-specific rejection wording and top-overlay consumption; plus the proof that the gate never inspects rendered labels (it reads request-ownership flags only) and that no write-phase label exists on any read path — Issue #44 owns write-phase feedback and the write-side integration of the generic gate.

5. Action-specific rejection wording and top-overlay consumption; plus the proof that the gate never inspects rendered labels (its decision input is request-ownership flags only) and that no write-phase label exists on any read path — Issue #44 owns write-phase feedback and the write-side integration of the generic gate.

```bash
cd /home/chris/sqloid && go test ./internal/ui/ -run 'TestFeedbackRejectionsAreActionSpecific|TestTopOverlayConsumesKeysBeforePendingFeedback|TestNoWritePhaseLabelsIntroduced' -v 2>&1 | grep -E '^(--- PASS|--- FAIL|ok|FAIL)' && echo '--- write-phase strings in production read-path UI sources (expect none):' && grep -rn 'Estimating matching\|beginning\|committing\|rollback' internal/ui/inflight_gate.go internal/ui/view.go internal/ui/results_grid.go internal/ui/model.go internal/ui/first_select.go internal/ui/paging.go internal/ui/count.go | wc -l
```

```output
--- PASS: TestFeedbackRejectionsAreActionSpecific (0.00s)
--- PASS: TestTopOverlayConsumesKeysBeforePendingFeedback (0.00s)
--- PASS: TestNoWritePhaseLabelsIntroduced (0.00s)
ok  	github.com/chris/sqloid/internal/ui	(cached)
--- write-phase strings in production read-path UI sources (expect none):
0
```

6. Full-suite proof: the complete verification pass (gofmt, vet, build, all tests) remains green with the new gate and feedback integrated.

```bash
cd /home/chris/sqloid && gofmt -l internal/ && go vet ./... && go test ./... 2>&1 | grep -E '^(ok|FAIL)' | head -12
```

```output
ok  	github.com/chris/sqloid/cmd/sqloid	(cached)
ok  	github.com/chris/sqloid/internal/cli	(cached)
ok  	github.com/chris/sqloid/internal/connection	(cached)
ok  	github.com/chris/sqloid/internal/d1	(cached)
ok  	github.com/chris/sqloid/internal/history	(cached)
ok  	github.com/chris/sqloid/internal/querybuilder	(cached)
ok  	github.com/chris/sqloid/internal/result	(cached)
ok  	github.com/chris/sqloid/internal/schema	(cached)
ok  	github.com/chris/sqloid/internal/ui	(cached)
```

Boundary statement: Issue #27 integrates only read requests — SELECT first-page, count, and later-page feedback and gating. Write-phase labels (estimating, beginning, executing, rollback, commit) and the write-side integration of the generic gate are owned by Issue #44; interrupt semantics for Ctrl+W remain with Issue #28. Reference: Notes/PRD-sqloid.md — Global Key Precedence and Context/Action Matrix, Execution and Result Lifecycle, and the related wiki page in-flight-gating.md.
