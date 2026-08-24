## Issue 36: Destructive-write estimate presentation and count

**Type**: AFK
**Blocked by**: Issue 33, Issue 34

### Parent PRD

`PRD-sqloid.md`

### What to build

Open destructive preparation before UPDATE/DELETE execution. Render operation, table, standalone literal SQL, prominent all-rows warning, and an independent matching-target estimate built from only the identical WHERE predicate.

### How to verify

- **Manual**: Open qualified/unqualified UPDATE and DELETE preparations, including UPDATE with Value/NULL SET assignments.
- **Automated**: QueryBuilder/Connection/model tests assert safe literal rendering, warning visibility, exact estimate SQL, WHERE-only params, loading text, and no history append.

### Acceptance criteria

- [ ] Given preparation opens, then operation, table, rendered SQL, and any no-WHERE warning remain continuously visible.
- [ ] Given estimation is pending, then `Estimating matching target rows…` appears and confirmation is disabled.
- [ ] Given UPDATE SET values, then they do not enter estimate SQL or params, which count only the identical target WHERE.

### User stories addressed

- User story 39: Present rendered destructive SQL and all-rows warning
- User story 40: Disable confirmation while estimating target rows
- User story 43: Estimate only matching targets under the identical WHERE

---
