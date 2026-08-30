# Issue #45: Outcome-unknown terminal workflow

*2026-08-29T16:24:19Z by Showboat 0.6.1*
<!-- showboat-id: d065a136-324d-4803-b3b6-2b5576e70fc6 -->

Issue #45 (Notes/PRD-sqloid.md 'Writes and commit boundary', 'Global Key Precedence and Context/Action Matrix') completes the write lifecycle's fourth terminal classification: when a settled write's rollback or commit cannot be resolved, the application holds the unresolved resolution until all transaction and driver work has ended, appends exactly one immutable non-tabular outcome-unknown entry, selects it as the newest result, and enters the outcome-unknown terminal state that forbids every database action, preserves in-memory history navigation, shows reduced help, and quits immediately with status 1. First: the exactly-once immutable entry and its non-persistence wording.

```bash
cd /home/chris/sqloid && go test ./internal/history -run 'TestOutcomeUnknown|TestUnknownPhaseString' -count=1 -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestOutcomeUnknownEntryRetainsUnresolvedFacts
--- PASS: TestOutcomeUnknownEntryRetainsUnresolvedFacts (Ts)
=== RUN   TestOutcomeUnknownEntryRollbackPhase
--- PASS: TestOutcomeUnknownEntryRollbackPhase (Ts)
=== RUN   TestOutcomeUnknownSummaryPhasesAndRows
=== RUN   TestOutcomeUnknownSummaryPhasesAndRows/unresolved_commit_with_rows_does_not_prove_persistence
=== RUN   TestOutcomeUnknownSummaryPhasesAndRows/unresolved_commit_without_a_row_count_makes_no_persistence_claim
=== RUN   TestOutcomeUnknownSummaryPhasesAndRows/unresolved_rollback_with_rows_does_not_prove_persistence
=== RUN   TestOutcomeUnknownSummaryPhasesAndRows/unresolved_rollback_without_a_row_count_makes_no_persistence_claim
--- PASS: TestOutcomeUnknownSummaryPhasesAndRows (Ts)
    --- PASS: TestOutcomeUnknownSummaryPhasesAndRows/unresolved_commit_with_rows_does_not_prove_persistence (Ts)
    --- PASS: TestOutcomeUnknownSummaryPhasesAndRows/unresolved_commit_without_a_row_count_makes_no_persistence_claim (Ts)
    --- PASS: TestOutcomeUnknownSummaryPhasesAndRows/unresolved_rollback_with_rows_does_not_prove_persistence (Ts)
    --- PASS: TestOutcomeUnknownSummaryPhasesAndRows/unresolved_rollback_without_a_row_count_makes_no_persistence_claim (Ts)
=== RUN   TestUnknownPhaseString
--- PASS: TestUnknownPhaseString (Ts)
=== RUN   TestOutcomeUnknownKindString
--- PASS: TestOutcomeUnknownKindString (Ts)
PASS
ok  	github.com/chris/sqloid/internal/history
```

The history tests prove the entry retains the outcome-unknown status, operation and table, exact executed SQL, commit-versus-rollback phase, driver error, and the optional statement RowsAffected with wording that explicitly says it does not prove persistence — and that a duplicate finalization for the same execution appends nothing. Next: barrier-controlled settlement in internal/ui. The phases are drained while settlement is withheld, proving no entry and no terminal state exist while transaction or driver work remains pending; only the settlement that arrives after the phases channel closed creates exactly one newest, initially selected entry.

```bash
cd /home/chris/sqloid && go test ./internal/ui -run 'TestUnresolved' -count=1 -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestUnresolvedCommitSettlesIntoTerminalEntry
--- PASS: TestUnresolvedCommitSettlesIntoTerminalEntry (Ts)
=== RUN   TestUnresolvedRollbackSettlesIntoTerminalEntry
--- PASS: TestUnresolvedRollbackSettlesIntoTerminalEntry (Ts)
=== RUN   TestUnresolvedInsertSettlesIntoTerminalEntry
--- PASS: TestUnresolvedInsertSettlesIntoTerminalEntry (Ts)
=== RUN   TestUnresolvedSettlementIsIdempotentAndStaleProof
--- PASS: TestUnresolvedSettlementIsIdempotentAndStaleProof (Ts)
PASS
ok  	github.com/chris/sqloid/internal/ui
```

