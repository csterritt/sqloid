# Tasks for #25: Serialized vertical result paging

Parent issue: #25
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify page SQL and ordering policy

**Type**: RED
**Output**: Failing tests cover LIMIT/OFFSET ranges, ordinary-rowid fallback ordering, all excluded object/query kinds, and user aggregate ORDER BY ASC/DESC without appended rowid.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add UI-independent table-driven tests in `internal/querybuilder` for Issue #25's page-request SQL, using the completed SELECT builder contracts and refreshed rowid metadata supplied by `internal/schema`. Require adjacent page requests to preserve the base SELECT, bound parameters, user Limit semantics, and exact LIMIT/OFFSET ranges without interpolating user values. Cover implicit `ORDER BY rowid` only when there is no user ORDER BY and the source is an ordinary rowid table with no declared `rowid`, `_rowid_`, or `oid` shadow; cover views, virtual tables, WITHOUT ROWID tables, every declared-rowid shadow case, aggregate-only and grouped queries, and other ineligible query shapes as explicit no-fallback cases with no stability claim. Require a selected aggregate or grouped ORDER BY expression in both ASC and DESC to remain byte-for-byte the user-selected expression and direction, with no appended rowid tie-breaker. Keep this task test-only and avoid encoding schema classification or UI paging state inside QueryBuilder tests.

---

### 2. Implement page request construction

**Type**: GREEN
**Output**: Page SQL and ordering-policy tests pass using schema rowid metadata.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the minimal page-construction API in `internal/querybuilder`, extending the existing safe SELECT rendering path rather than creating a second SQL builder. Consume the selected object's kind, rowid capability, and declared-rowid-shadow metadata from `internal/schema`; append fallback `ORDER BY rowid` only for the single eligible ordinary-table case, preserve every explicit user ORDER BY expression and ASC/DESC direction without adding a tie-breaker, and leave excluded object/query kinds unordered. Apply page LIMIT/OFFSET around the user's logical Limit so requests cannot read beyond it, preserve parameter ordering and identifier quoting, and expose enough immutable request metadata for `internal/ui` to ask `internal/connection` for an exact range. Implement only enough production code to make Task 1 pass, without adding navigation orchestration, cache behavior, or new ordering guarantees.

---

### 3. Specify serialized paging and exact page size

**Type**: RED
**Output**: Failing model tests cover adjacent Page Up/Down, count-plus-one-page concurrency, repeated/opposite key suppression, loading feedback, and complete visible-row calculation.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add deterministic scripted model tests in `internal/ui` with the controllable fake `internal/connection` seam established by Issues #22 and #24. For an idle active SELECT, require Page Down and Page Up to request exactly the adjacent absolute logical range through QueryBuilder's page API, stop at known low/high boundaries, and keep at most one page request pending while permitting only the independent count request to coexist. Hold page responses behind barriers and prove repeated and opposite page keys are consumed, create no additional connection request, preserve the requested range, and show page-loading feedback while horizontal column movement remains local. Add focused layout/model tests for every supported terminal height needed to prove page size equals all complete visible data rows after the results border, status/count line, and frozen header, with no partially visible row counted; require the next request after a size change to use that exact value. Keep this task test-only, assert commands/request counts rather than private fields, and do not take over viewport-generation rejection or cancellation behavior assigned to Issues #26 and #28.

---

### 4. Implement vertical paging orchestration

**Type**: GREEN
**Output**: Offsets, exact page sizes, serialized request count, and ignored-key feedback tests pass.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Integrate serialized Page Up/Down orchestration in `internal/ui`, using `internal/querybuilder` for page SQL/ranges and `internal/connection.ExecutePage` for database work. Derive the request limit from the existing results-area layout arithmetic as the exact count of complete data rows, track the one pending page request independently from Issue #24's count request, and issue only the required adjacent offset from the active SELECT's displayed logical range. While a page is pending, consume repeated or opposite page keys without stacking commands and retain visible loading feedback; continue to permit local horizontal movement and count settlement. Preserve active SELECT state, result headers, user Limit boundaries, and Issue #24's first-page/count behavior, and leave cancellation, stale generation handling, and cache-cap policy to their owning issues. Implement only enough to pass Tasks 1 and 3.

---

### 5. Document serialized paging

**Type**: DOCUMENT
**Output**: Wiki documentation records navigation, sizing, concurrency, ordering guarantees, and limitations.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #25 implementation and tests from `internal/querybuilder`, `internal/connection`, `internal/ui`, and the consumed `internal/schema` metadata into the appropriate pages under `Notes/wiki`. Document adjacent Page Up/Down navigation, LIMIT/OFFSET range construction, exact complete-visible-row sizing, one-page serialization, count-plus-one-page concurrency, ignored repeated/opposite keys and loading feedback, and preserved local horizontal interaction. State precisely that implicit `ORDER BY rowid` applies only to ordinary rowid tables without a declared rowid shadow and that views, virtual tables, WITHOUT ROWID or shadowed tables, aggregate/grouped queries, ties, and concurrent writes have no implied stability; record that explicit aggregate/grouped ASC/DESC ordering is preserved without an appended rowid. Cross-reference Issues #22, #24, and #25 and the Paging consistency, Grid rendering/cache, Resize/layout, Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the vertical-paging walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/025-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/025-06/code-walkthrough`. Demonstrate forward and backward adjacent paging over a large fixture, exact offsets and page sizes at multiple supported terminal heights, and no additional request when repeated or opposite Page keys arrive while a page is held pending. Show the maximum count-plus-one-page overlap, visible loading feedback, and unaffected local horizontal navigation. Include QueryBuilder evidence for eligible ordinary-rowid fallback ordering, every excluded object/query category, and explicit aggregate/grouped ORDER BY in ASC and DESC without an appended rowid. Reference Issue #25 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---

### 7. Review vertical paging

**Type**: REVIEW
**Output**: Human confirms forward/backward traversal and repeated/opposite keys during loading.
**Depends on**: 6

Review the completed page SQL in `internal/querybuilder`, request integration through `internal/connection`, orchestration and layout arithmetic in `internal/ui`, wiki updates, and `Notes/walkthroughs/025-06/code-walkthrough` against Issue #25. Traverse a large fixture forward and backward, resize across supported heights, and confirm each request uses the adjacent offset and exact number of complete visible rows. Hold a page request pending while pressing repeated and opposite Page keys and horizontal bindings; verify no page stacking, count-plus-one-page as the maximum concurrency, visible loading feedback, and local horizontal responsiveness. Inspect ordinary-rowid fallback and all documented exclusions, including explicit aggregate ORDER BY ASC/DESC, before approving the issue.

---
