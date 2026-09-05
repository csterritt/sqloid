# Audit Report: Sqloid v1

Parent PRD: `Notes/PRD-sqloid.md`  
Date: 2026-09-04  
Files in scope: 298

## Summary

Sqloid v1 is **not safe to consider production-complete as-is**. The most urgent defect is that a user's pre-commit cancellation can bypass the connection layer's atomic cancellation flag, allowing a destructive write to proceed or become outcome-unknown after Ctrl+W. The audit also found multiple production-only integration gaps in result identity, paging, 64 MiB enforcement, request settlement, terminal status propagation, save overwrite protection, and the release test gate.

## Critical findings

### 1. Destructive-write cancellation bypasses the atomic pre-COMMIT flag

**Location**: `internal/ui/write_exec.go:77-110`; `internal/session/session.go:312-327`; `internal/connection/write.go:288-307`  
**Category**: Logic / Security  
**Problem**: The UI cancels a parent `context.Context`, but production never calls `StartedWriteRequest.Cancel()`. `Request.CancelRequested()` is set only by `Request.Cancel()`, so a Ctrl+W arriving after statement success but before the pre-COMMIT check can leave the flag false. The transaction may commit despite the user's cancellation, or fail at commit with an unknown outcome.  
**Suggestion**: Expose a started-write handle whose cancellation path invokes `StartedWriteRequest.Cancel()`, and make the UI's cancellation command call that guarded method rather than cancelling only the parent context. Add a barrier test through the real session adapter at the after-statement/pre-COMMIT boundary.

---

## High findings

### 2. SELECT and write execution IDs collide and silently drop result-history entries

**Location**: `internal/result/select_identity.go:16-31`; `internal/history/result_entry.go:139-165`  
**Category**: Logic / Consistency  
**Problem**: SELECT and write IDs come from independent counters, both starting at 1, while the shared result store deduplicates only by numeric execution ID. The first write result is therefore rejected after the first SELECT result with the same ID, and vice versa.  
**Suggestion**: Use one process-wide actual-execution ID allocator for SELECT and write operations. Add mixed SELECT→write→SELECT history tests.

### 3. Writes neither finalize an active SELECT nor display normal write summaries

**Location**: `internal/ui/model.go:634-655`; `internal/ui/write_exec.go:164-227,253-275`; `internal/ui/result_history.go:24-74`  
**Category**: Logic  
**Problem**: Starting INSERT/UPDATE/DELETE does not call the active-SELECT finalizer, despite "actual new execution" being a required finalizer. On settlement, a normal write entry is appended but not selected or projected. Even when later browsed, `KindWrite` falls through to an empty `Reason` error rather than rendering `Summary`. Users can continue seeing an old SELECT and never see rows-affected/rows-added feedback.  
**Suggestion**: Finalize the active SELECT before every actual write starts, preserving the old query association. Select/project the retained write entry at settlement and render `KindWrite` from `Summary`.

### 4. Later pages are generated from the mutable builder, not the executed SELECT

**Location**: `internal/ui/first_select.go:97-160`; `internal/ui/paging.go:60-106`; `internal/ui/resize_recovery.go:134-171`  
**Category**: Logic  
**Problem**: The first page captures the current SQL and parameters, but later and resize-recovery pages call `m.QB.PageSQL()` and `m.QB.PageParams()` again. Builder editing and query-history restoration intentionally do not finalize an active SELECT, so later rows can come from a different WHERE/ORDER/LIMIT/table and be merged into the old execution's cache.  
**Suggestion**: Capture an immutable executed-query snapshot at execution start and derive every later page and recovery request from it.

### 5. Reaching the final page permanently disables forward navigation

**Location**: `internal/ui/paging.go:109-132,222-247`  
**Category**: Logic  
**Problem**: A short page sets `pageExhausted=true`, but a subsequent successful Page Up never clears it. Page Down from that earlier page is then rejected as if the final page were still displayed.  
**Suggestion**: Track whether the currently displayed range is the observed final range, or clear the flag when a full non-final page is displayed. Add final-page→Page Up→Page Down coverage.

