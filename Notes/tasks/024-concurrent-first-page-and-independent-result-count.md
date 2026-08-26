# Tasks for #24: Concurrent first page and independent result count

Parent issue: #24
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify page/count request construction

**Type**: RED
**Output**: Failing tests cover first-page SQL, complete SELECT count subquery including Limit, distinct request IDs, and exact count wording inputs.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add UI-independent table-driven tests across `internal/querybuilder`, `internal/connection`, and the result-state seam in `internal/result` for construction of the two requests launched by an actual SELECT. Starting from QueryBuilder's complete safely quoted SELECT and ordered bound parameters, require the first-page request to apply the exact page Limit/OFFSET semantics without changing user predicates, projection, grouping, ordering, or parameter order. Require the count statement to count the complete SELECT as a subquery, including the user's Limit inside that subquery so rows beyond the Limit are irrelevant, while avoiding any implication that this is a table count or pre-Limit count. Cover no user Limit, Limit smaller/larger than the fixture result, aggregate/grouped SELECTs, empty results, and bound WHERE values. Require one SELECT execution ID and two distinct nonzero/current request IDs, one for first page and one for count, with no interchangeable identity. Test count-presentation inputs separately so successful state can render exactly `Result count: N` without a user Limit and exactly `Result count: N (after Limit M)` with one. Keep this task test-only, do not start goroutines from QueryBuilder, and do not add later-page viewport generations assigned to Issue #26.

---

### 2. Implement concurrent page/count orchestration

**Type**: GREEN
**Output**: First page and count launch on distinct leases with independent autocommit results and request identities.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the minimal request construction and startup orchestration across `internal/querybuilder`, `internal/connection`, `internal/result`, and `internal/ui` needed by Task 1. Preserve QueryBuilder ownership of the complete SELECT and bound parameters, derive the page and complete-limited-result count statements through a UI-independent execution seam, and assign one actual SELECT execution ID plus distinct first-page and count request IDs. After Issue #21 validation succeeds, launch both Connection calls without waiting for either result; each must acquire and retain its own dedicated configured lease from Issue #5 through true settlement and execute as an independent autocommit read, with no transaction or shared snapshot tying them together. Reuse Issue #6's independent cancellation lifecycle and Issue #22's typed first-page conversion, preserve journal mode, and expose separately typed page and count completions to the Bubble Tea model. Do not silently serialize the calls, clamp rows to count, or implement later-page navigation/generation behavior.

---

### 3. Specify response identity and failure isolation

**Type**: RED
**Output**: Failing model tests cover execution/request guards, delayed superseded responses, count failure, drift, no clamping, and exact header variants.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add deterministic scripted `(model, msg) → (model, cmd)` tests under `internal/ui`, using the controllable fake Connection and typed result fixtures from Issues #6 and #22. Require a page or count completion to mutate active state only when both its SELECT execution ID and its role-specific request ID match the current identities; cover wrong-role IDs, duplicate responses, a delayed response from a superseded execution, count success arriving before or after page success, and an older delayed count arriving after a newer SELECT begins. While count is pending, require the established counting presentation; on success require exact `Result count: N` or `Result count: N (after Limit M)` based solely on the executed builder's user Limit. Make page and count observe deliberately different committed states and assert no rows are dropped, fabricated, or clamped when count is less than, greater than, or otherwise inconsistent with fetched positions. On count failure, require exact `Count unavailable`, documented/help-accessible independent-snapshot context, retained successful rows and paging capability, and no conversion of the active SELECT into a page failure. Conversely, require first-page failure to follow its ordinary result error path independently of count completion. Keep this task test-only and exercise cancellation/supersession settlement guards without extending later-page cache logic.

---

### 4. Integrate independent count state into results

**Type**: GREEN
**Output**: Stale-response, count wording, failure-isolation, and pageable-row tests pass.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Integrate independent count lifecycle state into `internal/result` and `internal/ui` while preserving the typed rows and grid from Issue #22. Store the current SELECT execution ID and separate page/count request IDs, gate every completion on both levels of identity, consume each role at most once, and discard delayed, duplicated, cancelled, or superseded messages without altering current rows, count text, history, or errors. Render counting, exact successful count variants, and exact `Count unavailable` from explicit state rather than inferring from row length; retain user Limit as executed metadata for wording. Keep page rows and paging availability independent from count success/failure and from inconsistent independently observed totals, never clamp cache/range/endpoints to count, and surface help that the count covers the complete limited SELECT and may drift independently. Preserve Connection's health and cancellation classifications, and avoid implementing Issue #26's later-page identities or broader cache endpoint inference.

---

### 5. Specify cross-journal overlap capability

