# Tasks for #28: Scoped Ctrl+W cancellation and bounded settlement

Parent issue: #28
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify scoped SELECT cancellation orchestration

**Type**: RED
**Output**: Failing fake/model tests cover independent active page/count interrupts, `cancelling…`, all-request settlement, no replacement, late success, and no force-close.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add deterministic fake lifecycle tests in `internal/connection` and scripted model tests in `internal/ui` for applying Issue #6's cancellation infrastructure to an active SELECT. Cover first-page plus count, count-only, first-page-only, and later-page-only work; when Ctrl+W is accepted, require one scoped cancellation request for each currently active page/count request, independent connection-scoped interrupt identities, and no effect on inactive or unrelated work. Hold settlement behind barriers and require visible `cancelling…` until every requested cancellation settles, no replacement SELECT/page/count work or lease reuse before then, cancellation-wins rejection of released late success, and correct error/partial-row handling through Issue #26's identity guards. Assert cancellation is idempotent, connections are never force-closed, and a subsequent request can use the healthy settled connection. Keep this task test-only and exclude schema validation, estimate, and write-phase integrations owned by Issues #21 and #41–#43.

---

### 2. Apply cancellation infrastructure to SELECT requests

**Type**: GREEN
**Output**: Ctrl+W cancellation and settlement tests pass for first page, count, and later page-only work.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Wire `internal/ui`'s scoped Ctrl+W handling for active SELECT work to the reusable cancellable request handles in `internal/connection` from Issue #6. Track page and count handles independently, request cancellation only for the currently active ones, retain `cancelling…` until all targeted requests truly settle, and defer finalization or replacement dispatch until that settlement. Feed every result through Issue #26's execution/request/generation and cancellation-wins classification so late success cannot update rows/cache even after another SELECT begins. Preserve existing rows and lifecycle metadata where cancellation occurs after rows, create the established cancelled-before-rows state where applicable, never force-close a connection, and leave SQL construction in `internal/querybuilder`. Implement only enough to make Task 1 pass without altering schema-validation, estimate, or write cancellation ownership.

---

### 3. Specify cross-platform SELECT cancellation bounds

**Type**: RED
**Output**: Failing Linux/macOS barrier tests cover independent CPU page/count interruption, isolation, one-second CPU settlement, five-second lock settlement, and unaffected next requests.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add mandatory release-blocking Linux/macOS integration tests in `internal/connection`, applying the exact modernc capability seam proven by Issue #6 to real SELECT page and count requests generated through `internal/querybuilder`. Use synchronization barriers to prove independent CPU-bound page and count work has started on distinct leased connections before cancellation, cancel either one and then both, and assert connection-scoped isolation plus all-request settlement. Require controlled CPU work to settle within one second and a controlled lock-wait page or count request no later than the configured five-second busy timeout; include cancellation-wins late success, no force-close, no replacement before settlement, and successful unaffected subsequent requests on each interrupted physical connection. Use sleeps only to measure explicit PRD bounds, retain journal-mode independence, and make capability failure block supported-platform release rather than becoming a best-effort skip.

---

### 4. Harden scoped interrupt behavior

**Type**: GREEN
**Output**: Mandatory SELECT cancellation capability tests pass on supported platforms.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Harden the SELECT page/count request path in `internal/connection` so each request's context and interrupt target the exact dedicated physical connection leased for that request, reusing Issue #6's vetted modernc integration without exposing driver APIs to `internal/ui` or `internal/querybuilder`. Preserve independent page/count cancellation and isolation, cancellation-wins result classification, true settlement before lease release/replacement, the configured five-second busy bound, and safe subsequent work with no force-close. Connect any required lifecycle signal back to `internal/ui` through the existing abstraction so visible all-request settlement remains accurate. If the pinned driver or implementation fails the mandatory one-second CPU, five-second lock, isolation, late-result, or reuse evidence on Linux/macOS, fix the vetted pin or integration rather than weakening or skipping the tests; implement only enough to make Task 3 pass.

---

### 5. Document scoped SELECT cancellation

**Type**: DOCUMENT
**Output**: Wiki documentation records Ctrl+W scope, visible settlement, isolation, bounds, and ownership split with other cancellable phases.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #28 implementation, fake/model tests, and Linux/macOS capability tests from `internal/connection`, `internal/ui`, and the consumed `internal/querybuilder` page/count request paths into the appropriate pages under `Notes/wiki`. Document Ctrl+W scope over active first-page, later-page, and count requests; independent interrupt identities and leases; `cancelling…` until all targeted work settles; cancellation-wins late-success rejection; no replacement or lease reuse before settlement; no force-close; isolation; and healthy subsequent requests. Record the mandatory one-second controlled CPU and five-second lock-wait bounds on both supported platforms. Clearly assign schema-validation cancellation to Issue #21, estimate cancellation to Issue #41, beginning/executing write cancellation to Issue #42, and post-commit-boundary behavior to Issue #43. Cross-reference Issues #6, #21, #26–#28, and #41–#43 and the Identities and state, SELECT lifecycle, Global Key Precedence, Errors and cancellation bounds, Connection/UI Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the SELECT-cancellation walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/028-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/028-06/code-walkthrough`. Demonstrate scoped Ctrl+W against concurrent first-page/count work and later-page-only work, independent interrupt targeting, persistent `cancelling…` until every targeted request settles, no replacement before settlement, cancellation-wins late-success rejection, and no force-close. Capture barrier-controlled CPU and lock-wait evidence for the one-second and five-second bounds on the available supported platform, show isolation when only page or count is cancelled, and run successful subsequent requests on the affected connections. Explain the Linux/macOS release requirement and the ownership split for schema validation, estimate, write execution, and post-commit phases. Reference Issue #28 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---

### 7. Review cancellation capability

**Type**: REVIEW
**Output**: Human confirms CPU, lock-wait, page/count isolation, late-result, and subsequent-request behavior on Linux and macOS.
**Depends on**: 6

Review SELECT cancellation orchestration in `internal/ui`, scoped interrupt and settlement behavior in `internal/connection`, page/count requests from `internal/querybuilder`, wiki updates, and `Notes/walkthroughs/028-06/code-walkthrough` against Issue #28. On Linux and macOS, cancel controlled CPU-bound and lock-wait first-page, count, and later-page requests independently and together; confirm one-second CPU and five-second lock settlement, unaffected concurrent work, `cancelling…` until all targets settle, and no replacement before settlement. Release a late success, verify it remains cancelled and cannot mutate rows/cache, confirm no connection is force-closed, and run healthy subsequent requests on each connection. Check that cancellation ownership for validation, estimate, and write phases remains with the documented later issues before approving the capability.

---
