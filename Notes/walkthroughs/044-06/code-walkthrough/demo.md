# Issue #44: Write-phase in-flight feedback

*2026-08-29T15:52:30Z by Showboat 0.6.1*
<!-- showboat-id: 5834182e-6723-4540-bc72-a46f86d30709 -->

Issue #44 (Notes/PRD-sqloid.md 'Writes and commit boundary', 'Global Key Precedence and Context/Action Matrix') adds exact write-phase in-flight feedback in internal/ui: beginning/executing render exactly Running…, the estimate renders exactly Estimating matching target rows…, committing exactly Committing…, rollback cleanup exactly Rolling back…, and a requested cancellation exactly cancelling… until settlement. The labels come from one authoritative typed mapping (Model.writePhaseStatus over connection.WritePhase, writeCancelling, writePending), and every write phase feeds Issue #27's single generic gate — no parallel write-only precedence ladder, no label inspection. Test durations below are normalized for deterministic verification.

```bash
cd /home/chris/sqloid && go test ./internal/ui -run '^TestWritePhaseLabelsRenderExactlyFromTypedState$|^TestWriteCancellingHoldsUntilSettlement$' -count=1 -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestWritePhaseLabelsRenderExactlyFromTypedState
=== RUN   TestWritePhaseLabelsRenderExactlyFromTypedState/estimating
=== RUN   TestWritePhaseLabelsRenderExactlyFromTypedState/estimate_cancellation_requested
=== RUN   TestWritePhaseLabelsRenderExactlyFromTypedState/beginning
=== RUN   TestWritePhaseLabelsRenderExactlyFromTypedState/executing
=== RUN   TestWritePhaseLabelsRenderExactlyFromTypedState/cancellation_requested
=== RUN   TestWritePhaseLabelsRenderExactlyFromTypedState/rollback_cleanup
=== RUN   TestWritePhaseLabelsRenderExactlyFromTypedState/committing
--- PASS: TestWritePhaseLabelsRenderExactlyFromTypedState (Ts)
    --- PASS: TestWritePhaseLabelsRenderExactlyFromTypedState/estimating (Ts)
    --- PASS: TestWritePhaseLabelsRenderExactlyFromTypedState/estimate_cancellation_requested (Ts)
    --- PASS: TestWritePhaseLabelsRenderExactlyFromTypedState/beginning (Ts)
    --- PASS: TestWritePhaseLabelsRenderExactlyFromTypedState/executing (Ts)
    --- PASS: TestWritePhaseLabelsRenderExactlyFromTypedState/cancellation_requested (Ts)
    --- PASS: TestWritePhaseLabelsRenderExactlyFromTypedState/rollback_cleanup (Ts)
    --- PASS: TestWritePhaseLabelsRenderExactlyFromTypedState/committing (Ts)
=== RUN   TestWriteCancellingHoldsUntilSettlement
--- PASS: TestWriteCancellingHoldsUntilSettlement (Ts)
PASS
ok  	github.com/chris/sqloid/internal/ui
```

Every phase fixture is held against the controllable fake write executor and preparation modal seams with no sleeps, and the exact label is captured in each phase and kept visible through a permitted local resize redraw. Requested cancellation holds cancelling… through the subsequent typed rollback-cleanup transition.

```bash
cd /home/chris/sqloid && go test ./internal/ui -run '^TestWriteLabelsResistStaleAndDuplicatePhaseMessages$|^TestEstimateLabelsStayExact$' -count=1 -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestWriteLabelsResistStaleAndDuplicatePhaseMessages
--- PASS: TestWriteLabelsResistStaleAndDuplicatePhaseMessages (Ts)
=== RUN   TestEstimateLabelsStayExact
--- PASS: TestEstimateLabelsStayExact (Ts)
PASS
ok  	github.com/chris/sqloid/internal/ui
```

Labels follow the current execution/request identity: a stale execution's phase message, a duplicate of the current phase, and a post-boundary regression attempt cannot replace or move the visible label backward (applyWritePhase guards), so feedback can neither regress nor be overwritten by stale or duplicate phase messages.

```bash
cd /home/chris/sqloid && go test ./internal/ui -run '^TestReadPhaseLabelsUnchanged$' -count=1 -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestReadPhaseLabelsUnchanged
--- PASS: TestReadPhaseLabelsUnchanged (Ts)
PASS
ok  	github.com/chris/sqloid/internal/ui
```

Read-request regression: Issue #27's SELECT first-page Running…, page loading, count, count-unavailable, and read cancellation labels are asserted unchanged — Issue #44 touches no read label.

