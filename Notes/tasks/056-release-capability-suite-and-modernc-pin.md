# Tasks for #56: Integrated release verification and modernc pin/upgrade gate

Parent issue: #56
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Define the cross-platform release test gate

**Type**: CONFIG
**Output**: Exact vetted modernc pin is confirmed; Linux/macOS CI jobs and one gated capability-suite command are defined without weakening failures.
**Depends on**: none

Inspect `go.mod`, `go.sum`, the existing test entry points, and CI configuration under `.github/workflows` or the repository's established CI path. Confirm `modernc.org/sqlite` is a direct exact version pin with no floating branch, wildcard, replace-to-local, or best-effort fallback, and record the currently vetted version as the only version accepted by the release gate. Define one canonical capability-suite command that selects all and only the integrated release-blocking capability tests while remaining runnable by developers and CI; both Linux and macOS jobs must invoke that identical command from a clean checkout with the pinned module graph. Make either platform, setup, test, timeout, race, or cleanup failure fail the job and block release; do not use continue-on-error, allowed failure, retries that hide reproducible failures, platform exclusions, or conditional skips for supported capabilities. Ensure a `modernc.org/sqlite` dependency change triggers both jobs and cannot merge as a successful upgrade unless the same gate passes. Keep this task limited to pin/gate configuration and suite selection, leaving capability assertions to later RED tasks.

---

### 2. Specify integrated journal and pool capabilities

**Type**: RED
**Output**: Gated tests cover WAL/rollback overlap, distinct leases, external writer delay/error, exact pool size, timeout, length limit, and unchanged journal mode.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Assemble release-tagged integration tests in `internal/connection`, with end-to-end `internal/ui` orchestration assertions only where needed, and register them under Task 1's single capability-suite command. Create equivalent WAL and rollback-journal fixtures, record journal mode before opening, and use synchronization barriers to hold page and complete-result count after they acquire two distinct physical leases from the same database. Prove actual overlap without an application mutex/queue, independent completion and cleanup, and a controlled external writer that produces the PRD-permitted delay or ordinary lock error while preserving successful independent work. Inspect every physical connection to require database/sql minimum and maximum pool size exactly two, five-second busy timeout, and connection-local `SQLITE_LIMIT_LENGTH` exactly 64 MiB, including newly opened/replacement connections; require journal mode to remain byte-for-byte unchanged throughout. Cover acquisition, request, error, cancellation, and teardown paths for lease release without concealing serialization. Use barriers rather than sleeps except explicit five-second latency bounds, make failures identify platform/mode/lease/phase, and keep this task test-only so the selected tests fail if any capability or exact pin assumption is absent.

---

### 3. Complete journal and pool release evidence

**Type**: GREEN
**Output**: Barrier-based journal/pool capability suite passes on both CI platforms.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Adjust `internal/connection` and the smallest necessary orchestration seams in `internal/ui` so Task 2's gated tests pass without weakening, skipping, or platform-special-casing their assertions. Preserve exactly two configured physical connections, apply the five-second busy timeout and 64 MiB length limit to every connection, lease distinct connections for concurrent page/count requests through true settlement, and remove any hidden serialization. Keep page/count as independent autocommit reads, preserve ordinary WAL and rollback-journal external-writer behavior, never issue a journal-changing pragma, and release leases on every success/error/cancellation path. Keep synchronization controls in test seams rather than production flow. Run the canonical capability-suite command in both Linux and macOS CI jobs and retain their passing evidence; if the exact modernc pin cannot satisfy the contract, change to an exact vetted version rather than relaxing behavior or tests.

---

### 4. Specify integrated cancellation capabilities

