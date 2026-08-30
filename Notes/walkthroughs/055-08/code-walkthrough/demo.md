# Issue #055 Code Walkthrough: Universal Quit Confirmation and Exact Restoration

*2026-08-29T23:34:49Z by Showboat 0.6.1*
<!-- showboat-id: de971685-6b37-486c-8051-bf91f0983d12 -->

Issue #55 (PRD Notes/PRD-sqloid.md, 'Global Key Precedence and Context/Action Matrix'): q and Ctrl+C open one shared quit confirmation in every enabled nonterminal context, suspending the exact current context — typed overlay and opener, focus/cursor/search/highlight, builder/result/history selection, viewport and first visible column, active request/preparation identities and phases, immutable save/export state, destination/format/path, and the too-small wrapper. Focused text/search owns literal q; deletion, replacement, and outcome-unknown terminals quit immediately with status 1. Esc and n restore the latest identity-valid suspended state with no key leakage; Enter/y/Ctrl+C accept and exit only after all owned cleanup settles.

First, the full table-driven quit matrix: both quit keys through every scripted context, with the quitLiteralQ exception (value prompt, searchable popup), the quitTerminalStatus1 exceptions, context-signature checks behind the confirmation, and repeated-q no-ops.

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/ui -run 'TestQuitConfirmationMatrix' -v 2>&1 | grep -E '^(    --- PASS|--- (PASS|FAIL)|ok|FAIL)' | sed -E 's/ ?\(0\.[0-9]+s\)//; s/[0-9]+\.[0-9]+s$/X/; s/ +$//'
```

```output
--- PASS: TestQuitConfirmationMatrix
    --- PASS: TestQuitConfirmationMatrix/ordinary_base_builder
    --- PASS: TestQuitConfirmationMatrix/focused_value_prompt
    --- PASS: TestQuitConfirmationMatrix/searchable_popup
    --- PASS: TestQuitConfirmationMatrix/scroll-only_popup
    --- PASS: TestQuitConfirmationMatrix/contextual_help_overlay
    --- PASS: TestQuitConfirmationMatrix/first_page_pending
    --- PASS: TestQuitConfirmationMatrix/noncancellable_write_phase_pending
    --- PASS: TestQuitConfirmationMatrix/schema_validation_pending
    --- PASS: TestQuitConfirmationMatrix/query_history_mode
    --- PASS: TestQuitConfirmationMatrix/too-small_screen
    --- PASS: TestQuitConfirmationMatrix/deletion_terminal
    --- PASS: TestQuitConfirmationMatrix/replacement_terminal
    --- PASS: TestQuitConfirmationMatrix/outcome-unknown_terminal
ok  	github.com/chris/sqloid/internal/ui	X
```

Every nonterminal context — ordinary base builder, scroll-only popup, contextual help overlay, first-page pending, noncancellable write phase, schema validation, query history, and the too-small screen — opens exactly one confirmation; focused text/search keeps literal q while Ctrl+C still opens it; the three terminals exit with status 1 and no confirmation. Repeated q inside the confirmation is a consumed no-op:

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/ui -run 'TestQuitConfirmationRepeatedOpensSuspended' -v 2>&1 | grep -E '^(    --- PASS|--- (PASS|FAIL)|ok|FAIL)' | sed -E 's/ ?\(0\.[0-9]+s\)//; s/[0-9]+\.[0-9]+s$/X/; s/ +$//'
```

```output
--- PASS: TestQuitConfirmationRepeatedOpensSuspended
    --- PASS: TestQuitConfirmationRepeatedOpensSuspended/ordinary_base_builder
    --- PASS: TestQuitConfirmationRepeatedOpensSuspended/scroll-only_popup
    --- PASS: TestQuitConfirmationRepeatedOpensSuspended/contextual_help_overlay
    --- PASS: TestQuitConfirmationRepeatedOpensSuspended/first_page_pending
    --- PASS: TestQuitConfirmationRepeatedOpensSuspended/noncancellable_write_phase_pending
    --- PASS: TestQuitConfirmationRepeatedOpensSuspended/schema_validation_pending
    --- PASS: TestQuitConfirmationRepeatedOpensSuspended/query_history_mode
    --- PASS: TestQuitConfirmationRepeatedOpensSuspended/too-small_screen
ok  	github.com/chris/sqloid/internal/ui	X
```

Cancellation with Esc and n from every suspended nonterminal context restores the exact context signature — helpOpen, popup opener/search/mode, prompt buffer/cursor, prep/write pending state, history browsing, result first column, too-small wrapper, builder focus/scroll/fields — with no command and no leakage:

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/ui -run 'TestQuitCancellationExactRestoration' -v 2>&1 | grep -E '^(    --- PASS|--- (PASS|FAIL)|ok|FAIL)' | sed -E 's/ ?\(0\.[0-9]+s\)//; s/[0-9]+\.[0-9]+s$/X/; s/ +$//'
```

```output
--- PASS: TestQuitCancellationExactRestoration
    --- PASS: TestQuitCancellationExactRestoration/ordinary_base_builder
    --- PASS: TestQuitCancellationExactRestoration/focused_value_prompt
    --- PASS: TestQuitCancellationExactRestoration/searchable_popup
    --- PASS: TestQuitCancellationExactRestoration/scroll-only_popup
    --- PASS: TestQuitCancellationExactRestoration/contextual_help_overlay
    --- PASS: TestQuitCancellationExactRestoration/first_page_pending
    --- PASS: TestQuitCancellationExactRestoration/noncancellable_write_phase_pending
    --- PASS: TestQuitCancellationExactRestoration/schema_validation_pending
    --- PASS: TestQuitCancellationExactRestoration/query_history_mode
    --- PASS: TestQuitCancellationExactRestoration/too-small_screen
