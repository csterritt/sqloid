# GROUP BY, ORDER BY, and LIMIT (Issue #18)

Issue #18 completes the SELECT grammar: assisted GROUP BY multi-selection over the complete grouping validity matrix, context-valid single-expression ORDER BY with a closed ASC/DESC direction, and a bounded LIMIT with one exact invalid reason. QueryBuilder owns every rule — candidate eligibility, validity, and SQL rendering — while `internal/ui` owns focus, acceptance, cancellation, direction toggling, and prompt plumbing. See the Query Grammar, Runnable-State Contract, Builder and Display Interaction, QueryBuilder, UI, and Testing Decisions sections of `Notes/PRD-sqloid.md`, and the projection foundations from Issues #15 and #16 ([projection-count-star.md](projection-count-star.md), [projection-ordered-editing.md](projection-ordered-editing.md)).

## GROUP BY: assisted multi-selection and the grouping matrix

`internal/querybuilder/group_by.go` keeps committed GROUP BY columns as immutable ordered state:

- `GroupByCandidates()` derives choices from the currently eligible table's visible columns in Schema order, excluding already-committed names so the multi-selection can never offer a duplicate.
- `AcceptGroupColumn(name)` appends one visible column name preserving acceptance order — deliberately not Schema order, because the user's selection order is significant for rendering and history comparison. An unknown, hidden, empty, or exactly duplicated identity is rejected as an immutable no-op reporting `false`; a stale or foreign identity can never become SQL.
- `GroupByEntries()` returns the committed names in selection order; `RemoveLatestGroup()` deletes the most recently accepted column (base-field Backspace seam, mirroring `RemoveLatestProjection`).
- Command replacement and table-loss refresh clear the groups wholesale, like all downstream state.

The validity matrix is enforced through `FirstInvalidIssue()` (field/reason pairs, checked in fixed order: GROUP BY, then ORDER BY, then LIMIT):

| Projection | Groups | Validity |
| --- | --- | --- |
| Empty / nonaggregate-only | none | valid |
| Mixed aggregate/nonaggregate | none | invalid — `every nonaggregate projected column must be grouped` |
| Mixed | every nonaggregate grouped | valid |
| Mixed | one or more required groups missing | invalid — same reason |
| Any | extra grouped columns | permitted |
| All-aggregate (incl. bare `COUNT(*)`) | none | valid |
| All-aggregate | any | valid |
| Wildcard | any GROUP BY | invalid — `the wildcard cannot be used together with GROUP BY` |
| Any | a group naming a column that vanished after refresh | invalid — `a grouped column no longer exists` |

Grouping never reorders the projection, never introduces parameters, and renders `GROUP BY "a", "b"` in commit order with single-atom identifier quoting after WHERE and before ORDER BY.

## ORDER BY: context-valid candidates, ASC default, one expression

`internal/querybuilder/order_by.go`:

- `OrderByCandidates()` derives the deterministic choices. Ordinary ungrouped SELECTs (no aggregates, no groups) offer every visible table column in Schema order. Aggregate or grouped queries offer exactly the committed GROUP BY columns (commit order), then selected aggregate expressions in projection order, then bare `COUNT(*)` when selected. Never offered: the wildcard, an ungrouped nonaggregate column, an aggregate absent from the projection, a stale Schema identity, or arbitrary text.
- Each `OrderByCandidate` carries a stable reserved-prefix `Key` separate from its `Display` label, so equal labels (a column beside its own aggregate, or a literal column named `COUNT(id)`) keep distinct identities.
- `AcceptOrderBy(key)` commits the sole expression, replacing any existing selection atomically and resetting the direction to the ASC default; unknown or stale keys are rejected unchanged. `SetOrderDirection`/`ToggleOrderDirection` flip within the closed ASC/DESC pair; `ClearOrderBy` removes the whole selection; `OrderBySelection()` resolves the committed identity against the current candidates and reports staleness otherwise (`the ordered expression is no longer offered` as a first-invalid issue).
- SQL renders one quoted/fixed-token clause — `ORDER BY "email" ASC` or `ORDER BY COUNT(*) DESC` — between GROUP BY and LIMIT, with no parameters.

