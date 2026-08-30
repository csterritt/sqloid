# Audit Report: Sqloid v1

Parent PRD: `Notes/PRD-sqloid.md`  
Date: 2026-08-29  
Files in scope: 272 tracked implementation and configuration files

## Summary

Sqloid v1 is **not safe to consider production-complete as-is**. The shipped `sqloid` binary validates and opens the database, then immediately closes it without constructing or running the TUI, making nearly the entire PRD inaccessible to users; several additional high-severity paging, result-metadata, export-classification, and save-safety defects remain behind that integration gap. The full test suite, build, vet, and vulnerability scan pass, but they do not exercise an end-to-end production application composition and therefore do not detect the critical failure.

## Critical findings

### 1. The shipped binary never starts the TUI

**Location**: `internal/connection/startup.go:386-396`, `cmd/sqloid/main.go:15-20`  
**Category**: Logic  
**Problem**: `main` routes both startup modes to handlers that ultimately call `connection.Session`. `Session` opens the database and immediately returns after a deferred close; its own comment states that “no TUI consumes the handle yet.” No production file imports `internal/ui`, constructs `ui.Model`, wires the database/export/file-picker executors, or calls `tea.NewProgram`. Consequently, `sqloid sqlite <file>` and `sqloid d1` exit silently after validation instead of providing any SELECT, write, paging, history, export, help, or quit behavior required by the PRD. The extensive module tests pass because they test disconnected components and fake seams rather than the shipped composition.  
**Suggestion**: Add a production application-composition layer that owns the opened `connection.DB`, loads the initial catalog, wires every `ui.Model` executor to the connection and filesystem implementations, runs the Bubble Tea program, maps terminal outcomes to exit status, and closes the database only after UI/request cleanup. Add an end-to-end binary test proving that a valid database enters the TUI and can execute at least the baseline select/write/export flow.

---

## High findings

### 1. Later-page value-limit errors report the wrong logical row

**Location**: `internal/connection/started_request.go:148-161`  
**Category**: Logic  
**Problem**: `StartPage` executes a SQL statement containing the requested `OFFSET`, but always calls `runFirstPage(..., 0)`. The offset argument is what converts scan failures into one-based absolute logical positions. An oversized value on a later page is therefore reported as row 1 (or another page-relative position) rather than `offset + row index + 1`, violating the exact failure-position contract.  
**Suggestion**: Add the logical offset to `StartPage` and pass it through to `runFirstPage`. Update every production adapter and capability test to supply the same offset used to build `PageSQL`.

### 2. Page result metadata is discarded at both settlement boundaries

**Location**: `internal/ui/first_select.go:163-173`, `internal/ui/paging.go:139-178`  
**Category**: Consistency  
**Problem**: `FirstPageResult` carries `ByteTruncated` and `LimitFailure`, and `ResultView` is designed to persist and render them. `applySelectSettled` copies only `Page` and `Err`. `applyPageSettled` preserves only metadata from the previous view and ignores the newly settled page's `ByteTruncated` and `LimitFailure`. A first or later page can therefore lose the required 64 MiB warning and exact oversized-page/value failure location. Existing tests manually assign these fields after settlement, so they prove rendering but not propagation.  
**Suggestion**: Copy both fields from the first-page result. For later pages, OR the new truncation flag with the prior/cache flag and prefer a new non-nil `LimitFailure` while retaining an earlier failure when appropriate. Add settlement tests that inject each field through `FirstPageResult` rather than directly mutating `ResultView`.

### 3. A short or empty first page does not establish the high endpoint

