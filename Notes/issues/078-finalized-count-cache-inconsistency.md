## Issue 78: Record count/cache inconsistency during finalization

**Type**: AFK
**Blocked by**: Issue 77 — second in the shared `history.Classify`/`TraversalFacts` sequence 77 → 78 → 79 → 80

### Parent PRD

`PRD-sqloid.md`

### What to build

Populate `Finalization.CountCacheInconsistent` in `internal/ui/active_select.go` before `SnapshotFacts` builds traversal metadata. When `m.countState.Status == result.CountSuccess` and the retained cache end exceeds `m.countState.Total`, record the contradiction rather than clamping either value or allowing `internal/history.Classify` to call the snapshot complete. Preserve the independent count and cache facts in finalized history and export metadata.

### How to verify

- **Verification sequencing**: Package/seam-level automated verification can proceed before Issue 57; manual/end-to-end steps that drive the shipped TUI must be re-run after Issue 57 lands.
- **Manual**: Use independently drifting count/page reads so the active cache retains a logical position beyond the successful count total, finalize the SELECT, and confirm history/export preserves both facts and does not label the snapshot complete.
- **Automated**: A finalization test constructs successful count state with retained cache end greater than total, finalizes through `appendFinalizedResultEntry`, and asserts `CountCacheInconsistent` reaches `TraversalFacts`, both original values remain unclamped, and completeness is never `complete`; boundary/control cases at equal or lower retained end and unavailable/failed count remain non-inconsistent.

### Acceptance criteria

- [ ] Given count succeeded with total T and the retained cache end is greater than T, then finalization records `CountCacheInconsistent=true`.
- [ ] Given count/cache inconsistency, then finalized history and export metadata preserve the independent total and retained range without clamping and classification is never `complete`.
- [ ] Given retained cache end is equal to or below a successful total, then this contradiction flag remains false.
- [ ] Given count is pending, unavailable, failed, or cancelled, then this successful-count contradiction check does not invent an inconsistency.

### User stories addressed

- User story 55: Preserve truthful known-total, range, and completeness metadata
- User story 59: Keep independent count evidence from clamping result pages
- User story 61: Finalize one immutable result entry with truthful metadata

---
