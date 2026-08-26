## Issue 53: Atomic saves and overwrite protection

**Type**: AFK
**Blocked by**: Issue 52

### Parent PRD

`PRD-sqloid.md`

### What to build

Complete file output using destination-local temporary files and rename, with overwrite confirmation and exact save-flow restoration. Preserve existing destinations and clean temporary files on every pre-rename failure.

### How to verify

- **Manual**: Save new and existing targets, cancel overwrite, confirm it, and trigger serialization/write/rename failures under restrictive permissions.
- **Automated**: Injected filesystem tests assert immutable payload/selection through confirmation, destination preservation, temp cleanup, atomic replacement, inline errors, and intact retry/cancel paths.

### Acceptance criteria

- [ ] Given an existing destination, then no replacement occurs until explicit overwrite confirmation.
- [ ] Given serialization or I/O failure before rename, then the existing destination is unchanged and temporary artifacts are cleaned.
- [ ] Given rename failure, then it remains an inline retry/cancel error and the UI never claims replacement occurred.

### User stories addressed

- User story 72: Confirm overwrite and save atomically without damaging existing files

---
