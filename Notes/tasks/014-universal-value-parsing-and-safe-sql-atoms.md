# Tasks for #14: Universal value parsing and safe SQL atoms

Parent issue: #14
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify deterministic value parsing and binding

**Type**: RED
**Output**: Failing table tests cover verbatim int64, finite decimal/hex float, overflow, signs, whitespace, empty text, typed `NULL`, and bound types.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Create UI-independent, table-driven tests in `internal/querybuilder/value_test.go` for the universal text parser and its parameter value. Follow the Numeric value parsing and rendering and SQL safety decisions in `Notes/PRD-sqloid.md`. Require verbatim INTEGER recognition only for `-?[0-9]+` values fitting signed int64, including leading zeros and the int64 boundaries; require every finite float64 accepted by `strconv.ParseFloat` after INTEGER classification to become REAL, including decimal, exponent, `1.`, `.5`, and valid hexadecimal floating-point forms such as `0x1p2`. Cover integer and float overflow, leading plus, negative and positive zero, whitespace in every position, empty input, typed `NULL`, `NaN`, `Inf`, hexadecimal integer `0x1A`, malformed hexadecimal float `0x1p`, and injection-looking text. Assert exact TEXT preservation for every fallback and exact Go/driver-facing bound type and value for INTEGER, REAL, and TEXT. Keep this task test-only, do not trim or normalize input, do not use declared column types, and do not introduce SQL NULL or BLOB input parsing.

---

### 2. Implement universal parsing and parameter values

**Type**: GREEN
**Output**: INTEGER/REAL/TEXT classification and binding tests pass exactly.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the pure universal value type, parser, and bound-parameter conversion in `internal/querybuilder/value.go`. Preserve the original text until classification is complete, recognize INTEGER before REAL, enforce signed-int64 and finite-float64 bounds, and fall back to exact TEXT for every nonmatching or nonfinite token. Ensure valid hexadecimal floating-point input produces its numeric REAL value while hexadecimal integers and malformed hexadecimal floats remain TEXT. Return database parameter values with stable concrete types appropriate to SQLite binding, while typed `NULL` and empty input remain strings rather than SQL null. Keep parsing independent of Schema declared types and `internal/ui`, bind rather than interpolate all user-entered values, and implement only enough to pass Task 1.

---

### 3. Specify identifier and fixed-token safety

**Type**: RED
**Output**: Failing tests cover embedded identifier quotes, schema-only identifiers, allowed operators/aggregates/directions, and injection-looking values.
**Depends on**: 2

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add table-driven safety tests in `internal/querybuilder/sql_atoms_test.go`, using typed object and column identities from `internal/schema` as the only source of identifiers. Require every table and column identifier to be double-quoted with each embedded double quote doubled, including empty-looking, punctuation-heavy, keyword, and schema-qualified-looking names; require the helper to quote one identifier atom rather than accepting user-authored SQL or schema qualification. Define exhaustive fixed choices for the v1 predicate operators, projection aggregates, and ordering directions and prove that only those typed choices render their exact SQL tokens. Include injection-looking user values and assert they remain parameter bindings with unchanged parsed types and never appear in SQL text. Include attempted operator, aggregate, direction, and identifier text outside the typed/schema-derived APIs and require it to be unrepresentable or rejected. Keep this task test-only and do not test standalone literal rendering yet.

---

### 4. Implement safe identifiers and fixed SQL tokens

**Type**: GREEN
**Output**: Identifier quoting and fixed-choice SQL atom tests pass with all user values still bound.
**Depends on**: 3

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement identifier quoting and typed fixed-choice SQL atoms in `internal/querybuilder/sql_atoms.go`, with any narrowly required shared types added to `internal/querybuilder/builder.go`. Accept identifiers only through `internal/schema` object/column identity at query-construction boundaries, quote each identifier atom with SQL-standard double-quote doubling, and do not parse or preserve caller-provided schema qualification. Represent operators, aggregates, and directions as closed typed choices whose renderers emit only the PRD-approved tokens; reject invalid zero or unknown values instead of passing through arbitrary text. Keep every user-entered value on the parameter list through the Task 2 bound-value contract, preserve deterministic parameter ordering for future query builders, avoid imports from `internal/ui`, and implement only enough to make Task 3 pass.

