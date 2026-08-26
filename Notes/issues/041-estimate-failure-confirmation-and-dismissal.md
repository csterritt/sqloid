## Issue 41: Estimate failure, confirmation, and dismissal

**Type**: AFK
**Blocked by**: Issue 6, Issue 40

### Parent PRD

`PRD-sqloid.md`

### What to build

Complete destructive preparation state transitions for estimate success, failure, cancellation, confirmation, and dismissal. This issue owns applying the Issue 6 infrastructure to estimate Ctrl+W, visible `cancelling…` settlement, and late-success rejection. Preserve SQL/warnings on failure and ensure preparation itself never creates history or an execution.

### How to verify

- **Manual**: Exercise successful, failed, Ctrl+W-cancelled, Esc/n-dismissed, and Enter/y-confirmed estimates.
- **Automated**: Scripted identity/lifecycle tests assert confirmation enablement, retained content, no history on nonconfirmation, and exactly one execution command after confirmation.

### Acceptance criteria

- [ ] Given estimate failure, then its error appears without hiding SQL/warnings and deliberate confirmation becomes available.
- [ ] Given Esc, n, dismissal, or cancelled estimation, then preparation closes after settlement with both histories unchanged.
- [ ] Given settled success or failure, then Enter/y confirms exactly one actual write and cannot confirm twice.
- [ ] Given Ctrl+W during estimation, then `cancelling…` remains visible until settlement, late success is discarded as cancelled, and both histories remain unchanged.

### User stories addressed

- User story 41: Allow deliberate confirmation after estimate failure
- User story 42: Confirm or dismiss without preparation history

---
