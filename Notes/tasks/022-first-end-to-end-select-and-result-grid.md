# Tasks for #22: First end-to-end SELECT and result grid

Parent issue: #22
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify shared result representation primitives

**Type**: RED
**Output**: Failing package tests cover full-set name deduplication, finite REAL tokens, control characters, maximal invalid UTF-8 replacement/metadata, and exact BLOB retention/display.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Create UI-independent table-driven contract tests in `internal/result` for the shared output-name and typed-value seam required by Issue #22 and later exporters. Require left-to-right deduplication across the entire original output-name set: first occurrence unchanged and each duplicate assigned the lowest available numeric suffix that collides with neither an earlier final name nor any original name; include empty labels, pre-suffixed labels, repeated computed labels such as `COUNT(*)`, and collision chains. Model SQLite NULL, INTEGER, REAL, TEXT, and BLOB without collapsing types. For every finite REAL, assert the exact shortest round-trip token from the PRD policy, including integral-looking values receiving `.0`, signed zero, exponents, precision edges, and locale independence, while INTEGER and identical-looking TEXT remain distinguishable. Require grid-facing TEXT rendering to replace tabs and newlines with the designated visible symbols, decode each maximal invalid UTF-8 byte sequence as exactly one U+FFFD, and set explicit warning metadata without changing row order or count. Require BLOB payloads to be copied/retained byte-for-byte, including invalid UTF-8 and empty bytes, while grid display is exactly `[BLOB n bytes]`. Keep this task test-only, do not import `internal/ui`, do not alter generated SQL or driver column metadata, and defer non-finite REAL policy to Issue #23 and CSV/JSON serialization to their later issues.

---

### 2. Implement the UI-independent result seam

**Type**: GREEN
**Output**: Shared result-name/value primitives pass without grid or exporter duplication.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add the minimal package-shaped representation in `internal/result` needed to pass Task 1. Define typed result columns, values, rows, and invalid-UTF warning metadata independently of Bubble Tea, database-driver concrete types, and exporter formats. Centralize full-set collision-safe output-name deduplication, finite REAL token generation, maximal-invalid-sequence TEXT decoding, visible grid control-character transformation, and BLOB display metadata while preserving exact BLOB bytes in the underlying value. Keep original driver labels available separately from deduplicated display/export names and ensure render helpers cannot convert TEXT that merely looks numeric, null, BLOB-like, or non-finite into another SQLite type. Expose one shared seam that `internal/ui` can consume now and future CSV/JSON packages can extend rather than copy; do not add grid layout, SQL execution, non-finite rendering, or exporter-specific escaping.

---

### 3. Specify first-page SELECT execution

**Type**: RED
**Output**: Failing fake/SQLite tests cover generated SQL/params, validation handoff, history append timing, typed rows, and execution errors.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add focused pure, fake-Connection, and `modernc.org/sqlite` integration tests across `internal/querybuilder`, `internal/connection`, `internal/history`, and the orchestration seam in `internal/ui` for the first production SELECT path. Start from representative runnable builder states and assert the exact safely quoted SQL and ordered bound parameters produced by QueryBuilder, including wildcard, duplicate-label projections, WHERE values, GROUP/ORDER choices, and user Limit where supported by completed builder issues. Require runnable Enter to complete Issue #21 schema validation before any page request, then start one actual SELECT execution, exit query/result history if necessary, and append query history only at that actual-execution boundary with existing consecutive-identical suppression. Exercise Connection's page API against fake and SQLite fixtures and require eager typed NULL/INTEGER/REAL/TEXT/BLOB rows plus original driver labels to cross into `internal/result` without string coercion or byte loss. Cover zero rows, query/scan errors, post-validation DDL errors, invalid UTF-8 TEXT metadata, and BLOBs; require execution failures to follow the ordinary result-error boundary while failed validation still creates no execution or history. Keep this task test-only and limit it to one first-page request; concurrent counting, later paging, cache caps, and full finalization are assigned to later issues.

---

### 4. Implement the production SELECT execution path

**Type**: GREEN
**Output**: Builder→validation→execution tests pass through Connection and History boundaries.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the first production SELECT route across `internal/querybuilder`, `internal/connection`, `internal/history`, `internal/result`, and `internal/ui`. Use QueryBuilder as the sole source of safely quoted SQL and ordered parameters, accept execution only from the successful current validation handoff introduced by Issue #21, assign the actual SELECT execution identity at that boundary, and append the complete builder state to query history at the established timing. Add or complete Connection's cancellable first-page execution method on a dedicated lease, eagerly scan original column labels and typed SQLite values, preserve BLOB bytes, and convert rows once into the shared `internal/result` representation. Return ordinary query and scan failures through typed orchestration messages without embedding database calls or driver details in Bubble Tea state. Preserve the Issue #6 request lifecycle and Issue #7 health classification, and implement only the first page needed by Issue #22; do not fake a count, serialize values to export formats, add later-page cache behavior, or take over Issue #24's concurrent page/count identities.

---

### 5. Specify frozen-grid rendering

