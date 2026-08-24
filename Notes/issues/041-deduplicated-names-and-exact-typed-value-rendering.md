## Issue 41: Deduplicated names and exact typed-value rendering

**Type**: AFK
**Blocked by**: Issue 19

### Parent PRD

`PRD-sqloid.md`

### What to build

Centralize result output names and value representation shared by grid and exporters. Apply full-set collision-safe deduplication, exact finite REAL tokens, visible grid control characters, maximal invalid UTF-8 replacement, and unchanged bounded BLOB bytes.

### How to verify

- **Manual**: View duplicate labels, finite/non-finite reals, NULL/empty text, tabs/newlines, invalid UTF-8 TEXT, and BLOB values.
- **Automated**: Exact-token/byte tests cover name collisions, `1.0`, `-0.0`, `1e+20`, precision edges, locale independence, maximal invalid sequences, metadata warnings, and BLOB identity.

### Acceptance criteria

- [ ] Given colliding output labels, then deterministic final names are shared by grid, CSV, and JSON without altering driver metadata or SQL.
- [ ] Given finite REAL values, then one exact shortest-round-trip REAL-preserving token is used everywhere.
- [ ] Given invalid UTF-8 TEXT, then each maximal invalid sequence becomes one U+FFFD with warning metadata while BLOB bytes remain unchanged.

### User stories addressed

- User story 74: Represent special SQLite values by documented format policies
- User story 75: Replace invalid UTF-8 consistently without changing BLOBs
- User story 83: Preserve finite REAL identity with exact shared tokens

---
