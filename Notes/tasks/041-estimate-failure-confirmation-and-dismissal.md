# Tasks for #41: Estimate failure, confirmation, and dismissal

Parent issue: #41
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify estimate settlement outcomes

**Type**: RED
**Output**: Failing model tests cover successful/failed estimates, retained SQL/warnings, confirmation enablement, identities, and no history/execution during preparation.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add deterministic estimate-settlement tests in `internal/ui`, supported by the controllable request fake in `internal/connection` and history assertions in `internal/history`, for Issue #41 and the Identities and state, destructive-preparation, estimate-modal, History, UI/Connection/History Module Design, and Testing Decisions contracts in `Notes/PRD-sqloid.md`. Start qualified and unqualified UPDATE/DELETE preparations and settle their independent estimate request as success or failure. Require the preparation/request identities to remain distinct from execution identity, accept only the current response, continuously retain operation, table, standalone SQL, and any all-rows warning, show either the estimated matching-target count or the estimate error, and enable Enter/y confirmation after either settled outcome. Assert opening and settling preparation dispatches no actual write and appends neither query nor result history. Include stale and duplicate settlement messages and prove they cannot replace current retained content, enable the wrong preparation, create an execution, or mutate either history. Keep this task test-only and preserve Issue #40's estimate SQL and presentation contracts.

---

### 2. Implement settled estimate states

**Type**: GREEN
**Output**: Success/failure presentation and confirmation-availability tests pass.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement explicit settled-success and settled-failure preparation transitions in `internal/ui`, consuming identity-bearing estimate outcomes from `internal/connection`. Preserve the operation, table, rendered SQL, warnings, and current preparation/request identities while replacing only the estimate status with its count or error; enable deliberate confirmation for either settled outcome and reject stale or duplicate responses. Keep estimation a pre-execution workflow: do not allocate an execution identity, dispatch an actual write, or call append operations in `internal/history` merely because preparation opened or settled. Reuse Issue #40's retained preparation model rather than rebuilding estimate SQL or serialization in the modal, and implement only enough to make Task 1 pass without adding cancellation, dismissal, or confirmation dispatch assigned to later tasks.

---

### 3. Specify estimate cancellation and dismissal

**Type**: RED
**Output**: Failing tests cover Ctrl+W, `cancelling…`, late success, settlement, Esc/n, both unchanged histories, and exact opener restoration.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend scripted model tests in `internal/ui` with controllable `internal/connection` estimate barriers and explicit `internal/history` snapshots to specify every cancellation and dismissal path. While estimation is pending, require Ctrl+W to issue one cancellation for the current estimate request, retain the preparation with exact `cancelling…` feedback until true settlement, reject a success released after cancellation as cancelled, dispatch no replacement work, and close only after settlement with query and result histories byte-for-byte unchanged. Require repeated Ctrl+W to be idempotent. For Esc/n dismissal from settled success and settled failure, close immediately without an execution or history append; cover the applicable dismissal path while pending without abandoning driver work. In every path restore the exact opener model state and focus—including builder fields, active SELECT/result state, viewport, warnings, and prior feedback—rather than a reconstructed base context. Include stale/duplicate cancellation messages and prove they neither close a newer preparation nor leak into its state. Keep this task test-only and use the shared Issue #6 cancellation lifecycle as the expected seam.

---

### 4. Implement cancellation and dismissal transitions

**Type**: GREEN
**Output**: Cancel/dismiss tests pass using shared cancellation infrastructure.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Wire estimate Ctrl+W handling in `internal/ui` to the reusable cancellable request handle and connection-scoped interrupt/settlement behavior in `internal/connection` established by Issue #6. Track cancellation against the exact preparation and request identities, transition once to `cancelling…`, keep the retained preparation visible until the request settles, classify late success as cancelled, and only then restore the captured opener without touching `internal/history` or starting replacement work. Route Esc/n and other allowed dismissals through one preparation-close transition that preserves exact opener state and focus, while ensuring pending work is cancelled and settled rather than abandoned. Make cancellation and dismissal idempotent under repeated keys and duplicate/late messages, never force-close a connection, and implement only enough to make Task 3 pass.

---

### 5. Specify exactly-once confirmation

