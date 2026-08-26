# Tasks for #26: SELECT request identities and stale-response rejection

Parent issue: #26
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify execution/request/generation guards

**Type**: RED
**Output**: Failing barrier-controlled tests deliver out-of-order first/later page responses across old execution IDs, request IDs, viewport generations, resize, and deactivation.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add barrier-controlled scripted tests in `internal/ui` using the fake `internal/connection` request seam from Issues #24 and #25. Deliver first-page and later-page successes and failures out of order while independently varying SELECT execution ID, page request ID, and viewport generation; require a response to mutate visible rows, range, loading state, or retained cache only when every applicable identity is current. Cover a superseded execution, a replaced request within the same execution, resize generation advancement, result-history entry or other SELECT deactivation/finalization, and a current response control case. Prove old responses cannot clear a newer request's feedback or alter cache metadata, and that first-page/count guards introduced by Issue #24 remain intact while later pages add independent generation tracking. Keep this task test-only and use explicit barriers/messages rather than timing sleeps.

---

### 2. Implement current-response acceptance guards

**Type**: GREEN
**Output**: Only responses whose execution, request, and generation are all current can mutate UI/cache state.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend the SELECT request state machine in `internal/ui` so first and later page commands capture immutable execution, request, and viewport-generation identities at dispatch and validate all applicable identities before any result, error, loading, or cache mutation. Advance the viewport generation on resize and on every deactivation/finalization path, preserve Issue #24's distinct count identity, and keep later-page request identity independent from count. Centralize acceptance at the response boundary rather than scattering partial checks through rendering or cache code; stale responses must be inert and must not clear current feedback. Preserve `internal/connection` as the owner of database request execution/settlement and `internal/querybuilder` as the owner of page construction, implementing only enough to make Task 1 pass.

---

### 3. Specify cancellation and replacement settlement ordering

**Type**: RED
**Output**: Failing tests cover cancelled late success, newer execution after cancellation, and prohibition of replacement work before predecessor settlement.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend the barrier-controlled tests in `internal/ui` and focused lifecycle tests in `internal/connection` to specify stale-result classification and replacement ordering for both first and later pages. Request cancellation, then release a late success and require cancellation to win: rows/cache remain unchanged, the old response is classified as cancelled, and it cannot clear state belonging to a newer execution. Start a newer SELECT only through the allowed settlement seam and prove no replacement page or count command is dispatched and no dedicated lease is reused until every predecessor being replaced has actually settled. Cover cancellation followed by a newer execution ID, same-execution page replacement after resize, late errors as well as success, and independent count/page settlement order. Keep this task test-only, preserve Issue #28's Ctrl+W interrupt wiring scope, and test observable commands and classifications rather than driver internals.

---

### 4. Implement stale-result classification and serialization

**Type**: GREEN
**Output**: Cancellation classification and replacement-settlement tests pass for first and later pages.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Integrate cancellation-wins result classification and predecessor-settlement serialization across `internal/connection` and the SELECT orchestration in `internal/ui`. Retain cancellation-requested state with each execution/request identity until true settlement, classify every post-cancellation success as cancelled even when a newer execution is current, and release or replace work only after all affected page/count predecessors settle. Ensure first-page, later-page, resize replacement, deactivation, and newer-SELECT paths share the same acceptance and settlement rules without serializing the independent page/count pair during normal Issue #24 startup. Do not force-close connections, permit a stale result to mutate cache/UI, or move page SQL ownership out of `internal/querybuilder`; implement only enough to make Tasks 1 and 3 pass while leaving the physical Ctrl+W capability proof to Issue #28.

---

### 5. Document request identity guards

**Type**: DOCUMENT
**Output**: Wiki documentation records all identities, generation changes, stale-response rules, and settlement ordering.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #26 implementation and tests from `internal/ui`, `internal/connection`, and the page-request boundary in `internal/querybuilder` into the appropriate pages under `Notes/wiki`. Document SELECT execution IDs, distinct count/page request IDs, later-page identities, viewport generations, and the requirement that every applicable identity be current before UI or cache mutation. Record exactly when resize, deactivation, finalization, and newer execution invalidate responses; explain stale success/error handling, cancellation-wins late-success classification, independent page/count settlement, and the rule that replacement work cannot start or reuse a lease before every replaced predecessor settles. Cross-reference Issues #24, #25, #26, and #28 and the Identities and state, SELECT lifecycle, Global Key Precedence, Errors and cancellation bounds, Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the stale-response walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/026-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/026-06/code-walkthrough`. Use deterministic barriers to show out-of-order first-page and later-page responses from old execution IDs, request IDs, and viewport generations being rejected after newer SELECTs, resize, result-history entry, and other deactivation. Demonstrate that a fully current response alone can update rows/cache, stale responses cannot clear newer feedback, cancellation wins over late success, and page/count replacement waits for predecessor settlement without breaking normal first-page/count concurrency. Reference Issue #26 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
