## Issue 12: Universal value parsing and safe SQL atoms

**Type**: AFK
**Blocked by**: Issue 8

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement pure QueryBuilder primitives for deterministic verbatim INTEGER/REAL/TEXT parsing, parameter binding, identifier quoting, and fixed-choice SQL tokens according to **Numeric value parsing and rendering** and **SQL safety**.

### How to verify

- **Manual**: Build queries with numeric-looking, whitespace-padded, empty, `NULL`, wildcard, and injection-looking text and inspect SQL plus bindings.
- **Automated**: Table-driven tests cover every parse boundary, int64/float64 overflow, leading signs/zeros, empty text, identifier quotes, and attempted injection.

### Acceptance criteria

- [ ] Given verbatim input, then only the documented finite INTEGER/REAL forms bind numerically and every other token binds as exact TEXT.
- [ ] Given empty input or typed `NULL`, then it remains TEXT rather than SQL NULL.
- [ ] Given generated SQL, then every user value is bound and every schema identifier is safely double-quoted.

### User stories addressed

- User story 38: Parse and bind values deterministically
- User story 76: Prevent injection through binding and schema-derived quoting

---
