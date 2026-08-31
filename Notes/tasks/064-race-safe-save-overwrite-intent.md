# Tasks for #64: Enforce overwrite intent at the atomic persistence boundary

Parent issue: #64
Parent PRD: PRD-sqloid.md
**Blocked by issues**: #63
**Acceptance criteria**: AC1 and AC4–AC5 → Tasks 1–2; AC2–AC3 → Tasks 3–4
**Manual verification**: Task 6 owns the issue's manual checks; shipped-TUI evidence begins after Issue #57 Phase A lands.

## Tasks

### 1. Specify race-safe no-replace and confirmed-replacement persistence

**Type**: RED  
**Output**: Failing synchronized export tests cover raced creation, changed confirmed destinations, unchanged replacement, destination-local staging, cleanup, and preserved short writes.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Begin only after Issue #63 is complete. Both issues modify the shared `WriteAtomic` persistence boundary. Add Linux/macOS filesystem-boundary tests in `internal/export` that carry explicit overwrite intent plus the exact destination state returned by inspection into `WriteAtomic`. Use deterministic barriers at the final persistence boundary to cover: inspection reports missing and another process creates the path before an unconfirmed save; inspection reports existing, the user confirms it, and another process replaces, removes/recreates, or materially changes that destination before persistence; and confirmed state remains unchanged. Require unconfirmed saves to use atomic no-replace creation and preserve the raced file, confirmed saves to compare the current destination with the inspected identity/state before destination-local temp-file-plus-rename, changed state to return a typed destination-existing/changed result without replacement, and unchanged state to replace atomically. In every branch require staging in the destination directory, exact immutable bytes, no leaked artifacts, and no false success. Retain all Issue #53 stage failures and Issue #63 `(n < len, nil)`/`io.ErrShortWrite` behavior. Keep this task test-only, separate portable contracts from OS-specific primitives with the repository's supported-platform conventions, and do not rely on sleeps.

---

### 2. Enforce overwrite intent inside `WriteAtomic`

**Type**: GREEN  
**Output**: The export persistence boundary atomically refuses unconfirmed/raced destinations and replaces only an unchanged confirmed target.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Refactor `internal/export/save_write.go` and the minimal supported-platform helper files so destination inspection returns a durable state token and `WriteAtomic` requires an explicit no-replace or confirmed-replace intent tied to that token. For a destination inspected as missing, publish only through an atomic exclusive operation such as `O_CREATE|O_EXCL`; never rename over a path that appeared later. For confirmed replacement, verify at the last safe point that the destination still matches the inspected state, then use a unique destination-local temporary file and atomic rename only when authorized. Return typed existing/changed errors when intent or state no longer permits persistence, preserve the raced destination byte-for-byte, and clean all staging artifacts. Maintain serialization/write/sync/close/rename stage attribution, Issue #63 short-write conversion, immutable data, and Linux/macOS behavior. Update injected `SaveFS` seams only as needed for deterministic tests, without a check-then-act fallback that reopens the race.

---

### 3. Specify save-flow reinspection and renewed confirmation

**Type**: RED  
**Output**: Failing UI tests require raced destinations to return to fresh inspection/confirmation while preserving immutable save and opener state.  
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend `internal/ui/save_write_test.go`, picker tests, and quit-restoration tests with synchronized `SaveFS` fakes that mutate the destination after `SaveInspectMsg` and before write settlement. Cover a new-file save raced by external creation and a confirmed existing-file save raced by replacement; require both persistence errors to preserve external bytes, clear running state, avoid completion, and route the user through a fresh destination inspection followed by a new overwrite confirmation for the latest state rather than blindly retrying the stale authorization. Require the original immutable path, format, payload, selection, warnings, picker/opener snapshot, and save identity to remain intact; stale/duplicate inspection or write messages must be inert. Exercise retry, Esc/n cancel, and q/Ctrl+C suspend/restore while the renewed confirmation is open, plus successful save after unchanged fresh confirmation. Keep ordinary create/write/sync/close/rename failures on their existing same-capture retry path and keep this task test-only.

---

### 4. Carry inspected state and intent through the UI save flow

**Type**: GREEN  
**Output**: The UI supplies exact overwrite intent, renews stale confirmation after races, and preserves all retry/cancel/quit behavior.  
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Update `internal/ui/save_write.go`, its `saveCapture`/typed messages, and only the necessary model fields so destination inspection stores the returned state token, a missing destination starts an explicitly unconfirmed no-replace write, and Enter/y on overwrite records confirmed replacement tied to that inspected state. Pass both state and intent to the new `internal/export.WriteAtomic` contract. Distinguish typed destination-existing/changed races from ordinary stage failures: on a race, preserve the immutable capture and intact picker/opener, issue a fresh inspection, and require renewed confirmation before any replacement; on ordinary failures retain the established retry/cancel path. Keep attempt identities monotonic so stale/duplicate messages cannot reuse old authorization, keep quit suspension/restoration exact, and report success only after authorized persistence completes. Do not reserialize or consult live builder/result/history state after capture. Implement only enough to make Task 3 pass and run focused export/UI tests, race-enabled tests, and the established Go verification command.

---

### 5. Document overwrite intent and race handling

**Type**: DOCUMENT  
**Output**: Wiki documentation records inspected-state tokens, no-replace creation, confirmed replacement, race recovery, staging, cleanup, and preserved short-write behavior.  
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #64 implementation and tests from `internal/export` and `internal/ui` into the atomic-save and picker pages under `Notes/wiki`. Document explicit unconfirmed/confirmed intent, the destination state captured at inspection, atomic exclusive creation for a previously missing destination, unchanged-state validation before authorized destination-local temp-plus-rename replacement, and typed existing/changed race failures that preserve external bytes. Explain fresh inspection and renewed confirmation, immutable capture retention, stale-attempt rejection, exact retry/cancel/quit restoration, destination-local staging, cleanup, and Issue #63 short-write behavior across both persistence paths. State the supported Linux/macOS primitive boundaries without claiming unsupported portability. Cross-reference Issues #53, #55, #63, and #64, user stories 72 and 86, and the Atomic saves, File picker, Global Key Precedence, Export Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the race-safe-overwrite walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/064-06/code-walkthrough`.  
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/064-06/code-walkthrough`, with the main file named `walkthrough.md`. On Linux or macOS, deterministically inspect a missing destination, create it externally before persistence, and show atomic no-replace refusal, exact external-byte preservation, no temp leak, fresh inspection, and overwrite confirmation. Then inspect/confirm an existing destination, externally replace it before persistence, and show destination-changed refusal and renewed confirmation; reinspect without further change and demonstrate successful destination-local atomic replacement. Capture immutable payload/selection, stale-message rejection, retry, cancellation, quit restoration, stage cleanup, and Issue #63 short-write behavior. After Issue #57, drive representative cases through the shipped TUI/headless composition path. Reference Issues #57, #63, and #64 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