**Type**: RED
**Output**: Gated tests cover independent page/count interrupts, isolation, late success, CPU/lock bounds, schema/estimate cancellation, no histories, and unaffected subsequent work.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add pinned-modernc capability tests under the same gate across `internal/connection`, `internal/ui`, and `internal/history`. On two distinct leases, hold controlled CPU-bound page and count operations independently; cancel each role alone and together, require context plus connection-scoped SQLite interrupt to target only the intended request, retain leases until true settlement, classify success arriving after cancellation as cancelled, and prove the other active request and a subsequent request on the same connection remain unaffected. Measure controlled CPU cancellation settlement within one second and lock-wait cancellation no later than the configured five-second busy timeout, using barriers for ordering and clocks only for those explicit bounds. Exercise cancellation of pre-execution schema validation and destructive estimate, including changed-schema refresh and lock/CPU cases; require no actual execution, no query history, no result history, no replacement request before settlement, and identity rejection for stale/duplicate/late completions. Verify no force-close, pool shrink, cross-connection interrupt, leaked lease, goroutine, or driver work, and that each fixture remains usable afterward. Keep this task test-only and ensure every case is selected by Task 1's one command on Linux and macOS.

---

### 5. Complete cancellation release evidence

**Type**: GREEN
**Output**: Full pinned-driver cancellation suite passes on Linux and macOS.
**Depends on**: 4

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Harden cancellation and settlement in `internal/connection`, request identity/state in `internal/ui`, and no-history/finalization guards in `internal/history` only as needed to satisfy Task 4. Keep one cancellable context and connection-scoped interrupt identity per dedicated lease; cancel page/count independently, reject post-cancel success, prevent lease reuse or replacement requests before settlement, and clear interrupt effects so later work is unaffected. Apply the same scoped lifecycle to schema validation and estimation without appending history or starting execution. Preserve the one-second controlled CPU and five-second lock-wait bounds without force-closing connections or serializing independent requests, and make stale/duplicate messages idempotent. Run the full canonical suite with the exact pin in Linux and macOS CI and retain passing evidence; treat any driver/platform failure as a blocker requiring implementation or exact-pin change, never a weakened assertion or skip.

---

### 6. Specify integrated identity and transaction capabilities

**Type**: RED
**Output**: Gated tests cover deletion/rename/replacement races/messages, pre-COMMIT rollback, post-boundary no interrupt, and commit enforcement.
**Depends on**: 5

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend the gated suite across `internal/connection`, `internal/ui`, and `internal/history` with deterministic filesystem-identity and transaction barriers. For health, cover deletion, rename-away, and same-path replacement before request/new-connection boundaries; require exact deletion and replacement terminal messages, no further database work, and immediate status-1 terminal quit. Race same-path replacement after a successful precheck with both request error and request success: require the error path to reclassify terminal immediately, the success path to be accepted only for the already-open original and replacement detected at the next boundary before more work, and same-inode in-place mutation to remain ordinary SQLite behavior. For writes, hold beginning, executing, successful statement completion, the atomic cancellation decision immediately before COMMIT, rollback cleanup, and committing. Require pre-boundary cancellation to win even after statement success and resolve through confirmed rollback before an untouched claim; require post-boundary Ctrl+W to issue no interrupt, commit/rollback cleanup to remain noncancellable, commit to be enforced exactly once, and unresolved resolution to settle before outcome unknown or exit. Assert one precheck for the whole transaction, no check between statement and COMMIT, no abandoned lease/transaction, and exact final database/history state. Keep this task test-only, use barriers rather than sleeps, and include every case in the single Linux/macOS gate.

---

### 7. Complete identity and transaction release evidence

**Type**: GREEN
**Output**: Health and transaction capability suite passes on both supported platforms.
**Depends on**: 6

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Adjust request-boundary identity handling and phased transactions in `internal/connection`, terminal and cancellation routing in `internal/ui`, and exactly-once outcome finalization in `internal/history` only enough to satisfy Task 6. Preserve startup device/inode identity, verify it before every request and newly opened connection and again after request errors, map typed absence/mismatch to exact terminal behavior, and retain the documented successful-race and same-inode rules. Make the post-statement/pre-COMMIT cancellation decision atomic; before it, cancellation must select rollback and await confirmation, while rollback cleanup and committing issue no interrupt and resolve exactly once. Do not add identity checks inside the transaction boundary, claim untouched state without confirmed rollback, or exit with pending work. Run the unchanged canonical capability command on Linux and macOS with the exact modernc pin and retain passing health/transaction evidence; fix implementation or change the vetted pin if needed rather than weakening the gate.

