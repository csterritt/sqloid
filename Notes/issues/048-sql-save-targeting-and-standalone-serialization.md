## Issue 48: SQL save targeting and standalone serialization

**Type**: AFK
**Blocked by**: Issue 14, Issue 35, Issue 36, Issue 42, Issue 45, Issue 46

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement Ctrl+S target selection and full-statement SQL assembly from **Query save targeting**: a viewed historical result's associated query, otherwise the current runnable builder, otherwise the last actual execution. In deletion/replacement and outcome-unknown terminal states, use only immutable in-memory targets: the Ctrl+P/N-selected query when present, otherwise the last actual execution. Produce one executable statement with Issue 14's canonical identifier/literal atoms and a semicolon; do not define a second literal serializer.

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
