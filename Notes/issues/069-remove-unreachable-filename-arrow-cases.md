## Issue 69: Remove unreachable filename arrow-key cases

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Remove the dead `tea.KeyLeft` and `tea.KeyRight` arms from `applyPickerFilenameKey`. The picker-level dispatcher consumes those keys first to toggle export format in every picker focus, so they can never reach filename editing. Preserve that current routing contract and all reachable filename editing, focus, submit, cancel, and format-selection behavior.

### How to verify

- **Verification sequencing**: Package/seam-level automated verification can proceed before Issue 57; manual/end-to-end steps that drive the shipped TUI must be re-run after Issue 57 lands.
- **Manual**: Open the export picker, focus both the directory list and filename, and confirm Left/Right still toggle format while runes, Backspace/Delete, Home/End, and Ctrl+A/Ctrl+E/Ctrl+U still edit the filename as before.
- **Automated**: Update picker key-routing tests to assert Left/Right are consumed by format toggling before filename dispatch and that every remaining filename-editing case is reachable; run existing picker focus, validation, cancellation, and completion tests unchanged.

### Acceptance criteria

- [ ] Given either picker focus, when Left or Right is pressed, then export format toggles exactly as before.
- [ ] Given `applyPickerFilenameKey`, then it contains no unreachable Left/Right cursor-movement cases.
- [ ] Given filename focus, then all remaining documented editing, submit, cancel, and focus transitions retain their existing behavior.

### User stories addressed

- User story 71: Keep directory, filename, extension, and format selection behavior deterministic
- User story 78: Preserve exact picker cancellation and opener restoration

---
