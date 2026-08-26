# Tasks for #54: Contextual help and overlay precedence

Parent issue: #54
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify the global non-quit key precedence matrix

**Type**: RED
**Output**: Failing table-driven tests cover terminal, top overlay, focused input/search, request restrictions, base context, key consumption, and no leakage.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add one table-driven scripted `(model, msg) → (model, cmd)` matrix in `internal/ui` for the non-quit portions of the authoritative Global Key Precedence and Context/Action Matrix in `Notes/PRD-sqloid.md`. Cover terminal deletion/replacement/outcome-unknown, every top overlay or modal, focused builder text, popup search and file-picker search, scroll-only popups, request-pending base state, ordinary builder/result base state, and too-small state. For Enter/printable, Esc, navigation, Ctrl+P/N, Ctrl+E/Y, Ctrl+S/Ctrl+X, Ctrl+W, and `?`, assert routing follows terminal → top overlay → focused input/search → request restriction → base order, exactly one layer consumes each key, and lower handlers cannot mutate focus, selection, viewport, history, save/export state, or issue commands. Include overlapping meanings, disallowed/no-op rows, cancellable versus noncancellable request phases, repeated keys, and command-count assertions proving no leakage. Keep this task test-only, reuse existing model fixtures and fake Connection request instrumentation, and leave universal q/Ctrl+C confirmation to Issue #55.

---

### 2. Implement centralized key dispatch precedence

**Type**: GREEN
**Output**: Every non-quit matrix row routes keys through one ordered dispatcher.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Centralize non-quit key routing in `internal/ui` so one ordered dispatcher classifies terminal state, current top overlay, focused input/search, request-in-flight restrictions, and base context before delegating to context-specific handlers. Make each handled, rejected, and no-op decision explicitly consume the key so no lower layer can run, while preserving typed request phase and terminal-state gates rather than inferring behavior from rendered labels. Route popup, text entry, picker, validation, estimate, confirmation, save, help, result, builder, and too-small behavior through the same precedence entry point without changing their owned transitions. Preserve local horizontal scrolling during permitted pending states, cancellation only in cancellable phases, and all established history/save/export restrictions. Implement only enough to make Task 1 pass; do not add Issue #55's quit suspension behavior or the contextual content owned by later tasks.

---

### 3. Specify contextual `?` routing

**Type**: RED
**Output**: Failing tests cover literal `?` in text/search, contextual base help, no-op overlay cases, and exact focus preservation.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend the `internal/ui` precedence matrix with focused `?` cases for every builder value input, Limit and filename entry, searchable table/column/GROUP/ORDER popup, directory/file-picker search, scroll-only popup, non-quit modal, help, request-pending base context, ordinary builder fields, result view, terminal view, and too-small screen. Require `?` to insert one literal character at the current cursor in focused text/search without opening help or changing completed selection; require it to open context-tagged help only from eligible base builder/result and reduced terminal contexts; and require no-op consumption in overlays, confirmations, scroll-only popups, and too-small state where no search is focused. Capture exact opener focus, cursor, search query, highlighted item, popup viewport, builder/result viewport, and selected history before opening help, then require dismissal to restore them exactly. Assert repeated `?` never stacks help and no case leaks into a lower base action. Keep this task test-only and do not assert the detailed help copy owned by Task 5.

---

### 4. Implement help versus literal-input routing

**Type**: GREEN
**Output**: Input and base-context `?` tests pass across builder and picker search.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement `?` handling in the centralized `internal/ui` dispatcher according to focused-input precedence. Delegate to the active text/search component for builder inputs, popup searches, and picker search so insertion respects cursor/editing behavior and cannot open help; consume `?` as a no-op in nonsearch overlays and scroll-only contexts. From eligible base contexts, capture one immutable opener descriptor containing exact focus, cursor/search/highlight/viewport, selected history, and base context, then open one nonstacking contextual-help overlay. On dismissal, restore that descriptor exactly without rebuilding state from visible text or applying the dismissal key below it. Keep reduced terminal help routed through the terminal branch and preserve request-pending/base distinctions. Implement only enough to make Task 3 pass, leaving required WHERE/result/terminal copy to Tasks 5-6.

---

### 5. Specify required help content

**Type**: RED
**Output**: Failing tests cover WHERE SQL-NULL guidance, independent limited-result count semantics, and reduced terminal actions with no database suggestions.
**Depends on**: 4

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add exact semantic-content tests in `internal/ui` for contextual help opened from WHERE value/operator context, a result with independent count state, and each deletion, replacement, and outcome-unknown terminal state. Require WHERE help to state that a typed token spelled `NULL` is TEXT, direct SQL-null intent to `IS NULL` and `IS NOT NULL`, and explain that ordinary comparisons and LIKE do not match actual NULL values. Require result-count help to explain that the count covers the complete executed SELECT including the user's Limit, is not a table count or pre-Limit row count, runs as an independent autocommit read that may drift from page rows, and never clamps fetched pages or cache. For each terminal, derive reduced help from actually available in-memory actions and require applicable query/result history selection, Ctrl+S query saving, Ctrl+X's tabular-selection rule or non-tabular rejection, help dismissal, and immediate status-1 quit while prohibiting validation, refresh, execution, estimate, paging, rerun, cancellation, recovery, or any database suggestion. Test populated and empty histories and preserve exact opener state. Keep this task test-only and reuse terminal contracts from Issues #45-#46.

