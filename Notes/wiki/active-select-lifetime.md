# Issue #34 — Active SELECT lifetime and exactly-once finalization

Issue #34 separates the **active SELECT lifetime** from any individual request in flight, enumerates the exhaustive finalizing and non-finalizing event categories, and guarantees that every actual SELECT execution produces **exactly one immutable result-history snapshot** no matter how, when, or how often finalization is requested. It builds on [concurrent-page-count.md](concurrent-page-count.md) (Issue #24 identities), [select-request-identities.md](select-request-identities.md) (Issue #26 generations), [positional-result-cache.md](positional-result-cache.md) (Issues #30/#31 cache), [snapshot-metadata.md](snapshot-metadata.md) (Issue #33 metadata), and [in-flight-gating.md](in-flight-gating.md) (Issue #27 gate/quit). See `Notes/PRD-sqloid.md` — Identities and state, SELECT (finalization list, item 6/7), Cache and snapshot invariant, Global Key Precedence and Context/Action Matrix, UI/History Module Design, and the active-finalization Testing Decisions.

## Active SELECT, execution ID, request ID, and viewport generation

- **Active SELECT** — the model-level lifetime owned by `Model.selectActive` / `activeExecID`. It is *not* request state: it survives every non-finalizing event and every individual request settlement, and it owns the ability to dispatch future serialized page requests.
- **Execution ID** — one fresh nonzero identity per actual execution (`result.NextSelectExecutionID()`), captured in `activeExecID` for the lifetime and in `finalizedExecID` once finalized.
- **Request ID** — one per first-page / count / later-page request, guarded by the Issue #24 tracker. Settling a request never ends the active lifetime by itself.
- **Viewport generation** — page-request state only (Issue #26). Finalization/deactivation and resize advance it, so late page responses from a finalized SELECT are inert.

Finalization marks `finalizedExecID = activeExecID` **before** any other work, so a duplicate or late finalizer message can never append a second entry or rewrite the first. Late first-page and count messages are rejected by the tracker; late page messages by request ID + generation + `!selectActive` (page dispatch is refused through `handlePageKey` and `requestRecoveryPage` once inactive).

## Exhaustive finalizing events

Only these four categories end an active SELECT, and each finalizes exactly once through `Model.finalizeActiveSelect()` in `internal/ui/active_select.go`:

1. **Actual new execution start** — `startSelectPage()` calls `activateSelect(exec)`, which finalizes the previous active SELECT before replacing the lifetime. Starting a write execution finalizes the same way; merely opening a destructive estimate or validation (pre-execution workflows) never does.
2. **Entering result history** — the `enterResultHistory()` seam (consumed by Issue #36 navigation) deactivates and invalidates future page mutation.
3. **Cancellation or failure that ends the SELECT** — a first-page settlement classified cancelled (before any row is retained) finalizes with a non-tabular Cancelled entry; a first-page ordinary failure finalizes with an error entry while still rendering the ordinary result-error boundary; a later-page ordinary failure after rows is recorded (`noteSelectFailed`) and typed into the snapshot when a real finalizer fires, preserving captured rows. A later-page cancellation after rows records the ending so the snapshot is typed cancelled-after-rows; a subsequent healthy page settlement clears the recorded ending because the execution continued. Ctrl+W that cancels later-page-only work does not by itself end the SELECT (replacement paging remains allowed — see [scoped-select-cancellation.md](scoped-select-cancellation.md)).
4. **Accepted quit** — `acceptedQuitCleanup()` cancels and finalizes once, with or without pending count/page work; unaccepted quit (Esc/n in the confirmation) restores the exact suspended context and finalizes nothing.

## Non-finalizing event categories (each proven in `active_select_lifecycle_test.go`)

Builder edits and focus changes; popups and help overlays; save/export keys; query-history browsing keys and gated result-history keys; destructive-estimate workflows (pre-execution by construction — no implemented flow finalizes); query-history restoration; resize (including suspension restore, which advances the generation but not the lifetime); serialized paging dispatch and settlement; each page/count success and count failure (`Count unavailable` preserves rows and paging); rejected/invalid execution attempts and settled validation without an execution start; and idle periods. In every combination of count-pending, page-pending, count-settled, and page-settled states, `SelectIsActive()` and the execution identity are preserved and no result entry is created.

## Exactly-once immutable finalization

`appendFinalizedResultEntry()` converts the authoritative state exactly once:

- **Tabular snapshots** (`history.KindTabular`) — success, count-failed rows, partial page failure after rows, and cancelled/failure after rows all preserve the cache's retained rows in ascending logical position order plus columns, the Issue #33 `SnapshotMetadata` (retained range, known total, endpoints, eviction, byte-cap, terminal outcome with reason), and the truthful `Classify` completeness. Issue #73 extends endpoint computation: `pageExhausted` from an accepted short or empty first page establishes `ReachedHigh` (and `ReachedLow` for an empty page where both endpoints sit at position 0) even when the cache retained no rows, so a fully retained count-unavailable short/empty result classifies complete.
- **Non-tabular entries** — cancellation before rows creates one `KindCancelled` entry and first-page failure before rows one `KindError` entry, each carrying its reason and typed terminal outcome metadata.
- Terminal outcome comes from the recorded ending (cancellation reason or failure cause), defaulting to success for ordinary idle finalization.

`history.ResultStore.AppendFinalized` (see [snapshot-metadata.md](snapshot-metadata.md) for the metadata model) is the single append point: it deep-copies columns/rows (BLOB bytes included), rejects a second entry for an already-finalized execution ID deterministically, and never mutates the first entry — replayed duplicate finalizers, late old-execution results, repeated cancellation settlements, repeated history-entry commands, and repeated quit cleanup are all no-ops.
