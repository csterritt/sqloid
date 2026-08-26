## Issue 17: Guided WHERE predicates and SQL NULL semantics

**Type**: AFK
**Blocked by**: Issue 12, Issue 14

### Parent PRD

`PRD-sqloid.md`

### What to build

Deliver the reusable one-predicate WHERE flow for SELECT/UPDATE/DELETE: column, fixed operator, and conditional universal value entry, including the exact SQL-NULL guidance and verbatim LIKE behavior.

### How to verify

- **Manual**: Build predicates with every operator, typed `NULL`, empty text, `%`/`_`, and both null operators.
- **Automated**: QueryBuilder and scripted UI tests assert operator availability, binding, no-value null operators, inline hint/help, and ordinary NULL comparison semantics.

### Acceptance criteria

- [ ] Given any eligible column, then all documented operators are offered and only `IS NULL`/`IS NOT NULL` omit a value.
- [ ] Given typed `NULL`, then it binds as TEXT and the UI directs SQL-null intent to the null operators.
- [ ] Given LIKE text containing `%` or `_`, then it is bound verbatim with SQLite wildcard behavior.

### User stories addressed

- User story 28: Build one guided predicate with explicit SQL NULL semantics

---
