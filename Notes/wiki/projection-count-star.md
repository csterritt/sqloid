# SELECT Wildcard and COUNT(*) Projection Path (Issue #15)

Issue #15 implemented the empty Column(s) projection path from the Query Grammar of [`../PRD-sqloid.md`](../PRD-sqloid.md): `*` first/default, synthetic bare `COUNT(*)` second, direct sentinel commits, conditional sentinel visibility, named-column continuation to aggregate selection, and sentinel reappearance when the projection returns to empty. The implementation lives in `internal/querybuilder/projection.go` (UI-independent state/transitions), `internal/querybuilder/builder.go` + `command_table.go` (state integration), `internal/ui/projection_popup.go` (Column(s) popup wiring), `internal/ui/command_table.go` (field-bar rendering), and `internal/ui/table_popup.go`/`model.go` (key routing); see [source-code.md](source-code.md).

## Candidate identities vs. display text

A `qb.ProjectionCandidate` carries its identity as a `ProjectionKind` plus optional `Column`, deliberately separate from the `Display()` label — so a real column named `*` or `COUNT(*)` collides in text but never in identity with the synthetic entries:

- `ProjectionWildcard` — the sole wildcard; displays exactly `*`.
- `ProjectionCountStar` — the synthetic bare count sentinel; displays exactly `COUNT(*)`.
- `ProjectionColumn` — a declared visible column with its name.

Candidates expose `Key()` for flat-ID consumers (the popup layer): `"wildcard:*"`, `"count-star:(*)"`, or `"column:<name>"`. The reserved prefixes make synthetic keys unrepresentable by any real column name. `Display()` is presentation only: two candidates may share it while remaining distinct.

Aggregates reuse the typed closed `Aggregate` enum from `sql_atoms.go`; `AggregateValue` names the zero aggregate (plain column). Aggregates exist only on named-column identities applied through `CompleteProjectionAggregate(column, agg)`, where `column` must be a visible declared name of the selected object — so `MIN(*)`, `MAX(*)`, `AVG(*)`, and `SUM(*)` are impossible by construction, not merely filtered out.

## Candidate derivation

`QueryBuilder.ProjectionCandidates()` derives deterministic choices only for a SELECT with a selected table (`projectionReady`):

- **Empty projection** — wildcard first (and therefore the popup's default highlight via the Issue #12 reset contract), bare `COUNT(*)` second, then every visible (`!Hidden`) column of the selected object in Schema (declared) order; see [schema-catalog.md](schema-catalog.md).
- **Any committed entry** — both synthetic items disappear; only named columns remain offered.
- **Committed wildcard** — nothing further is offered: the wildcard is the sole projection and cannot coexist with entries.

Empty column metadata still yields exactly the two synthetic candidates.

## Transitions

Transitions stay immutable value-level methods returning a new snapshot:

- `AcceptProjection(candidate)` — for wildcard or bare `COUNT(*)`: appends the dedicated identity directly (no aggregate step ever), focuses `FieldColumns`, and reports `ReopenColumns: true` (sentinel) so the UI reopens Column(s). Wildcard does not reopen. For a named column: no commit happens at all; the outcome hands back `PendingAggregate` holding the chosen identity while builder state stays untouched. Out-of-context accepts (wrong command/state, empty start) return the receiver unchanged.
- `CompleteProjectionAggregate(column, agg)` — finishes a pending named-column acceptance by appending `(column, aggregate)` in insertion order, focusing Column(s), and requesting reopen. Unsupported aggregates, unknown columns, or aggregating `*` are rejected outright.
- `RemoveProjection(index)` — removes one committed entry by position; out-of-range indexes are ignored. Sentinels reappear through `ProjectionCandidates` as soon as removal empties the projection. Issue #16 adds ordered editing and deduplication, documented in [projection-ordered-editing.md](projection-ordered-editing.md): entries stay append-only in insertion order across Value/Count/Min/Max/Avg/Sum; exact repeated `(column, aggregate)` pairs — including `(column, Value)` — are rejected as full no-ops without reordering or focus-transition change; bare `COUNT(*)` coexists with later named aggregates while a direct duplicate-sentinel transition outside the conditional UI path is a no-op; wildcard selection from any state atomically replaces the whole list and stays sole (no transition may append beside it) until removal empties the projection; `RemoveLatestProjection()` immutably removes exactly the latest entry, emptying on a sole wildcard and no-op when empty.
- Accessors: `ProjectionEntries()` (insertion order, fresh slice) and `ProjectionEmpty()`.

State lives on `QueryBuilder` and follows existing lifecycle rules: command replacement discards the projection (downstream clearing), and a table vanishing on schema refresh clears it too.

## Column(s) popup wiring (TUI)

`internal/ui/projection_popup.go` connects the transitions to the reusable Issue #12 popup contract ([searchable-popups.md](searchable-popups.md)):

- Enter on a focused **Column(s)** field opens a fresh searchable single-select popup whose candidates come verbatim from `QueryBuilder.ProjectionCandidates()`, preserving typed identity through `Key()`-encoded popup IDs; viewport capped at 8 rows like the Table popup.
- Accepting **wildcard** or bare **COUNT(*)** goes through the same close→commit order as every single-select popup: opener restored, then `AcceptProjection` applied via `applyBuilder`. When the outcome requests reopen, `reopenColumnsPopup` reselects exact Column(s) focus and reinstalls a fresh popup (search cleared, highlight reset to the first candidate). No aggregate popup is ever opened for either.
- Accepting a **named column** opens the scroll-only `Value/Count/Min/Max/Avg/Sum` aggregate popup opened by Column(s)' identity — no search input modality. Its acceptance calls `CompleteProjectionAggregate` and reopens Column(s), preserving previously completed entries in the field bar.
- The field bar gains a `Column(s)` field once a SELECT has a table; content renders committed entries comma-joined as `*`, `COUNT(*)`, plain names, or `column(AGGREGATE)` using `SQLToken()`.

The UI never synthesizes candidates, never duplicates projection rules, and cannot express an aggregate-on-wildcard choice because no code path can construct one.

Cross-references: [builder-command-table.md](builder-command-table.md), [searchable-popups.md](searchable-popups.md), [schema-catalog.md](schema-catalog.md), [sql-atoms-and-literals.md](sql-atoms-and-literals.md), [unit-tests.md](unit-tests.md), and the Builder and Display Interaction, QueryBuilder Module Design, UI Module Design, and Testing Decisions sections of [`../PRD-sqloid.md`](../PRD-sqloid.md).
