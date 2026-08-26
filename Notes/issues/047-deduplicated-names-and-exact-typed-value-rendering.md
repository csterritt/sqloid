## Issue 47: Finalize shared typed rendering for exporters

**Type**: AFK
**Blocked by**: Issue 22

### Parent PRD

`PRD-sqloid.md`

### What to build

Finalize the UI-independent output-name and typed-value seam introduced by Issue 22 for CSV and JSON exporters (Issues 50, 51). Issue 22 owns the initial package-shaped implementation and immediate grid consumption of full-set collision-safe deduplication, exact finite REAL tokens, visible grid control characters, maximal invalid UTF-8 replacement, BLOB-byte identity, and `[BLOB n bytes]` display. This issue hardens the shared API and metadata propagation for format-specific exporter policies without moving or reimplementing Issue 22 logic; Issues 50 and 51 prove CSV and JSON consumption.

Non-finite REAL grid rendering is handled in Issue 23. Non-finite REAL CSV/JSON rendering remains in Issues 50/51 per their format-specific policies.

### How to verify

- **Manual**: View duplicate labels, finite/non-finite reals, NULL/empty text, tabs/newlines, invalid UTF-8 TEXT, and BLOB values.
- **Automated**: Shared-module exact-token/byte and consumer-contract tests cover name collisions, `1.0`, `-0.0`, `1e+20`, precision edges, locale independence, maximal invalid sequences, metadata warnings, BLOB identity, and absence of grid/exporter-private copies.

### Acceptance criteria

- [ ] Given Issue 22's package-shaped rendering seam, then it remains the shared implementation consumed by the grid and exposed to exporters without relocation or duplication.
- [ ] Given colliding output labels, then the shared deduplication module produces deterministic final names for the grid and exposes the same names for Issues 50 and 51 without altering driver metadata or SQL.
- [ ] Given finite REAL values, then the shared token logic produces one exact shortest-round-trip REAL-preserving token for the grid and exposes it for format-specific exporters.
- [ ] Given invalid UTF-8 TEXT, then the shared replacement logic produces one U+FFFD per maximal invalid sequence with warning metadata, while BLOB bytes remain unchanged.

### User stories addressed

- User story 74: Represent special SQLite values by documented format policies
- User story 75: Replace invalid UTF-8 consistently without changing BLOBs
- User story 83: Preserve finite REAL identity with exact shared tokens

---
