## Issue 44: RFC 4180 CSV export

**Type**: AFK
**Blocked by**: Issue 43

### Parent PRD

`PRD-sqloid.md`

### What to build

Serialize captured rows to exact RFC 4180 UTF-8 CSV using deduplicated names and the value policies in **Export formats and values**, without embedding snapshot warnings or metadata.

### How to verify

- **Manual**: Export rows containing commas, quotes, CR/LF, tabs, NULL, empty strings, BLOBs, finite/non-finite reals, and invalid UTF-8 replacements.
- **Automated**: Byte-golden tests assert CRLF, header, minimal quoting, embedded content, lowercase BLOB hex, the identical empty field produced by NULL and empty TEXT, exact numeric tokens, row order, and no warning records.

### Acceptance criteria

- [ ] Given captured rows, then CSV has one header, ascending retained-row order, CRLF records, and minimal RFC 4180 quoting.
- [ ] Given SQL NULL or empty TEXT, then both emit the same empty CSV field as the documented accepted lossy limitation; CSV does not preserve their distinction.
- [ ] Given BLOB, control characters, or REALs, then each follows the documented CSV representation exactly.
- [ ] Given metadata warnings, then export still succeeds without adding CSV rows or columns for them.

### User stories addressed

- User story 73: Export deterministic RFC 4180 CSV
- User story 74: Represent all supported values by CSV policy
- User story 75: Export normalized invalid UTF-8 without extra records

---
