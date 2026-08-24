## Issue 8: Schema catalog and table eligibility

**Type**: AFK
**Blocked by**: Issue 5

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement the Schema module and catalog metadata described in **Schema scope, cache, and validation** and **Schema metadata**. List eligible main-schema objects and column capabilities independently of the UI.

### How to verify

- **Manual**: Open fixtures containing ordinary, virtual, view, WITHOUT ROWID, generated, hidden, system, and `_cf_METADATA` objects.
- **Automated**: Table-driven catalog and SQLite integration tests assert object kind, rowid capability/shadowing, declared type, insertability, and exclusions.

### Acceptance criteria

- [ ] Given the main schema, then eligible ordinary tables, virtual tables, and views are returned while `sqlite_%` and `_cf_METADATA` are excluded.
- [ ] Given a view, then it is SELECT-only; given table metadata, then write and insert eligibility are represented accurately.
- [ ] Given hidden or generated columns, then they are marked noninsertable without adding type-specific input behavior.

### User stories addressed

- User story 20: Refresh and list eligible main-schema objects

---