---

### 6. Implement context-specific help views

**Type**: GREEN
**Output**: WHERE, result-count, and all terminal help-content tests pass.
**Depends on**: 5

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement context-specific help models and rendering in `internal/ui`, selected from typed builder/result/terminal context captured by Task 4 rather than from display-string matching. Add the required SQL-NULL guidance to WHERE help, complete-limited-SELECT and independent-autocommit drift/no-clamping guidance to result-count help, and reduced action lists for deletion, replacement, and outcome-unknown terminals. Build terminal lists from capabilities actually available for the current immutable selection and history state, retaining separately owned Ctrl+S/Ctrl+X behavior while never suggesting database work. Keep help nonstacking, preserve exact opener focus/state on Esc, and ensure opening or closing it neither finalizes an active SELECT nor changes history, request, viewport, or save state. Implement only enough to make Task 5 pass without changing overlay cancellation rules beyond help.

---

### 7. Specify top-overlay Esc restoration

**Type**: RED
**Output**: Failing tests cover one top overlay only, nonstacking rules, exact opener state/focus, completed multi-selections, and no key leakage.
**Depends on**: 6

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add a table-driven Esc restoration suite in `internal/ui` for help, searchable and scroll-only popups, stale-validation retry, schema validation, estimate and write confirmation, generic errors, directory picker, filename entry, export warnings, overwrite confirmation, save failure/retry, and any existing modal flow. For each opener, capture exact focus, cursor/search text, highlighted item, popup and result viewport, builder values, immutable copy/selection, destination/format/path, request/preparation identity, and displayed error as applicable. Require Esc to cancel only the current top overlay, never a second underlying layer; enforce nonstacking for ordinary modals, preserve already completed multi-select additions while discarding only the incomplete current choice, and return to the exact opener or the flow-specific intact parent path. Exercise repeated Esc, overlays opened from history/result/builder states, request settlement while an overlay is visible, and stale/duplicate messages; assert the Esc key produces no lower-level error dismissal, navigation, edit, request cancellation, or command. Keep this task test-only and exclude quit's exceptional one-overlay suspension, which belongs to Issue #55.

---

### 8. Implement overlay cancellation and restoration

**Type**: GREEN
**Output**: Esc behavior passes across help, popups, modals, picker, validation, estimate, and save flows.
**Depends on**: 7

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement one top-overlay cancellation path in `internal/ui` using explicit opener snapshots and flow-owned cancellation transitions. Prevent ordinary modal stacking at creation time, identify the top overlay by typed state, consume Esc before input/request/base handlers, and restore exact focus plus cursor/search/highlight/viewport and immutable workflow data from the opener snapshot. Preserve completed popup multi-selections while cancelling only the active incomplete step; route validation, estimate, picker, overwrite, and save cancellation through their established cleanup or intact-parent behavior without starting replacement work or finalizing an active SELECT. Accept settlement updates that are contractually allowed behind an overlay and restore the latest valid underlying state while rejecting stale identities. Ensure repeated Esc after closure is handled only by the newly exposed context on a later key event, never by leakage from the closing event. Implement only enough to make Task 7 pass and retain Issue #55's quit exception for later work.

---

### 9. Document key precedence and help

**Type**: DOCUMENT
**Output**: Wiki documentation records the matrix, `?`, required content, overlay rules, and restoration.
**Depends on**: 8

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #54 implementation and tests from `internal/ui` into the appropriate pages under `Notes/wiki`. Document the authoritative terminal → top overlay → focused input/search → request restriction → base ordering for non-quit keys, single consumption, disallowed/no-op behavior, and no key leakage across every matrix row. Record literal `?` insertion in focused builder, popup, and picker text/search; base-only contextual help; no-op overlay cases; and nonstacking help with exact focus/cursor/search/highlight/viewport restoration. Include complete WHERE SQL-NULL guidance, complete-limited-result independent count/drift/no-clamping semantics, and reduced terminal help listing only available in-memory actions with no database suggestions. Explain ordinary nonstacking overlays, Esc cancelling only the top overlay, completed multi-selection preservation, flow-specific picker/validation/estimate/save restoration, and the quit-suspension exception owned by Issue #55. Cross-reference Issues #12, #17, #24, #45-#46, #52-#55 and the Global Key Precedence, Builder and Display Interaction, Paging consistency, UI Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` for every added or removed page and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 10. Create the contextual-help walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/054-10/code-walkthrough`.
**Depends on**: 9

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/054-10/code-walkthrough`. Drive representative and overlapping keys through terminal, overlay, focused text/search, request-pending, base, and too-small contexts, showing one ordered dispatch, key consumption, and no leakage. Insert literal `?` in builder text, searchable popup, and picker search; show no-op overlay cases; then open base help and capture exact focus/search/highlight/viewport restoration. Display and verify the full WHERE SQL-NULL guidance, independent complete-limited-result count semantics, and each reduced terminal help view with no database suggestions. Open and Esc-cancel help, popups, validation, estimate, confirmation, picker, overwrite, and save-error states; demonstrate one top overlay, nonstacking, completed multi-selection retention, exact opener restoration, and no action beneath the dismissed overlay. Reference Issue #54 and `Notes/PRD-sqloid.md`, distinguish Issue #55 quit suspension, and place every showboat-generated artifact under the approved directory.

---
