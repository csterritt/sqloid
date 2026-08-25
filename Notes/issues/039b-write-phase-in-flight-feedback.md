## Issue 39b: Write-phase in-flight feedback

**Type**: AFK
**Blocked by**: Issue 23, Issue 38, Issue 39

### Parent PRD

`PRD-sqloid.md`

### What to build

Reuse Issue 23's generic request-in-flight gating contract and own all write-phase feedback integration without altering SELECT/page/count labels. Surface exact `Running…` feedback during beginning/executing and `Estimating matching target rows…`, `Committing…`, `Rolling back…`, and `cancelling…` feedback during the relevant phases, and enforce the request-in-flight restrictions from the **Global Key Precedence and Context/Action Matrix** for write-phase pending states: Enter ignored with a Ctrl+W hint, history/save/export rejected with explanatory feedback, and permitted local interaction preserved.

### How to verify

- **Manual**: Hold fake beginning/executing/estimate/commit/rollback phases and try Enter, history, save/export, quit, and cancellation keys.
- **Automated**: Scripted model tests assert the exact label for every beginning/executing/estimate/commit/rollback/cancelling phase, Enter hints, blocked actions, permitted local actions, and unchanged request count during write phases.

### Acceptance criteria

- [ ] Given beginning or executing write work, then exact `Running…` feedback remains visible; given estimate, commit, rollback, or cancellation, then its documented phase-specific feedback remains visible; throughout, the UI stays responsive.
- [ ] Given Enter during a write request, then it is consumed with a Ctrl+W hint and starts no stacked request.
- [ ] Given history/save/export during a write request, then it is rejected with explanatory feedback.

### User stories addressed

- User story 13: Keep the UI interactive with phase feedback (write phases)
- User story 14: Ignore Enter while a request is in flight (write phases)

---
