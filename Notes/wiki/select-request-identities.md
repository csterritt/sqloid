# Issue #26 — SELECT request identities and stale-response rejection

Issue #26 completes the request-identity system for SELECT executions: every first-page and later-page request captures its immutable execution ID, request ID, and viewport generation at dispatch, and a response may mutate UI or retained-cache state only when **every applicable identity is still current**. It also fixes the cancellation and settlement ordering: late successes after cancellation are classified cancelled, and no replacement page or count work may start (and no dedicated lease is reused) before every replaced predecessor has actually settled. See `Notes/PRD-sqloid.md` (Identities and state, SELECT, Errors and cancellation bounds, Global Key Precedence, Module Design, Testing Decisions), and cross-references to [concurrent-page-count.md](concurrent-page-count.md) (Issue #24 two-level identities), [serialized-vertical-paging.md](serialized-vertical-paging.md) (Issue #25 serialized paging), [cancellation-infrastructure.md](cancellation-infrastructure.md) (Issue #6 request lifecycle), and [responsive-tui-shell.md](responsive-tui-shell.md) (Issue #8 resize/suspension).

## The three identity levels

- **SELECT execution ID** (`result.NextSelectExecutionID()`) — one fresh nonzero identity per actual execution; owned by Issue #24 and unchanged here.
- **Request ID** (`result.NextSelectRequestID()`) — fresh nonzero role-specific identities: one for the first page (`RoleFirstPage`), one for the count (`RoleCount`), and one per later page request. Count identity stays exactly Issue #24's two-level guard; later pages add their own request IDs independently of the count's.
- **Viewport generation** (new in this issue) — page-request state only. A page command captures the generation current at dispatch; the response carries it back verbatim and mutates state only while it is still current. The generation is monotonically increasing and never reused.

The generation applies to **page roles only** (first page and later pages alike). The count is not a page request and tracks no generation — a resize does not invalidate a pending count, whose Issue #24 identity guard remains intact.

## When the generation advances

`internal/ui` owns generation changes, centralized in `Model.bumpViewportGeneration()`:

- **Resize** — every visible resize advances the generation, so each page response dispatched before the resize (first or later page) becomes inert regardless of its other identities. Entering undersized suspension leaves hidden state exactly frozen (no bump), because the suspension contract forbids any hidden-state change; **restoring visibility is itself a resize** and advances the generation then.
- **SELECT deactivation/finalization** — `Model.deactivateActiveSelect()` advances the generation whenever the active SELECT is deactivated or finalized. The finalization paths that own their own events (result-history entry, accepted quit, an ending cancellation/failure) call this when they deactivate the SELECT.
- **New execution start** — `startSelectPage()` finalizes the previous active SELECT first (advancing the generation) before assigning the fresh execution ID and dispatching its page/count pair under the new generation.

## Acceptance rules at the response boundary

Acceptance is centralized in `Update`'s message cases and the paging seam — never scattered through rendering or cache code:

- **First page** (`SelectSettledMsg`): accepted only when the `SelectTracker` accepts the exact (execution ID, first-page request ID) pair, the generation is still current, and the boundary has not classified the response cancelled. A stale-generation success mutates no rows and never lands on the result-error boundary; a stale failure creates no error state.
- **Later page** (`PageSettledMsg`, full identity in `applyPageSettled`): only a response whose request ID matches the one pending request settles its guard — a mismatched response can never clear a newer request's pending guard or its `loading next page…` feedback. Within that, rows, the absolute logical range, and the exhausted boundary mutate only when the response's execution ID and generation are also current and the response is not classified cancelled; any other settled outcome is inert except for releasing the pending slot. This means a resize-superseded page response still frees the one pending slot (the request did settle), so the same execution can issue a replacement page under the advanced generation.
- **Count** (`CountSettledMsg`): the Issue #24 two-level guard plus cancellation classification; no generation tracking.

## Cancellation-wins classification

`FirstPageResult` and `CountResult` now carry a `Cancelled` flag recording the Connection boundary's classification (mirroring `connection.OutcomeCancelled`): a success arriving after cancellation was requested, or the work failing with a cancellation error, is classified cancelled. At the response boundary a cancelled classification is fully inert — rows, absolute range, retained cache, count presentation, and pending feedback of a newer execution are never touched, even when the classification arrives after a newer execution is already current. Ordinary (non-cancelled) late errors from a superseded execution are equally rejected by their identities. A cancellation-classified count is neither an exact total nor the exact `Count unavailable` failure wording: the pending presentation survives.

Ctrl+W interrupt wiring itself (routing cancellation requests to the owned requests, `cancelling…` feedback) remains Issue #28's scope; this issue only fixes what a cancellation-classified settlement may and may not do.

## Replacement settlement ordering

- **Same-execution replacement** — the serialized paging guard stands: while one page request is pending (including one awaiting a cancellation-classified settlement), repeated/opposite keys issue no replacement, and a replacement page command is dispatched only after the predecessor actually settles. This holds after cancellation and after resize rejection alike.
- **Independent page/count settlement** — the pair still settles in either order without serialization: the count may settle while a page is pending (leaving the page's guard intact) and a page may settle while the count is pending, preserving Issue #24's normal concurrent launch.
- **Lease reuse prohibition** — on the Connection boundary, a cancelled request holds its dedicated lease until true settlement (`StateCancelling` observable throughout), so no replacement work can begin on or reuse that lease early; only after `Settle` (which classifies the late nil success as `OutcomeCancelled`, never success) and `Close` does the lease return to the pool. Cancellation never force-closes the connection (Issue #6 unchanged).
- **Newer executions** — a newer SELECT starts through the allowed execution-start seam and dispatches its own page/count pair on fresh leases; it does not wait for the superseded execution's requests, which settle independently and stay inert.

## Tests

- `internal/ui/identity_guards_test.go` — barrier-held (message-deferred, never sleeps) out-of-order first-page and later-page successes/failures across execution IDs, request IDs, and viewport generations; resize advancement; deactivation/finalization; superseded executions; replacement within the same execution after resize; stale responses unable to clear newer pending feedback or cache metadata; a fully current control case; and count identity surviving a resize.
- `internal/ui/cancellation_settlement_test.go` — cancellation wins over late success for first pages, later pages, and counts; inert cancelled/late-error responses after a newer execution; replacement-refusal before predecessor settlement and replacement application afterwards; count-first independent settlement while a page is pending; and newer-execution dispatch through the execution-start seam with inert superseded settlements.
- `internal/connection/cancellation_settlement_test.go` — barrier-proven lease holding through the cancelling state until true settlement (no third lease admitted, late nil success classified `OutcomeCancelled`, clean reuse after settlement) and independent page/count settlement in either order with no serialization between the roles.
- `internal/ui/vertical_paging_test.go` and `internal/ui/concurrent_count_test.go` — the Issue #25/#24 suites continue to pass unchanged, proving normal startup page/count concurrency and serialized paging behavior are preserved.
- `internal/ui/suspension_test.go` — updated only to exclude the viewport generation from the exact-context snapshot: restoring visibility is itself a generation-advancing resize, so the generation is identity bookkeeping, not user-visible context.

Cross-referenced Issues #6, #8, #24, #25, #26, and #28. Later pages add generation tracking; the count deliberately keeps only its two-level identity.
