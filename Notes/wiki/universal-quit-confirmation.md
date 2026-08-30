# Universal Quit Confirmation and Exact Restoration (Issue #55)

The shared quit confirmation extended to every enabled nonterminal context: `q` and Ctrl+C open exactly one confirmation anywhere in the TUI — suspending the exact current context rather than replacing it — with Esc/n restoring the latest identity-valid state and accepted quit exiting only after all owned cleanup settles. Deletion, replacement, and outcome-unknown terminal states remain the immediate status-1 exceptions. Pages: [in-flight-gating.md](in-flight-gating.md), [commit-boundary-quit-cleanup.md](commit-boundary-quit-cleanup.md), [contextual-help-and-overlay-precedence.md](contextual-help-and-overlay-precedence.md).

## Key routing

- `internal/ui/model.go` keeps `handleKey`'s terminal classification first: `TerminalDeleted`, `TerminalReplaced`, and `TerminalOutcomeUnknown` emit the immediate status-1 quit (no confirmation) in every quit-path test.
- The shared confirmation moved to the top of the nonterminal dispatcher — above Issue #54's contextual help, save/export flows, picker, preparation modal, popups, and focused input — and it also wraps the too-small suspended state: `handleKey` consults `quitConfirm` before the suspension shortcut and routes Ctrl+C (and `q`, since the too-small wrapper owns no text) to `openQuitConfirmation()`.
- Focused text/search owns literal `q`: value prompts (WHERE entry, INSERT prompts, Limit, search) consume `q` as input while Ctrl+C still opens the confirmation. Searchable popups behave the same; scroll-only popups own no search, so `q` opens the confirmation (`internal/ui/popup.go`).
- Overlay handlers that previously handled only Ctrl+C now route both keys — overwrite confirmation and save failure (`save_write.go`), export warnings (`export.go`) — via the same `openQuitConfirmation()`.
- The confirmation consumes every key: repeated `q` is a consumed no-op (Ctrl+C inside is the accept key), no key reaches text, navigation, cancellation, save, or database work, and ordinary overlays never stack behind the one quit overlay (quit is the sole one-overlay suspension exception over Issue #54's nonstacking rule).

## Suspension and restoration

- `openQuitConfirmation()` (`internal/ui/inflight_gate.go`) snapshots the whole model into `quitSuspended *Model`; nothing in the underlying context changes merely by opening and no command is dispatched.
- Esc/n restore `*quitSuspended` with the quit frame removed atomically, returning the exact opener, focus/cursor, popup type/search/mode/highlight, builder focus/scroll/fields, result first visible column, history browsing state, errors/warnings, and too-small wrapper — the dismissal key is consumed by quit so it cannot also close the revealed overlay (`internal/ui/quit_restoration_test.go` proves overlay survival and no-leak parity; a second Esc/n is then the restored context's ordinary key).
- Asynchronous schema-validation, estimate, page/count, and write settlements continue to route into the suspended model through their ordinary identity guards, so cancellation reveals the latest identity-valid state (e.g. an awaiting estimate restored with its result and confirmation-enabled state), not the stale opening snapshot.

## Accepted-quit lifecycle

- Enter/y/Ctrl+C accept: pending writes enter Issue #43's `acceptQuitWithWrite` settlement coordinator (one cancellation before the COMMIT boundary, no interrupt after it, exit only via the settled message after exactly-once finalization of definite or unresolved outcomes); every other context runs `acceptedQuitCleanup()` (`internal/ui/active_select.go`), which cancels still-owned read requests and finalizes the active SELECT exactly once — one immutable result entry, duplicate/late finalizers inert, no lease reuse — before `tea.Quit`. Awaiting preparation (estimate/pre-confirmation) dismisses with no history appended.
- Exit is never emitted while transaction, driver, lease, finalization, or cleanup work remains; repeated acceptance and stale settlement are idempotent.

## Tests

- `internal/ui/quit_matrix_test.go` — the table-driven quit matrix over base builder, focused value prompt, searchable and scroll-only popups, contextual help, first-page pending, noncancellable write phase, schema validation, query history, too-small screen, and the three terminal exceptions; both quit keys per context with expectations `quitOpensConfirmation`, `quitLiteralQ` (q literal, Ctrl+C confirms), `quitTerminalStatus1`; context-signature checks behind the confirmation and repeated-`q` no-op coverage.
- `internal/ui/quit_restoration_test.go` — Esc/n from every suspended nonterminal context with exact field-by-field restoration, no command, overlay survival after cancellation, and repeated-cancellation ordinary behavior.
- Issue #55's accepted-quit coordination reuses the Issue #34 finalizer and Issue #43 write settlement tests (`write_quit_settlement_test.go`, `snapshot_finalize_once_test.go`) unchanged.

Cross-references: Issues #6, #21, #27, #28, #34, #40–#46, #52–#55; the Global Key Precedence and Context/Action Matrix, Execution and Result Lifecycle, Writes and commit boundary, and Testing Decisions sections of `Notes/PRD-sqloid.md`.