**Type**: RED
**Output**: Failing model tests cover deduplicated frozen headers, absolute range/status, typed cells, invalid-UTF warning, and exact `No rows`.
**Depends on**: 4

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add scripted model and focused view tests under `internal/ui`, consuming only `internal/result` fixtures and following the results-region ownership and responsive arithmetic established by Issues #8 and #11. For a successful first page, require a bordered result region with one frozen header row using shared full-set deduplicated names, stable while data rows are viewed, and a status line showing the displayed inclusive absolute logical range rather than page-relative indexes. Assert typed grid cells preserve distinctions among NULL, INTEGER, finite REAL, TEXT, and BLOB: finite REAL uses the shared exact token, tabs/newlines use visible symbols, identical-looking TEXT remains text, and BLOB display is exactly `[BLOB n bytes]` while backing bytes remain unchanged. When maximal invalid UTF-8 replacement occurs, require the rendered U+FFFD result and persistent result-warning metadata in the header/status without adding a row or column. For a successful SELECT with zero rows, require exact `No rows`, no misleading data range, and clear distinction from the pre-execution startup prompt. Cover long/Unicode values through the existing width/ellipsis seam without implementing Issue #23 non-finite tokens, Issue #24 independent counts, or later horizontal/paging/cache contracts. Keep this task test-only and forbid grid-local name/value formatting copies.

---

### 6. Implement the first production result grid

**Type**: GREEN
**Output**: End-to-end SELECT/grid tests pass using the shared result seam.
**Depends on**: 5

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Integrate the first-page result messages into the Bubble Tea model and results view in `internal/ui`, using `internal/result` as the only source of deduplicated names and typed cell display. Replace the idle results content after successful execution with a bordered grid whose frozen header, status/absolute range, complete-row sizing, cell widths, Unicode width handling, and ellipsis behavior follow the existing layout seams and the PRD. Preserve raw typed rows and warning metadata separately from rendered cells, display invalid-UTF warnings without modifying tabular data, and show exact `No rows` for an executed empty SELECT. Route first-page errors to the ordinary error view while retaining the history behavior established by Task 4 and keeping older finalized results available through the existing seam. Do not introduce private UI formatting for output names, finite REALs, invalid UTF-8, or BLOBs, and do not simulate result counts or implement non-finite REAL, concurrent count, later paging, export, or dual-cap cache behavior.

---

### 7. Remove the disposable tracer runtime path

**Type**: REFACTOR
**Output**: Only the production builder→validation→execution route remains; reusable fixtures/helpers are retained and architecture checks pass.
**Depends on**: 6

Remove or fully replace the hardcoded Issue #10 production runtime path from `internal/ui`, command startup wiring, and any runtime-facing Connection adapter now superseded by the builder → schema validation → production SELECT execution flow. Trace all call sites and delete hardcoded SQL, tracer-only result fabrication, duplicate row/value types, and alternate execution shortcuts so a user-visible SELECT can reach Connection only through the current QueryBuilder and validation orchestration. Retain and rename reusable fixture creation, fake Connection controls, integration harnesses, and test helpers where they serve the production tests; do not remove evidence that still validates startup, layout, cancellation, or SQLite plumbing. Add or update focused architecture assertions to prevent UI-private result representation and a second production execution route, then run the relevant package tests plus repository build/vet checks, changing no externally specified behavior from Tasks 1–6.

---

### 8. Document SELECT execution and result representation

**Type**: DOCUMENT
**Output**: Wiki documentation records module flow, grid behavior, shared result contracts, and tracer removal.
**Depends on**: 7

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #22 implementation and tests from `internal/querybuilder`, `internal/schema`, `internal/connection`, `internal/history`, `internal/result`, and `internal/ui` into the appropriate pages under `Notes/wiki`. Document the production builder → validation → actual execution → first-page → grid flow, exact query-history append boundary, typed row scanning, error handling, frozen deduplicated headers, absolute displayed range/status, invalid-UTF warning, exact `No rows`, visible control characters, exact BLOB preservation/display, and finite REAL token policy. Record the full-set output-name collision rule and the architectural requirement that grid and future exporters share `internal/result` rather than duplicate representation logic. Explain removal of the disposable Issue #10 runtime path and identify any retained test fixtures/helpers. Cross-reference Issues #10, #21, and #22 and the Builder and Display Interaction, Execution and Result Lifecycle, Grid rendering/cache, Invalid UTF-8 TEXT, Output names, Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` for all new or materially changed pages and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 9. Create the first-SELECT walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/022-09/code-walkthrough`.
**Depends on**: 8

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/022-09/code-walkthrough`. Demonstrate a runnable builder producing safe SQL/parameters, successful schema validation, the actual history append boundary, SQLite first-page execution, and the resulting bordered grid with frozen headers and absolute range/status. Include normal rows, full-set duplicate-label collisions, a successful empty result showing exact `No rows`, typed NULL/INTEGER/finite REAL/TEXT values, visible tabs/newlines, maximal invalid UTF-8 replacement with warning metadata, exact BLOB bytes with `[BLOB n bytes]` display, and an ordinary execution error. Show package or architecture evidence that `internal/result` is shared and UI-independent and that the hardcoded Issue #10 production tracer route no longer exists while reusable fixtures remain. Reference Issue #22 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