### 6. Ordinary later-page failures are invisible

**Location**: `internal/ui/paging.go:183-220`; `internal/ui/results_grid.go:161-186`  
**Category**: Logic  
**Problem**: On a later-page error, the code copies `m.Result.Err`—normally nil—instead of `msg.Result.Err`. Loading disappears and the previous grid remains with no failure notice, although the snapshot is later classified as failed.  
**Suggestion**: Retain the previous rows while storing and visibly rendering the new page failure in a separate status/error field.

### 7. Resizing during the first page discards it without scheduling the required replacement

**Location**: `internal/ui/model.go:820-868`; `internal/ui/resize_recovery.go:34-48`; `internal/ui/model.go:656-700`  
**Category**: Logic  
**Problem**: Resize advances the viewport generation, but recovery returns immediately while `Result` is nil. The old first-page response is later rejected by generation, and no correctly sized replacement is deferred. The active SELECT can be left with no result page.  
**Suggestion**: Treat an in-flight first page like a later page: cancel it, remember the new page size/range, wait for settlement, then issue one replacement under the new generation.

### 8. The 64 MiB failure contract breaks at four cross-module boundaries

**Location**: `internal/session/session.go:151-181`; `internal/ui/resize_recovery.go:174-198`; `internal/ui/first_select.go:178-207`; `internal/ui/export.go:125-148`; `internal/history/snapshot_classify.go:71-130`  
**Category**: Logic / Consistency  
**Problem**:

- `mapFirstPage` sets both `Err` and `LimitFailure`, routing real value-limit failures through the ordinary error path instead of a retained partial-page warning.
- `mergePageIntoCache` discards the cache's typed page-envelope failure and callers display the untrimmed page while exporting only retained cache rows.
- Active export omits `LimitFailureKind/Position`.
- Completeness classification ignores a retained limit failure and can label the snapshot complete.

This makes production behavior differ from fake-executor tests and can show/export different row sets.  
**Suggestion**: Define one typed "partial page plus terminal limit failure" contract, propagate it without converting it into an ordinary error-only view, display only cache-admitted rows, and include the failure in active/finalized metadata and completeness classification.

### 9. Backward cache eviction does not release evicted payload memory

**Location**: `internal/resultcache/cache.go:305-338`  
**Category**: Logic / Best practices  
**Problem**: Backward eviction only shortens the slice. Removed tail rows—and their large BLOB/TEXT backing references—remain in the backing array scanned by the GC, while payload accounting says they were removed. Repeated traversal can retain far more than the advertised 64 MiB envelope.  
**Suggestion**: Zero the removed tail elements before reslicing or copy the retained prefix into a new slice. Add a memory-retention regression test with large BLOBs.

### 10. Production schema validation cancellation is presentation-only

**Location**: `internal/ui/schema_validation.go:41-119,246-258`; `internal/ui/schema_refresh.go:63-104`; `internal/session/session.go:228-296`  
**Category**: Logic / Consistency  
**Problem**: `VersionReader` and `CatalogRefresher` accept no context; production adapters use `context.Background()`. Ctrl+W during validation emits only `CancelValidationMsg`, so the database read is never interrupted. Table refresh has no cancellation handle at all. The UI can show `cancelling…` while work continues.  
**Suggestion**: Make both seams context-aware, retain per-request cancel functions, and exercise cancellation through the production adapters rather than context-free fakes.

### 11. Accepted quit and health-terminal entry do not wait for non-write requests to settle

