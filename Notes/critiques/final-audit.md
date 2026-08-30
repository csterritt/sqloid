# Audit Report: Sqloid v1

Parent PRD: `Notes/PRD-sqloid.md`  
Date: 2026-08-30 (merged from two independent final-audit passes: 2026-08-29 and 2026-08-30)  
Files in scope: 276 tracked Go implementation files plus configuration and CI

## Provenance

This report is the union of two independent final audits of the same commit range.
The first pass (2026-08-29) established the critical production-composition gap and
the initial high/medium/low findings. The second pass (2026-08-30) re-read the PRD
and every package, independently re-verified every prior finding (all confirmed
accurate), and added further high-, medium-, and low-severity findings across the
write commit boundary, completeness classification, stale-identifier gates, startup
classification, DSN encoding, input handling, and grid layout. Verification was
re-run: `go build ./...`, `go vet ./...`, and `go test ./...` all pass, and
`govulncheck` v1.7.0 reports **no vulnerabilities** for `modernc.org/sqlite v1.57.0`
across `./...` on 2026-08-30.

## Summary

Sqloid v1 is **not safe to consider production-complete as-is**. The shipped `sqloid`
binary validates and opens the database, then immediately closes it without
constructing or running the TUI, making nearly the entire PRD inaccessible to users;
several additional high-severity defects sit behind that integration gap — a failed
COMMIT is reported as a confirmed clean rollback instead of the PRD's outcome-unknown
state (a destructive-write data-integrity misreport), later-page and completeness
metadata is dropped or mis-derived at settlement, SELECT projection and INSERT column
staleness are not gated, and a save can overwrite a file created after inspection. The
full test suite, build, vet, and vulnerability scan pass, but they do not exercise an
end-to-end production application composition and therefore do not detect the critical
failure or several of the high findings (which are proved only through injected
fixtures rather than real driver behavior).

## Critical findings

### 1. The shipped binary never starts the TUI

**Location**: `internal/connection/startup.go:386-396`, `cmd/sqloid/main.go:15-20`  
**Category**: Logic  
**Problem**: `main` routes both startup modes to handlers that ultimately call `connection.Session`. `Session` opens the database and immediately returns after a deferred close; its own comment states that “no TUI consumes the handle yet.” No production file imports `internal/ui`, constructs `ui.Model`, wires the database/export/file-picker executors, or calls `tea.NewProgram` (the only real imports of `internal/ui` are in `Notes/walkthroughs/.../_demo*/main.go` demos and tests). Consequently, `sqloid sqlite <file>` and `sqloid d1` exit silently after validation instead of providing any SELECT, write, paging, history, export, help, or quit behavior required by the PRD. The extensive module tests pass because they test disconnected components and fake seams rather than the shipped composition.  
**Suggestion**: Add a production application-composition layer that owns the opened `connection.DB`, loads the initial catalog, wires every `ui.Model` executor to the connection and filesystem implementations, runs the Bubble Tea program, maps terminal outcomes to exit status, and closes the database only after UI/request cleanup. Add an end-to-end binary test proving that a valid database enters the TUI and can execute at least the baseline select/write/export flow.

---

## High findings

### 1. A failed COMMIT is reported as a confirmed clean rollback instead of outcome-unknown