## LIMIT: bounded parsing with one exact reason

`internal/querybuilder/limit.go` keeps the entered representation byte-for-byte (`LimitInput()`) beside its optional accepted integer (`LimitValue()`):

- Only empty input means the unbounded logical result: no accepted value, no invalid report, no LIMIT clause.
- Only nonempty base-10 integer text whose value lies in `[1, 9223372036854775807]` is accepted and rendered canonically (`LIMIT 7` even if entered as `07`). Leading zeros are tolerated at entry.
- Everything else — zero, negatives, a leading `+`, whitespace, decimal/exponent/hex forms, nonnumeric text, signed-int64 overflow, arbitrarily long input — classifies invalid with exactly one user-facing reason: `Limit must be an integer from 1 to 9223372036854775807`, reported as field `Limit` through `FirstInvalidIssue`. The entered bytes are always preserved verbatim for correction and history comparison, and invalid input is never bound or interpolated. Universal-value REAL/TEXT coercion is not reused: LIMIT has its own closed grammar.

## UI wiring

`internal/ui` adds three SELECT field-bar entries in Query Grammar order after Column(s) and Where: `Group By`, `Order By`, and `Limit` (`internal/ui/command_table.go`), with new `FieldGroupBy`/`FieldOrderBy`/`FieldLimit` identities in `internal/querybuilder/command_table.go`.

- `internal/ui/group_by_popup.go`: Enter on the focused Group By field opens a searchable popup from `GroupByCandidates()`; each acceptance commits through `AcceptGroupColumn` and reopens the popup fresh (search cleared, highlight reset) with remaining candidates — assisted multi-selection column by column, duplicate/stale acceptances reopening as immutable no-ops. Esc restores exact opener focus. Backspace/Delete on the base field removes the latest accepted group.
- `internal/ui/order_by_popup.go`: Enter opens a searchable popup from `OrderByCandidates()` (candidate keys as identities); acceptance commits atomically and restores focus to Order By; Esc cancels. Up/Down in the focused base field with a committed selection toggle ASC/DESC deterministically without opening a popup or moving focus (uncommitted Up/Down keep ordinary navigation); Backspace/Delete clears the whole value.
- `internal/ui/limit_field.go`: Enter opens the Issue #14 `ValuePrompt` seeded byte-for-byte with the currently entered text; Enter submits the verbatim buffer through `SetLimitInput` (empty input commits as unbounded; invalid input keeps the text and the field bar shows the builder's reason verbatim, `entered — Limit must be an integer from 1 to 9223372036854775807`); Esc discards the draft leaving the prior value untouched; Backspace/Delete clears the whole value. No second UI validator exists.
- `internal/ui/model.go` routes the new base-field Backspace/Up/Down behavior below popup and value-prompt modality, and `handleValuePromptKey` dispatches Enter/Esc to the Limit flow by opener label.

Tests: pure builder coverage in `internal/querybuilder/group_by_test.go`, `order_by_test.go`, and (Task 5's table-driven matrix, landing with Task 6) `limit_test.go`, asserting exact first-invalid field/reason pairs, candidate order, safely quoted SQL, and projection-order/parameter preservation; scripted model coverage in `internal/ui/group_by_test.go`, `order_by_test.go`, and `limit_test.go` per the reusable Issue #12 popup contracts ([searchable-popups.md](searchable-popups.md)). The `InvalidIssue` first-invalid contract lives in `internal/querybuilder/validation.go` (`FieldIdentityGroupBy`/`OrderBy`/`Limit`), consumed by the runnable-state seam described in [where-guided-predicates.md](where-guided-predicates.md).
