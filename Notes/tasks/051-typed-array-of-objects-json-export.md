# Tasks for #51: Typed array-of-objects JSON export

Parent issue: #51
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify deterministic JSON object structure

**Type**: RED
**Output**: Failing exact-byte/parsed tests cover array shape, shared deduplicated keys/order, ascending rows, string escaping, and no warning objects/properties.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Create UI-independent exact-byte and parsed-structure tests in `internal/export` for Issue #51 and the Export formats and values, Output names, Invalid UTF-8 TEXT, Export Module Design, and exact-export Testing Decisions in `Notes/PRD-sqloid.md`. Build immutable fixtures from `internal/result` with zero, one, and multiple rows; full-set duplicate/colliding labels; deliberately nonascending source positions; and strings containing JSON metacharacters, quotes, reverse solidus, controls, tabs, CR/LF, and Unicode. Require one top-level JSON array, one object per retained row in ascending logical-position order, and every object to emit the shared deduplicated keys in identical left-to-right column order without map iteration or key reordering. Assert exact deterministic whitespace/termination policy, valid JSON escaping, repeatable exact bytes, and parsed equivalence. Supply every Issue #49 warning combination and prove no warning object, property, key, wrapper, or other byte appears. Keep this task test-only and defer typed SQLite value policy to Task 3.

---

### 2. Implement deterministic row-object serialization

**Type**: GREEN
**Output**: Array/object/key/order tests pass without map-order nondeterminism.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the minimal deterministic JSON structural writer in `internal/export`, consuming immutable positioned rows and shared full-set deduplicated output names from `internal/result`. Emit a top-level array and construct each row object directly in column order rather than through an unordered map; traverse rows by ascending logical position without mutating the captured input. Use one explicit compact byte layout and standards-compliant JSON string escaping for keys and provisional string fields so repeated runs are byte-identical. Keep Issue #49 metadata outside the serializer input/data path and avoid dependencies on `internal/ui`, locale, or map ordering. Implement only enough to make Task 1 pass; typed SQLite value rendering belongs to Tasks 3-4.

---

### 3. Specify typed JSON values

**Type**: RED
**Output**: Failing tests cover raw INTEGER/finite REAL tokens, quoted non-finite tokens, JSON null, empty/TEXT strings, base64 BLOB, normalized invalid UTF-8, and exact output.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend `internal/export` exact-byte and parsed tests with every typed SQLite value from `internal/result`. Require INTEGER, including signed 64-bit boundaries, and every finite REAL to emit unquoted raw number tokens using the shared locale-independent formatting and REAL-identity rules, including integral values, negative zero, subnormals, exponents, and precision edges. Require pre-existing non-finite REALs to emit exact JSON strings `"Inf"`, `"-Inf"`, or `"NaN"`; SQL NULL to emit JSON `null`; empty and nonempty TEXT to remain distinct JSON strings with exact escaping; and empty/arbitrary BLOB bytes to emit standard base64 JSON strings without changing source bytes. Cover NUL and controls, valid multibyte Unicode, and multiple maximal invalid UTF-8 sequences already normalized by the shared policy to one U+FFFD each. Assert exact bytes and parsed types, identical finite REAL tokens to grid/CSV, and absence of warning properties for every metadata combination. Keep this task test-only and do not duplicate names, numeric formatting, base typed values, or UTF normalization.

---

### 4. Implement JSON value serialization

**Type**: GREEN
**Output**: Every SQLite type and invalid-UTF policy passes using shared result primitives.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement deterministic typed JSON value emission in `internal/export` over the closed SQLite value primitives and normalized TEXT from `internal/result`. Write INTEGER and finite REAL using the authoritative raw numeric tokens without passing them through a lossy generic number or map encoder; write non-finite values as exact quoted policy strings, SQL NULL as the JSON literal, TEXT as correctly escaped strings, and BLOB bytes as standard base64 strings. Preserve empty TEXT versus NULL, source BLOB identity, shared key order, ascending row order, and the compact exact-byte structure from Task 2. Do not inspect declared types, use locale-sensitive formatting, duplicate UTF normalization, or allow warning metadata into objects. Implement only enough to make Task 3 and all prior exact-byte/parsed tests pass.

---

### 5. Document JSON export

**Type**: DOCUMENT
**Output**: Wiki documentation records deterministic structure, keys, typed values, BLOB/non-finite handling, and warning exclusion.
**Depends on**: 4

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #51 implementation and tests from `internal/export` and `internal/result` into the appropriate pages under `Notes/wiki`. Document the top-level array-of-objects shape, ascending logical-position rows, shared full-set deduplicated keys in stable column order, direct ordered writing instead of map iteration, exact compact byte policy, and JSON string escaping. Record every typed representation: raw INTEGER/shared finite REAL tokens, quoted `Inf`/`-Inf`/`NaN`, JSON `null`, distinct empty/nonempty TEXT strings, standard base64 BLOB strings, and normalized invalid UTF-8 with one U+FFFD per maximal invalid sequence. Explain that snapshot metadata and Issue #49 warnings never add wrappers, objects, properties, or keys. Cross-reference Issues #14, #23, #33, #49, and #51 and the Numeric value parsing and rendering, Invalid UTF-8 TEXT, Export formats and values, Output names, Result export scope, Export Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` for any added or removed page and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 6. Create the JSON-export walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/051-06/code-walkthrough`.
**Depends on**: 5

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/051-06/code-walkthrough`. Serialize deterministic fixtures with duplicate/colliding labels, deliberately out-of-order positions, JSON metacharacters and controls, SQL NULL, empty TEXT, INTEGER boundaries, finite and non-finite REALs, empty/nonempty BLOBs, Unicode, and multiple invalid UTF-8 sequences. Capture exact bytes and parsed types for the top-level array, stable object/key order, ascending rows, raw numeric tokens, quoted non-finite tokens, `null`, escaped strings, standard base64, and replacement runes; repeat serialization to prove map-order independence. Add every Issue #49 warning combination and show identical output with no warning objects or properties. Reference Issue #51 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---

### 7. Review JSON output

**Type**: REVIEW
**Output**: Human verifies duplicate labels, all value types, invalid UTF, row order, and absent warning properties.
**Depends on**: 6

Review deterministic structure/value writing in `internal/export`, shared output names and typed result primitives in `internal/result`, wiki updates, and `Notes/walkthroughs/051-06/code-walkthrough` against Issue #51. Manually export empty, one-row, and multirow fixtures with duplicate labels, reversed traversal, every SQLite value type, numeric edges, non-finite REALs, empty and control-bearing TEXT, BLOBs, valid Unicode, and invalid UTF-8. Compare raw bytes across repeated runs and parse the result to confirm array/object shape, stable keys and order, ascending rows, exact raw-versus-quoted/null/string/base64 types, and normalization. Repeat warned exports and verify no warning wrapper, object, property, or key before approving the issue.

---
