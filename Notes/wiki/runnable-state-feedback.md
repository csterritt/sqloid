# Issue #19: Authoritative runnable-state feedback

The authoritative, UI-independent runnable report, whole-value clearing, and base-context Enter gating, per the **Runnable-State Contract** and **Builder and Display Interaction** decisions in `Notes/PRD-sqloid.md`.

## The runnable report (`internal/querybuilder/runnable.go`)

`QueryBuilder.RunnableReport()` returns one pure `RunnableReport{Runnable bool, Field RunField, Reason string}` — either runnable data, or the first invalid **typed field** plus one **specific reason**, ordered by each command's **visual builder order** rather than validation implementation order. It reuses the Issue #18 grouping/ORDER BY/LIMIT validators and Schema identity contracts instead of duplicating them, carries no UI action, and never starts validation, estimation, execution, or history append. Test-asserted reason constants include `select a command`, `select a table`, `the selected table no longer exists`, `select at least one column`, `the projected column no longer exists`, `complete the open value prompt`, `the where column no longer exists`, `add at least one SET assignment`, `SET columns must be unique`, `the SET column no longer exists`, `table has no insertable columns`, `complete the choice for column %s`, and `submit a value for column %s`; LIMIT failures reuse exactly `Limit must be an integer from 1 to 9223372036854775807`.

`RunField` identities map onto visual fields: `Command`, `Table`, `Column(s)`, `SET`, `INSERT`, `WHERE`, `GROUP BY`, `ORDER BY`, `Limit`.

### Common gates

- **Selected command**: an unselected command blocks at `Command`.
- **Refreshed identifiers**: the selected table must still exist in the latest eligible catalog; a committed named projection (Value or aggregate) naming a vanished column blocks at `Column(s)` with `the projected column no longer exists` (Issue #65); a committed WHERE naming a vanished column blocks at `WHERE`; stale groups/orders report through the Issue #18 reasons. The synthetic wildcard and `COUNT(*)` sentinel identities are exempt by `ProjectionKind`, never by display text, so a real column literally named `*` or `COUNT(*)` is still validated as a named identifier.
- **No incomplete value prompt**: an open WHERE draft or any structurally incomplete predicate blocks at `WHERE`; pending write choices and unsubmitted Value entries block at their write fields (see below).

### Command prerequisites in visual order

- **SELECT**: nonempty projection → every committed named projection entry validated against refreshed visible columns (Issue #65) → complete/present WHERE → grouping matrix → valid grouped ORDER BY → empty (unbounded) or valid Limit.
- **UPDATE**: at least one SET assignment → unique SET columns → every assignment's complete {Value, NULL} choice with submitted Value entries → optional complete WHERE.
- **DELETE**: eligible table only; the optional WHERE must not be incomplete.
- **INSERT**: at least one insertable column (zero blocks with `table has no insertable columns`) → every per-column prompt carrying exactly one {Value, NULL, Default/Omit} choice with submitted Value entries; **all-omit is valid**.

Universal-value nuances are structural: submitted empty text completes as empty TEXT, and a typed `NULL` submission stays TEXT — distinct from the SQL-NULL choice, which binds no parameter.

## Placeholder write-state seam (`internal/querybuilder/write_state.go`)

Typed forward-compatible state so Issues #37/#39 can adopt the report unchanged: `SetAssignment` (UPDATE, {Value, NULL} only) and `InsertColumn` (INSERT {Value, NULL, Default/Omit}) with immutable transitions — `AcceptSetColumn`/`ChooseSetAssignment`/`SubmitSetValue`, `BeginInsertPrompts`/`ChooseInsertColumn`/`SubmitInsertValue`, plus `WithSetAssignments` for representing states (such as duplicate SET columns) the guided flow itself never constructs. `InsertableColumns()` derives visible insertable columns from Schema metadata.

## Whole-value clearing (`internal/querybuilder/whole_value.go`)

One immutable general contract removing an entire completed value — exact entered text, parsed/bound type, and submission marker — atomically while preserving surrounding structural choices:

- `ClearWhereValue()`: a completed value-taking committed predicate reopens as an incomplete awaiting-value draft with the same column/operator; absent predicates, null operators, and open drafts are exact no-ops. Both Backspace and Delete on the base Where field route through it (`internal/ui/model.go`).
- `ClearLimitValue()`: restores the valid unbounded state; an already empty Limit is an identical snapshot.
- `ClearSetValue(column)` / `ClearInsertValue(column)`: keep the Value choice and column identity but drop the submission; unsubmitted entries are no-ops.

Keys inside an open popup or focused value prompt keep that context's editing behavior; Issue #16's remove-latest projection behavior on Column(s) is separate and untouched. The authoritative report derives resulting validity.

## TUI integration (`internal/ui/runnable_feedback.go`)

Base-context Enter consults the report **after** every higher-precedence context (popups, value prompts, pending requests, the stale-refresh overlay, suspension, terminal states) has consumed the key:

- **Invalid data**: the key is consumed — focus moves to the report's typed first-invalid field and its reason renders verbatim inline; no validation, estimation, execution, or history command returns. The `Set` and `Insert` field-bar entries render UPDATE/INSERT prompt targets in visual order. Issue #65 extends this to stale named projections: Enter on a stale Value or aggregate projection starts no request, appends no query or result history, and focuses `Column(s)` with the exact `the projected column no longer exists` reason — the existing generic `RunFieldProjection` → `columnsFieldLabel` mapping and `showRunnableReason` rendering satisfy the contract with no UI production change.
- **Runnable data**: Enter emits only the `PreExecutionRequestedMsg` seam (consumed by Issue #21's schema validation and later destructive preparation); no SQL executes in this issue.
- **Field openers**: fields owning Enter-driven editing openers (Table, Column(s), Set, Where, Group By, Order By, Limit) keep consuming Enter locally, preserving the Issue #12–#18 and #37 openers. The authoritative exception is the Limit field holding nonempty invalid committed text: Enter never reopens universal entry — it shows exactly `Limit must be an integer from 1 to 9223372036854775807`, or moves focus to an earlier invalid field when the report points elsewhere.

Inline reasons are transient: the next `applyBuilder` rebuild rebuilds the field bar from the authoritative snapshot, so correcting or clearing a field removes superseded feedback without a second validity engine in the UI.

## Cross-references

Issues #15–#19, #65; forward to Issues #21 (pre-execution schema validation), #37/#39 (write flows), #38; the [where-guided-predicates.md](where-guided-predicates.md), [group-order-limit.md](group-order-limit.md), [projection-count-star.md](projection-count-star.md), [projection-ordered-editing.md](projection-ordered-editing.md), and [schema-validation-workflow.md](schema-validation-workflow.md) contracts; the Runnable-State Contract, Builder and Display Interaction, Global Key Precedence, QueryBuilder, UI, and Testing Decisions of `Notes/PRD-sqloid.md`.
