## Issue 35: INSERT choices, omission, and prompt restoration

**Type**: AFK
**Blocked by**: Issue 8, Issue 12, Issue 17

### Parent PRD

`PRD-sqloid.md`

### What to build

Deliver INSERT construction across every insertable column with Value/NULL/Default/Omit choices, exact revisiting, safe SQL/params, the all-omit `DEFAULT VALUES` path, and explicit zero-insertable-column blocking.

### How to verify

- **Manual**: Insert empty TEXT, NULL, omitted values, mixed values, all omitted, INTEGER PRIMARY KEY omission, and a zero-insertable-column fixture.
- **Automated**: Schema/QueryBuilder/model/SQLite tests assert prompts, stored choices, parameter order, `DEFAULT VALUES`, generated/hidden exclusion, and virtual-table best-effort errors.

### Acceptance criteria

- [ ] Given insertable columns, then each is prompted with exactly Value, NULL, or Default/Omit and revisiting restores exact state.
- [ ] Given all columns omitted, then the runnable statement is `INSERT INTO "table" DEFAULT VALUES`.
- [ ] Given zero insertable columns, then prompts do not open and `table has no insertable columns` prevents execution.

### User stories addressed

- User story 35: Distinguish INSERT Value, NULL, and omission
- User story 36: Support DEFAULT VALUES and reject zero-column tables
- User story 37: Revisit write prompts with exact prior state

---
