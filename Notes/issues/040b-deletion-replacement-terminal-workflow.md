## Issue 40b: Deletion and replacement terminal workflow

**Type**: AFK
**Blocked by**: Issue 6, Issue 32

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement the deletion and replacement terminal-state UI workflow specified in **Global Key Precedence and Context/Action Matrix** (terminal deletion/replacement row) and **User story 80**, mirroring Issue 40 (outcome-unknown terminal workflow) for the deletion and replacement cases.

When Issue 6 detects deletion (`Database file no longer exists — session ended`) or same-path replacement (`Database file was replaced — session ended`), the session enters a terminal state that forbids further database work but preserves in-memory query/result history selection and query saving. Specifically: the initial result-history selection is the most recent result-history entry; Ctrl+E/Y navigates entries; Ctrl+S saves SQL from the last executed query (with Ctrl+P/N selection); Ctrl+X rejects non-tabular entries with `selected result has no tabular data to export` and opens no picker; `?` shows reduced help; and `q` or Ctrl+C exits immediately with status 1. No transaction or driver work remains pending in these states.

### How to verify

- **Manual**: Trigger deletion and replacement during a session, then browse history, save SQL, attempt export, and quit.
- **Automated**: Fake-Connection/model tests assert terminal entry, initial result-history selection, Ctrl+E/Y navigation, Ctrl+S targeting, Ctrl+X rejection, no database work can start, and immediate status-1 quit.

### Acceptance criteria

- [ ] Given deletion or replacement is detected, then the session enters the appropriate terminal state with the exact terminal message and no further database work can start.
- [ ] Given the terminal state, then Ctrl+E/Y navigates result history, Ctrl+S saves the last executed query, and Ctrl+X rejects non-tabular entries without opening a picker.
- [ ] Given `q` or Ctrl+C in the terminal state, then the application exits immediately with status 1.

### User stories addressed

- User story 80: Forbid terminal database work while retaining applicable in-memory actions (deletion/replacement)
- User story 84: Distinguish same-path replacement from same-inode mutation (terminal UI)

---