ok  	github.com/chris/sqloid/internal/ui	X
```

The dismissal key is consumed by quit alone: in contexts whose overlays own Esc (help, value prompt, scroll-only popup), the overlay remains open immediately after cancellation, and a second Esc/n is thereafter the restored context's ordinary key:

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/ui -run 'TestQuitCancellationDoesNotDismissRestoredOverlay|TestQuitCancellationTwiceIsOrdinary' -v 2>&1 | grep -E '^(    --- PASS|--- (PASS|FAIL)|ok|FAIL)' | sed -E 's/ ?\(0\.[0-9]+s\)//; s/[0-9]+\.[0-9]+s$/X/; s/ +$//'
```

```output
--- PASS: TestQuitCancellationDoesNotDismissRestoredOverlay
    --- PASS: TestQuitCancellationDoesNotDismissRestoredOverlay/help_overlay
    --- PASS: TestQuitCancellationDoesNotDismissRestoredOverlay/value_prompt
    --- PASS: TestQuitCancellationDoesNotDismissRestoredOverlay/scroll-only_popup
--- PASS: TestQuitCancellationTwiceIsOrdinary
ok  	github.com/chris/sqloid/internal/ui	X
```

Accepted quit during write phases reuses Issue #43's settlement coordinator: cancellable beginning/executing phases request one cancellation and wait through confirmed rollback; noncancellable rollback-cleanup/committing phases issue no interrupt and wait for resolution; unresolved (outcome-unknown) metadata finalizes only after all transaction and driver work ends; duplicate acceptance, stale settlement, and decline are idempotent or restore the exact phase; and quit-wait prohibits replacement work. No exit is emitted while any work remains:

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/ui -run 'TestWriteGateQuitConfirmationPerPhase|TestAcceptedQuitDuring|TestAcceptedQuitResolves|TestQuitWaitProhibitsReplacementWork' -v 2>&1 | grep -E '^(    --- PASS|--- (PASS|FAIL)|ok|FAIL)' | sed -E 's/ ?\(0\.[0-9]+s\)//; s/[0-9]+\.[0-9]+s$/X/; s/ +$//'
```

```output
--- PASS: TestWriteGateQuitConfirmationPerPhase
    --- PASS: TestWriteGateQuitConfirmationPerPhase/estimating/q
    --- PASS: TestWriteGateQuitConfirmationPerPhase/estimating/ctrl+c
    --- PASS: TestWriteGateQuitConfirmationPerPhase/estimate_cancellation_requested/q
    --- PASS: TestWriteGateQuitConfirmationPerPhase/estimate_cancellation_requested/ctrl+c
    --- PASS: TestWriteGateQuitConfirmationPerPhase/beginning/q
    --- PASS: TestWriteGateQuitConfirmationPerPhase/beginning/ctrl+c
    --- PASS: TestWriteGateQuitConfirmationPerPhase/executing/q
    --- PASS: TestWriteGateQuitConfirmationPerPhase/executing/ctrl+c
    --- PASS: TestWriteGateQuitConfirmationPerPhase/cancellation_requested/q
    --- PASS: TestWriteGateQuitConfirmationPerPhase/cancellation_requested/ctrl+c
    --- PASS: TestWriteGateQuitConfirmationPerPhase/rollback_cleanup/q
    --- PASS: TestWriteGateQuitConfirmationPerPhase/rollback_cleanup/ctrl+c
    --- PASS: TestWriteGateQuitConfirmationPerPhase/committing/q
    --- PASS: TestWriteGateQuitConfirmationPerPhase/committing/ctrl+c
--- PASS: TestAcceptedQuitDuringCancellableWriteWaitsForRollback
--- PASS: TestAcceptedQuitDuringNoncancellablePhasesWaitsForResolution
    --- PASS: TestAcceptedQuitDuringNoncancellablePhasesWaitsForResolution/committing_commits
    --- PASS: TestAcceptedQuitDuringNoncancellablePhasesWaitsForResolution/rollback_cleanup_resolves_failed
--- PASS: TestAcceptedQuitResolvesUnresolvedOutcomesWithoutAbandoningWork
    --- PASS: TestAcceptedQuitResolvesUnresolvedOutcomesWithoutAbandoningWork/unresolved_rollback
    --- PASS: TestAcceptedQuitResolvesUnresolvedOutcomesWithoutAbandoningWork/unresolved_commit
--- PASS: TestQuitWaitProhibitsReplacementWork
ok  	github.com/chris/sqloid/internal/ui	X
```

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/ui -run 'TestFinalizersEndActiveSelectExactlyOnce|TestLateMessagesAfterFinalizationAreInert|TestQuit|TestAcceptedQuit' 2>&1 | tail -1 | sed -E 's/[0-9]+\.[0-9]+s/X/' && go test -count=1 ./internal/ui 2>&1 | tail -1 | sed -E 's/[0-9]+\.[0-9]+s/X/'
```

```output
ok  	github.com/chris/sqloid/internal/ui	X
ok  	github.com/chris/sqloid/internal/ui	X
```
