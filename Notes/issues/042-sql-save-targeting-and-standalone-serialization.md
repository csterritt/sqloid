## Issue 42: SQL save targeting and standalone serialization

**Type**: AFK
**Blocked by**: Issue 31, Issue 32, Issue 38

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement Ctrl+S target selection and SQL serialization from **Query save targeting**: viewed historical query, current runnable builder, then last actual execution. Produce one executable statement with safe literals, quoted identifiers, and a semicolon.

### How to verify

- **Manual**: Save from each targeting context and with strings, NULL, BLOB, numeric values, UPDATE/INSERT choices, and no available query.
- **Automated**: Export/model tests assert target priority, no-picker error, exact SQL bytes, quote doubling, BLOB hex, NULL keywords, and round-trip execution.

### Acceptance criteria

- [ ] Given Ctrl+S, then the viewed result query wins, otherwise runnable builder, otherwise last executed query.
- [ ] Given no target, then `no runnable query to save` appears and no picker opens.
- [ ] Given a target, then output is one standalone statement with quoted identifiers, safely serialized values, and trailing semicolon.

### User stories addressed

- User story 67: Choose the correct query to save
- User story 68: Serialize one safe executable SQL statement

---
