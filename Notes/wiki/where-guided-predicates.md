# Guided WHERE Predicates and SQL NULL Semantics (Issue #17)

Issue #17 delivers the single optional WHERE predicate flow shared unchanged by the SELECT, UPDATE, and DELETE consumers, per the **Query Grammar**, **Builder and Display Interaction**, and **SQL safety** decisions of [Notes/PRD-sqloid.md](../PRD-sqloid.md). The guided sequence is column → fixed operator → conditional universal value entry, with explicit SQL-NULL guidance at value entry and verbatim LIKE wildcard binding.

The predicate state machine lives in `internal/querybuilder/predicate.go`; UI wiring lives in `internal/ui/where_popup.go` and `internal/ui/value_input.go`. Value classification is never duplicated in the UI — it always flows through Issue #14's universal parser ([sql-atoms-and-literals.md](sql-atoms-and-literals.md)).

## One optional predicate, structurally distinct states

`WherePredicate` is an immutable value with four structural stages (no booleans): `WhereAbsent`, `WhereColumnChosen`, `WhereAwaitingValue`, `WhereComplete`. Accessors (`State`, `Column`, `ChosenOperator`, `SubmittedValue`, `Entered`) expose everything callers need; a zero value behaves as absent. Transitions always return new values:

- `AbsentWhere()` starts empty.
- `SelectColumn(col)` accepts one schema-derived identity, rejecting nameless columns unchanged.
- `ChooseOperator(op)` rejects unknown operators via the closed token renderer, completes immediately on `IS NULL`/`IS NOT NULL` discarding any stale submitted value, or moves to awaiting-value.
- `SubmitValue(text)` parses through `ParseValue` and completes; submissions outside the awaiting-value stage are ignored.

Only a complete predicate renders: `SQL()` emits the safely quoted column atom plus the fixed operator token plus `?` exactly where a bound value belongs (entered text never appears in SQL); `Params()` returns nil for null operators and exactly one driver-typed parameter otherwise, in deterministic order.

## Builder-owned state for S/U/D consumers

`QueryBuilder` holds one committed completed predicate plus at most one open draft (`HasWhere`, `WherePredicate`, `WhereParams`, `WhereDrafting`, `WhereDraft`). Both clear together with all downstream state whenever the command replaces or the table vanishes. The draft is seeded from the commitment, so revising the same column restores its prior operator, exact entered text (`Entered()`), and bound type wholesale; choosing a different column restarts cleanly. `StartWhere(column)` validates the identity against the current visible columns of the selected object (hidden columns excluded, declared order preserved) and refuses while another draft is open or WHERE is unavailable (no table, or INSERT, which owns no WHERE). `ApplyWhereDraft`, `CancelWhereDraft`, and `CommitWhereDraft` complete the immutable edit cycle: cancel keeps any earlier completion untouched with no partial commits; commit requires structural completion and continued eligibility of the identity. `FixedOperators()` exposes the closed nine-operator set in deterministic presentation order (`= != < <= > >= IS NULL IS NOT NULL LIKE`) with no declared-type filtering; `WhereCandidates()` lists every visible column of the selected object.

## SQL NULL semantics

- Typed `NULL` (any case), empty input, whitespace-only input, and injection-shaped text bind as verbatim TEXT strings — never SQL null.
- SQL NULL intent uses only the explicit `IS NULL` / `IS NOT NULL` operators, which take no value and bind no parameter.
- Ordinary comparisons and LIKE do not match rows whose column holds actual SQL NULL; this SQLite behavior is preserved deliberately, not rewritten into null operators.
- LIKE values containing `%` or `_` bind byte-for-byte with no escaping or interpolation and no v1 escape mechanism, preserving SQLite wildcard semantics.

AND/OR, parentheses, IN, multiple predicates, and type-based operator filtering do not exist by design (excluded from v1 per PRD).

## UI flow

Enter on the field-bar **Where** entry (rendered for SELECT, UPDATE, and DELETE after table selection, trailing projection fields per Query Grammar order) opens a searchable single-select popup of eligible columns. Acceptance begins/revises the draft through `StartWhere` and opens the scroll-only fixed-operator popup listing all nine tokens with revision-time highlight restoration onto the previously chosen operator. Accepting a no-value operator commits immediately with focus back on Where — no value prompt ever opens; accepting a value-taking operator opens universal text entry seeded byte-for-byte with the restored entered representation when revising the same column. Enter submits the exact buffer through QueryBuilder's parser and commits; Esc cancels the entire open draft, restoring the prior completed predicate and exact opener focus without partial commits.

The value prompt renders the inline hint `'NULL' binds as literal TEXT — use IS NULL / IS NOT NULL for SQL NULL` plus contextual help explaining that ordinary comparisons/LIKE do not match actual NULL rows and that `%`/`_` keep their SQLite wildcard meaning. Rendering follows the Issue #8 overlay pattern (compose, never reflow); key precedence keeps overlays input-modal (popup or prompt consumes keys before base context). Focus ownership during staging stays on the Where field so accept/cancel restore it deterministically; NULL-operator completions return focus to the same builder bar as the guided flow's next step.

Cross-references: [searchable-popups.md](searchable-popups.md) (Issue #12 popup contract), [sql-atoms-and-literals.md](sql-atoms-and-literals.md) (Issue #14 parsing/binding/quoting), [projection-count-star.md](projection-count-star.md) (Column(s) seam consumed before WHERE), [builder-command-table.md](builder-command-table.md) (downstream clearing owns WHERE), and Issues #12/#14/#17.
