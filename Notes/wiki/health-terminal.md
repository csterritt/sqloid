# Deletion/Replacement Terminal Workflow

Issue #46 completes the session-health lifecycle tail: when any request boundary classifies Issue #7's **typed deletion** (`HealthDeleted`) or **typed same-path replacement** (`HealthReplaced`) outcome, the UI leaves every workflow — live shell, stale-schema flow, validation, paging, estimates, writes — and enters the corresponding terminal state. The two states implement the PRD's **Session health** and **Global Key Precedence** terminal rows exactly like Issue #45's outcome-unknown state: no transaction or driver work may remain pending on entry, every database-capable action is suppressed before any command can be built, in-memory query/result history selection stays available, a reduced help lists only what works, and `q`/Ctrl+C quit immediately with status 1. Ctrl+S integration in these states is Issue #48; Ctrl+X export is now implemented by Issue #49 ([immutable-export-capture.md](immutable-export-capture.md)), targeting the current in-memory result selection without database work.

## Typed classification, UI-owned strings (`internal/ui/health_terminal.go`)

- **Typed only, never text**: `healthTerminalFor(err)` consumes the classification exclusively through `errors.As` into `*connection.HealthError` and maps `HealthKind` → terminal state (`HealthDeleted` → `TerminalDeleted`, `HealthReplaced` → `TerminalReplaced`). No driver error text, no error-string matching anywhere selects a terminal — a decoy error whose text is exactly the UI message is classified as an ordinary failure and finalizes one ordinary error entry.
- **Exact messages owned by `internal/ui`**: `DeletedSessionEndedMessage` (`Database file no longer exists — session ended`) and `ReplacedSessionEndedMessage` (`Database file was replaced — session ended`) are defined only in `internal/ui/schema_refresh.go`; the health/connection layer's `HealthError.Error()` and `HealthKind.String()` carry neutral diagnostics with no terminal copy (Issue #7 deliberately deferred this wording).

## Request-boundary and ordinary-activity entry points

The typed classification ends the session from every request boundary:

- **First-page SELECT** (`SelectSettledMsg.Err`), **independent count** (`CountSettledMsg.Err`), and **later page during ordinary paging** (`PageSettledMsg.Err`) — each routes through `endSelectIntoHealthTerminal`, which finalizes the active SELECT exactly once **without appending a snapshot entry** (`suppressFinalizedAppend` consumed by `appendFinalizedResultEntry`: the database is gone or replaced, so no truthful entry can be constructed) and then enters the terminal state.
- **Destructive estimate** (`EstimateSettledMsg.Result.Err`) — dismisses preparation and enters the terminal state instead of an estimate failure.
- **Phased write** (`WriteSettledMsg.Result.Health`) — the authoritative typed health field on `connection.WriteResult`; the settlement proves the transaction and driver work ended, so the terminal state is entered with no pending write lifecycle and no appended entry.
- **Pre-execution validation** (`schema.RevalidateDeleted`/`RevalidateReplaced` from the version read) and **Table-popup catalog refresh** (`schema.RefreshDeleted`/`RefreshReplaced`) — the pre-existing Issue #13/#21 typed mappings route into the same `enterTerminal` transition.

## No-pending-work entry (`enterTerminal`, `internal/ui/schema_refresh.go`)

`enterTerminal` is the atomic entry boundary for both health terminals: it closes any popup (restoring opener focus), dismisses preparation, ends validation, and — new in Issue #46 — `retirePendingDatabaseWork` interrupts every owned cancellation context, clears all pending request flags (first page, count, page, write, resize deferral, cancelling feedback), retires the write lifecycle, and advances the viewport generation so any late response can never mutate the terminal state. Popups, prompts, history browsing modes, quit confirmation, and in-flight feedback all clear; terminal entry happens only after all of it has ended.

## History selection and navigation (`internal/history` stores, immutable)

