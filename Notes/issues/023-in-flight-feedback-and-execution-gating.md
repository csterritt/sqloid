## Issue 23: In-flight feedback and execution gating (SELECT/page/count)

**Type**: AFK
**Blocked by**: Issue 20, Issue 21

### Parent PRD

`PRD-sqloid.md`

### What to build

Surface responsive phase-specific request feedback and enforce the request-in-flight row of the **Global Key Precedence and Context/Action Matrix** for SELECT, page, and count requests. This issue defines the generic gating behavior and owns only SELECT/page/count integration: show `Running…`, `Counting rows…`, and page-loading feedback; prevent execution/history/save/export stacking while retaining permitted local interaction. It excludes all write-phase labels and integration, which Issue 39b owns.

### How to verify

- **Manual**: Hold fake SELECT/page/count/write phases and try Enter, history, save/export, horizontal scroll, quit, and cancellation keys.
- **Automated**: Scripted model tests assert phase labels, Enter hints, blocked actions, permitted local actions, and unchanged request count.

### Acceptance criteria

- [ ] Given pending SELECT/page/count work, then the relevant running/count/page-loading feedback remains visible and the UI stays responsive.
- [ ] Given Enter during a SELECT/page/count request, then it is consumed with a Ctrl+W hint and starts no stacked request.
- [ ] Given history/save/export during a SELECT/page/count request, then it is rejected with explanatory feedback.

### User stories addressed

- User story 13: Keep the UI interactive with phase feedback
- User story 14: Ignore Enter while a request is in flight

---
