## Issue 58: Classify COMMIT failure as outcome unknown

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Correct the destructive-write commit boundary in `internal/connection/write.go` and `internal/ui/write_exec.go`. When `tx.Commit()` returns an error, do not interpret a subsequent `sql.ErrTxDone` from `tx.Rollback()` as proof that persistence did not occur: preserve the commit phase and error, leave `RollbackConfirmed` false, and route the settled result through the outcome-unknown terminal workflow. The UI and write summary must never classify this case as `WriteFailed` with a confirmed untouched/rolled-back database. Add a Connection-level test that induces an actual driver COMMIT failure rather than injecting a preclassified `WriteResult`.

### How to verify

- **Verification sequencing**: Package/seam-level automated verification can proceed before Issue 57; manual/end-to-end steps that drive the shipped TUI must be re-run after Issue 57 lands.
- **Manual**: Run a write with a test fixture that fails COMMIT after the statement executes; confirm Sqloid waits for settlement, reports the commit error and unresolved persistence, enters outcome-unknown, and never says the database is untouched or rolled back.
- **Automated**: A real Connection/driver-boundary test forces `tx.Commit()` to fail and asserts the result preserves the commit phase/error, has `RollbackConfirmed == false`, is classified outcome-unknown, creates exactly one non-persistence summary entry, and enters the terminal workflow; a control test retains confirmed-rollback behavior for a pre-COMMIT failure whose rollback truly succeeds.

### Acceptance criteria

- [ ] Given `tx.Commit()` returns an error, then `sql.ErrTxDone` from a later rollback attempt is not treated as rollback confirmation and persistence remains explicitly unprovable.
- [ ] Given a failed COMMIT settles, then the result preserves the commit error and phase, routes through `finalizeOutcomeUnknown`, and produces exactly one outcome-unknown entry.
- [ ] Given that outcome-unknown entry and summary, then neither uses untouched, clean rollback, or other wording that claims the write did not persist.
- [ ] Given statement failure or cancellation before COMMIT followed by a genuinely successful rollback, then the existing confirmed-untouched classification remains available.

### User stories addressed

- User story 45: Claim untouched state only after confirmed rollback
- User story 47: End unresolved writes in a safe outcome-unknown terminal state
- User story 85: Describe unresolved phase and error without implying persistence

---
