# Tasks for #83: Add a Revalidation payload invariant

Parent issue: #83
Parent PRD: PRD-sqloid.md

## Tasks

### 1. Specify the Revalidation validity matrix

**Type**: RED  
**Output**: Failing table-driven schema tests define every valid and invalid `Revalidation` status/payload combination and require all production revalidation outcomes to satisfy the invariant.  
**Depends on**: none

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Begin only after Issue #82 has landed and finalized malformed-attempt settlement. Extend `internal/schema/revalidate_test.go` with a complete truth table for a new `Revalidation.Valid()` method, following the structure and payload-discipline coverage of `Attempt.Valid()` and its tests in `internal/schema/refresh.go` and `refresh_test.go`. Require unchanged and refreshed statuses to accept exactly a non-nil catalog with nil cause; refresh failed to accept exactly a non-nil cause with nil catalog; deleted and replaced to accept only nil catalog and nil cause; and zero or unknown statuses to return false. Include every missing-required-field and forbidden-extra-field combination, then exercise unchanged, changed-success, ordinary-failure, terminal, and Issue #82 malformed-attempt outputs from `Revalidate` and require each actual result to be valid. Keep this task test-only and do not alter the settled status mapping.

---

### 2. Implement the Revalidation invariant guard

**Type**: GREEN  
**Output**: `Revalidation.Valid()` enforces the exact settled payload contract, and all schema revalidation results pass the truth table.  
**Depends on**: 1

Before changing code, read and follow the coding standards in `Notes/skills/AGENTS.md`.

Add `Revalidation.Valid()` to `internal/schema/revalidate.go` beside the `Revalidation` type, mirroring the explicit status switch and exact field checks used by `Attempt.Valid()` in `internal/schema/refresh.go`. Encode the complete matrix from Task 1 without accepting zero or unknown statuses, required-field omissions, or extra contradictory payloads. Update nearby field and method documentation only as needed to make the invariant authoritative, but do not change constructors, status values, `Revalidate` mapping, catalog identity, cause preservation, or UI behavior. Run focused schema tests and repository-wide Go tests, including Issue #82's malformed-attempt cases, and implement only enough to satisfy the invariant suite.

---

### 3. Document the Revalidation payload invariant

**Type**: DOCUMENT  
**Output**: The wiki records the exact `Revalidation.Valid()` status/payload matrix and its relationship to Attempt settlement and schema consumers.  
**Depends on**: 2

Read and follow `Notes/wiki/wiki-rules.md` and the schema in `Notes/wiki/AGENTS.md`, then ingest the completed Issue #83 method and truth-table tests from `internal/schema/revalidate.go` and `revalidate_test.go` into the relevant pages under `Notes/wiki`. Document every accepted status/payload combination, every rejected missing or extra field category, and the rejection of zero and unknown statuses. Explain how Issue #82 ensures malformed attempts are first converted into a valid refresh-failed value, and how the invariant mirrors `Attempt.Valid()` without changing runtime consumer semantics. Cross-reference Issues #13, #21, #82, and #83 and the Schema scope, cache, and validation and Testing Decisions sections of `Notes/PRD-sqloid.md`; update `Notes/wiki/index.md` as required and append the dated ingest to `Notes/wiki/log.md` without modifying prior entries.

---

### 4. Create the Revalidation invariant walkthrough

**Type**: CODE WALKTHROUGH  
**Output**: Showboat walkthrough exists at `Notes/walkthroughs/083-04/code-walkthrough`.  
**Depends on**: 3

Use showboat, consulting `uvx showboat --help`, to create the walkthrough at exactly `Notes/walkthroughs/083-04/code-walkthrough`, with the main file named `walkthrough.md`. Walk the complete `Revalidation.Valid()` truth table: valid unchanged/refreshed catalog payloads, valid refresh-failed cause payload, payload-free terminal states, and representative missing, extra, zero, and unknown invalid cases. Run the production-output assertions to show every `Revalidate` path, including Issue #82's malformed-attempt fallback, now produces a value accepted by the invariant. Reference Issues #82-#83 and `Notes/PRD-sqloid.md`, and place all showboat-generated files under the approved directory.

---
