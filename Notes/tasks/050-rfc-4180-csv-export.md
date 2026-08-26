# Tasks for #50: RFC 4180 CSV export

Parent issue: #50
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify CSV structure and quoting

**Type**: RED
**Output**: Failing byte-golden tests cover deduplicated header, ascending rows, CRLF, minimal quoting, commas, quotes, CR/LF, tabs, and no warning rows/columns.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Create UI-independent exact-byte golden tests in `internal/export` for Issue #50 and the Export formats and values, Output names, Invalid UTF-8 TEXT, Export Module Design, and exact-export Testing Decisions in `Notes/PRD-sqloid.md`. Build immutable inputs from shared `internal/result` rows and full-set deduplicated output names, including duplicate/colliding labels, zero rows, one and multiple rows presented in nonascending source order, empty fields, ordinary ASCII and UTF-8, commas, double quotes, CR, LF, CRLF, and tabs. Require exactly one header record, retained rows serialized by ascending logical position, CRLF after every record, quote doubling, and minimal RFC 4180 quoting only for fields requiring it; tabs alone remain unquoted while CR/LF are preserved inside quoted fields. Pass every Issue #49 metadata warning combination alongside the same data and prove exact output is unchanged, with no warning row, column, prefix, comment, or altered header. Keep this task test-only and defer typed SQLite value policy to Task 3.

---

### 2. Implement RFC 4180 record serialization

**Type**: GREEN
**Output**: Header, ordering, line-ending, quoting, and embedded-content tests pass.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the minimal deterministic RFC 4180 record writer in `internal/export`, consuming an already immutable capture and shared deduplicated names/positioned rows from `internal/result`. Emit one header, sort or traverse rows by ascending retained logical position without mutating input, terminate records with CRLF, preserve embedded content, double embedded quotes, and quote only fields containing RFC-required comma, quote, CR, or LF characters. Keep metadata and warnings outside the serializer API/data path so they cannot become records or columns, avoid UI/model dependencies and locale-sensitive behavior, and make repeated serialization byte-identical. Implement only enough to make Task 1 pass; typed-value conversion belongs to Tasks 3-4.

---

### 3. Specify CSV typed-value policies

**Type**: RED
**Output**: Failing tests cover identical NULL/empty fields, INTEGER/finite/non-finite REAL tokens, lowercase BLOB hex, normalized invalid UTF-8, controls, and exact bytes.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend `internal/export` byte-golden tests with the complete SQLite typed-value matrix supplied by `internal/result`. Require SQL NULL and empty TEXT to produce the identical empty CSV field as the documented accepted lossy limitation, while surrounding delimiters and records remain exact. Cover signed INTEGER boundaries and representative values; finite REAL identity and locale-independent shared tokens including integral values, negative zero, subnormals, exponent forms, and precision edges; pre-existing non-finite REAL text `Inf`, `-Inf`, and `NaN`; empty and arbitrary BLOBs as lowercase hexadecimal; and empty/multibyte TEXT. Include NUL and other controls, tabs, commas, quotes, CR/LF/CRLF, and multiple maximal invalid UTF-8 sequences normalized according to the shared `internal/result` policy to exactly one U+FFFD per maximal invalid sequence. Assert character-for-character UTF-8 bytes, unchanged BLOB bytes before encoding, no warning data, and identical finite REAL tokens to grid/shared result formatting. Keep this task test-only and do not add a second output-name or UTF-normalization implementation.

---

### 4. Implement CSV value serialization

**Type**: GREEN
**Output**: Every documented value policy passes using shared names/result primitives.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement CSV field conversion in `internal/export` over the closed typed-value primitives and normalized TEXT supplied by `internal/result`. Reuse shared full-set deduplicated output names and the authoritative INTEGER/finite REAL token formatter; map non-finite REALs to exact textual tokens, BLOB bytes to lowercase hexadecimal, SQL NULL and empty TEXT to the same empty field, and preserve all normalized TEXT/control content for the RFC 4180 quoting layer. Do not inspect declared SQLite types, use locale formatting, mutate source bytes, renormalize BLOBs, or create private name/REAL/UTF policy copies. Keep warning metadata absent from serializer records and preserve deterministic ascending rows and CRLF structure from Task 2. Implement only enough to make Task 3 and prior byte-golden tests pass.

---

### 5. Document CSV export

**Type**: DOCUMENT
**Output**: Wiki documentation records RFC 4180 structure, all value policies, lossy NULL/empty behavior, and warning exclusion.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #50 implementation and tests from `internal/export` and `internal/result` into the appropriate pages under `Notes/wiki`. Document one deduplicated header, ascending logical-position rows, CRLF record endings, minimal RFC 4180 quoting and quote doubling, and preservation of embedded commas, quotes, CR/LF, and tabs. Record every value policy: identical empty fields for SQL NULL and empty TEXT as an intentional lossy limitation; exact INTEGER and shared finite REAL tokens; textual `Inf`, `-Inf`, and `NaN`; lowercase hexadecimal BLOB; normalized invalid UTF-8 with one U+FFFD per maximal invalid sequence; and preserved controls subject only to field quoting. Explain that capture metadata and Issue #49 warnings never add rows, columns, comments, or prefixes. Cross-reference Issues #14, #23, #33, #49, and #50 and the Numeric value parsing and rendering, Invalid UTF-8 TEXT, Export formats and values, Output names, Result export scope, Export Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` for any added or removed page and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the CSV-export walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/050-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/050-06/code-walkthrough`. Serialize a deterministic fixture with duplicate/colliding labels, out-of-order logical positions, commas, quotes, CR/LF/CRLF, tabs, controls, SQL NULL, empty TEXT, signed INTEGER edges, finite and non-finite REALs, empty/nonempty BLOBs, Unicode, and multiple invalid UTF-8 sequences. Show exact bytes for one header, ascending rows, CRLF endings, minimal quoting, doubled quotes, shared numeric tokens, identical NULL/empty fields, lowercase BLOB hex, and normalized text. Repeat with every Issue #49 warning combination and prove byte-for-byte identical CSV with no metadata rows or columns. Reference Issue #50 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---

### 7. Review CSV bytes

**Type**: REVIEW
**Output**: Human exports all special-value fixtures and verifies exact structure and representations.
**Depends on**: 6

Review record/value serialization in `internal/export`, shared names and typed result primitives in `internal/result`, wiki updates, and `Notes/walkthroughs/050-06/code-walkthrough` against Issue #50. Manually export fixtures containing duplicate labels, reversed traversal order, empty and multiline records, commas, quotes, tabs, controls, NULL, empty TEXT, INTEGER boundaries, finite/non-finite REALs, BLOBs, valid Unicode, and invalid UTF-8. Inspect raw bytes rather than spreadsheet rendering to confirm one deduplicated header, ascending rows, CRLF, minimal quoting, exact numeric tokens, identical NULL/empty fields, lowercase BLOB hex, and U+FFFD normalization. Repeat warned exports and verify no warning row/column or byte change before approving the issue.

---
