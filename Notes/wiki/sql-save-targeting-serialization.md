# SQL save targeting and standalone serialization (Issue #48)

Issue #48 implements the Ctrl+S query-save path: pure in-memory target resolution (ordinary and terminal priority), the exact no-target feedback, and UI-independent standalone SQL assembly of exactly one executable statement with one trailing semicolon. It prepares one save target only — the filesystem picker, atomic temp-file-plus-rename save, and overwrite confirmation are owned by later issues, and loading saved SQL is unsupported (one-way export only).

## Target resolution (Tasks 1–2)

The resolver is a pure, UI-independent function in `internal/export` (`save_target.go`) consuming only immutable in-memory state; `internal/ui` (`sql_save.go`) collects the candidates and wires Ctrl+S. Resolution issues zero validation, schema, connection, or database work by construction, and never opens a picker or serializes anything itself.

### Ordinary priority

In ordinary (nonterminal) states, Ctrl+S resolves in exact order:

1. **Viewed historical result's query** — the query associated with the currently viewed historical result. The association is the result entry's stable `QueryEntryID` (recorded at actual execution start, on `history.ResultEntry` at finalization) resolved against the retained query-history store by stable ID — never through visible rendered text. An evicted or missing association falls through to the next candidate.
2. **Current runnable builder** — the builder contributes only when `RunnableReport()` accepts it (Issue #19's authoritative verdict).
3. **Last actual execution** — the query-history entry recorded as the execution's association at its actual execution start.

Every pairwise and all-present combination resolves accordingly; an absent or non-runnable builder is skipped; with no candidate the exact inline feedback `no runnable query to save` shows and **no picker opens and nothing serializes**.

### Terminal-only priority

In the deletion, replacement (Issues #45/#46 terminal states), and outcome-unknown terminal states, priority is deliberately reduced: only the Ctrl+P/N-selected immutable query history entry wins, otherwise the last actual execution. The current builder and the viewed-result candidates are ignored entirely there, and no database inspection occurs.

### Zero database work

Target resolution and picker preparation use immutable in-memory state only — `history.Store`/`history.ResultStore` stable-ID lookups, `QueryBuilder.RunnableReport`, and the recorded execution association. Tests assert zero validation, schema-refresh, page, count, or write requests, that no command is returned, and that no validation workflow opens.

## Standalone serialization (Tasks 3–4)

`internal/export`'s `SerializeSQLQuery` renders one immutable complete `querybuilder.HistoryState` into exactly one standalone executable statement with exactly one trailing semicolon, no placeholders, and no second statement:

- **SELECT** — projection in commit order (wildcard, bare `COUNT(*)`, `TOKEN("column")` aggregates, quoted plain columns), WHERE, GROUP BY in commit order, committed ORDER BY expression with direction, and the accepted Limit.
- **UPDATE** — `UPDATE "table" SET "col" = <literal>, ...` in SET order with preserved Value/NULL choices, then the optional WHERE.
- **DELETE** — `DELETE FROM "table"` with the optional predicate appended exactly once; the absent-WHERE bare form targets every row.
- **INSERT** — Value columns in schema prompt order with rendered literals, NULL columns as the SQL keyword, Default/Omit columns absent from both lists; the all-omit form is exactly `INSERT INTO "t" DEFAULT VALUES`.

Every identifier quotes through Issue #14's canonical `QuoteIdentifier` atom (double quotes doubled), fixed tokens (`=`, `>=`, `!=`, `IS NULL`, `IS NOT NULL`, `LIKE`, aggregates, `ASC`/`DESC`) render only through their closed typed choices, and every INTEGER/REAL/TEXT/NULL/BLOB literal renders through Issue #14's sole canonical `RenderSQLLiteral` — there is no second literal serializer. Incomplete states (missing table, empty SELECT projection, unsubmitted values, pending choices) and unsupported commands return the typed `ErrUnsupportedQueryState`; no partial statement is ever returned. BLOB payloads that can never arrive from user text entry serialize through the same typed atom via `SerializeSQLLiteral`.

Round-trip tests execute the exact serialized bytes against modernc SQLite and verify TEXT quote-doubling, SQL NULL, signed int64 boundaries, REAL shortest-round-trip identity (integral `5.0`, `-0.0`, exponent, subnormal, precision edges), and empty/non-empty BLOB payloads byte-for-byte.

## No-target feedback and unsupported loading

When no target resolves, Ctrl+S shows exactly `no runnable query to save` inline, opens no picker, and serializes nothing. Saved SQL is one-way export only: this issue assembles one standalone statement for the picker flow owned by later issues (atomic saves and overwrite protection) and does not support loading or importing saved SQL.

## References

- Issues #14 (canonical identifier/fixed-token/literal atoms), #20/#35/#36 (immutable query and result history and the executed-query association), #19 (authoritative runnable verdict), #42 (write execution recording the query association), #45/#46 (terminal states owning terminal-only targeting), #48.
- `Notes/PRD-sqloid.md`: Query save targeting, SQL safety, and the terminal context/action matrix rows.
- Related pages: [sql-atoms-and-literals.md](sql-atoms-and-literals.md), [query-history-navigation.md](query-history-navigation.md), [result-history-browsing.md](result-history-browsing.md), [outcome-unknown-terminal.md](outcome-unknown-terminal.md), [health-terminal.md](health-terminal.md), [transactional-writes.md](transactional-writes.md), [in-flight-gating.md](in-flight-gating.md).