- **Populated result history**: terminal entry selects the newest stable-backed immutable entry (stable `EntryID` cursor, locally projected through the Issue #36 primitives — never a slice index, never synthesized). Ctrl+E moves older and Ctrl+Y newer with deterministic no-op boundaries; Ctrl+Y at the newest stays inside the terminal result view, and Esc-then-Ctrl+E re-selects the newest. Query navigation (Ctrl+P/N through the Issue #35 machinery, restoring complete immutable builder states) never moves the result selection.
- **Empty result history**: the selection stays empty (`resultHistoryMode` false, cursor 0, no projection) and the **exact terminal message remains the whole primary view** — no synthetic entry, no absent stable ID, no stale columns/rows, no missing-backed rendering, and no entry appended for the health-failed execution itself. Ctrl+E/Y are deterministic no-ops.
- **Empty query history**: Ctrl+P/N open nothing. Resize inside either terminal reprojects locally, issues no requests, and keeps the terminal state and message.
- Navigation never touches the wired executors: every seam proves zero requests.

## Terminal view and reduced help (`internal/ui/health_terminal.go`)

The view always leads with the exact primary message. With nothing selected it is the whole view. With a selected result entry, the immutable projection (tabular rows via the Issue #36 projection, or the entry summary/SQL for non-tabular kinds) renders below the message; selected query-history states render the complete restored SQL. The footer hint lists the only available actions (`Ctrl+P / Ctrl+N query history · Ctrl+E / Ctrl+Y result history · ? help · q or Ctrl+C quits (status 1)`). `?` toggles a reduced help listing only the in-memory history selection, help dismissal (Esc), and the immediate quit — never execution, refresh, paging, rerun, cancellation, or any other database suggestion. Help dismissal preserves the exact terminal message.

## Immediate quit

`q` and Ctrl+C take precedence in every terminal subview — primary message, query-history selection, result-history selection, and reduced help — set `exitStatus` to 1, and return exactly `tea.Quit` with no quit-confirmation overlay, no cancellation request, no cleanup or delayed command, and no state restoration (terminal entry already guarantees nothing is pending). The key set is shared with Issue #45's terminal handler (`handleTerminalHealthKey` delegates to the same reduced handling).

## Tests

- `internal/ui/health_terminal_test.go` — table-driven typed classifications injected at the SELECT, count, page, estimate, write, validation, and refresh boundaries (both kinds) entering the exact terminal states with no pending work; error-text decoy classified ordinary; string ownership (connection layer carries no terminal copy); every database-capable key from both terminals producing no command and zero fake-executor calls.
- `internal/ui/health_terminal_history_test.go` — populated/empty query and result histories in both terminal variants: newest selection on entry, Ctrl+P/N and Ctrl+E/Y traversal with deterministic boundaries, empty fallbacks keeping the exact message as the primary view with no synthetic/missing-backed entry, and resize issuing no requests.
- `internal/ui/health_terminal_help_quit_test.go` — reduced-help contents with no database suggestions in populated and empty variants, immediate status-1 `q`/Ctrl+C from every context bypassing confirmation, nothing scheduled, and database work staying blocked while help is open.

## Cross-references

- Issue #7 (typed `HealthDeleted`/`HealthReplaced` request-boundary classifications these states consume; terminal copy deliberately absent there), #13 (stale-refresh flow suppressed by terminal precedence), #21 (validation terminal precedence), #35/#36 (immutable query/result history stores, stable-ID selection, and projection primitives), #45 (the outcome-unknown terminal whose key handling and reduced help these states share), #46 (this workflow).
- Issue #48 owns the Ctrl+S save integration; Issue #49 ([immutable-export-capture.md](immutable-export-capture.md)) owns Ctrl+X export targeting, gating, and warnings inside these terminal states.
- PRD sections: Session health (typed classification → exact terminal messages, request-boundary basis), Global Key Precedence and Context/Action Matrix (terminal row, immediate status-1 quit), History Module Design.
- Related wiki pages: [session-health.md](session-health.md), [stale-schema-refresh.md](stale-schema-refresh.md), [schema-validation-workflow.md](schema-validation-workflow.md), [outcome-unknown-terminal.md](outcome-unknown-terminal.md), [query-history-navigation.md](query-history-navigation.md), [result-history-browsing.md](result-history-browsing.md), [in-flight-gating.md](in-flight-gating.md).
