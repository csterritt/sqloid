## Issue 51: Typed array-of-objects JSON export

**Type**: AFK
**Blocked by**: Issue 49

### Parent PRD

`PRD-sqloid.md`

### What to build

Serialize captured rows as a JSON array of objects with deterministic keys and exact SQLite value policies, preserving raw INTEGER/finite REAL tokens and representing BLOB/non-finite values as documented.

### How to verify

- **Manual**: Export mixed typed rows with duplicate labels, BLOBs, NULL, empty strings, non-finite reals, and invalid UTF-8 text.
- **Automated**: Exact-byte and parsed-structure tests assert key order/names, ascending rows, raw numeric tokens, null/string distinctions, base64 BLOBs, quoted non-finite tokens, and absent warning properties.

### Acceptance criteria

- [ ] Given captured rows, then JSON is an array of objects using the shared deterministic deduplicated output names.
- [ ] Given INTEGER, finite/non-finite REAL, NULL, TEXT, or BLOB, then its documented typed JSON representation is emitted exactly.
- [ ] Given invalid UTF or snapshot warnings, then normalized text is exported while no warning object/property is added.

### User stories addressed

- User story 73: Export deterministic array-of-objects JSON
- User story 74: Represent all supported values by JSON policy
- User story 75: Export normalized invalid UTF-8 without extra properties

---