**Location**: `internal/connection/write.go:296-327`, `internal/ui/write_exec.go:284`  
**Category**: Logic (data integrity)  
**Problem**: When `tx.Commit()` returns an error, `run` calls `w.rollback(...)`. Inside `rollback`, `tx.Rollback()` returns `sql.ErrTxDone` because `database/sql` has already marked the transaction done, so the `!errors.Is(err, sql.ErrTxDone)` guard is false and the code sets `res.RollbackConfirmed = true`. The write is then classified `WriteFailed` with `RollbackConfirmed = true`. In the UI, `write_exec.go:284` treats `res.Outcome == WriteCommitted || res.RollbackConfirmed` as a resolved write, so it never enters `finalizeOutcomeUnknown`, and `WriteSummary(..., RollbackConfirmed=true, ...)` presents the destructive write as untouched/rolled back. A COMMIT whose persistence is genuinely unprovable is therefore reported to the user as definitely not persisted. This violates the PRD Writes-and-commit-boundary rules, User Story 47, and the Transaction invariant ("Any cancelled/failed write described as untouched has confirmed rollback"; unresolved commit/rollback is terminal outcome-unknown). The existing tests inject `WriteResult` values directly, so no test exercises a real `tx.Commit()` error and the gap is invisible. This finding borders on Critical (destructive-write data-integrity misreport) and is limited only by how often COMMIT fails and by the fact that the TUI is not yet wired.  
**Suggestion**: Do not treat a COMMIT failure as a confirmed rollback. On `tx.Commit()` error, route to the outcome-unknown workflow with `RollbackConfirmed = false` (persistence unprovable), preserving the commit error and phase. Add a Connection-level test that forces a real COMMIT failure and asserts the outcome-unknown classification and non-untouched summary.

### 2. Later-page value-limit errors report the wrong logical row

**Location**: `internal/connection/started_request.go:151-162`  
**Category**: Logic  
**Problem**: `StartPage` executes a SQL statement containing the requested `OFFSET`, but always calls `runFirstPage(..., 0)`. The offset argument is what converts scan failures into one-based absolute logical positions. An oversized value on a later page is therefore reported as row 1 (or another page-relative position) rather than `offset + row index + 1`, violating the exact failure-position contract.  
**Suggestion**: Add the logical offset to `StartPage` and pass it through to `runFirstPage`. Update every production adapter and capability test to supply the same offset used to build `PageSQL`.

### 3. Page result metadata is discarded at both settlement boundaries

**Location**: `internal/ui/first_select.go:163-174`, `internal/ui/paging.go:139-179`  
**Category**: Consistency  
**Problem**: `FirstPageResult` carries `ByteTruncated` and `LimitFailure`, and `ResultView` is designed to persist and render them. `applySelectSettled` copies only `Page` and `Err` (and never reads `viewportCache.TruncatedByByteCap()` after the first-page merge). `applyPageSettled` preserves only metadata from the previous view and ignores the newly settled page's `ByteTruncated` and `LimitFailure`. A first or later page can therefore lose the required 64 MiB warning and exact oversized-page/value failure location. Existing tests manually assign these fields after settlement, so they prove rendering but not propagation.  
**Suggestion**: Copy both fields from the first-page result (and OR in the cache truncation flag after the first-page merge). For later pages, OR the new truncation flag with the prior/cache flag and prefer a new non-nil `LimitFailure` while retaining an earlier failure when appropriate. Add settlement tests that inject each field through `FirstPageResult` rather than directly mutating `ResultView`.

### 4. A short or empty first page does not establish the high endpoint

**Location**: `internal/ui/first_select.go:97-155`, `internal/ui/first_select.go:163-174`, `internal/ui/paging.go:110-129`  
**Category**: Logic  
**Problem**: The first-page request size is not retained, and `applySelectSettled` never sets `pageExhausted` when the returned page is shorter than requested. Page Down can consequently issue another request at the same offset for an empty first page, and snapshot/export classification cannot use the observed short page to establish the high endpoint when count is unavailable. This violates the short/empty-final-page lifecycle contract.  
**Suggestion**: Store the requested first-page size with the request identity and, on accepted settlement, set `pageExhausted` whenever the result contains fewer rows than requested. Ensure empty and short first pages feed `ObservedShortFinalPage` into finalization and active export facts.

### 5. Active exports cannot be classified as complete correctly

