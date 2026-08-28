# Issue #17: Guided WHERE predicates and SQL NULL semantics

*2026-08-27T10:44:54Z by Showboat 0.6.1*
<!-- showboat-id: bbe879f0-8c2a-47e4-a2e6-06760f3f55c3 -->

Issue #17 delivers the single optional WHERE predicate flow shared unchanged by SELECT, UPDATE, and DELETE, per the Query Grammar, Builder and Display Interaction, and SQL safety decisions of Notes/PRD-sqloid.md. Guided sequence: column → fixed operator → conditional universal value entry, with explicit SQL-NULL guidance at value entry and verbatim LIKE wildcard binding.

The state machine lives in internal/querybuilder/predicate.go (pure), with UI wiring in internal/ui/where_popup.go and universal text entry in internal/ui/value_input.go. Every artifact in this walkthrough lives under this approved directory: _demo17/main.go is the runnable demonstration program.

## 1. Pure predicate tests: every operator, every column, exact binding types

The table-driven suite walks each consumer's eligible columns (visible, declared order) across all nine operators, asserting incomplete-until-submission and exactly one '?' plus the parsed bound value at its concrete Go type — including typed NULL, empty input as TEXT, and LIKE wildcards byte-for-byte. Null operators complete immediately with no placeholder and no parameter; defensive rejections guard ineligible identities and invalid operators.

```bash
go test ./internal/querybuilder -count=1 -v -run 'TestWhereCandidates|TestFixedOperators|TestNullOperatorsComplete|TestEveryOperatorOnEveryColumn|TestTypedNullCompletes|TestLikeWildcardsBound|TestPredicateStates|TestPredicateTransitionsAreImmutable|TestWhereIdentifiersAreSafelyQuoted|TestWhereContractConsumedUnchangedByAllConsumers' 2>&1 | grep -E '^(=== RUN|--- (PASS|FAIL)|PASS|FAIL)' | sed -E 's/ \([0-9.]+s\)//; /^=== RUN/d'
```

```output
--- PASS: TestWhereCandidatesDeriveFromSchemaPerCommand
--- PASS: TestFixedOperatorsClosedAndDeterministicOrder
--- PASS: TestNullOperatorsCompleteWithoutValueOrParameter
--- PASS: TestEveryOperatorOnEveryColumnViaBuilder
--- PASS: TestTypedNullCompletesThroughBuilder
--- PASS: TestLikeWildcardsBoundVerbatimAbsentFromSQL
--- PASS: TestPredicateStatesStructurallyDistinct
--- PASS: TestPredicateTransitionsAreImmutable
--- PASS: TestWhereIdentifiersAreSafelyQuoted
--- PASS: TestWhereContractConsumedUnchangedByAllConsumers
PASS
```

## 2. Runnable demonstration of the reusable predicate state

_demo17/main.go builds predicates through the real transitions (StartWhere → ChooseOperator → ApplyWhereDraft/SubmitValue → CommitWhereDraft) and prints exact SQL and parameter evidence. Note section 2: only IS NULL / IS NOT NULL bypass value entry and produce zero bound parameters.

```bash
go run ./Notes/walkthroughs/017-06/code-walkthrough/_demo17
```

```output
== 1. eligible columns + closed operator set (no type filtering) ==
SELECT columns=[id email weird""col]
              operators=[= != < <= > >= IS NULL IS NOT NULL LIKE]
UPDATE columns=[id email weird""col]
              operators=[= != < <= > >= IS NULL IS NOT NULL LIKE]
DELETE columns=[id email weird""col]
              operators=[= != < <= > >= IS NULL IS NOT NULL LIKE]

== 2. null operators: complete immediately, no placeholder, no parameter ==
committed SQL="\"email\" IS NULL"  params=[]
committed SQL="\"email\" IS NOT NULL"  params=[]

== 3. typed NULL, empty input, LIKE wildcards: exact bound types ==
SQL="\"id\" = ?"  param="NULL" (string)
SQL="\"id\" >= ?"  param="" (string)
SQL="\"weird\"\"\"\"col\" LIKE ?"  param="%a_b%" (string)   [wildcard text absent from SQL: true]

== 4. value operators stay incomplete until submission ==
after choosing '=': State=awaiting-value  SQL=""
after submitting '-7': State=complete  SQL="\"weird\"\"\"\"col\" = ?"  param=-7 (int64)

== 5. same-column revision restores; Esc-style cancel preserves ==
revisit draft input="tricky'x_50%" restored=true
cancel → HasWhere=true SQL="\"email\" = ?" params=[]interface {}{"tricky'x_50%"}

== 6. identical rendering/parameter contract across consumers ==
SELECT  SQL="\"email\" != ?"  param=1 42 (int64)
UPDATE  SQL="\"email\" != ?"  param=1 42 (int64)
DELETE  SQL="\"email\" != ?"  param=1 42 (int64)
```