**Type**: RED
**Output**: Failing tests require Enter/y after settled success or failure to emit one actual-write command and reject repeat confirmation.
**Depends on**: 4

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add focused `(model, msg) → (model, cmd)` tests in `internal/ui`, with command capture in `internal/connection` and unchanged-until-execution assertions in `internal/history`, for deliberate confirmation from both settled estimate success and settled estimate failure. Require Enter and y to be equivalent only while confirmation is enabled; the first accepted key must consume the current preparation exactly once, allocate/retain the correct actual-write execution identity, and emit exactly one command carrying the confirmed operation and rendered statement. Replay Enter, y, duplicate estimate settlement, stale preparation IDs, and re-entrant update messages before and after command delivery, asserting request count remains one and no second execution can start. Prove pending/cancelling/dismissed preparations cannot confirm, and prove preparation itself still creates no query or result history—the execution-start lifecycle owns those changes in Issue #42. Keep this task test-only and do not implement transactional write execution here.

---

### 6. Implement deliberate write confirmation

**Type**: GREEN
**Output**: Confirmation identity and exactly-once command tests pass.
**Depends on**: 5

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement one guarded confirmation transition in `internal/ui` for Enter/y from settled-success or settled-failure preparation. Atomically consume the current confirmation capability, bind the actual-write command to the retained preparation and a new execution identity, close the preparation through its authoritative transition, and mark confirmation as dispatched before any duplicate key or message can re-enter the path. Reject stale preparation/request identities and all pending, cancelling, dismissed, or already-confirmed states. Emit the existing typed write command expected by `internal/connection` without executing SQL in the model and without directly appending `internal/history`; Issue #42 will own execution-start query append and result finalization. Implement only enough to make Task 5 pass while preserving exact opener and retained-content behavior from Tasks 2 and 4.

---

### 7. Document estimate lifecycle

**Type**: DOCUMENT
**Output**: Wiki documentation records success/failure/cancel/dismiss/confirm transitions, retained content, histories, and identities.
**Depends on**: 6

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #41 implementation and tests from `internal/connection`, `internal/history`, and `internal/ui` into the appropriate pages under `Notes/wiki`. Document preparation and request identities separately from actual-write execution identity; pending, settled-success, settled-failure, cancelling, dismissed, and confirmed transitions; confirmation availability after success or failure; and stale/duplicate-message rejection. Record continuous retention of operation, table, SQL, estimate/error, and all-rows warnings, exact opener/focus restoration, Ctrl+W `cancelling…` through settlement, cancellation-wins late-success handling, and the rule that preparation, failure, cancellation, and dismissal change neither history while one deliberate confirmation emits only one actual-write command. Cross-reference Issues #6, #40, and #41 and the Identities and state, Writes and commit boundary, Global Key Precedence and Context/Action Matrix, estimate-modal Implementation Decision, Connection/UI/History Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` for any added or removed page and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 8. Create the estimate-lifecycle walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/041-08/code-walkthrough`.
**Depends on**: 7

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/041-08/code-walkthrough`. Demonstrate qualified and unqualified destructive preparations through successful and failed estimate settlement while SQL and warnings remain visible and confirmation becomes available. Hold an estimate behind a deterministic barrier, press Ctrl+W, capture persistent `cancelling…`, release a late success, and show cancellation winning only after settlement with both histories unchanged. Exercise Esc/n dismissal and exact opener/focus restoration, then confirm settled success and settled failure with Enter/y and replay confirmation to prove exactly one actual-write command. Include identity/stale-message evidence and history/request-count assertions, reference Issue #41 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---

### 9. Review estimate outcomes

**Type**: REVIEW
**Output**: Human confirms success, failure, Ctrl+W, Esc/n, Enter/y, and double-confirm prevention.
**Depends on**: 8

Review estimate request handling in `internal/connection`, history boundaries in `internal/history`, preparation transitions and opener restoration in `internal/ui`, wiki updates, and `Notes/walkthroughs/041-08/code-walkthrough` against Issue #41. Manually open qualified and all-rows UPDATE/DELETE preparations and settle estimates successfully and unsuccessfully; confirm SQL/warnings remain visible and Enter/y becomes available. Cancel held estimation with Ctrl+W and verify `cancelling…` persists through settlement, late success is discarded, no replacement starts, and both histories remain unchanged; dismiss settled states with Esc/n and verify exact opener/focus restoration. Confirm success and failure once with Enter and y, repeat both keys, and verify exactly one correctly identified actual-write command with no preparation-time history before approving the issue.

---
