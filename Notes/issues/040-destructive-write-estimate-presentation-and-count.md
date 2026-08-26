## Issue 40: Destructive-write estimate presentation and count

**Type**: AFK
**Blocked by**: Issue 37, Issue 38

### Parent PRD

`PRD-sqloid.md`

### What to build

Open destructive preparation before UPDATE/DELETE execution. Render operation, table, standalone literal SQL through Issue 14's canonical identifier/literal atoms, prominent all-rows warning, and an independent matching-target estimate built from only the identical WHERE predicate. Do not define a modal-private SQL literal serializer.

### How to verify

- **Manual**: Open qualified/unqualified UPDATE and DELETE preparations, including UPDATE with Value/NULL SET assignments.
- **Automated**: QueryBuilder/Connection/model tests assert reuse of canonical safe literal rendering, warning visibility, exact estimate SQL, WHERE-only params, loading text, and no history append.

### Acceptance criteria

- [ ] Given preparation opens, then operation, table, rendered SQL produced with Issue 14's shared atoms, and any no-WHERE warning remain continuously visible.
- [ ] Given estimation is pending, then `Estimating matching target rows…` appears and confirmation is disabled.
- [ ] Given UPDATE SET values, then they do not enter estimate SQL or params, which count only the identical target WHERE.

### User stories addressed

- User story 39: Present rendered destructive SQL and all-rows warning
- User story 40: Disable confirmation while estimating target rows
- User story 43: Estimate only matching targets under the identical WHERE

---
