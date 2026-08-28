# Builder command and table selection lifecycle (Issue #11)

Issue #11 delivers the initial idle view and the first stretch of the QueryBuilder path: one-key Command selection, focus advancement to Table, immutable replacement transitions with downstream clearing, and Schema-owned table eligibility including the special view-to-write clearing rule. It is the first production slice of the QueryBuilder module design section of `Notes/PRD-sqloid.md` and cross-references [responsive-tui-shell.md](responsive-tui-shell.md), [schema-catalog.md](schema-catalog.md), and [early-integration-tracer.md](early-integration-tracer.md).

## Idle state (before any execution)

At startup the bordered results region shows exactly `Select a command (S/U/D/I) to begin` as its status line — no frozen header, no displayed result range, and no count. Normal Issue #8 layout arithmetic is untouched: results still own border + status/count + frozen-header rows, the view partitions into exactly H rows, and nothing is special-cased in `CalculateLayout`. The idle content is deliberately distinct from an executed-empty SELECT's future `No rows` state; today an executed presentation exists only through the disposable tracer grid/error path, which already renders differently from the idle prompt.

## Initial focus and one-key selection

The builder starts with no command selected (`CommandUnselected`) and the next required field is Command. A single plain key — `S`, `U`, `D`, or `I` — immediately selects or replaces the command while the Command field holds UI focus. Every selection or replacement advances both the builder's next-required-field identity and the rendered field bar to Table; the Table field does not even render before a command exists.

## Immutable transitions and downstream clearing

All builder transitions are value-level and immutable: `QueryBuilder` methods return a fresh snapshot and never mutate their receiver. Choosing any command key replaces the current command, bumps the downstream generation (observable as `DownstreamGeneration()` — everything below Table is discarded per the Builder lifecycle decision), recomputes the eligible-object list under the new command's rules, and focuses Table. Selecting a table (through `SelectTable`, before popups exist) only ever accepts a name present in the current eligible list.

## Schema-owned eligibility

Eligibility never duplicates catalog rules: it consumes `Object.WriteEligible` and kinds from `internal/schema`.

- **SELECT** offers every refreshed cataloged object, views included.
- **UPDATE/DELETE/INSERT** offer only ordinary and virtual tables; views are SELECT-only.
- Refreshing the schema (`RefreshSchema` with a new `*schema.Catalog` snapshot) replaces metadata wholesale, and a selected name that vanished from the refresh clears on the next transition.
- Ordinary and virtual tables survive every command replacement; a table absent from the refreshed catalog survives nothing.

## View-to-write clearing

Switching a selected view to UPDATE, DELETE, or INSERT clears the Table selection entirely while focusing Table anyway and leaving the refreshed eligible write-table list populated with the ordinary and virtual tables. No other retention/clearing combination differs from the generic eligibility rule.

## Revisiting Command and focus outcomes

Tab/Shift+Tab (and Up/Down) keep moving rendered-field focus exactly as the Issue #8 shell defined. While any non-Command field holds focus, plain letters are inert — S/U/D/I routes nowhere else. Shifting back to Command exposes one-key replacement again: replacement lands on Table with all downstream state cleared and Schema-driven retention applied.

## UI integration boundary

`internal/ui/command_table.go` maps `s`/`u`/`d`/`i` keys onto `QueryBuilder.SelectCommand` only while Command is focused, rebuilds the field bar from builder state after each applied snapshot, and consumes refreshed catalogs via `SchemaRefreshedMsg{Catalog}` (an injected `*schema.Catalog`). The UI owns rendering and routing only — eligibility logic stays in `internal/querybuilder`, which imports `internal/schema` but neither Bubble Tea nor `internal/ui`. No popup/searchable-selection behavior exists yet (Issue #12), and the disposable tracer remains isolated pending its wholesale replacement (Issue #22).

## Testing decisions

Pure table-driven tests (`internal/querybuilder/command_table_test.go`) cover the initial unselected state, all sixteen S/U/D/I cross-replacements with eligibility-only retention, downstream clearing visibility, view-to-write clearing, and metadata-driven eligible lists using fixture catalogs of ordinary/virtual/view objects. Scripted `(model, msg) → (model, cmd)` tests (`internal/ui/command_table_test.go`) assert startup Command focus, one-key advancement, revisit-and-replace, catalog injection, off-focus letter inertness, and UI-reflected view clearing plus write-table retention. Rendering assertions (`internal/ui/view_test.go`) pin the exact prompt text, absence of `No rows`/count decoration, blank interior below the status row, unchanged row partitioning at 80×24/100×30/160×50, and the distinction from settled tracer output. See [unit-tests.md](unit-tests.md).
