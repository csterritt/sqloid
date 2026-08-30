## Issue 78: Preserve typed over-limit failures in result history

**Type**: AFK
**Blocked by**: Issue 60

### Parent PRD

`PRD-sqloid.md`

### What to build

Extend SELECT finalization and snapshot metadata to persist the typed 64 MiB `LimitFailure`, including whether it is a page or value failure and its one-based logical position. Restore that typed failure when projecting a historical tabular entry so the historical view reproduces exactly `result page exceeds the 64 MiB v1 limit at row N` or `result value exceeds the 64 MiB v1 limit at row N`, while retaining complete leading rows and immutable export metadata.

### How to verify

- **Manual**: Produce separate page-limit and value-limit failures, finalize each SELECT, browse its historical snapshot at a different terminal size, and confirm the same failure kind, one-based row N, exact message, and retained leading rows remain visible.
- **Automated**: UI lifecycle tests finalize and reproject tabular results with each `LimitFailure` kind and representative one-based positions, asserting typed snapshot round-trip, exact historical message, unchanged retained rows/positions, and immutable export capture; a no-failure snapshot remains unset.

### Acceptance criteria

- [ ] Given an active result has a typed page or value `LimitFailure` at one-based position N, when finalized, then snapshot metadata persists both the kind and N.
- [ ] Given that historical snapshot is projected, then `ResultView.LimitFailure` is restored and renders the exact corresponding 64 MiB failure line at row N.
- [ ] Given complete leading rows were retained before the failure, then history and export preserve those rows and their logical positions without a partial row.
- [ ] Given a snapshot had no limit failure, then projection does not synthesize one.

### User stories addressed

- User story 55: Preserve truthful snapshot failure metadata
- User story 64: Browse immutable historical results without re-fetching
- User story 89: Preserve exact page/value over-limit kind and logical position

---
