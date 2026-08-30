## Issue 64: Enforce overwrite intent at the atomic persistence boundary

**Type**: AFK
**Blocked by**: Issue 63 — preserve the short-write handling while restructuring the shared `WriteAtomic` persistence boundary

### Parent PRD

`PRD-sqloid.md`

### What to build

Close the inspection-to-rename race in `internal/export/save_write.go` on the supported Linux/macOS platforms by carrying explicit overwrite intent and the inspected destination state to the persistence boundary. An unconfirmed new-file save must use an atomic no-replace creation path such as `O_CREATE|O_EXCL` so a file created after `InspectDestination` cannot be overwritten; do not rename over an unconfirmed destination. Replacement may use destination-local temporary-file-plus-rename only after confirmation tied to the destination state the user inspected; if that state changes before persistence, return a destination-changed/existing error to the save flow and require fresh inspection/confirmation instead of replacing silently. Preserve destination-local staging, atomicity where replacement is authorized, cleanup, retry, and cancellation behavior.

### How to verify

- **Verification sequencing**: Package/seam-level automated verification can proceed before Issue 57; manual/end-to-end steps that drive the shipped TUI must be re-run after Issue 57 lands.
- **Manual**: Inspect a missing destination, create a file at that path from another process before confirming the save, and verify Sqloid preserves it and returns to overwrite confirmation. Repeat after confirming an existing destination, replace that file externally before persistence, and verify the changed file is not silently overwritten; then reinspect, confirm, and save successfully.
- **Automated**: Deterministically synchronized Linux/macOS filesystem tests create or replace the destination between inspection and final persistence, asserting exclusive no-replace creation for unconfirmed saves, inspection-state validation for confirmed replacement, preservation of the raced file, a destination-changed/existing error with renewed confirmation, destination-local staging, temporary-file cleanup, and successful atomic save when state is unchanged.

### Acceptance criteria

- [ ] Given inspection found no destination and another process creates it before persistence, then an atomic `O_CREATE|O_EXCL`-style no-replace operation fails without modifying that file and the UI requires overwrite confirmation.
- [ ] Given overwrite was confirmed for an inspected destination but the destination state changes before persistence, then Sqloid preserves the changed file and requires fresh inspection/confirmation.
- [ ] Given overwrite intent is confirmed and the inspected destination state remains unchanged, then destination-local temporary-file-plus-rename replacement succeeds atomically.
- [ ] Given either the unconfirmed no-replace path or confirmed replacement path, then every staging or temporary file is created in the destination directory.
- [ ] Given any race-safe persistence failure before replacement, then temporary artifacts are cleaned and retry/cancel paths remain intact.

### User stories addressed

- User story 72: Confirm overwrite and save atomically without damaging existing files

---