---

### 5. Specify canonical standalone literals

**Type**: RED
**Output**: Failing exact-token tests cover INTEGER, finite REAL identity, quote-doubled TEXT, NULL, and `X'hex'` BLOB.
**Depends on**: 4

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Create exact, table-driven renderer tests in `internal/querybuilder/sql_literal_test.go` for one UI-independent typed-value-to-standalone-SQL-literal contract. Require INTEGER to render in exact canonical decimal form at signed-int64 boundaries; require every finite REAL to use the PRD's locale-independent shortest round-trip token and append `.0` when the token contains none of `.`, `e`, or `E`, preserving REAL identity for integral values, negative zero, subnormal values, and exponent output. Require TEXT to be enclosed in single quotes with every embedded single quote doubled, preserving empty strings, whitespace, NUL bytes, and injection-looking content. Require typed SQL NULL to emit exactly `NULL` and typed BLOB bytes to emit uppercase `X` with lowercase hexadecimal payload in `X'hex'` form, including empty and zero/high-byte data. Reject nonfinite REAL values according to the typed renderer contract. Keep this task test-only; BLOB remains unsupported as universal user input, and the renderer must not depend on model or modal state.

---

### 6. Implement the shared SQL literal renderer

**Type**: GREEN
**Output**: One UI-independent renderer passes exact tests and is available to Issues 40 and 48.
**Depends on**: 5

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the single canonical typed literal renderer in `internal/querybuilder/sql_literal.go`, reusing the typed value definitions from `internal/querybuilder/value.go` without duplicating parsing. Render exact decimal INTEGER, PRD-formatted finite REAL with preserved REAL identity, quote-doubled TEXT, SQL NULL, and lowercase-hex BLOB with the exact required token forms; return a typed error for unsupported or nonfinite values rather than producing unsafe SQL. Keep the API UI-independent and suitable for both destructive modal SQL and saved standalone SQL, while ordinary query execution continues to use bindings rather than rendered literals. Do not add private renderers in `internal/ui` or couple this primitive to Issues #40 or #48; expose one ownership point and implement only enough to make Task 5 pass.

---

### 7. Document parsing and SQL atom contracts

**Type**: DOCUMENT
**Output**: Wiki documentation records parsing grammar, binding, quoting, and canonical literal ownership.
**Depends on**: 6

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #14 implementation and tests from `internal/querybuilder` and its `internal/schema` identity boundary into the appropriate pages under `Notes/wiki`. Document the verbatim INTEGER-first then finite-REAL parsing grammar, every important fallback to exact TEXT, typed `NULL` and empty-text behavior, concrete binding types, the absence of declared-type coercion in the builder, schema-only atom-by-atom identifier quoting, closed operator/aggregate/direction choices, and the rule that all user values remain bound during execution. Record the exact standalone INTEGER, REAL, TEXT, NULL, and BLOB forms, nonfinite rejection, BLOB input exclusion, and `internal/querybuilder/sql_literal.go` as the sole serializer owned for later Issues #40 and #48. Cross-reference Issue #14 and the Query Grammar, Numeric value parsing and rendering, SQL safety, Query save targeting, and Testing Decisions of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 8. Create the SQL-atoms walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/014-08/code-walkthrough`.
**Depends on**: 7

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/014-08/code-walkthrough`. Demonstrate table-driven evidence for signed-int64 boundaries, decimal and hexadecimal finite REALs, malformed and overflow fallbacks, signs, leading zeros, whitespace, empty text, typed `NULL`, and injection-looking input with exact bound types. Show schema-derived identifier quoting with embedded quotes, exhaustive fixed operator/aggregate/direction tokens, and the absence of user values in executable SQL text. Demonstrate exact canonical literals for INTEGER, REAL identity including integral and negative-zero values, quote-doubled TEXT, NULL, and empty/nonempty BLOB, identifying the shared renderer as the future dependency for Issues #40 and #48. Reference Issue #14 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