**Location**: `internal/ui/export.go:113-145`  
**Category**: Consistency  
**Problem**: `activeExportFacts` builds `SnapshotMetadata` without `ReachedLow` or `ReachedHigh` and builds `TraversalFacts` without `ObservedShortFinalPage`. `history.Classify` requires the low endpoint and either a known total or observed short final page for completeness. Thus, an active result whose full limited result is retained can still be labeled partial/truncated, and a count-unavailable result with an observed short final page cannot be recognized as complete. The export warning shown before destination selection is therefore untruthful. This diverges from `appendFinalizedResultEntry`, which does populate these endpoint facts (see High #9).  
**Suggestion**: Derive endpoint facts from the cache, successful limited-result count, and `pageExhausted`, matching `appendFinalizedResultEntry`. Prefer a single shared helper for active-export and finalization metadata so their classifications cannot drift.

### 6. A file created after destination inspection can be overwritten without confirmation

**Location**: `internal/export/save_write.go:150-164`, `internal/export/save_write.go:174-213`  
**Category**: Security  
**Problem**: The UI first calls `InspectDestination`, then later calls `WriteAtomic`, whose final `os.Rename` always replaces the destination. If another process creates or replaces the path between inspection and rename, Sqloid overwrites that file despite the user never confirming overwrite for that file. This check-then-act race creates a realistic local data-loss risk and violates the existing-files-require-confirmation contract.  
**Suggestion**: Make overwrite intent explicit at the persistence boundary. Use a no-replace atomic primitive for unconfirmed new-file saves (with supported-platform implementations), and use replacement only after confirmation tied to the inspected destination state. Surface a destination-changed/existing error and return to confirmation rather than replacing silently.

### 7. Completeness classification mislabels partial/empty results as truncated

**Location**: `internal/history/snapshot_classify.go:62-118`  
**Category**: Logic  
**Problem**: `Classify` uses `meta.ReachedLow` for the `complete` label but never for `partial`. For a snapshot whose low endpoint was not observed and no eviction occurred (e.g. `RetainedStart=11, RetainedEnd=20`, known total 20, `ReachedLow=false`, `ReachedHigh=true`, work finished), `complete` is false, all `partial` terms are false, and `truncated` is false, so the function falls through to the default `Completeness{Truncated: true}`. The result is labeled `truncated` when positions 1–10 are simply unseen, which is `partial`. The same fall-through mislabels an empty result (`high == 0`) whose `ReachedLow` is false as `truncated` instead of `complete`. `truncated` is reserved for evicted/byte-cap rows or rows known to exceed the retained range.  
**Suggestion**: Add `!meta.ReachedLow` to the `partial` condition, and make `complete` not require `ReachedLow` when the result is empty: `complete := ... && (high == 0 || meta.ReachedLow) && fullRetention`. Add table tests for the unseen-low-endpoint and empty-complete cases.

### 8. SELECT projection staleness is not gated by the runnable report or renderers

**Location**: `internal/querybuilder/runnable.go:150-167`, `internal/querybuilder/select_sql.go` (`renderSelectCore`)  
**Category**: Logic / Consistency  
**Problem**: `reportWhere`, `reportUpdate`, and `reportInsert` each reject committed identifiers that no longer exist in the refreshed schema, but `reportSelect` only checks that the projection is non-empty — it never verifies that each `ProjectionColumn`/`Aggregate` entry still names a visible column. Combined with SELECT/page/count renderers not gating on the runnable report (Medium #3), a dropped projected column yields `Runnable=true` and `SELECT "vanished_col" …`, which fails at execution. This violates the common gate "all identifiers still exist in refreshed schema".  
**Suggestion**: Add a `validateProjection` step in `reportSelect` returning an `InvalidIssue{Field: RunFieldProjection}` when any projected column is absent from the selected object's visible columns, and gate the SELECT renderers on the authoritative report.

### 9. Finalized snapshots never record count/cache inconsistency, so a snapshot can be labeled complete despite contradiction

**Location**: `internal/ui/active_select.go:152-160`, `internal/ui/snapshot_metadata.go:27-39`  
**Category**: Logic / Consistency  
**Problem**: `appendFinalizedResultEntry` constructs `Finalization` without setting `CountCacheInconsistent`. `SnapshotFacts` copies it into `TraversalFacts`, and `history.Classify` relies on `inconsistent` to prevent a `complete` label when count/cache evidence contradicts (e.g. the cache retains rows beyond a known `countState.Total`). Because the field is always false, a snapshot can be classified `complete` despite the very inconsistency the PRD says must never be clamped or hidden. (The `Finalization` construction also omits `HasFailurePosition`/`FailurePosition`; see Medium #11 for the related `LimitFailure` loss.)  
**Suggestion**: Populate `Finalization.CountCacheInconsistent` when `m.countState.Status == result.CountSuccess` and the retained cache end exceeds `m.countState.Total`, and add a finalization test that asserts an inconsistent snapshot is never `complete`.

---

## Medium findings

### 1. Invalid UTF-8 maximal-subpart decoding is incorrect for two input shapes

**Location**: `internal/result/result.go:184-253`  
**Category**: Logic  
**Problem**: Once a string contains any malformed bytes, the decode loop treats every decoded `utf8.RuneError` as malformed even when it came from a valid three-byte U+FFFD encoding. In addition, `maximalSubpart` leaves E0-EF sequences at size 2 and never examines their third byte. Mixed valid-U+FFFD/malformed text and malformed three-byte sequences can therefore emit the wrong number of replacement characters, violating the requirement for exactly one U+FFFD per maximal invalid sequence.  
**Suggestion**: Distinguish valid U+FFFD from malformed input using the decoder's returned size (`size > 1` is valid), and implement the full three-byte maximal-subpart checks for E0-EF leads. Add tests combining valid U+FFFD with a later malformed byte and covering valid first/second continuation bytes followed by an invalid or missing third byte.

### 2. Finalized and historical results lose invalid-UTF and byte-truncation warnings

**Location**: `internal/ui/active_select.go:124-160`, `internal/ui/result_history.go:24-47`  
**Category**: Consistency  
**Problem**: `appendFinalizedResultEntry` never supplies `Finalization.InvalidUTF` from the active page, so snapshot metadata records it as false. When a historical tabular entry is projected, `projectHistoryEntry` also omits `Metadata.InvalidUTF` from the reconstructed page and `Metadata.TruncatedByByteCap` from `ResultView`. Browsing or exporting immutable history can therefore hide warnings that were true for the executed result.  
**Suggestion**: Carry the active page's invalid-UTF flag into finalization, and restore both invalid-UTF and byte-cap metadata when projecting a history entry. Add an end-to-end settlement → finalization → history-view/export test for each warning.

### 3. SELECT renderers can emit SQL for a state the authoritative validator rejects

**Location**: `internal/querybuilder/select_sql.go:16-30`, `internal/querybuilder/page_sql.go:17-46`, `internal/querybuilder/count_sql.go:13-19`  
**Category**: Consistency  
**Problem**: UPDATE, DELETE, INSERT, and estimate renderers gate on `RunnableReport`, but SELECT, page, and count renderers only check whether their component parts can be formatted. They can emit SQL for invalid grouping, stale identifiers, incomplete value state, or invalid limit state even though the runnable-state contract rejects that builder. Current UI call paths normally validate first, but the internal API contract is inconsistent and future callers can execute a state that should be refused. (See also High #8 for the stale-projection gap in the report itself.)  
**Suggestion**: Gate `SelectSQL` and `PageSQL` on the authoritative runnable report (and SELECT command); let `CountSQL` inherit that behavior. Add tests asserting empty output for every rejected SELECT validity class.

### 4. The only CI workflow does not run the repository's full definition-of-done checks

**Location**: `.github/workflows/capability-suite.yml:20-61`, `scripts/capability-suite.sh:19-22`  
**Category**: Best practices  
**Problem**: The workflow runs race tests only for `internal/connection`, `internal/ui`, and `internal/history`. It omits `cmd/sqloid`, `internal/cli`, `internal/d1`, `internal/export`, `internal/filepicker`, `internal/querybuilder`, `internal/result`, `internal/resultcache`, and `internal/schema`, and it does not run `go build ./...` or `go vet ./...`. Regressions in the shipped command, discovery, SQL generation, serialization, or schema logic can merge while the sole CI workflow remains green. This directly contributed to the critical production-composition gap remaining invisible.  
**Suggestion**: Add pure-Go `go test ./...`, `go build ./...`, and `go vet ./...` jobs on Linux and macOS, while retaining the targeted race/capability suite for its specialized cancellation guarantees. Add an actual binary/TUI integration test to the release gate.

### 5. Atomic save reports a nil cause for a short write

**Location**: `internal/export/save_write.go:183-199`  
**Category**: Logic  
**Problem**: If `SaveFile.Write` returns fewer bytes than requested with a nil error, `WriteAtomic` correctly treats it as failure but wraps the nil value in `StageError`. The user receives `save failed at write: <nil>` rather than an actionable I/O cause.  
**Suggestion**: Convert a nil-error short write to `io.ErrShortWrite` before constructing `StageError`, and add a test for that exact boundary result.

### 6. Stale INSERT prompts can leak into generated SQL

**Location**: `internal/querybuilder/runnable.go:207-224`, `internal/querybuilder/insert_sql.go:20-43`  
**Category**: Logic / Consistency  
**Problem**: `reportInsert` iterates the current `InsertableColumns()` and requires each to have a completed prompt, but it never checks the reverse: `q.inserts` may still hold prompts for columns that became non-insertable (hidden, generated, or dropped). `InsertSQL` (which is gated only by the runnable report) then iterates `q.inserts` directly and quotes those stale columns, producing e.g. `INSERT INTO "t" ("stale_hidden_col") VALUES (?)` — violating the identifier-existence gate.  
**Suggestion**: In `reportInsert`, build the insertable set and also iterate `q.inserts`; block with a stale-column reason when a prompt's column is not insertable. Alternatively build `InsertSQL`'s lists from the intersection with `InsertableColumns()`.

### 7. Permission-denied stat failures are classified as "missing"

**Location**: `internal/connection/startup.go:303-306`, `internal/connection/startup.go:109-112`  
**Category**: Logic  
**Problem**: `Open` maps every `os.Stat` failure to `FailureMissing`, always rendering `<path>: no such file or directory`. If the path exists but its parent directory denies traversal (EACCES/EPERM), the user sees a false "missing file" diagnostic, contradicting the PRD's "missing, unreadable, … rejected" and EACCES/EPERM classification rules.  
**Suggestion**: Distinguish `os.IsNotExist(err)` from EACCES/EPERM in the stat branch; permission-denied stat failures should map to `FailureUnreadable` (or a dedicated kind) so the message is `permission denied` with the cause preserved.

### 8. Relative SQLite DSNs are not percent-encoded

**Location**: `internal/connection/startup.go:287-292` (`mustFileURL`)  
**Category**: Logic  
**Problem**: Relative paths are placed into `url.URL.Opaque` unescaped. The PRD requires the DSN builder to percent-encode so reserved characters such as `?` or `#` stay part of the filename. Because `Opaque` is emitted raw, a relative filename containing `?` or `#` collides with the URI query/fragment separators and the SQLite URI parser misparses the path.  
**Suggestion**: For relative paths, use `url.URL{Scheme: "file", Path: path, OmitHost: true}` so `URL.String()` escapes the path while still avoiding an invented `//` authority; verify against filenames containing `?`, `#`, and spaces.

### 9. A request cancelled before its lease is acquired is classified as failed, not cancelled

**Location**: `internal/connection/health.go:122-130` (`RunRequest`), `internal/connection/started_request.go:50-59` (`startRequest`), `internal/connection/write.go:186-194` (`StartWrite`)  
**Category**: Logic / Consistency  
**Problem**: When `db.Lease(ctx)` fails because the supplied context was cancelled, all three entry points classify the outcome as `OutcomeFailed`/`WriteFailed`; none check `errors.Is(err, context.Canceled)`. `Request.Settle` already treats `context.Canceled` as cancelled, so a SELECT/write cancelled before the pool hands out a connection is inconsistently surfaced as an execution error rather than a cancellation.  
**Suggestion**: In each lease-error branch, test for `context.Canceled` before the `HealthError`/general-failure cases and return the cancelled classification.

### 10. Space key is dropped in universal value entry and filename input

**Location**: `internal/ui/value_input.go:46-89` (`ValuePrompt.HandleKey`), `internal/ui/filepicker.go:213-233` (`applyPickerFilenameKey`)  
**Category**: Logic / Consistency  
**Problem**: Bubble Tea delivers the space bar as `tea.KeySpace` (the popup search handler explicitly handles it at `internal/ui/popup.go:421`). `ValuePrompt.HandleKey` and the picker filename input only handle `tea.KeyRunes`, so a space is silently consumed without insertion. Users cannot type spaces in WHERE/LIKE/SET values, LIMIT input, or filenames, contradicting the universal-text-entry contract.  
**Suggestion**: Add a `tea.KeySpace` case to `ValuePrompt.HandleKey` (insert one space at the cursor) and to `applyPickerFilenameKey` (insert a space rune), mirroring the popup handler.

### 11. Horizontal grid packing under-counts separators and can overflow the row

**Location**: `internal/ui/horizontal_layout.go:49-65` (`visibleGridLayout`)  
**Category**: Logic  
**Problem**: The packing loop keeps `used` as the sum of column widths only, but the fit test uses `used + gridSeparatorWidth + w`. Because previously placed separators are never added to `used`, the cumulative rendered width (`sum(widths) + (n-1)*3`, since `renderGridRow` joins with `" | "`) can exceed the available width by roughly one separator per added column, drawing extra columns off-screen or wrapping the row.  
**Suggestion**: In the packing branch, update `used += w` to `used += gridSeparatorWidth + w` so the invariant matches the rendered row. Add a layout test with several narrow columns near the width boundary.

### 12. Finalized and historical snapshots drop the typed over-limit failure

**Location**: `internal/ui/active_select.go:152-160`, `internal/ui/snapshot_metadata.go:27-39`, `internal/ui/result_history.go:31-47`  
**Category**: Consistency  
**Problem**: `Finalization`/`SnapshotMetadata` carry no `LimitFailure` (kind and one-based position), and `appendFinalizedResultEntry` does not persist it, so `projectHistoryEntry` cannot restore the exact `result … exceeds the 64 MiB v1 limit at row N` line when a historical over-limit snapshot is reprojected. The warning that was true at execution is silently lost from history.  
**Suggestion**: Persist the `LimitFailure` kind and position through `Finalization`/`SnapshotMetadata`, and restore `ResultView.LimitFailure` in `projectHistoryEntry`. Add a finalization → history-view test.

---

## Low findings

### 1. Rollback's connection-aware test hook always receives nil

**Location**: `internal/connection/startup.go:171-181`, `internal/connection/write.go:305-320`  
**Category**: Consistency  
**Problem**: `beforeWriteRollback` has the same `func(context.Context, *sql.Conn)` signature as the other phase hooks, but rollback invokes it with nil while begin, execute, and commit receive the leased connection. A hook that inspects connection identity will panic or cannot validate rollback isolation. This is currently a test-seam defect rather than a production transaction defect.  
**Suggestion**: Pass the leased connection through the rollback helper, or change the hook contract to the transaction object actually intended for rollback assertions.

### 2. Malformed schema refresh status produces an unsettled revalidation result

**Location**: `internal/schema/revalidate.go:101-125`  
**Category**: Logic  
**Problem**: `schema.Attempt` is an exported struct, but an unknown/zero status reaches the default branch and returns `Revalidation{}` with another unknown/zero status. This contradicts the function's settled-result contract and shifts malformed-state handling to every consumer. Constructors do not produce the state, so the risk is limited to internal misuse or future extension.  
**Suggestion**: Map unknown attempts to a settled refresh-failed result with a concrete cause, and add a `Revalidation.Valid` invariant check and test (see Low #4).

### 3. The result cache does not deep-copy BLOB bytes

**Location**: `internal/resultcache/cache.go:149-153`, `internal/resultcache/cache.go:295-298`, `internal/resultcache/cache.go:338-342`  
**Category**: Consistency / Best practices  
**Problem**: `copyValues` and `Rows()` shallow-copy `result.Value`, so a BLOB's `[]byte` payload is shared with caller storage on both ingest and retrieval, despite the package comments promising the cache never aliases caller storage and that `Rows()` is safe to mutate. `internal/history/copyRows` correctly clones BLOB bytes, so the two payload owners are inconsistent. No current caller mutates the returned bytes, so this is latent.  
**Suggestion**: Clone `v.Bytes` for `result.KindBlob` in `copyValues`, and build `Rows()` output via `copyValues` per row rather than a shallow `copy`.

### 4. `Revalidation` lacks a payload `Valid()` guard its sibling has

**Location**: `internal/schema/revalidate.go:67-71` vs `internal/schema/refresh.go:81-91`  
**Category**: Consistency / Best practices  
**Problem**: `Attempt` has a `Valid()` method enforcing payload rules (catalog only on OK, cause only on failure, etc.), but `Revalidation` — which follows the same discipline — has none, so callers and tests have no typed way to assert a settled, internally consistent value.  
**Suggestion**: Add `func (r Revalidation) Valid() bool` mirroring `Attempt.Valid()`.

### 5. Unused zero value in the rowid-capability enum

**Location**: `internal/schema/schema.go:49-79`  
**Category**: Consistency / Dead code  
**Problem**: The PRD's rowid capability set is `{has-rowid, without-rowid, not-applicable}`, but the enum adds a fourth `RowidApplicable = iota` (value 0) that is never assigned and has no `String` case, unlike every other schema enum which starts at `iota + 1` so zero is an unset sentinel.  
**Suggestion**: Remove `RowidApplicable` and start the block at `iota + 1`, or add an explicit `RowidUnknown` sentinel with a `String` case if a zero value is intended.

### 6. Dead/unreachable filename cursor-movement cases

**Location**: `internal/ui/filepicker.go:213-233`, `internal/ui/filepicker.go:179-181`  
**Category**: Best practices  
**Problem**: `applyPickerFilenameKey` handles `tea.KeyLeft`/`tea.KeyRight` to move the filename cursor, but `handlePickerKey` consumes `left`/`right` at the top level (export-format toggle) before the filename path runs, so those arms are unreachable.  
**Suggestion**: Remove the dead cases, or route `left`/`right` into the filename input when the picker focus is the filename field.

### 7. Dead completeness-classification fields

**Location**: `internal/history/snapshot_classify.go:45-52`  
**Category**: Best practices  
**Problem**: `TraversalFacts` carries `HasLimit` and `Limit`, but `Classify` never reads them (the known total already reflects the limited result). They are currently dead, misleading fields.  
**Suggestion**: Remove them, or document explicitly why they are intentionally unused.

### 8. Finite-REAL token logic is duplicated across packages

**Location**: `internal/querybuilder/value.go:121-127` (`realToken`), `internal/result/result.go:145-159` (`RealToken`)  
**Category**: Consistency / Duplicated logic  
**Problem**: The PRD's finite-REAL token rule (`strconv.FormatFloat(v, 'g', -1, 64)` plus `.0` when no `.`/`e`/`E`) is implemented twice. If one copy is changed, saved SQL literals and grid/CSV/JSON rendering can silently diverge.  
**Suggestion**: Keep one canonical implementation (have `querybuilder` call `result.RealToken`, or extract a shared helper) used by both.

### 9. `Value.Display()` is documented as a shared exporter seam but is grid-only

**Location**: `internal/result/result.go:1-16`, `internal/result/result.go:112-127`  
**Category**: Consistency / Best practices  
**Problem**: The package/method comments describe `Display()` as the shared presentation token for the grid "and future exporters," but it applies grid-only transforms (tabs→`⇥`, newlines→`⏎`, `[BLOB n bytes]`) that would corrupt CSV/JSON output, which must keep bytes verbatim and emit hex/base64 and unquoted numbers.  
**Suggestion**: Document `Display()` as grid-only; treat the typed `Value` fields as the shared export seam and keep format-specific serialization in the export package.

### 10. Stale non-Go scripts in `scripts/`

**Location**: `scripts/run-all-tests.sh`, `scripts/set-up-for-wiki-fixes.sh:4-5`  
**Category**: Consistency / Dead code  
**Problem**: These scripts reference a Node/Playwright project (`src/`, `tests/`, `e2e-tests/`, `npm`, `playwright`, `wrangler`, `mailpit`) that does not exist in this Go repo and are not invoked by any workflow; they are misleading and will fail if run.  
**Suggestion**: Delete them (keep `capability-suite.sh` and any sync/tar helpers still in use).

---

## No findings

- **Authentication/authorization**: intentionally out of scope for the single-user local v1 application.
- **SQL injection**: user-entered values are parameter-bound; identifiers are selected from schema metadata and consistently double-quoted with embedded quotes doubled; aggregate/operator/direction tokens are closed choices. The stale-identifier findings above are correctness/consistency issues, not injection.
- **Sensitive-data logging**: no logging path was found, consistent with the PRD.
- **Known dependency vulnerabilities**: `govulncheck` v1.7.0 reported no vulnerabilities for `modernc.org/sqlite v1.57.0` across `./...` on 2026-08-30 (the 2026-08-29 pass reported the same with an earlier tool version).
- **D1 discovery diagnostics and candidate rules**: the exact path, case-sensitive `.sqlite` suffix filtering, lowercase-`metadata`/`-shm`/`-wal` exclusions, non-recursive lookup, and zero/multiple-candidate messages match the PRD.
- **Schema object eligibility and insertability**: ordinary tables, virtual tables, views, rowid capability/shadowing, hidden/generated columns, and INSERT insertability (including the INTEGER PRIMARY KEY hint) are represented consistently.
- **CLI routing / exit codes**: `mow.cli` help/version behavior and the `Main` return-2/return-1 mapping to one-line stderr diagnostics match the contract.
- **CSV/JSON/SQL serialization values**: RFC 4180 CRLF/minimal quoting, JSON array-of-objects with raw numbers and quoted non-finite REALs, BLOB hex/base64, NULL-vs-empty, and identifier quoting align with the PRD.
- **Filepicker sort order**: `..` first, then children in bytewise case-sensitive Go string order (locale-independent).
- **Layout partitioning and global key precedence**: the reserved footer row, `floor(H/3)` builder cap, results > half-height, ordered precedence dispatcher, and quit suspension/restoration match the matrix (the layout/input findings above are separate defects, not precedence violations).
- **Cache eviction direction, adjacency rejection, and non-BLOB payload accounting**: forward/backward opposite-end eviction, nonadjacent stale rejection, and byte accounting for non-BLOB/valid-UTF8 values are correct.
- **Core verification**: `go test ./...`, `go vet ./...`, and `go build ./...` all pass locally during both audits. Passing verification does not mitigate the critical missing production composition or several high findings, because no test asserts that the shipped binary launches the implemented UI, and key write/settlement paths are exercised only through injected fixtures rather than real driver behavior.

## Overall assessment

The feature should **not** be left in production or considered done. Resolve the critical application-composition finding first. Before release, fix the high-severity write commit-boundary misreport (High #1), the result-lifecycle metadata/endpoint defects (High #3–#5, #9), the completeness-classification bug (High #7), the stale-projection gate (High #8), and the save overwrite race (High #6). Then address the medium encoding/classification/input/layout defects and the CI gap that currently allows these to remain undetected, followed by the low-severity consistency and dead-code cleanups. Re-run the full `go test ./...`, `go vet ./...`, `go build ./...`, and an added end-to-end binary/TUI test as the release gate.