Action gating across every write phase: the table-driven fixtures hold estimating, estimate cancellation, beginning, executing, cancellation-requested, rollback-cleanup, and committing, then exercise Ctrl+P/N (query history), Ctrl+E/Y (result history), Ctrl+S (save), and Ctrl+X (export) in each. Every blocked action is consumed with its exact explanatory feedback, no command dispatch, the fake write executor's call count unchanged, and focus/phase label untouched — no stacked execution and no second estimate.

```bash
cd /home/chris/sqloid && go test ./internal/ui -run '^TestWriteGateBlocksActionsPerPhase$' -count=1 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
ok  	github.com/chris/sqloid/internal/ui
```

Enter is consumed in every phase with phase-appropriate Ctrl+W guidance: the phase status plus the exact 'press Ctrl+W to cancel' hint for cancellable phases, and the exact 'Commit in progress; cancellation is no longer available' boundary message for rollback cleanup and committing.

```bash
cd /home/chris/sqloid && go test ./internal/ui -run '^TestWriteGateEnterFeedbackPerPhase$|^TestWriteGateCtrlWPerPhase$' -count=1 -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestWriteGateEnterFeedbackPerPhase
=== RUN   TestWriteGateEnterFeedbackPerPhase/estimating
=== RUN   TestWriteGateEnterFeedbackPerPhase/estimate_cancellation_requested
=== RUN   TestWriteGateEnterFeedbackPerPhase/beginning
=== RUN   TestWriteGateEnterFeedbackPerPhase/executing
=== RUN   TestWriteGateEnterFeedbackPerPhase/cancellation_requested
=== RUN   TestWriteGateEnterFeedbackPerPhase/rollback_cleanup
=== RUN   TestWriteGateEnterFeedbackPerPhase/committing
--- PASS: TestWriteGateEnterFeedbackPerPhase (Ts)
    --- PASS: TestWriteGateEnterFeedbackPerPhase/estimating (Ts)
    --- PASS: TestWriteGateEnterFeedbackPerPhase/estimate_cancellation_requested (Ts)
    --- PASS: TestWriteGateEnterFeedbackPerPhase/beginning (Ts)
    --- PASS: TestWriteGateEnterFeedbackPerPhase/executing (Ts)
    --- PASS: TestWriteGateEnterFeedbackPerPhase/cancellation_requested (Ts)
    --- PASS: TestWriteGateEnterFeedbackPerPhase/rollback_cleanup (Ts)
    --- PASS: TestWriteGateEnterFeedbackPerPhase/committing (Ts)
=== RUN   TestWriteGateCtrlWPerPhase
=== RUN   TestWriteGateCtrlWPerPhase/estimating
=== RUN   TestWriteGateCtrlWPerPhase/estimate_cancellation_requested
=== RUN   TestWriteGateCtrlWPerPhase/beginning
=== RUN   TestWriteGateCtrlWPerPhase/executing
=== RUN   TestWriteGateCtrlWPerPhase/cancellation_requested
=== RUN   TestWriteGateCtrlWPerPhase/rollback_cleanup
=== RUN   TestWriteGateCtrlWPerPhase/committing
--- PASS: TestWriteGateCtrlWPerPhase (Ts)
    --- PASS: TestWriteGateCtrlWPerPhase/estimating (Ts)
    --- PASS: TestWriteGateCtrlWPerPhase/estimate_cancellation_requested (Ts)
    --- PASS: TestWriteGateCtrlWPerPhase/beginning (Ts)
    --- PASS: TestWriteGateCtrlWPerPhase/executing (Ts)
    --- PASS: TestWriteGateCtrlWPerPhase/cancellation_requested (Ts)
    --- PASS: TestWriteGateCtrlWPerPhase/rollback_cleanup (Ts)
    --- PASS: TestWriteGateCtrlWPerPhase/committing (Ts)
PASS
ok  	github.com/chris/sqloid/internal/ui
```

Ctrl+W routing: cancellable estimate/beginning/executing dispatch exactly one deduplicated cancellation with exact cancelling… until settlement; rollback cleanup and committing issue no interrupt and give only the exact boundary feedback. Repeat Ctrl+W during an already-requested cancellation is an idempotent no-op with no second interrupt.

