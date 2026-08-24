## Issue 40: Outcome-unknown terminal workflow

**Type**: AFK
**Blocked by**: Issue 32, Issue 39

### Parent PRD

`PRD-sqloid.md`

### What to build

When rollback or commit resolution fails, wait until no work remains, append/select exactly one outcome-unknown entry, and enter the terminal in-memory workflow specified in **Writes and commit boundary**.

### How to verify

- **Manual**: Inject unresolved commit and rollback outcomes, browse entries, save SQL, attempt export/database actions, and quit.
- **Automated**: Fake-Connection/model tests assert settlement, newest selected entry fields, non-persistence wording, terminal restrictions, history navigation, save targeting, export rejection, and status-1 quit.

### Acceptance criteria

- [ ] Given unresolved commit/rollback after pending work ends, then the newest selected entry records operation, SQL, phase, driver error, and non-proving RowsAffected information.
- [ ] Given outcome-unknown terminal state, then no database work can start while in-memory history and SQL saving remain available.
- [ ] Given Ctrl+X on its non-tabular entry or q/Ctrl+C, then export is rejected exactly and quit is immediate with status 1.

### User stories addressed

- User story 47: End unresolved writes in a safe outcome-unknown terminal state
- User story 80: Forbid terminal database work while retaining applicable in-memory actions
- User story 85: Select and describe the outcome-unknown result without misleading export

---
