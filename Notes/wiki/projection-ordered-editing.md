# Ordered Projection Editing and Deduplication (Issue #16)

Issue #16 extends the Issue #15 projection path (see [projection-count-star.md](projection-count-star.md)) with insertion-ordered editing: committed entries retain exact selection order, exact repeated named pairs are rejected as no-ops, the bare `COUNT(*)` sentinel duplicates defensively, wildcard selection atomically replaces the whole list, and Backspace/Delete from the base Column(s) field removes exactly the latest entry. Implementation lives in `internal/querybuilder/projection.go` (transitions), `internal/ui/model.go` (base-field key routing), `internal/ui/projection_popup.go` (popup/openers unchanged in contract), and the tests `internal/querybuilder/projection_test.go` plus `internal/ui/projection_editing_test.go`.

## Insertion-ordered typed entries

`ProjectionEntry` commits append-only in selection order across Value, Count, Min, Max, Avg, and Sum. The same column may appear under distinct aggregates (`email(MIN)`, `email(AVG)`) and different columns may share one aggregate (`email(MIN)`, `id(MIN)`); only an exact repeated `(column identity, aggregate)` pair — including the zero plain-value aggregate on the pair `(column, Value)` — is rejected. Rejection is a full no-op: entries neither reorder nor replace, no reopen is requested, and focus-transition data stays untouched; later distinct appends continue at the end. Duplicate history from rejected transitions never becomes removable state.

The dedicated sentinel coexists with later named aggregates (`COUNT(*)`, then `email(MIN)`, …). Invoking `AcceptProjection` for the sentinel again directly — outside the conditional UI path that hides it once any entry exists — is an identical no-op preserving entries, candidates, and focus.

## Wildcard exclusivity

Wildcard acceptance from any prior state — empty, populated, or already sole-wildcard — replaces the whole projection with exactly one wildcard entry atomically, focuses Column(s), and requests no reopen. While a wildcard is committed, no transition may append anything beside it until removal empties the projection: direct sentinel accepts are no-ops and `CompleteProjectionAggregate` is rejected outright. Malformed identities cannot bypass these invariants: unknown candidate kinds and junk-carrying synthetic identities (a wildcard or sentinel with column text) normalize or reject without state change, while a real column literally named `COUNT(*)` keeps its distinct named identity and can still coexist with the committed bare sentinel.

## Remove-latest editing keys

Backspace/Delete with the base Column(s) field focused (no popup open, not suspended) routes through the immutable builder transition:

- `RemoveLatestProjection()` removes exactly one committed entry per call, walking backward through named entries and the bare sentinel; earlier entries keep their identities and insertion order. Removal never changes focus.
- Removing a sole wildcard — or the final remaining entry — empties the projection entirely, so QueryBuilder's own candidate derivation restores the Issue #15 empty sequence (`*` default-highlighted first, bare `COUNT(*)` second); the UI never patches candidate lists.
- Empty-state removal is an exact unchanged no-op.

Per the Context/Action matrix of [`../PRD-sqloid.md`](../PRD-sqloid.md), when a popup owns input the same keys follow the reusable popup contract instead ([searchable-popups.md](searchable-popups.md)): backspace inside a searchable Column(s) popup edits search text with full reset semantics and deletes nothing; the scroll-only aggregate popup ignores search entirely and never loses committed entries. Removal through base keys also never closes, reopens, reorders, or corrupts any open popup, because those paths consume the key before base-context handling.

Cross-references: [builder-command-table.md](builder-command-table.md), [projection-count-star.md](projection-count-star.md), [searchable-popups.md](searchable-popups.md), [unit-tests.md](unit-tests.md), and the Query Grammar, Builder and Display Interaction, keyboard context matrix, QueryBuilder Module Design, UI Module Design, and Testing Decisions sections of [`../PRD-sqloid.md`](../PRD-sqloid.md). Issues #15 and #16 together cover the full Column(s) lifecycle.
