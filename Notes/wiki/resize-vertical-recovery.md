# Issue #32 — Resize-safe vertical viewport recovery

Issue #32 makes terminal resizes safe while an active SELECT's result is displayed and paged: every visible resize recomputes the exact new page size from complete visible rows and runs a **pure recovery decision** that requires exactly one explicit outcome for the prior first displayed logical row — preserve it, clamp to a known retained endpoint, or request the exact new-size page containing it. Cross-references: [serialized-vertical-paging.md](serialized-vertical-paging.md) (the page-request lifecycle this reuses), [positional-result-cache.md](positional-result-cache.md) and [byte-cap-oversized-values.md](byte-cap-oversized-values.md) (the retained-range, dual-cap, and endpoint metadata the decision consumes), [select-request-identities.md](select-request-identities.md) (generation advancement and stale-response rejection), [scoped-select-cancellation.md](scoped-select-cancellation.md) (the scoped cancellation handle invoked on pending old-size work), and [in-flight-gating.md](in-flight-gating.md). Per `Notes/PRD-sqloid.md` this touches only the SELECT lifecycle, Cache and snapshot invariant, UI/Connection Module Design, resize Testing Decisions, and the manual layout matrix — Issue #29's horizontal resize behavior is untouched.

## Exact page-size recomputation

On every `tea.WindowSizeMsg` the model relays through `resize` and then `applyResizeRecovery`, which recomputes `CalculateLayout(m.Height, m.Fields).PageRows` — the count of **complete** data rows after the results border, status/count line, frozen header, builder border/padding, and global footer (11 rows at 80×24, 15 at 100×30, 34 at 160×50). Grow, shrink, unchanged, and minimum-supported dimensions all yield the exact new limit for the next adjacent or recovery request; below minimum size the shell suspends and hidden state stays frozen, and the restoring resize is itself the recovery-point resize. Too-small restoration never issues a fetch on its own.

## The pure decision seam (`RecoverViewport`)

`internal/ui/viewport_recovery.go` implements the UI-independent decision. `ViewportMetaFromCache` derives `ViewportMeta` from the authoritative resultcache — the inclusive retained range `Start..End`, the `RowCapEvictions`/`PayloadBytes`/`TruncatedByByteCap` dual-cap disclosure (context only; neither cap affects the decision), the exhausted-final-page flag, and the settled limited-SELECT count. `RecoverViewport(meta, prior, newSize)` applies this decision order:

1. **Preserve** the exact prior first logical row when it remains valid and retained inside the post-eviction contiguous range. Both row-cap and byte-cap eviction produce the same equivalence: only the surviving range matters.
2. **Clamp to the known retained low endpoint** (`Start`) when the prior row sits below the retained range — the retained start is always a safe display boundary.
3. **Clamp to the retained high endpoint** (`End`) only when that endpoint is established: the final page was short, or a settled count exists **and does not exceed the retained end** — an inconsistent count is never used to clamp.
4. **Fetch**: otherwise request the new-size page **containing** the target — `((prior-1)/newSize)·newSize + 1`, in absolute logical positions with the exact new page size floored at one complete row. Empty or unknown metadata yields this deterministic safe fetch.

The result is typed (`RecoveryPreserve`/`RecoveryClampLow`/`RecoveryClampHigh`/`RecoveryFetch` plus the resolved first row and size) so orchestration distinguishes local preserve/clamp from a fetch without issuing any Bubble Tea command or database dispatch.

## Orchestration in `internal/ui` (`resize_recovery.go`)

- **Cache seeding and merging.** The first page of each fresh execution seeds the active cache at positions `1..len`; every accepted later response merges by absolute position with the traversal direction choosing the eviction end exactly as serialized paging does, keeping the 10,000-position and 64 MiB contiguous invariants on every recovery-related merge. Each new execution resets to a **fresh cache** so a smaller first page never merges into a previous result's stale retained range.
- **Idle branch.** With no pending page, preserve/clamp decisions apply locally (`rebuildRetainedView` re-slices from the retained cache rows, clamping the high endpoint also marking the exhausted boundary) with **no request**; a fetch dispatches **exactly one** cancellable containing-page request through the same identity/generation/cancellation lifecycle as ordinary paging (`LIMIT <newSize> OFFSET <containingStart-1>`).
- **Pending old-size branch.** When a page request is pending, the resize still resolves the required row, advances the viewport generation (making the old-size response inert regardless of its other identities), invokes the request's scoped cancellation handle, and a required fetch is **deferred** (`resizeFetchPending`/`resizeFetchRow`/`resizeFetchSize`) until true settlement. Late **success and failure** from the old generation are both rejected by the full identity rule and only release the pending slot; exactly one replacement request then dispatches at the latest exact size. An invalidation-avoiding local decision issues no replacement at all.
- **Repeated resizes** coalesce: the deferred row/size are overwritten to the latest decision, so settlement dispatches exactly one request for the latest generation and size.
- **Count independence.** The pending or settling independent count (Issue #24) is never restarted, cancelled, or disturbed by resize recovery; count work and page-plus-count concurrency behave exactly as serialized paging specifies.
- **Inert contexts.** Inactive (no result), suspended, validating, and finalized/deactivated sessions never fetch; finalization clears the deferral so no replacement outlives the result window.

## Testing

`internal/ui/viewport_recovery_test.go` — pure, table-driven decision coverage: exact first-row preservation, single-row ranges, low/high endpoint clamps, count-established high endpoints (including inconsistent-count exclusion), empty/unknown determinism, row-cap and byte-cap eviction equivalence, absolute containing-page boundary arithmetic, and non-positive page sizes.

`internal/ui/resize_recovery_model_test.go` — scripted Bubble Tea model coverage with the `internal/connection`-style fakes: idle preserve/clamp/fetch with exact SQL, pending old-size rejection of late success and failure, mandatory settlement before exactly one correctly sized replacement, repeated resize coalescing to the latest size, count independence, too-small suspension/restoration, inactive/finalized no-fetch controls, page-size recomputation (grow/shrink/unchanged), fresh-execution cache replacement, and shared assertions of the resultcache contiguity and both cap invariants after every accepted response.

## Status

Implementation complete (`internal/ui/viewport_recovery.go`, `resize_recovery.go`, and the paging/model integration); manual walkthrough for Issue #32 pending under `Notes/walkthroughs/032-06/`.
