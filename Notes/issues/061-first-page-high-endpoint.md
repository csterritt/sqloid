## Issue 61: Establish the high endpoint from a short first page

**Type**: AFK
**Blocked by**: Issue 60 — coordinate the shared `applySelectSettled` change after settlement metadata is preserved

### Parent PRD

`PRD-sqloid.md`

### What to build

Retain the requested first-page size with the first-page request identity in `internal/ui/first_select.go`. On an accepted current settlement, compare returned rows with that exact requested size and set `pageExhausted` when the page is short, including zero rows. Feed this observation into `ObservedShortFinalPage` for active export facts and finalization so count-unavailable short/empty first results establish the high endpoint, and prevent Page Down from issuing another request at the same offset after an empty result. Stale settlements must not update the endpoint.

### How to verify

- **Verification sequencing**: Package/seam-level automated verification can proceed before Issue 57; manual/end-to-end steps that drive the shipped TUI must be re-run after Issue 57 lands.
- **Manual**: With count forced unavailable, execute one empty SELECT and one SELECT shorter than the visible first-page capacity; confirm `No rows` or the short range appears, Page Down performs no duplicate fetch, and active/finalized export labels the fully retained result complete rather than unknown.
- **Automated**: First-page lifecycle tests retain the requested size, settle empty, short, and full pages, and assert `pageExhausted`/`ObservedShortFinalPage`, no repeated empty-page request, and truthful active-export/finalized completeness; stale request identities and exactly-full pages must not establish a high endpoint.

### Acceptance criteria

- [ ] Given an accepted first page returns fewer rows than its retained requested size, then `pageExhausted` is set and the observed short final page establishes the high endpoint.
- [ ] Given an accepted empty first page, then Page Down does not request the same offset again and count-unavailable metadata can classify the empty fully retained result as complete.
- [ ] Given count is unavailable and a short first page is fully retained, then both active export and finalized snapshot facts include `ObservedShortFinalPage` and can be classified complete.
- [ ] Given an exactly full or stale first-page response, then it does not falsely establish the high endpoint.

### User stories addressed

- User story 51: Show explicit empty SELECT results
- User story 55: Export truthful endpoint and completeness metadata
- User story 56: Infer the high endpoint from an observed short or empty final page

---
