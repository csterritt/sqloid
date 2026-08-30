## Issue 63: Enforce overwrite intent at the atomic persistence boundary

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Close the inspection-to-rename race in `internal/export/save_write.go` by carrying explicit overwrite intent and the inspected destination state to the persistence boundary. An unconfirmed new-file save must use a supported-platform atomic no-replace primitive so a file created after `InspectDestination` cannot be overwritten. Replacement is allowed only after confirmation tied to the destination state the user inspected; if that state changes before persistence, return a destination-changed/existing error to the save flow and require fresh inspection/confirmation instead of replacing silently. Preserve destination-local temporary-file atomicity, cleanup, retry, and cancellation behavior.

### How to verify

- **Manual**: Inspect a missing destination, create a file at that path from another process before confirming the save, and verify Sqloid preserves it and returns to overwrite confirmation. Repeat after confirming an existing destination, replace that file externally before persistence, and verify the changed file is not silently overwritten; then reinspect, confirm, and save successfully.
- **Automated**: Deterministically synchronized filesystem tests create or replace the destination between inspection and final persistence, asserting no-replace behavior for unconfirmed saves, inspection-state validation for confirmed replacement, preservation of the raced file, a destination-changed/existing error with renewed confirmation, temporary-file cleanup, and successful atomic save when state is unchanged.

### Acceptance criteria

- [ ] Given inspection found no destination and another process creates it before persistence, then atomic no-replace fails without modifying that file and the UI requires overwrite confirmation.
- [ ] Given overwrite was confirmed for an inspected destination but the destination state changes before persistence, then Sqloid preserves the changed file and requires fresh inspection/confirmation.
- [ ] Given overwrite intent is confirmed and the inspected destination state remains unchanged, then atomic replacement succeeds.
- [ ] Given any race-safe persistence failure before replacement, then temporary artifacts are cleaned and retry/cancel paths remain intact.

### User stories addressed

- User story 72: Confirm overwrite and save atomically without damaging existing files

---
