# Write-phase in-flight feedback (Issue #44)

Exact write-phase presentation mapping and the write-side integration of the Issue #27 generic request-in-flight gate in `internal/ui`. Issue #44 owns only presentation (the authoritative typed label mapping) and feeding the typed write and estimate state into the shared gate; the estimate/transaction/cancellation/commit lifecycle semantics remain with `internal/connection` (Issues #40–#43), the generic gate and its read integration remain with Issue #27, and read labels and SELECT/page/count behavior are unchanged. Cross-references: [in-flight-gating.md](in-flight-gating.md) (Issue #27 gate and read feedback), [destructive-preparation.md](destructive-preparation.md) (Issues #40–#41 estimate modal), [transactional-writes.md](transactional-writes.md) (Issue #42 lifecycle), [commit-boundary-quit-cleanup.md](commit-boundary-quit-cleanup.md) (Issue #43 boundary), [cancellation-infrastructure.md](cancellation-infrastructure.md), and the PRD's Writes and commit boundary, Global Key Precedence and Context/Action Matrix, UI/Connection/History Module Design, and Testing Decisions sections.

## Authoritative typed phase-to-label mapping

`Model.writePhaseStatus()` in `internal/ui/write_exec.go` is the single authoritative write-phase presentation mapping. It reads only typed state — `connection.WritePhase` retained in `writePhase`, the `writeCancelling` request flag, and `writePending` — never rendered text, command shape, or inferred strings:

- `WritePhaseRollbackCleanup` → exactly `Rolling back…` (`WriteRollingBackIndicator`).
- `WritePhaseCommitting` → exactly `Committing…` (`WriteCommittingIndicator`).
- Requested cancellation (`writeCancelling`) during a cancellable phase → exactly `cancelling…` (`SelectCancellingIndicator`), held until settlement.
- Cancellable beginning/executing (`writePending`) → exactly `Running…` (`SelectRunningIndicator`).
- No pending write → empty text.

Rollback cleanup and committing are the most specific typed phases and take precedence over the cancellation request. `View` renders the mapping as the results-region status while `writePending` (`internal/ui/view.go`), ahead of history/snapshot/result rendering, so the label stays visible through permitted local updates (resize redraws, horizontal one-column movement). The estimate phases keep their established modal labels: exactly `Estimating matching target rows…` (`DestructivePrepPendingStatus`) while pending and exactly `cancelling…` (`DestructivePrepCancellingStatus`) after a Ctrl+W request, both driven by `prepPending`/`prepCancelling`.

Identity guards make labels follow the current execution/request identity: `applyWritePhase` (Issue #42) discards stale executions and post-settlement arrivals, treats duplicate deliveries of the current phase as idempotent no-ops, and (Issue #43) refuses post-boundary phase regressions — so no stale, duplicate, or regressed message can replace, move, or un-cancel the visible label.

## Generic gate integration (no parallel write ladder)

`handleKey`'s gate condition is now `selectRequestPending() || writePending`, so every write phase reaches Issue #27's single authoritative gate — there is exactly one write precedence path; the former base-context write Ctrl+W branch was removed, never duplicated. The gate consumes, with no command dispatch and unchanged database request counts:

- **Enter** — consumed with `Model.inFlightEnterFeedback()`: for a pending write, the typed phase status plus the exact `press Ctrl+W to cancel` hint (`Running… — press Ctrl+W to cancel`, `cancelling… — press Ctrl+W to cancel`), or the exact `Commit in progress; cancellation is no longer available` boundary message once rollback cleanup or committing has begun. Requests never stack.
- **Ctrl+P/N, Ctrl+E/Y, Ctrl+S, Ctrl+X** — the established exact explanatory feedback (`query history is unavailable while a request is in flight`, `result history is unavailable while a request is in flight`, `saving is unavailable while a request is in flight`, `export is unavailable while a request is in flight`) via the shared `inFlightBlockedFeedback` mapping; query/result selection is unchanged.
- **`q` / Ctrl+C** — the shared quit confirmation (with Issue #43's accepted-quit write settlement behind it).
- **Ctrl+W** — routed by typed state only: in the cancellable phases one deduplicated cancellation request with exact `cancelling…` until settlement; in rollback cleanup/committing, refused with the exact boundary feedback and no interrupt. Routing never inspects label text.

Blocked feedback renders in the footer via `inFlightNotice` and clears on permitted keys exactly as for read phases. Permitted local interaction stays ungated: horizontal `,`/`.` movement (label preserved, notice cleared), page keys, field navigation, and base `?` help.

## Estimate-phase gating inside the preparation modal

While the estimate request is in flight (`prepPending` or `prepCancelling`), `handlePreparationKey` feeds the same gate vocabulary rather than a modal-only ladder: Enter/y stay consumed no-ops with `estimateEnterFeedback()` (`Estimating matching target rows… — press Ctrl+W to cancel` or the `cancelling…` variant); Ctrl+P/N, Ctrl+E/Y, Ctrl+S, and Ctrl+X carry the same shared blocked-action feedback; Ctrl+W keeps the scoped estimate cancellation; and `q`/Ctrl+C open the shared quit confirmation above the modal. No estimate or write request is ever dispatched by any of these keys, and settled preparations keep the Issue #41 confirmation behavior unchanged.

## Ownership split

- Issue #27 owns the generic gate, its precedence point, and all read-phase labels; no read label or SELECT/page/count behavior changed.
- Issues #40–#43 (with `internal/connection`) own estimate, transactional write, cancellation, commit-boundary, and quit-settlement lifecycle semantics; nothing moved out of `internal/connection`.
- Issue #44 owns write-phase presentation (the typed label mapping, its view rendering) and feeding typed write/estimate state into the shared gate; it implements no transaction orchestration of its own.

## Tests

- `internal/ui/write_feedback_test.go` — table-driven held-write fixtures over estimating, estimate cancellation, beginning, executing, cancellation-requested, rollback-cleanup, and committing: exact label rendering and persistence through resize; `cancelling…` held through typed rollback cleanup; stale/duplicate/post-boundary phase-message guards; every blocked action consumed with exact feedback, no command, unchanged fake call counts and focus; phase-appropriate Enter hints and the exact boundary feedback; Ctrl+W routing/dedupe/boundary; shared quit confirmation open and Esc restore per phase; permitted horizontal movement staying local with the label intact; and the Issue #27 read regression (`Running…`, `loading next page…`, `Counting rows…`, `Count unavailable`, read `cancelling…`).
- `internal/ui/destructive_prep_test.go` and `destructive_prep_confirmation_test.go` — estimate modal labels and confirmation behavior unchanged.