## 3. Guided UI flow: column → operator → conditional value entry

The scripted Bubble Tea tests drive Model.Update end-to-end. Enter on the focused Where field opens the searchable eligible-column popup (Issue #12 contract: search reset, highlight on first candidate); acceptance opens the scroll-only fixed-operator popup; IS NULL / IS NOT NULL commit immediately with focus back on Where and no value prompt; value-taking operators open universal entry (Issue #14 seam) seeded with the exact prior text when revising the same column; Esc at every stage restores the prior completed predicate (-5.25 REAL) with exact opener focus and no partial commits.

```bash
go test ./internal/ui -count=1 -v -run 'TestWhereColumnPopupSearchableForEveryConsumer|TestNullOperatorsCompleteWithoutValuePrompt|TestValueOperatorsOpenUniversalEntryUntilSubmission|TestTypedNullAndEmptyBindAsText|TestLikeWildcardsBoundVerbatimThroughFlow|TestEscRestoresPriorCompletionWithoutPartialCommit|TestRevisitRestoresExactPriorStateOnSameColumn' 2>&1 | grep -E '^(--- (PASS|FAIL)|PASS|FAIL)' | sed -E 's/ \([0-9.]+s\)//'
```

```output
--- PASS: TestWhereColumnPopupSearchableForEveryConsumer
--- PASS: TestNullOperatorsCompleteWithoutValuePrompt
--- PASS: TestValueOperatorsOpenUniversalEntryUntilSubmission
--- PASS: TestTypedNullAndEmptyBindAsText
--- PASS: TestLikeWildcardsBoundVerbatimThroughFlow
--- PASS: TestEscRestoresPriorCompletionWithoutPartialCommit
--- PASS: TestRevisitRestoresExactPriorStateOnSameColumn
PASS
```

## 4. Inline SQL-NULL guidance and view invariants

The open value prompt renders the exact inline hint ('NULL' binds as literal TEXT — use IS NULL / IS NOT NULL for SQL NULL) plus contextual help: ordinary comparisons and LIKE do not match rows whose column holds actual SQL NULL, and '%'/'_' keep their SQLite wildcard meaning inside LIKE values (no v1 escape mechanism). The prompt overlay composes over the shell without reflowing any region (identical line count and widths), and Tab cannot move builder focus while the popup or the value prompt owns the context.

```bash
go test ./internal/ui -count=1 -v -run 'TestWhereFieldAbsentUntilCompleted|TestValuePromptShowsHintAndContextualGuidance|TestValuePromptNeverReflowsRegions|TestPopupOverlayPrecedenceOverWhereFlow' 2>&1 | grep -E '^(--- (PASS|FAIL)|PASS|FAIL)' | sed -E 's/ \([0-9.]+s\)//'
```

```output
--- PASS: TestWhereFieldAbsentUntilCompleted
--- PASS: TestValuePromptShowsHintAndContextualGuidance
--- PASS: TestValuePromptNeverReflowsRegions
--- PASS: TestPopupOverlayPrecedenceOverWhereFlow
PASS
```

Closing: full-suite verification of the touched packages.

Issue #17 · parents Issue #12 (searchable popup contract) and Issue #14 (universal value parsing + safe SQL atoms) · contract pages in Notes/wiki/where-guided-predicates.md, seeded from Notes/PRD-sqloid.md. Excluded by design in v1: multiple predicates, AND/OR, IN, type-based operator filtering, write-specific assignment and execution flows.

```bash
go test ./internal/querybuilder ./internal/ui -count=1 2>&1 | sed -E 's/[[:space:]]+[0-9]+\.[0-9]+s$//'
```

```output
ok  	github.com/chris/sqloid/internal/querybuilder
ok  	github.com/chris/sqloid/internal/ui
```
