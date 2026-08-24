## Issue 30: Active SELECT lifetime and single finalization

**Type**: AFK
**Blocked by**: Issue 22, Issue 29

### Parent PRD

`PRD-sqloid.md`

### What to build

Separate active-SELECT lifetime from individual request lifetime and implement the exhaustive finalization list in **SELECT**. Finalization captures exactly one immutable history entry with rows and metadata appropriate to success, partial failure, or cancellation.

### How to verify

- **Manual**: Edit, open overlays, save/export, estimate, browse query history, resize, page, cancel, enter result history, and start a new execution.
- **Automated**: Lifecycle tests enumerate every finalizing and nonfinalizing event and assert exactly one entry with the correct tabular/non-tabular outcome.

### Acceptance criteria

- [ ] Given builder/UI activity or idle page completion, then the active SELECT remains active unless an enumerated finalizer occurs.
- [ ] Given a new actual execution, result-history entry, terminal cancellation/failure, or accepted quit, then the SELECT finalizes exactly once.
- [ ] Given cancellation/failure after rows, then captured rows and metadata remain; before rows, the defined non-tabular entry is created.

### User stories addressed

- User story 60: Keep active SELECT lifetime independent of request lifetime
- User story 61: Finalize only on defined events into one immutable entry

---