**Location**: `internal/ui/inflight_gate.go:185-233`; `internal/ui/active_select.go:60-90`; `internal/ui/paging.go:253-271`; `internal/ui/schema_refresh.go:180-249`  
**Category**: Logic  
**Problem**: Only pending writes use a settlement coordinator. Accepted quit for SELECT, validation, estimate, refresh, or save returns `tea.Quit` immediately. Active-SELECT finalization clears cancel handles without invoking them. Health-terminal entry invokes some cancel functions but immediately clears state and enables status-1 quit without waiting for settlement. This violates the explicit cleanup-before-exit/no-pending-terminal contract.  
**Suggestion**: Add a unified quit/terminal settlement coordinator for every request class. Cancel owned work, retain pending identities until responses settle, and emit `tea.Quit` or terminal entry only afterward.

### 12. Stale settlements corrupt the current request's guards

**Location**: `internal/ui/schema_validation.go:133-155`; `internal/ui/schema_refresh.go:107-123`  
**Category**: Logic  
**Problem**: Both handlers clear pending/cancellable state before checking the message identity. An old response arriving while a newer retry is running disables Ctrl+W and can allow another request to start concurrently.  
**Suggestion**: Validate attempt/preparation identity first; mutate pending and cancellation ownership only for the matching current response.

### 13. Terminal status 1 is discarded by the production runner

**Location**: `internal/ui/outcome_unknown_keys.go:21-36`; `internal/session/session.go:330-387`  
**Category**: Logic / Consistency  
**Problem**: Terminal key handling records `exitStatus=1`, but `RunSQLiteWith` discards the final `tea.Model` returned by the runner and returns nil when the TUI itself had no error. The process therefore exits 0 after terminal deletion, replacement, or outcome-unknown quit.  
**Suggestion**: Propagate a typed terminal status from the final model through the session and CLI without printing an extra stderr diagnostic.

### 14. Commit/rollback outcome classification depends on asynchronous phase-message ordering

**Location**: `internal/ui/write_exec.go:95-115,135-176,277-288`  
**Category**: Logic  
**Problem**: `writeUnresolved` reads mutable `m.writeNoncancellable`, which is set only after the UI consumes a phase message. Because write settlement and phase relay run in a `tea.Batch`, settlement may arrive before the noncancellable phase message, causing a failed COMMIT or failed rollback to be misclassified as an ordinary definite failure.  
**Suggestion**: Classify from the settled `WriteResult.Phase` and `RollbackConfirmed`, not UI delivery timing. Phase messages should remain presentation-only.

### 15. Overwrite protection still has a stat-to-rename TOCTOU window

**Location**: `internal/export/save_write.go:359-379`; `internal/export/save_write_unix.go:19-55`  
**Category**: Security / Logic  
**Problem**: The destination is re-statted, compared, and then renamed in separate filesystem operations. Another process can replace or modify the destination after the check but before `Rename`, and its change is overwritten despite the "destination changed" guarantee. Existing tests only race before the final stat.  
**Suggestion**: Use a platform-specific conditional publication strategy or cooperative locking that ties identity verification atomically to replacement; otherwise explicitly weaken the guarantee and require renewed confirmation. Add a barrier immediately between final verification and rename.

### 16. `WITHOUT ROWID, STRICT` tables are misclassified as rowid tables

**Location**: `internal/schema/catalog.go:95-115,149-165`  
**Category**: Logic  
**Problem**: Detection requires `WITHOUT ROWID` to be the final two whitespace-delimited tokens. Valid SQLite table-option forms such as `WITHOUT ROWID, STRICT` are classified as rowid tables, so later paging appends `ORDER BY rowid` and fails.  
**Suggestion**: Parse the table-option list after the final `)` and recognize `WITHOUT ROWID` in either legal option order. Add real SQLite fixtures for both `WITHOUT ROWID, STRICT` and `STRICT, WITHOUT ROWID`.

### 17. The release integration gate neither exercises required behavior nor passes a full race run

