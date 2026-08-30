## Issue 71: Report an actionable cause for atomic-save short writes

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

When an atomic save's write operation returns fewer bytes than requested with a nil error, convert that boundary result to `io.ErrShortWrite` before constructing the write-stage error. Preserve the existing destination and temporary-file cleanup exactly as for every other pre-rename write failure, and keep the failure attributed to the write stage.

### How to verify

- **Manual**: Inject a save writer that accepts only part of the serialized output without returning an error; confirm the save flow reports a write-stage short-write cause rather than `<nil>`, preserves an existing destination, and leaves no temporary file.
- **Automated**: Export save tests fake the exact `(n < len(p), nil)` result and assert `errors.Is` matches `io.ErrShortWrite`, the `StageError` identifies the write stage, the user-facing error contains an actionable short-write cause, no rename occurs, the destination is unchanged, and temp cleanup runs.

### Acceptance criteria

- [ ] Given `SaveFile.Write` returns a short byte count and nil error, then `WriteAtomic` fails with a write-stage cause wrapping `io.ErrShortWrite`, never `<nil>`.
- [ ] Given that short write occurs before rename, then an existing destination remains byte-for-byte unchanged and the temporary file is removed.
- [ ] Given a complete write or a non-nil write error, then existing success and error behavior remains unchanged.

### User stories addressed

- User story 72: Preserve destinations and clean temporary files on pre-rename save failure

---