Settlement covers INSERT, UPDATE, and DELETE: each ends in exactly one outcome-unknown entry carrying the kind, execution identity, operation/table, exact executed SQL, commit-versus-rollback phase, driver error, and non-proving RowsAffected wording, with duplicate, late, and stale settlement messages inert. Next: the terminal state's restrictions and navigation — every database-capable action yields zero commands and zero fake-executor requests, Ctrl+P/N and Ctrl+E/Y traverse the retained histories entirely in memory with deterministic boundaries and empty-history no-ops, and the reduced help lists only available actions.

```bash
cd /home/chris/sqloid && go test ./internal/ui -run 'TestTerminal' -count=1 -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestTerminalStateForbidsDatabaseWork
--- PASS: TestTerminalStateForbidsDatabaseWork (Ts)
=== RUN   TestTerminalQueryHistoryNavigation
--- PASS: TestTerminalQueryHistoryNavigation (Ts)
=== RUN   TestTerminalQueryHistoryEmptyIsNoOp
--- PASS: TestTerminalQueryHistoryEmptyIsNoOp (Ts)
=== RUN   TestTerminalResultHistoryNavigation
--- PASS: TestTerminalResultHistoryNavigation (Ts)
=== RUN   TestTerminalResultHistoryDefensiveEmptyFallback
--- PASS: TestTerminalResultHistoryDefensiveEmptyFallback (Ts)
=== RUN   TestTerminalReducedHelp
--- PASS: TestTerminalReducedHelp (Ts)
=== RUN   TestTerminalQuitFromPrimaryView
--- PASS: TestTerminalQuitFromPrimaryView (Ts)
=== RUN   TestTerminalQuitFromQueryHistory
--- PASS: TestTerminalQuitFromQueryHistory (Ts)
=== RUN   TestTerminalQuitFromResultHistory
--- PASS: TestTerminalQuitFromResultHistory (Ts)
=== RUN   TestTerminalQuitFromHelp
--- PASS: TestTerminalQuitFromHelp (Ts)
=== RUN   TestTerminalQuitWithEmptyHistories
--- PASS: TestTerminalQuitWithEmptyHistories (Ts)
=== RUN   TestTerminalHealthOverridesLockError
--- PASS: TestTerminalHealthOverridesLockError (Ts)
=== RUN   TestTerminalOverridesValidation
--- PASS: TestTerminalOverridesValidation (Ts)
PASS
ok  	github.com/chris/sqloid/internal/ui
```

Finally, the immediate quit: from the primary view, selected query/result history, the reduced help, and empty histories, q and Ctrl+C return exactly tea.Quit with exit status 1 — no confirmation overlay, no cancellation request, no cleanup command, no delayed settlement, and no state restoration, because terminal entry already guarantees nothing remains pending; repeated keys re-assert the same exit.

```bash
cd /home/chris/sqloid && go test ./internal/ui -run 'TestTerminalQuit' -count=1 -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestTerminalQuitFromPrimaryView
--- PASS: TestTerminalQuitFromPrimaryView (Ts)
=== RUN   TestTerminalQuitFromQueryHistory
--- PASS: TestTerminalQuitFromQueryHistory (Ts)
=== RUN   TestTerminalQuitFromResultHistory
--- PASS: TestTerminalQuitFromResultHistory (Ts)
=== RUN   TestTerminalQuitFromHelp
--- PASS: TestTerminalQuitFromHelp (Ts)
=== RUN   TestTerminalQuitWithEmptyHistories
--- PASS: TestTerminalQuitWithEmptyHistories (Ts)
PASS
ok  	github.com/chris/sqloid/internal/ui
```

```bash
cd /home/chris/sqloid && go test ./internal/history ./internal/ui -count=1 2>&1 | sed -E 's/[0-9.]+s$/Ts/'
```

```output
ok  	github.com/chris/sqloid/internal/history	Ts
ok  	github.com/chris/sqloid/internal/ui	Ts
```

Every database-capable terminal action above was asserted against wired counting fakes (select, count, page, refresh, estimate) with zero requests and zero returned commands. The reduced help contents, the entry's non-persistence wording, and the immediate quit are also asserted verbatim in the tests. Ctrl+S save and Ctrl+X export integration inside this terminal state are deliberately not implemented here — Issues #48 and #49 own them. Reference: Issue #45, Notes/PRD-sqloid.md 'Writes and commit boundary' and the Global Key Precedence terminal row; wiki page Notes/wiki/outcome-unknown-terminal.md.
