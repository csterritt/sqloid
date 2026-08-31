# Tasks for #71: Preserve absolute logical positions for later-page failures

Parent issue: #71
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify the started later-page offset contract

**Type**: RED  
**Output**: Failing connection tests prove StartPage receives nonzero logical offsets and reports absolute value/page-limit failure positions.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add focused tests under `internal/connection` for `StartPage`, following `value_limit_test.go`, `page_test.go`, and the controllable started-request seams. Start later-page SQL rendered with several nonzero OFFSET values and pass the same logical offset separately to the execution boundary. Inject or construct oversized values at the first and later page-relative rows and page-envelope failures after complete leading rows; require typed `*result.LimitFailure` positions equal `offset + relative index + 1`, exact shared value/page error strings, and no partial failing row. Add offset-zero controls for `StartFirstPage` and `StartPage`, cancellation/ordinary-error controls, and assertions that the supplied execution offset matches the SQL request's intended range. Keep this task test-only.

---

### 2. Carry offset through StartPage scanning

**Type**: GREEN  
**Output**: StartPage passes its requested logical offset to the shared scanner and all connection diagnostics use absolute positions.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Change the `StartPage` signature in `internal/connection/started_request.go` to accept the logical offset explicitly and pass it unchanged to `runFirstPage`; keep `StartFirstPage` fixed at offset zero. Align comments with the existing offset contract in `internal/connection/page.go` and `firstpage.go`, and rely on their shared scanner to compute `offset + rowIdx + 1` for execute, scan, iteration, oversized-value, and page-cap failures. Update direct production call sites and connection tests to compile, passing the exact QueryBuilder PageSQL offset rather than parsing SQL. Preserve dedicated leases, cancellation, partial complete rows, typed causes, and settlement behavior.

---

### 3. Specify offset propagation across UI and adapters

**Type**: RED  
**Output**: Failing adapter/UI tests prove the PageSQL offset and execution-boundary offset are identical for forward, backward, resize, and capability requests.  
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend later-page orchestration tests in `internal/ui/vertical_paging_test.go` and any production connection-adapter tests so the page executor fake records a structured logical offset in addition to SQL and parameters. For forward and backward ranges, nonzero current pages, Limit-clamped ranges, and resize/refetch paths, require the offset passed to the adapter and ultimately `connection.StartPage` to equal the OFFSET used by `QueryBuilder.PageSQL`. Inject value and page `LimitFailure` outcomes at known relative rows and require the UI-visible exact absolute row-N message. Update `internal/connection/select_cancellation_capability_test.go` expectations to use the same signature while retaining its later-page cancellation proof. Keep this task test-only and do not parse OFFSET from statement text in adapters.

---

### 4. Thread logical offset through every page executor seam

**Type**: GREEN  
**Output**: Production adapters, interfaces, fakes, and capability tests share one explicit offset contract and later-page diagnostics are absolute.  
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Update the `PageExecutor` contract in `internal/ui/paging.go`, the page command capture, application wiring, production connection adapter, fakes, and capability call sites so each later-page request carries `(sql, params, logicalOffset)` from the single `pageRange` calculation. Pass that value unchanged to `connection.DB.StartPage`; do not derive it from displayed rows later, parse rendered SQL, or substitute cache-relative positions. Keep request/execution/generation identities, serialized pending behavior, cancellation context, user-Limit clamping, and first-page zero-offset behavior unchanged. Run focused querybuilder, connection, and UI paging/cancellation tests after updating all implementations of the function type.

---

### 5. Document absolute later-page failure positions

**Type**: DOCUMENT  
**Output**: Wiki documentation records the explicit PageSQL-to-scanner offset contract and exact one-based value/page diagnostics.  
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest Issue #71's changes from `internal/connection/started_request.go`, `page.go`, `firstpage.go`, `internal/ui/paging.go`, production adapters, and their tests into the appropriate `Notes/wiki` pages. Document that one logical zero-based offset is calculated with the page range, rendered into PageSQL, passed explicitly through every executor, and used by the scanner to report one-based absolute `offset + relative index + 1` positions. Record exact value/page 64 MiB diagnostics, complete-leading-row/no-partial-row behavior, and unchanged first-page offset zero. Cross-reference Issue #71 and the paging, cache envelope, Connection/UI Module Design, and high-risk Testing Decisions in `Notes/PRD-sqloid.md`; update the index and append the required dated wiki log entry.

---

### 6. Create the later-page offset walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/071-06/code-walkthrough`.  
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/071-06/code-walkthrough`, with the main file named `walkthrough.md`. Trace one nonzero offset from `pageRange` through PageSQL, the UI executor/production adapter, `StartPage`, and `runFirstPage`. Trigger oversized value and page-cap failures at first and later relative rows on multiple pages and show the exact one-based absolute row-N diagnostics with complete leading rows only; compare an offset-zero first-page control. Include forward/backward and Limit-clamped contract evidence plus the updated cancellation capability test, reference Issue #71 and `Notes/PRD-sqloid.md`, and keep all artifacts under the approved directory.

---