```bash
cd /home/chris/sqloid && go test ./internal/ui -run '^TestWriteGateQuitConfirmationPerPhase$|^TestWriteGateKeepsLocalInteractionDuringWrite$' -count=1 -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestWriteGateQuitConfirmationPerPhase
=== RUN   TestWriteGateQuitConfirmationPerPhase/estimating/q
=== RUN   TestWriteGateQuitConfirmationPerPhase/estimating/ctrl+c
=== RUN   TestWriteGateQuitConfirmationPerPhase/estimate_cancellation_requested/q
=== RUN   TestWriteGateQuitConfirmationPerPhase/estimate_cancellation_requested/ctrl+c
=== RUN   TestWriteGateQuitConfirmationPerPhase/beginning/q
=== RUN   TestWriteGateQuitConfirmationPerPhase/beginning/ctrl+c
=== RUN   TestWriteGateQuitConfirmationPerPhase/executing/q
=== RUN   TestWriteGateQuitConfirmationPerPhase/executing/ctrl+c
=== RUN   TestWriteGateQuitConfirmationPerPhase/cancellation_requested/q
=== RUN   TestWriteGateQuitConfirmationPerPhase/cancellation_requested/ctrl+c
=== RUN   TestWriteGateQuitConfirmationPerPhase/rollback_cleanup/q
=== RUN   TestWriteGateQuitConfirmationPerPhase/rollback_cleanup/ctrl+c
=== RUN   TestWriteGateQuitConfirmationPerPhase/committing/q
=== RUN   TestWriteGateQuitConfirmationPerPhase/committing/ctrl+c
--- PASS: TestWriteGateQuitConfirmationPerPhase (Ts)
    --- PASS: TestWriteGateQuitConfirmationPerPhase/estimating/q (Ts)
    --- PASS: TestWriteGateQuitConfirmationPerPhase/estimating/ctrl+c (Ts)
    --- PASS: TestWriteGateQuitConfirmationPerPhase/estimate_cancellation_requested/q (Ts)
    --- PASS: TestWriteGateQuitConfirmationPerPhase/estimate_cancellation_requested/ctrl+c (Ts)
    --- PASS: TestWriteGateQuitConfirmationPerPhase/beginning/q (Ts)
    --- PASS: TestWriteGateQuitConfirmationPerPhase/beginning/ctrl+c (Ts)
    --- PASS: TestWriteGateQuitConfirmationPerPhase/executing/q (Ts)
    --- PASS: TestWriteGateQuitConfirmationPerPhase/executing/ctrl+c (Ts)
    --- PASS: TestWriteGateQuitConfirmationPerPhase/cancellation_requested/q (Ts)
    --- PASS: TestWriteGateQuitConfirmationPerPhase/cancellation_requested/ctrl+c (Ts)
    --- PASS: TestWriteGateQuitConfirmationPerPhase/rollback_cleanup/q (Ts)
    --- PASS: TestWriteGateQuitConfirmationPerPhase/rollback_cleanup/ctrl+c (Ts)
    --- PASS: TestWriteGateQuitConfirmationPerPhase/committing/q (Ts)
    --- PASS: TestWriteGateQuitConfirmationPerPhase/committing/ctrl+c (Ts)
=== RUN   TestWriteGateKeepsLocalInteractionDuringWrite
--- PASS: TestWriteGateKeepsLocalInteractionDuringWrite (Ts)
PASS
ok  	github.com/chris/sqloid/internal/ui
```

q/Ctrl+C in every phase open the shared quit confirmation (Esc restoring the exact suspended pending context), and permitted local interaction stays responsive: horizontal one-column movement is ungated, clears stale rejection feedback, and keeps the phase label visible.

Evidence that the generic gating never inspects labels: the Issue #27 gate proves pending state is derived from request-ownership flags rather than rendered phase-label strings (TestGateDerivesPendingFromOwnershipNotLabels), and the write-side integration routes only by typed state (writePending, writeNoncancellable, writeCancelling) — writePhaseStatus appears in the gate only for Enter feedback composition, never in routing.

```bash
cd /home/chris/sqloid && go test ./internal/ui -run '^TestGateDerivesPendingFromOwnershipNotLabels$|^TestHigherPrecedenceConsumesKeysBeforeInFlightGate$' -count=1 -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/' && grep -n 'writePhaseStatus' internal/ui/inflight_gate.go && grep -c 'writePhaseStatus' internal/ui/destructive_prep.go || true
```

```output
=== RUN   TestHigherPrecedenceConsumesKeysBeforeInFlightGate
--- PASS: TestHigherPrecedenceConsumesKeysBeforeInFlightGate (Ts)
=== RUN   TestGateDerivesPendingFromOwnershipNotLabels
--- PASS: TestGateDerivesPendingFromOwnershipNotLabels (Ts)
PASS
ok  	github.com/chris/sqloid/internal/ui
78:		return m.writePhaseStatus() + " — " + CancelHintSuffix
0
```

Finally, the full module suite stays green: the write-phase mapping and gate integration regress nothing across internal/connection, internal/history, internal/ui, and the rest.

```bash
cd /home/chris/sqloid && go test ./... 2>&1 | grep -v '^ok' ; go test ./... 2>&1 | grep -c '^ok'
```

```output
10
```