**Location**: `cmd/sqloid/pty_integration_test.go:34-140`; `.github/workflows/capability-suite.yml:55-89`; `scripts/capability-suite.sh:19-22`  
**Category**: Logic / Best practices  
**Problem**: The PTY test only verifies startup rendering and normal quit; it never drives the required SELECT, write, or export paths. Its shared `bytes.Buffer` is also read and written concurrently without synchronization. `CGO_ENABLED=1 go test -race ./...` fails on this race, while the targeted capability script excludes `cmd/sqloid`, allowing the release gate to stay green.  
**Suggestion**: Make output capture synchronized and drive the built binary through baseline SELECT, write, and CSV/JSON or SQL-save behavior with real adapters. Add the binary package to a race-gated job.

---

## Medium findings

### 18. UPDATE/DELETE execute serialized literals instead of bound values

**Location**: `internal/ui/destructive_prep.go:132-177,255-279`; `internal/ui/write_exec.go:253-275`; `internal/querybuilder/update_sql.go:6-41`  
**Category**: Security / Consistency  
**Problem**: The destructive modal's standalone literal SQL is passed to `ExecContext`, along with parameters from the parameterized builder. The pinned driver reports unknown input count and ignores the extra arguments because the statement has no placeholders. Current literal escaping prevents direct injection, but the promised "all user values are bound" defense is absent for UPDATE/DELETE, and tests codify the mismatch.  
**Suggestion**: Capture both forms at confirmation: use `UpdateSQL`/`DeleteSQL` plus matching parameters for execution, and retain the standalone rendered form only for confirmation/history/save display.

### 19. Invalid-UTF disclosure is not cumulative across retained pages

**Location**: `internal/ui/paging.go:198-226`; `internal/ui/active_select.go:150-174`; `internal/ui/export.go:125-148`  
**Category**: Logic / Consistency  
**Problem**: Each accepted page replaces `Result.Page`, and finalization/export read `InvalidUTF` only from that currently displayed page. If an earlier still-retained page contained malformed TEXT and the current page does not, retained/exported replacement characters lose their warning.  
**Suggestion**: Track invalid-UTF metadata with cached rows/pages and derive the flag over the retained cache at capture/finalization.

---

## Low findings

### 20. Dead and temporary artifacts remain in the production tree

**Location**: `internal/connection/startup.go:425-436`; `internal/ui/model.go:1288-1309`; `internal/ui/popup.go.tmp`  
**Category**: Best practices  
**Problem**: `connection.Session` is an unused pre-TUI startup helper with stale comments; `handleWriteMsg` has no caller; and an empty tracked `.tmp` file remains under `internal/ui`.  
**Suggestion**: Remove the dead helper, unreachable router, and empty temporary file after confirming no external API commitment requires them.

---

## No findings

- **Known dependency vulnerabilities**: `govulncheck v1.7.0 ./...` reported no vulnerabilities.
- **SQL identifier injection**: identifiers are schema-derived and consistently double-quoted.
- **CSV/JSON type serialization**: typed NULL/INTEGER/REAL/TEXT/BLOB handling and name deduplication are centralized and consistent.
- **Authentication/authorization/PII logging**: not applicable to this single-user local TUI; v1 contains no logging.
- **CLI and D1 discovery diagnostics**: no remaining systemic mismatch found.

## Verification performed

- `go test -count=1 -timeout 20m ./...` — passed
- `go vet ./...` — passed
- `go build ./...` — passed
- `scripts/capability-suite.sh` — passed
- `govulncheck v1.7.0 ./...` — no vulnerabilities
- `CGO_ENABLED=1 go test -race -count=1 -timeout 20m ./...` — **failed** on the PTY test's concurrent `bytes.Buffer` access

## Overall assessment

The feature should **not** be left in production as-is. Finding 1 can allow a destructive write to proceed after the user requests cancellation, and the high-severity findings include silent result loss, cross-query page mixing, broken limit enforcement, request cleanup violations, incorrect terminal status, and a release gate that does not exercise the required shipped paths.
