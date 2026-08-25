## Issue 41: Centralize rendering for exporters (extraction from grid)

**Type**: AFK
**Blocked by**: Issue 19

### Parent PRD

`PRD-sqloid.md`

### What to build

Extract and centralize the result output-name deduplication and value-rendering logic that Issue 19 implements for the grid, defining the reusable module that CSV and JSON exporters (Issues 44, 45) will consume. Issue 19 owns the initial implementation of full-set collision-safe deduplication, exact finite REAL tokens, visible grid control characters, maximal invalid UTF-8 replacement, and `[BLOB n bytes]` display. This issue factors that logic into a reusable module without duplication and migrates the grid to consume it; Issues 44 and 45 prove CSV and JSON consumption.

Non-finite REAL grid rendering is handled in Issue 19b. Non-finite REAL CSV/JSON rendering remains in Issues 44/45 per their format-specific policies.

### How to verify

- **Manual**: View duplicate labels, finite/non-finite reals, NULL/empty text, tabs/newlines, invalid UTF-8 TEXT, and BLOB values.
- **Automated**: Exact-token/byte tests cover name collisions, `1.0`, `-0.0`, `1e+20`, precision edges, locale independence, maximal invalid sequences, metadata warnings, and BLOB identity.

### Acceptance criteria

- [ ] Given the rendering logic from Issue 19, then it is factored into a reusable module and the grid consumes that module without duplication.
- [ ] Given colliding output labels, then the shared deduplication module produces deterministic final names for the grid and exposes the same names for Issues 44 and 45 without altering driver metadata or SQL.
- [ ] Given finite REAL values, then the shared token logic produces one exact shortest-round-trip REAL-preserving token for the grid and exposes it for format-specific exporters.
- [ ] Given invalid UTF-8 TEXT, then the shared replacement logic produces one U+FFFD per maximal invalid sequence with warning metadata, while BLOB bytes remain unchanged.

### User stories addressed

- User story 74: Represent special SQLite values by documented format policies
- User story 75: Replace invalid UTF-8 consistently without changing BLOBs
- User story 83: Preserve finite REAL identity with exact shared tokens

---
