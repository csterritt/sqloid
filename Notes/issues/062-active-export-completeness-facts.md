## Issue 62: Classify active exports from complete endpoint facts

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Make `activeExportFacts` in `internal/ui/export.go` classify an active result from the same authoritative endpoint evidence used when finalizing it. Populate `SnapshotMetadata.ReachedLow` and `ReachedHigh` from the retained cache and successful limited-result count, and populate `TraversalFacts.ObservedShortFinalPage` from `pageExhausted`, without clamping contradictory count/cache evidence. Prefer one shared helper used by active export and `appendFinalizedResultEntry` so complete/partial/truncated labels and the warning shown before destination selection cannot drift.

### How to verify

- **Manual**: Export an active fully retained result with a known limited count, then one with count unavailable but an observed short final page; confirm the pre-picker warning and completed export label both say complete. Repeat with a missing endpoint and confirm it remains partial.
- **Automated**: Active-export/finalization parity table tests cover known-total, observed-short-final-page, missing-low, missing-high, eviction, and contradictory count/cache cases, asserting identical endpoint facts and completeness labels for the same active state and its finalized snapshot.

### Acceptance criteria

- [ ] Given an active cache retains the full limited result from the low endpoint through a successful known total, then active export records both endpoints and classifies the snapshot `complete`.
- [ ] Given count is unavailable but `pageExhausted` records an observed short/empty final page and the low endpoint/full range are retained, then active export records `ObservedShortFinalPage` and can classify the snapshot `complete`.
- [ ] Given either endpoint remains unobserved or rows were evicted/exceeded the cache, then the active export warning remains truthfully partial/truncated rather than claiming complete.
- [ ] Given identical active state is exported and then finalized, then both paths derive equivalent endpoint/completeness facts through shared logic.

### User stories addressed

- User story 55: Export truthful endpoint and completeness metadata
- User story 56: Use observed short final pages when count is unavailable
- User story 70: Show truthful complete/partial/truncated warnings before export

---
