## Issue 42: SQL save targeting and standalone serialization

**Type**: AFK
**Blocked by**: Issue 31, Issue 32, Issue 38, Issue 40, Issue 40b

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement Ctrl+S target selection and SQL serialization from **Query save targeting**: a viewed historical result's associated query, otherwise the current runnable builder, otherwise the last actual execution. In deletion/replacement and outcome-unknown terminal states, use only immutable in-memory targets: the Ctrl+P/N-selected query when present, otherwise the last actual execution. Produce one executable statement with safe literals, quoted identifiers, and a semicolon.

### How to verify

- **Manual**: Save from each ordinary and terminal targeting context and with strings, NULL, BLOB, numeric values, UPDATE/INSERT choices, and no available query.
- **Automated**: Export/model tests assert ordinary and terminal target priority, in-memory-only terminal behavior, no-picker error, exact SQL bytes, quote doubling, BLOB hex, NULL keywords, and round-trip execution.

### Acceptance criteria

- [ ] Given Ctrl+S, then the viewed result query wins, otherwise runnable builder, otherwise last executed query.
- [ ] Given no target, then `no runnable query to save` appears and no picker opens.
- [ ] Given a target, then output is one standalone statement with quoted identifiers, safely serialized values, and trailing semicolon.
- [ ] Given a deletion/replacement or outcome-unknown terminal state, then Ctrl+S uses the Ctrl+P/N-selected in-memory query when present, otherwise the last actual execution, and starts no database work.

### User stories addressed

- User story 67: Choose the correct query to save
- User story 68: Serialize one safe executable SQL statement
- User story 80: Preserve terminal-state in-memory query saving without database work

---
