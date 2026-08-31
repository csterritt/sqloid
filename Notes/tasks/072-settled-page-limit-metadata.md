# Tasks for #72: Retain page truncation and limit-failure metadata at settlement

Parent issue: #72
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify first-page settlement metadata retention

**Type**: RED  
**Output**: Failing first-page settlement tests inject ByteTruncated and both LimitFailure kinds through FirstPageResult and require persistent ResultView rendering.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend `internal/ui/first_select_test.go` and focused result rendering tests, following the real `SelectSettledMsg` identity/update path rather than assigning `ResultView` fields directly. Inject successful and typed-failure `FirstPageResult` values with `ByteTruncated`, `LimitFailure{KindValue}`, and `LimitFailure{KindPage}` at known one-based positions; require accepted settlement to copy each fact into `ResultView` and render exactly `Result truncated: 64 MiB cache limit` plus the shared row-N failure text. Add a fixture where merging the first page makes `viewportCache.TruncatedByByteCap()` true even if the incoming flag is false, and require the stored truncation to be true. Cover cancellation/stale-identity inertness and replacement by a fresh execution. Keep this task test-only and avoid direct post-settlement mutation.

---

### 2. Retain first-page metadata after cache merge

**Type**: GREEN  
**Output**: applySelectSettled stores incoming failure metadata and ORs incoming/cache byte truncation after merging the first page.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Update `applySelectSettled` in `internal/ui/first_select.go` so a current non-cancelled result initializes `ResultView` with `Page`, `Err`, `ByteTruncated`, and `LimitFailure` from `FirstPageResult`. When a page exists, merge it into the fresh viewport cache first, then set `ResultView.ByteTruncated` to the incoming fact OR `viewportCache.TruncatedByByteCap()` so cache-derived truncation cannot be lost. Preserve ordinary-error handling, cancellation inertness, offset zero, fresh-execution replacement, request identities, and no builder/history changes. Do not make renderers infer metadata from error strings or tests mutate the view directly.

---

### 3. Specify later-page metadata merge precedence

**Type**: RED  
**Output**: Failing paging tests enforce monotonic truncation and new-non-nil/old-when-absent LimitFailure retention through accepted settlements.  
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend `internal/ui/vertical_paging_test.go` and result-view tests to send metadata through real `PageSettledMsg` updates. Build a matrix over prior `ResultView.ByteTruncated`, incoming `FirstPageResult.ByteTruncated`, and post-merge cache truncation and require logical OR so once true it never becomes false. Seed an earlier value or page `LimitFailure`, then require a newly reported non-nil failure to replace it and a nil incoming failure to retain it; cover both kind transitions and exact row positions. Navigate forward/backward and trigger resize/cache changes afterward to prove warning and diagnostic remain visible. Include stale, cancelled, duplicate, and wrong-generation messages that must not mutate retained metadata. Keep this task test-only and do not assign final metadata directly after applying messages.

---

### 4. Merge later-page metadata without discarding prior facts

**Type**: GREEN  
**Output**: applyPageSettled preserves prior/new/cache truncation and applies the required LimitFailure replacement rule.  
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Update `applyPageSettled` in `internal/ui/paging.go` at the matched current-response path, applying metadata before any ordinary-error return can discard it. Compute byte truncation as the OR of prior `ResultView.ByteTruncated`, `msg.Result.ByteTruncated`, and `viewportCache.TruncatedByByteCap()` after any accepted complete-page merge. Initialize retained failure from the prior view and replace it only when `msg.Result.LimitFailure` is non-nil, including a typed limit-failure settlement that also carries `Err`; when the incoming failure is nil, preserve the prior one. Carry those facts into the retained or replacement `ResultView` while preserving the existing displayed-page/offset rule for ordinary later-page errors. Preserve stale/cancelled identity rejection, pending-slot settlement, exhausted detection, and previous-page display behavior. Coordinate this shared `applySelectSettled`/`applyPageSettled` surface before Issue #73 and do not fold failure metadata into the truncation boolean.

---

### 5. Document settled page-limit metadata

**Type**: DOCUMENT  
**Output**: Wiki documentation records first/later settlement rules, monotonic truncation, failure precedence, exact rendering, and identity guards.  
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest Issue #72's implementation/tests from `internal/ui/first_select.go`, `paging.go`, result rendering, and their focused tests into the appropriate pages under `Notes/wiki`. Document first-page copying plus post-merge cache OR, later-page prior/new/cache OR, and the rule that a new non-nil LimitFailure replaces the old while nil preserves it. Record exact persistent 64 MiB warning and page/value row-N diagnostics, navigation/resize persistence, and stale/cancelled identity inertness. Note the required completion before Issue #73's shared settlement edit. Cross-reference Issue #72 and the cache/export, paging, result metadata, UI Module Design, and high-risk Testing Decisions in `Notes/PRD-sqloid.md`; update the wiki index and append the required dated log entry.

---

### 6. Create the settlement-metadata walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/072-06/code-walkthrough`.  
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/072-06/code-walkthrough`, with the main file named `walkthrough.md`. Drive first-page and later-page results through their real settlement messages, showing incoming and cache-derived byte truncation produce the exact persistent warning without direct ResultView mutation. Demonstrate page/value LimitFailure storage, replacement by a newer non-nil failure, preservation when a later page reports nil, and persistence through forward/backward navigation and resize. Include stale/cancelled message controls and explain the shared-surface ordering before Issue #73. Reference Issue #72 and `Notes/PRD-sqloid.md`, and place every generated artifact under the approved directory.

---
