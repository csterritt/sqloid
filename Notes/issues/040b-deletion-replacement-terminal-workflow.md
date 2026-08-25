## Issue 40b: Deletion and replacement terminal workflow

**Type**: AFK
**Blocked by**: Issue 6, Issue 32

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement the deletion and replacement terminal-state UI workflow specified in **Global Key Precedence and Context/Action Matrix** (terminal deletion/replacement row) and **User story 80**, mirroring Issue 40 (outcome-unknown terminal workflow) for the deletion and replacement cases.

When Issue 6 detects deletion (`Database file no longer exists — session ended`) or same-path replacement (`Database file was replaced — session ended`), the session enters a terminal state that forbids further database work but preserves in-memory query/result history selection. This issue owns terminal entry, initial result-history selection, Ctrl+P/N and Ctrl+E/Y navigation, reduced help, and immediate status-1 quit. If result history is empty, result selection remains empty, the terminal message remains the primary view, navigation is a no-op, and no synthetic entry or missing backing rows are rendered. Issue 42 owns terminal Ctrl+S integration and Issue 43 owns terminal Ctrl+X integration. No transaction or driver work remains pending in these states.

### How to verify

- **Manual**: Trigger deletion and replacement during a session, then browse query/result history, attempt database actions, open reduced help, and quit.
- **Automated**: Fake-Connection/model tests assert terminal entry with populated and empty histories, initial result-history selection or empty fallback, Ctrl+P/N and Ctrl+E/Y navigation/no-op behavior, no synthetic or missing-backed entry, reduced help, no database work, and immediate status-1 quit.

### Acceptance criteria

- [ ] Given deletion or replacement is detected, then the session enters the appropriate terminal state with the exact terminal message and no further database work can start.
- [ ] Given the terminal state, then Ctrl+P/N and Ctrl+E/Y navigate immutable in-memory history, `?` opens reduced help, and no database work can start.
- [ ] Given result history is empty on terminal entry, then result selection remains empty, the deletion/replacement message remains primary, Ctrl+E/Y is a no-op, and no synthetic entry or missing backing rows are rendered.
- [ ] Given `q` or Ctrl+C in the terminal state, then the application exits immediately with status 1.

### User stories addressed

- User story 80: Forbid terminal database work while retaining applicable in-memory actions (deletion/replacement)
- User story 84: Distinguish same-path replacement from same-inode mutation (terminal UI)

---
