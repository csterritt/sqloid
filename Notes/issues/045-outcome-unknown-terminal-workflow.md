## Issue 45: Outcome-unknown terminal workflow

**Type**: AFK
**Blocked by**: Issue 36, Issue 43

### Parent PRD

`PRD-sqloid.md`

### What to build

When rollback or commit resolution fails, wait until no work remains, append/select exactly one outcome-unknown entry, and enter the terminal in-memory workflow specified in **Writes and commit boundary**. This issue owns terminal entry, initial selection, in-memory history navigation, database-work prohibition, reduced help, and immediate status-1 quit. Issue 48 owns terminal Ctrl+S integration and Issue 49 owns terminal Ctrl+X integration.

### How to verify

- **Manual**: Inject unresolved commit and rollback outcomes, browse query/result entries, attempt database actions, open reduced help, and quit.
- **Automated**: Fake-Connection/model tests assert settlement, newest selected entry fields, non-persistence wording, terminal restrictions, history navigation, reduced help, and status-1 quit.

### Acceptance criteria

- [ ] Given unresolved commit/rollback after pending work ends, then the newest selected entry records operation, SQL, phase, driver error, and non-proving RowsAffected information.
- [ ] Given outcome-unknown terminal state, then no database work can start while Ctrl+P/N and Ctrl+E/Y continue to navigate immutable in-memory history and `?` opens reduced help.
- [ ] Given q or Ctrl+C in the terminal state, then quit is immediate with status 1.

### User stories addressed

- User story 47: End unresolved writes in a safe outcome-unknown terminal state
- User story 80: Forbid terminal database work while retaining applicable in-memory actions
- User story 85: Select and describe the outcome-unknown result with non-persistence wording

---