**Location**: `internal/ui/first_select.go:97-155`, `internal/ui/first_select.go:163-174`, `internal/ui/paging.go:110-129`  
**Category**: Logic  
**Problem**: The first-page request size is not retained, and `applySelectSettled` never sets `pageExhausted` when the returned page is shorter than requested. Page Down can consequently issue another request at the same offset for an empty first page, and snapshot/export classification cannot use the observed short page to establish the high endpoint when count is unavailable. This violates the short/empty-final-page lifecycle contract.  
**Suggestion**: Store the requested first-page size with the request identity and, on accepted settlement, set `pageExhausted` whenever the result contains fewer rows than requested. Ensure empty and short first pages feed `ObservedShortFinalPage` into finalization and active export facts.

### 4. Active exports cannot be classified as complete correctly

**Location**: `internal/ui/export.go:113-145`  
**Category**: Consistency  
**Problem**: `activeExportFacts` builds `SnapshotMetadata` without `ReachedLow` or `ReachedHigh` and builds `TraversalFacts` without `ObservedShortFinalPage`. `history.Classify` requires the low endpoint and either a known total or observed short final page for completeness. Thus, an active result whose full limited result is retained can still be labeled partial/truncated, and a count-unavailable result with an observed short final page cannot be recognized as complete. The export warning shown before destination selection is therefore untruthful.  
**Suggestion**: Derive endpoint facts from the cache, successful limited-result count, and `pageExhausted`, matching `appendFinalizedResultEntry`. Prefer a single shared helper for active-export and finalization metadata so their classifications cannot drift.

### 5. A file created after destination inspection can be overwritten without confirmation

**Location**: `internal/export/save_write.go:150-164`, `internal/export/save_write.go:174-213`  
**Category**: Security  
**Problem**: The UI first calls `InspectDestination`, then later calls `WriteAtomic`, whose final `os.Rename` always replaces the destination. If another process creates or replaces the path between inspection and rename, Sqloid overwrites that file despite the user never confirming overwrite for that file. This check-then-act race creates a realistic local data-loss risk and violates the existing-files-require-confirmation contract.  
**Suggestion**: Make overwrite intent explicit at the persistence boundary. Use a no-replace atomic primitive for unconfirmed new-file saves (with supported-platform implementations), and use replacement only after confirmation tied to the inspected destination state. Surface a destination-changed/existing error and return to confirmation rather than replacing silently.

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
**Problem**: UPDATE, DELETE, INSERT, and estimate renderers gate on `RunnableReport`, but SELECT, page, and count renderers only check whether their component parts can be formatted. They can emit SQL for invalid grouping, stale identifiers, incomplete value state, or invalid limit state even though the runnable-state contract rejects that builder. Current UI call paths normally validate first, but the internal API contract is inconsistent and future callers can execute a state that should be refused.  
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
**Suggestion**: Map unknown attempts to a settled refresh-failed result with a concrete cause, and add a `Revalidation.Valid` invariant check and test.

---

## No findings

- **Authentication/authorization**: intentionally out of scope for the single-user local v1 application.
- **SQL injection**: user-entered values are parameter-bound; identifiers are selected from schema metadata and consistently double-quoted; aggregate/operator/direction tokens are closed choices.
- **Sensitive-data logging**: no logging path was found, consistent with the PRD.
- **Known dependency vulnerabilities**: `govulncheck` v1.5.0 reported no reachable vulnerabilities for `./...` on 2026-08-29.
- **D1 discovery diagnostics and candidate rules**: the exact path, case-sensitive suffix filtering, metadata/sidecar exclusions, and zero/multiple-candidate messages match the PRD.
- **Schema object eligibility**: ordinary tables, virtual tables, views, rowid capability/shadowing, hidden/generated columns, and insertability are represented consistently.
- **Core verification**: `go test ./...`, `go vet ./...`, and `go build ./...` all passed locally during this audit. Passing verification does not mitigate the critical missing production composition because no test asserts that the shipped binary launches the implemented UI.

## Overall assessment

The feature should **not** be left in production or considered done. The critical application-composition finding must be resolved first; the high-severity result-lifecycle and atomic-save findings should then be fixed before release, followed by the metadata/encoding and CI gaps that currently allow these defects to remain undetected.
