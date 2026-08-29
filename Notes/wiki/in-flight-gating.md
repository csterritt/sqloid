# In-flight feedback and execution gating (Issue #27)

Generic request-in-flight action gating in `internal/ui`, plus responsive read-request phase feedback for SELECT first-page, count, and later-page work. The gate is a single authoritative precedence point between focused input/overlays and base-context handling; its behavior derives from request-ownership flags, never from rendered phase-label strings. Cross-references: [concurrent-page-count.md](concurrent-page-count.md) (Issue #24), [serialized-vertical-paging.md](serialized-vertical-paging.md) (Issue #25), [select-request-identities.md](select-request-identities.md) (Issue #26), [cancellation-infrastructure.md](cancellation-infrastructure.md) (Issues #6/#28), [responsive-tui-shell.md](responsive-tui-shell.md) (Issue #8), and Issue #44, which owns write-phase labels and the write-side integration of the generic gate.

## Generic request-in-flight gate

`Model.selectRequestPending()` is the single authoritative pending source: it reads only `firstPagePending` (the first-page claim installed at execution start), `pagePending` (Issue #25's serialized page slot), and `countPendingFlag` (Issue #24's count claim) — never `Running…`, `Counting rows…`, or any other rendered text. The same gate serves SELECT first-page, later-page, and count requests without per-phase code paths.

Placement follows the PRD's Global Key Precedence: **terminal state → quit confirmation → top overlay (popup/value prompt) → focused text/search input → request-in-flight gate → base context**. In `handleKey`, the gate runs after the terminal, quit-confirmation, popup, and value-prompt branches and before the stale-schema/base-context switch. Higher contexts consume keys first: terminal states ignore everything, an open quit confirmation consumes all keys, a popup or focused text/search input keeps its own Enter/printable behavior, and the gate only sees keys that reached the otherwise-base context.

While any SELECT read request is in flight, the gate consumes with **no command dispatch** (request counts can therefore never change):

- **Enter** — consumed with contextual phase feedback plus the exact `press Ctrl+W to cancel` hint: `Running… — press Ctrl+W to cancel` while the first page is pending, `loading next page… — press Ctrl+W to cancel` during later-page work, `Counting rows… — press Ctrl+W to cancel` while the count is pending. Requests never stack.
- **Ctrl+P/N** — exact `query history is unavailable while a request is in flight`.
- **Ctrl+E/Y** — exact `result history is unavailable while a request is in flight`.
- **Ctrl+S** — exact `saving is unavailable while a request is in flight`.
- **Ctrl+X** — exact `export is unavailable while a request is in flight`.
- **`q` / Ctrl+C** — open the shared quit confirmation.
- **Ctrl+W** — routed to the scoped cancellation command only while `ActiveCancellable` owns a request; with no ownership the key is ignored with no state change. The command dispatches exactly once per press.

Every blocked action records its explanatory feedback in `Model.inFlightNotice`, which `renderFooter` renders in the footer row in place of the default ` q quit ? help ` hints. The notice is strictly transient: it clears when any permitted key is handled while idle and whenever a request settles.

Permitted local interaction falls through to base handling unchanged: serialized Page Up/Down keys (still governed by Issue #25's one-pending-page rule), Tab/arrow field navigation, Backspace/Delete field clearing, and the horizontal one-column movement keys `,`/`.` (whose scrolling behavior Issue #29 owns — the gate leaves them strictly local and never produces feedback for them). Base `?` help also stays reachable per the matrix, and no gate action ever mutates builder state.

## Shared quit confirmation

`q` and Ctrl+C in every nonterminal base or gate-reached context open the same shared confirmation (`Model.quitConfirm` with a full `quitSuspended` snapshot). It renders a `Quit Sqloid?` box with `Enter/y/Ctrl+C confirm quit — Esc/n cancel` over the composed shell; all other keys are consumed with no leakage. Enter/y/Ctrl+C confirm: `deactivateActiveSelect` closes the active SELECT response window (Issue #26's generation bump makes every in-flight response inert — the lifecycle cleanup) and `tea.Quit` is returned. Esc/n restores the exact suspended context — focus, pending ownership, gate feedback, and all state — with no key leakage. Issue #55 will generalize suspension of overlays and pickers; the confirmation itself and its restoration are already in place.

## Scoped Ctrl+W cancellation handoff

When Ctrl+W reaches a cancellable owned request, the model marks `selectCancelling` before the command dispatches (mirroring the Issue #21 validation workflow) and the gate's `CancelCommand` produces `SelectCancelRequestedMsg`, which Update accepts without settling anything — real interrupt semantics (connection-scoped interrupts, bounded settlement) remain with Issue #28. The exact `cancelling…` status renders alongside the still-pending phase wording until **every** owned read request has settled (`clearSelectCancellingIfSettled`); a settled last request clears it. Settlement of any first-page, count, or page response releases that request's gate claim regardless of outcome classification (success, failure, or cancellation), so stale or cancelled responses can never wedge the gate.

## Read-phase feedback contracts

The results status/count line composes independent parts via `joinStatusParts` (count header — `Running…` — `cancelling…` — page loading — range), never clamping or replacing displayed rows:

- `Running…` — exactly while the first-page request of an actual SELECT execution is in flight (`SelectRunningIndicator`, rendered via `runningText`), including before any result exists.
- `Counting rows…` — while the independent count request is pending (Issue #24's `CountState.Header()`).
- `loading next page…` — while the one later-page request is pending (Issue #25's `PageLoadingIndicator`); distinct from `Running…`.
- `cancelling…` — from the Ctrl+W request until settlement, as above.
- `Count unavailable` — the established exact count-failure state (Issue #24), reached without ever converting a count failure into a page failure.

Labels update at settlement in either arrival order and remain visible through permitted local interaction (Tab navigation does not clear `Running…`). Resize, overlays, and quit suspension leave feedback responsive: suspension freezes state exactly, and the restore-restore/resize generation bump cannot resurrect stale labels because claims release at settlement, not at rendering. No write-phase label (`Estimating matching target rows…`, `beginning`, `executing`, `committing`, `rollback`) is produced by any read path; Issue #44 owns write-phase feedback and the write-side integration of this gate.

## Tests

- `internal/ui/inflight_gating_test.go` — table-driven gating across first-page/later-page/count phases (every blocked action consumed with exact feedback, no command, unchanged request counts; Enter hints; horizontal `,`/`.` staying local with no feedback; quit confirmation open/restore/confirm; Ctrl+W single-dispatch with handoff vs. ignored without ownership; terminal/quit-confirmation/top-overlay/focused-input/gate/base precedence; flag-driven gating without labels; settlement release in either arrival order; count failure wording).
- `internal/ui/read_feedback_test.go` — exact `Running…`/`Counting rows…`/`loading next page…`/`cancelling…`/`Count unavailable` rendering contracts, label updates in either settlement order with `Result count: 7` landing, feedback surviving permitted interaction, action-specific rejection strings, popup Enter consumption, and an explicit no-write-phase-label guard.

Production changes: `internal/ui/inflight_gate.go` (gate, feedback constants, quit confirmation), `internal/ui/model.go` (gate state fields, settlement claim release, precedence wiring, base `q`/Ctrl+C), `internal/ui/first_select.go` (`firstPagePending`/`countPendingFlag` claims, `ActiveCancellable`/`CancelCommand` wiring, `SelectCancelRequestedMsg`), `internal/ui/paging.go` (deactivation releases gate claims), `internal/ui/view.go` (footer notice, quit overlay, status composition), `internal/ui/results_grid.go` (`Running…`/`cancelling…` status parts).
