## Issue 38: DELETE predicate builder

**Type**: AFK
**Blocked by**: Issue 17, Issue 19

### Parent PRD

`PRD-sqloid.md`

### What to build

Deliver DELETE builder behavior using an eligible table and the shared optional one-predicate WHERE flow. Generate safely quoted/parameterized SQL and make both qualified and unqualified states data-runnable before destructive preparation.

### How to verify

- **Manual**: Build DELETE with no WHERE, value WHERE, LIKE, and null operators and inspect its runnable report and SQL.
- **Automated**: QueryBuilder/model tests assert eligibility, optional predicate completion, SQL/params, invalid focus, and transition to preparation rather than direct execution.

### Acceptance criteria

- [ ] Given an eligible table and no WHERE, then DELETE is data-runnable and generates an unqualified statement for later warning.
- [ ] Given a complete WHERE, then it reuses the documented predicate SQL and parameter semantics.
- [ ] Given an incomplete WHERE, then Enter focuses its first invalid component and starts no preparation.

### User stories addressed

- User story 34: Build DELETE with optional assisted WHERE and safety workflow

---