**Type**: RED
**Output**: Failing Linux/macOS barrier tests cover distinct leases, WAL and rollback-journal overlap, unchanged mode, delayed count, and external-writer delay/error.
**Depends on**: 4

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add mandatory release- and dependency-upgrade-blocking `modernc.org/sqlite` integration tests in `internal/connection`, with end-to-end orchestration assertions where needed, for the concurrent page/count capability required by Issue #24. Build equivalent fixtures already configured in WAL and rollback-journal modes, record their mode before opening, and use synchronization barriers to hold first-page and count operations simultaneously after they have acquired leases. Prove the operations occupy two distinct physical connections from the exact-two pool, neither waits for artificial application serialization, and both can settle independently. Delay the count behind a controlled barrier while allowing the page to complete and remain usable; then interleave a controlled external writer to demonstrate the PRD-permitted independent snapshots, drift, and journal-specific delay or ordinary `database is locked` error behavior. Assert Connection never changes journal mode, busy handling remains the established five-second configuration, count failure does not invalidate successful page rows, and all barriers cleanly release leases on success/error. Use no sleeps for ordering except explicit timeout/busy bounds, and make failures identify journal mode, lease identity, and blocked phase.

---

### 6. Harden page/count overlap behavior

**Type**: GREEN
**Output**: Mandatory capability tests pass without hidden serialization or journal mutation.
**Depends on**: 5

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Adjust the production Connection and orchestration implementation only as needed to satisfy Task 5's Linux/macOS capability suite. Ensure first-page and count requests independently acquire distinct dedicated physical connections, run concurrently as autocommit reads, retain their leases until true settlement, and release them safely on every success, error, cancellation, and health-terminal path. Remove any shared mutex, transaction, single worker queue, or request sequencing that serializes these independent reads, while preserving the exact-two pool, per-connection busy/length settings, connection-scoped interrupts, and no-reuse-before-settlement rule from Issues #5 and #6. Do not issue journal-changing pragmas, conceal rollback-journal lock behavior, retry in ways that erase observable failure isolation, or promise a shared snapshot. Keep external-writer delay/error and drift as accepted outcomes delivered through the independent page/count state from Task 4, and retain barrier hooks only in test seams rather than production control flow.

---

### 7. Document independent result counts

**Type**: DOCUMENT
**Output**: Wiki documentation records count SQL, Limit semantics, independent snapshots/drift, IDs, wording, failures, and no clamping.
**Depends on**: 6

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #24 implementation and tests from `internal/querybuilder`, `internal/connection`, `internal/result`, and `internal/ui` into the appropriate pages under `Notes/wiki`. Document first-page request construction, the complete SELECT count subquery with the user's Limit inside it, ordered parameter preservation, one SELECT execution ID, distinct page/count request IDs, and two dedicated leases. Explain concurrent independent-autocommit behavior in WAL and rollback-journal modes, absence of a shared snapshot, permitted count/page drift and external-writer delay/error, unchanged journal mode, and the identity rules that discard stale or superseded responses. Record exact `Result count: N`, `Result count: N (after Limit M)`, and `Count unavailable` presentation, count-failure isolation, continued paging, and the strict no-clamping rule. Cross-reference Issues #5, #6, #22, and #24 and the SELECT lifecycle, Paging consistency, Connection pool, Module Design, and high-risk Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` for every new or materially changed page and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 8. Create the concurrent-count walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/024-08/code-walkthrough`.
**Depends on**: 7

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/024-08/code-walkthrough`. Demonstrate an actual SELECT launching first-page and complete-result count concurrently with one execution ID, distinct request IDs, and distinct dedicated leases. Show the generated count covering the complete SELECT including user Limit, both exact successful header variants, count arriving before and after page, a deliberately delayed count, and stale responses from a superseded execution being ignored. In controlled WAL and rollback-journal fixtures, capture barrier-backed evidence of overlap, unchanged journal mode, independent snapshots/drift, and an external writer causing the permitted delay or error. Show count failure producing exact `Count unavailable` while rows and paging remain usable and no inconsistent count clamps them. Reference Issue #24 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---

### 9. Review count/page concurrency

**Type**: REVIEW
**Output**: Human verifies WAL/rollback fixtures, delayed count, external writer, exact wording, and paging after count failure.
**Depends on**: 8

Review the Issue #24 changes and tests in `internal/querybuilder`, `internal/connection`, `internal/result`, and `internal/ui`, the wiki updates, and `Notes/walkthroughs/024-08/code-walkthrough`. Run the first-page/count flow against controlled WAL and rollback-journal fixtures and verify distinct leases, actual overlap, independent settlement, unchanged journal mode, and no hidden shared transaction or serialization. Exercise SELECTs with and without user Limit and confirm the count subquery semantics and exact two successful header variants. Delay and supersede count/page responses to verify execution/request guards, then interleave an external writer to observe permitted drift, delay, or ordinary lock error without row clamping. Force count failure and confirm exact `Count unavailable`, retained rows, and continued paging capability; also confirm first-page failure remains isolated from count behavior before approving the issue.

---
