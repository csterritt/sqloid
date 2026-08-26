# Tasks for #23: Non-finite REAL grid rendering

Parent issue: #23
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify non-finite REAL display tokens

**Type**: RED
**Output**: Failing shared-rendering/grid tests distinguish REAL `Inf`, `-Inf`, and `NaN` from finite REAL and identical-looking TEXT.
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Extend the UI-independent typed-value tests in `internal/result` and the focused result-grid model/view tests in `internal/ui` introduced by Issue #22. Construct REAL values for positive infinity, negative infinity, and representative NaN payloads through the result seam and require exact grid display tokens `Inf`, `-Inf`, and `NaN` respectively. In the same tables and rows, include finite REAL values, INTEGER values, and TEXT containing the literal strings `Inf`, `-Inf`, and `NaN`; assert that each retains its original SQLite/result type and follows its own rendering policy even when visible glyphs match. Require finite REALs to continue using Issue #22's shortest-round-trip REAL-preserving token, including `.0` where required, and ensure non-finite values do not flow through finite formatting or invalid-UTF handling. Cover signed infinities, multiple NaN bit patterns without exposing payload-specific text, frozen-grid rendering, and retained row values. Keep this task test-only, do not alter CSV or JSON serialization, and do not duplicate token selection inside `internal/ui`.

---

### 2. Implement non-finite REAL grid rendering

**Type**: GREEN
**Output**: Exact textual-token tests pass without changing the underlying SQLite value type.
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Implement the minimal non-finite REAL display policy in the shared typed-value rendering seam under `internal/result`, and have the existing `internal/ui` grid continue to consume that seam without a private branch. Detect positive infinity, negative infinity, and any NaN only after the value is known to be SQLite REAL, returning the exact display token required by Issue #23 while retaining the original float64-backed REAL value and type in rows, snapshots, and metadata. Preserve Issue #22's finite REAL formatting, TEXT identity, invalid-UTF replacement, control-character rendering, and BLOB behavior unchanged. Keep the change display-only and grid-scoped at the public contract level: do not normalize stored values, reinterpret identical-looking TEXT, add warning metadata, or preempt the separate CSV and JSON policies in later exporter issues.

---

### 3. Document non-finite grid policy

**Type**: DOCUMENT
**Output**: Wiki documentation records grid tokens and separation from finite, TEXT, CSV, and JSON policies.
**Depends on**: 2

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #23 implementation and tests from `internal/result` and `internal/ui` into the appropriate pages under `Notes/wiki`. Document exact grid tokens for positive infinity, negative infinity, and NaN; preservation of the underlying SQLite REAL type; treatment of every NaN payload as the one `NaN` display token; and the distinction from finite REAL token generation and identical-looking TEXT values. Explicitly separate this grid-only policy from CSV's later textual form and JSON's later quoted-string form, noting that shared typed representation permits format-specific rendering without value coercion. Cross-reference Issues #22 and #23 and the Numeric value parsing and rendering, Grid rendering/cache, Export formats and values, Module Design, and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` for all new or materially changed pages and append the required dated ingest entry to `Notes/wiki/log.md` without rewriting prior entries.

---

### 4. Create the non-finite-REAL walkthrough

**Type**: CODE WALKTHROUGH
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/023-04/code-walkthrough`.
**Depends on**: 3

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/023-04/code-walkthrough`. Demonstrate a controlled typed-result fixture containing REAL positive infinity, REAL negative infinity, multiple REAL NaN representations, representative finite REALs, and TEXT values containing the same visible strings. Show the production grid rendering exact `Inf`, `-Inf`, and `NaN` tokens, finite values continuing to use their Issue #22 tokens, and test evidence that underlying REAL versus TEXT identities remain distinct despite identical-looking cells. Include package-level evidence that token selection lives in the shared `internal/result` seam and that no CSV/JSON policy was changed. Reference Issue #23 and `Notes/PRD-sqloid.md`, and place every showboat-generated artifact under the approved directory.

---
