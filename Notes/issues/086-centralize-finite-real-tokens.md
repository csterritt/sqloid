## Issue 86: Centralize finite REAL token generation

**Type**: AFK
**Blocked by**: None — can start immediately

### Parent PRD

`PRD-sqloid.md`

### What to build

Use one canonical implementation of the PRD's finite-REAL token rule for both query-literal serialization and result rendering. Remove `querybuilder`'s duplicate formatter and delegate finite SQL literal rendering to `result.RealToken` (or an equally shared lower-level helper), while preserving non-finite literal rejection and the existing grid, CSV, JSON, and saved-SQL output contracts.

### How to verify

- **Manual**: Save queries containing representative REAL values and compare their literals with grid, CSV, and JSON tokens for the same finite values, including integral REAL, negative zero, exponent, and precision-edge cases.
- **Automated**: Add a cross-package table asserting query literals and `result.RealToken` are identical for `1.0`, `-0.0`, `1e+20`, subnormal, and adjacent-float cases; retain round-trip, locale-independence, INTEGER, and non-finite rejection tests, and ensure no second FormatFloat-plus-suffix implementation remains.

### Acceptance criteria

- [ ] Given any finite float64 used in saved SQL or a result, then every consumer receives the same shortest-round-trip, REAL-preserving token from one implementation.
- [ ] Given an integral REAL or negative zero, then the token retains REAL identity exactly as `1.0` or `-0.0`.
- [ ] Given a non-finite REAL literal, then query serialization continues to reject it while existing result-format policies remain unchanged.
- [ ] Given the codebase is searched for the finite-token algorithm, then only the canonical implementation contains the formatting and suffix logic.

### User stories addressed

- User story 68: Serialize standalone SQL literals deterministically
- User story 83: Use one exact finite-REAL token in grid, CSV, and JSON

---
