# Tasks for #85: Centralize finite REAL token generation

Parent issue: #85
Parent PRD: PRD-sqloid.md
**Blocked by issues**: none
**Acceptance criteria**: AC1–AC4 → Tasks 1–2
**Manual verification**: Task 4 owns the issue's manual checks; shipped-TUI evidence begins after Issue #57 Phase A lands.

## Tasks

### 1. Lock finite REAL token behavior across consumers

**Type**: REFACTOR
**Output**: Passing cross-package tests lock identical finite REAL tokens and preserved non-finite policies before implementation deduplication.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend `internal/querybuilder/sql_literal_test.go` with cross-package cases comparing finite REAL output from `RenderSQLLiteral` to `result.RealToken` for integral REAL `1.0`, negative zero, `1e+20`, the smallest subnormal, maximum finite float, and adjacent/precision-edge float64 values. Preserve exact round-trip and locale-independent expectations and keep explicit rejection of positive infinity, negative infinity, and NaN by query-literal serialization while result formatting retains its existing non-finite policy. Run these external-contract tests before production edits and record that they are green. Do not add an architecture/source-contract unit test; the single-implementation requirement is verified by a supplemental focused source/static check in Task 2.

---

### 2. Delegate finite SQL literals to the canonical token

**Type**: REFACTOR
**Output**: Query-literal serialization uses the shared canonical finite REAL token, with all grid, CSV, JSON, saved-SQL, and non-finite contracts passing unchanged.
**Verification obligation**: Task 1's cross-package behavioral tests pass unchanged; no consumer contract changes.
**Supplemental checks**: Focused grep/static evidence shows one formatting-plus-suffix implementation; repository build and test pass.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Remove the duplicate finite formatter from `internal/querybuilder/value.go` and make the finite REAL branch used by `RenderSQLLiteral` delegate to `internal/result.RealToken`, or to an equally shared lower-level helper only if dependency direction requires it. Preserve querybuilder's typed non-finite rejection before calling the shared result policy, because `result.RealToken` intentionally returns display/export tokens for non-finite database values. Clean up imports and comments to identify a single authoritative finite implementation; do not alter parsing, bound parameter types, text quoting, INTEGER/NULL/BLOB literals, result display, CSV/JSON serializers, or saved-SQL assembly. Run the unchanged focused `internal/querybuilder`, `internal/result`, and `internal/export` behavioral tests and repository-wide Go tests, then use a focused grep/static build check as supplemental evidence that the formatting-plus-suffix algorithm has one implementation; do not encode private implementation shape in a unit test. Package/seam verification may complete now, but record that shipped-TUI manual and end-to-end comparison must be rerun after Issue #57 lands.

---

### 3. Document canonical finite REAL tokens

**Type**: DOCUMENT  
**Output**: The wiki identifies one finite REAL token authority, all consumers, preserved non-finite separation, and Issue #57 sequencing for end-to-end evidence.  
**Depends on**: 2

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest Issue #85's implementation and tests from `internal/result/result.go`, `internal/querybuilder/value.go`, `sql_literal.go`, the affected querybuilder/result tests, and relevant `internal/export` verification into the appropriate pages under `Notes/wiki`. Document the shortest-round-trip plus REAL-identity suffix rule, the representative integral, negative-zero, exponent, subnormal, and precision-edge cases, and the single shared call path used by grid, CSV, JSON, and query/saved-SQL literals. Keep the distinct non-finite rules explicit: query literals reject them while result formats retain their established tokens and quoting. State that package-level evidence is valid before Issue #57 but shipped-TUI manual/end-to-end checks must be rerun after Issue #57. Cross-reference Issues #14, #47, #50-#51, #57, and #85 and the Numeric value parsing and rendering and Export formats and values sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` and append the dated ingest to `Notes/wiki/log.md`.

---

### 4. Create the finite REAL token walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/085-04/code-walkthrough`.  
**Depends on**: 3

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/085-04/code-walkthrough`, with the main file named `walkthrough.md`. Trace representative finite values through the canonical `result.RealToken` implementation and query-literal serialization, then compare exact grid, CSV, JSON, and saved-SQL tokens for `1.0`, `-0.0`, `1e+20`, a subnormal, and precision-edge values. Demonstrate that query literals still reject Inf, -Inf, and NaN while result formatting remains unchanged, run the behavioral parity tests and show supplemental focused source/static evidence that no duplicate FormatFloat-plus-suffix implementation remains, and include passing focused verification. Clearly mark shipped-binary/TUI evidence as dependent on rerunning after Issue #57. Reference Issue #85 and `Notes/PRD-sqloid.md`, and place every generated artifact under the approved directory.

---
