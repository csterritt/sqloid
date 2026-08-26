## Issue 14: Universal value parsing and safe SQL atoms

**Type**: AFK
**Blocked by**: Issue 9

### Parent PRD

`PRD-sqloid.md`

### What to build

Implement pure QueryBuilder primitives for deterministic verbatim INTEGER/REAL/TEXT parsing, parameter binding, identifier quoting, fixed-choice SQL tokens, and one canonical typed-value-to-SQL-literal renderer according to **Numeric value parsing and rendering**, **SQL safety**, and **Query save targeting**. The renderer is the single definition site consumed by destructive modal SQL and saved standalone SQL: INTEGER uses exact decimal form, finite REAL uses the PRD's REAL-preserving token, TEXT is single-quoted with embedded quotes doubled, NULL is the keyword, and BLOB is `X'hex'`.

### How to verify

- **Manual**: Build queries with numeric-looking, whitespace-padded, empty, `NULL`, wildcard, and injection-looking text and inspect SQL, bindings, and rendered literals.
- **Automated**: Table-driven tests cover every parse boundary, int64/float64 overflow, leading signs/zeros, valid `0x1p2` and malformed `0x1p` hexadecimal-float cases, empty text, identifier quotes, attempted injection, and exact canonical literals for INTEGER, finite REAL, TEXT with quotes, NULL, and BLOB.

### Acceptance criteria

- [ ] Given valid hexadecimal floating-point input such as `0x1p2`, then it binds as finite REAL `4.0`; hexadecimal integer `0x1A` and malformed hexadecimal float `0x1p` remain exact TEXT.
- [ ] Given verbatim input, then only the documented finite INTEGER/REAL forms bind numerically and every other token binds as exact TEXT.
- [ ] Given empty input or typed `NULL`, then it remains TEXT rather than SQL NULL.
- [ ] Given generated SQL, then every user value is bound and every schema identifier is safely double-quoted.
- [ ] Given a typed value for standalone rendering, then the shared renderer emits exact decimal INTEGER, PRD-formatted finite REAL, safely quote-doubled TEXT, `NULL`, or `X'hex'` as applicable; Issues 40 and 48 consume this renderer rather than defining private serializers.

### User stories addressed

- User story 38: Parse and bind values deterministically
- User story 76: Prevent injection through binding and schema-derived quoting

---
