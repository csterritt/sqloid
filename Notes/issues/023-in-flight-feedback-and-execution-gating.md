## Issue 23: In-flight feedback and execution gating

**Type**: AFK
**Blocked by**: Issue 19

### Parent PRD

`PRD-sqloid.md`

### What to build

Surface responsive phase-specific request feedback and enforce the request-in-flight row of the **Global Key Precedence and Context/Action Matrix**, preventing execution/history/save/export stacking while retaining permitted local interaction.

### How to verify

- **Manual**: Hold fake SELECT/page/count/write phases and try Enter, history, save/export, horizontal scroll, quit, and cancellation keys.
- **Automated**: Scripted model tests assert phase labels, Enter hints, blocked actions, permitted local actions, and unchanged request count.

### Acceptance criteria

- [ ] Given pending work, then the relevant running/count/page/estimate/commit/rollback feedback remains visible and the UI stays responsive.
- [ ] Given Enter during a request, then it is consumed with a Ctrl+W hint and starts no stacked request.
- [ ] Given history/save/export during a request, then it is rejected with explanatory feedback.

### User stories addressed

- User story 13: Keep the UI interactive with phase feedback
- User story 14: Ignore Enter while a request is in flight

---