---

### 8. Document release traceability and procedures

**Type**: DOCUMENT
**Output**: Wiki contains one-to-one PRD high-risk test mapping, suite invocation, modernc upgrade gate, and full manual matrix checklist.
**Depends on**: 7

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #56 implementation, capability tests, `go.mod` pin, and CI configuration from `internal/connection`, `internal/ui`, `internal/history`, and the repository CI paths into the appropriate pages under `Notes/wiki`. Record the one canonical suite invocation, exact vetted modernc version, Linux/macOS job names and triggers, release-blocking semantics, dependency-upgrade trigger, and prohibition on skips, allowed failures, best-effort acceptance, or weakened assertions. Build a one-to-one traceability table from every case in PRD high-risk Testing Decisions items 2 and 3 to a unique named gated test, including journal/pool configuration, external-writer overlap, all cancellation isolation/bounds/no-history cases, and pre/post-COMMIT behavior; also map Issue #56's required identity race and transaction evidence. Define the modernc upgrade procedure: change the exact pin and sums, run the same local gate, require both CI platforms, reject or replace any failing version, and retain evidence. Add one reusable release checklist covering every exact layout arithmetic, focused-field scroll, horizontal/oversized column, multiline/overlay, row/column preserve-clamp-fetch resize, active-request resize, and below/above-minimum restoration scenario at 80×24, 100×30, and 160×50 on Linux and macOS, with fields to record release/version/platform/results. Cross-reference Issues #5-#7, #21, #24, #28-#29, #32, #41-#43, #55-#56 and the Implementation Decisions, Module Design, Testing Decisions, and Acceptance Criteria sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` for every added or removed page and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 9. Create the release-suite walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/056-09/code-walkthrough`.
**Depends on**: 8

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/056-09/code-walkthrough`. Show the exact modernc pin, canonical capability command, Linux/macOS CI jobs and dependency-change trigger, then execute or present retained passing output from both platforms without omitted tests. Demonstrate barrier-backed WAL and rollback-journal page/count overlap on distinct leases, external-writer delay/error, exact pool/busy/length configuration, and unchanged journal mode. Exercise independent CPU and lock cancellation, late-success rejection, unaffected concurrent/subsequent work, schema/estimate no-history cancellation, deletion/rename/replacement races and exact terminal messages, and pre-COMMIT rollback versus post-boundary no interrupt/commit enforcement. Walk every PRD high-risk items 2 and 3 traceability row to its named gated test and show that a deliberate capability failure blocks the gate rather than being skipped. Present the completed Linux/macOS 80×24, 100×30, and 160×50 manual checklist with every required rendering/resize scenario. Reference Issue #56 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---

### 10. Review integrated release evidence

**Type**: REVIEW
**Output**: Human confirms both CI platforms, upgrade blocking, complete traceability, and the 80×24/100×30/160×50 matrix on Linux and macOS.
**Depends on**: 9

Review the exact pin in `go.mod`, integrated tests in `internal/connection`, `internal/ui`, and `internal/history`, CI configuration, wiki traceability/procedures, and `Notes/walkthroughs/056-09/code-walkthrough` against Issue #56. Confirm the one canonical capability command passes from clean checkouts in both Linux and macOS jobs and that a controlled capability failure or modernc version change triggers and blocks both jobs without allowed failures or skips. Audit every PRD Testing Decisions item 2 and 3 requirement against one named executed test, then inspect journal/pool, cancellation, identity-race, and transaction evidence for barriers, exact bounds/settings/messages, cleanup, and unchanged journal mode. Execute and sign one full manual matrix on Linux and one on macOS at 80×24, 100×30, and 160×50, checking exact footer/builder/results arithmetic and page rows, focused builder scrolling, one-column and oversized-column behavior, multiline values and edge overlays, first-row/first-column preservation plus clamp/fetch branches, resize during active requests, and exact below/above-80×24 restoration. Approve release evidence only when both platforms, upgrade blocking, traceability, and every checklist cell are complete.

---
