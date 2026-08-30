# Issue #48: SQL save targeting and standalone serialization

*2026-08-29T18:22:22Z by Showboat 0.6.1*
<!-- showboat-id: 5821424a-bf56-4b32-8071-eb33d09648ac -->

Issue #48 (Notes/PRD-sqloid.md 'Query save targeting', 'SQL safety', and the terminal context/action matrix rows) implements the Ctrl+S query-save path: pure in-memory target resolution with ordinary priority (viewed historical result's associated query → runnable builder → last actual execution), terminal-only priority in the deletion/replacement/outcome-unknown states, the exact `no runnable query to save` feedback with no picker, and UI-independent standalone serialization of exactly one executable statement. First: ordinary target resolution covers every pairwise and all-present priority combination plus absent and non-runnable builders, all from immutable in-memory history — never from a database.

```bash
cd /home/chris/sqloid && go test ./internal/export -run 'TestOrdinarySaveTargetPriority|TestEvictedAssociationFallsThrough' -count=1 -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestOrdinarySaveTargetPriority
=== RUN   TestOrdinarySaveTargetPriority/all_present_chooses_viewed_result_query
=== RUN   TestOrdinarySaveTargetPriority/viewed_beats_runnable_builder
=== RUN   TestOrdinarySaveTargetPriority/viewed_beats_last_execution
=== RUN   TestOrdinarySaveTargetPriority/viewed_alone
=== RUN   TestOrdinarySaveTargetPriority/runnable_builder_beats_last_execution
=== RUN   TestOrdinarySaveTargetPriority/builder_alone
=== RUN   TestOrdinarySaveTargetPriority/non-runnable_builder_falls_to_last_execution
=== RUN   TestOrdinarySaveTargetPriority/last_execution_alone
=== RUN   TestOrdinarySaveTargetPriority/nothing_to_save
--- PASS: TestOrdinarySaveTargetPriority (Ts)
    --- PASS: TestOrdinarySaveTargetPriority/all_present_chooses_viewed_result_query (Ts)
    --- PASS: TestOrdinarySaveTargetPriority/viewed_beats_runnable_builder (Ts)
    --- PASS: TestOrdinarySaveTargetPriority/viewed_beats_last_execution (Ts)
    --- PASS: TestOrdinarySaveTargetPriority/viewed_alone (Ts)
    --- PASS: TestOrdinarySaveTargetPriority/runnable_builder_beats_last_execution (Ts)
    --- PASS: TestOrdinarySaveTargetPriority/builder_alone (Ts)
    --- PASS: TestOrdinarySaveTargetPriority/non-runnable_builder_falls_to_last_execution (Ts)
    --- PASS: TestOrdinarySaveTargetPriority/last_execution_alone (Ts)
    --- PASS: TestOrdinarySaveTargetPriority/nothing_to_save (Ts)
=== RUN   TestEvictedAssociationFallsThrough
--- PASS: TestEvictedAssociationFallsThrough (Ts)
PASS
ok  	github.com/chris/sqloid/internal/export
```

The same priority table runs at the model level in internal/ui, proving the UI collects candidates only from immutable stores and that no resolution press ever touches the wired database seams. The no-target case shows the exact inline feedback, opens no picker, and serializes nothing.

```bash
cd /home/chris/sqloid && go test ./internal/ui -run 'TestOrdinarySaveTargetingPriority|TestOrdinarySaveNoTargetFeedback|TestSaveBlockedDuringInFlight' -count=1 -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestOrdinarySaveTargetingPriority
=== RUN   TestOrdinarySaveTargetingPriority/all_present_chooses_viewed_result_query
=== RUN   TestOrdinarySaveTargetingPriority/viewed_beats_runnable_builder
=== RUN   TestOrdinarySaveTargetingPriority/viewed_beats_last_execution
=== RUN   TestOrdinarySaveTargetingPriority/viewed_alone
=== RUN   TestOrdinarySaveTargetingPriority/runnable_builder_beats_last_execution
=== RUN   TestOrdinarySaveTargetingPriority/runnable_builder_alone
=== RUN   TestOrdinarySaveTargetingPriority/non-runnable_builder_falls_to_last_execution
=== RUN   TestOrdinarySaveTargetingPriority/last_execution_alone
--- PASS: TestOrdinarySaveTargetingPriority (Ts)
    --- PASS: TestOrdinarySaveTargetingPriority/all_present_chooses_viewed_result_query (Ts)
    --- PASS: TestOrdinarySaveTargetingPriority/viewed_beats_runnable_builder (Ts)
    --- PASS: TestOrdinarySaveTargetingPriority/viewed_beats_last_execution (Ts)
    --- PASS: TestOrdinarySaveTargetingPriority/viewed_alone (Ts)
    --- PASS: TestOrdinarySaveTargetingPriority/runnable_builder_beats_last_execution (Ts)
    --- PASS: TestOrdinarySaveTargetingPriority/runnable_builder_alone (Ts)
    --- PASS: TestOrdinarySaveTargetingPriority/non-runnable_builder_falls_to_last_execution (Ts)
    --- PASS: TestOrdinarySaveTargetingPriority/last_execution_alone (Ts)
=== RUN   TestOrdinarySaveNoTargetFeedback
--- PASS: TestOrdinarySaveNoTargetFeedback (Ts)
=== RUN   TestSaveBlockedDuringInFlight
--- PASS: TestSaveBlockedDuringInFlight (Ts)
PASS
ok  	github.com/chris/sqloid/internal/ui
```

The viewed-result association is real, not synthesized: one executed SELECT's finalization carries the stable query-history association, and Ctrl+S over the viewed snapshot resolves the exact executed query state — the backing immutable history entry, never visible text.

```bash
cd /home/chris/sqloid && go test ./internal/ui -run 'TestOrdinarySaveViewedResultAssociation' -count=1 -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestOrdinarySaveViewedResultAssociation
--- PASS: TestOrdinarySaveViewedResultAssociation (Ts)
PASS
ok  	github.com/chris/sqloid/internal/ui
```

Now the three terminal states. With no Ctrl+P/N selection the last actual execution is the target; after selecting an immutable query-history entry with Ctrl+P that selection wins, deliberately overriding the builder and last execution. Every terminal press issues zero database work.

```bash
cd /home/chris/sqloid && go test ./internal/ui -run 'TestTerminalSaveTargetingOutcomeUnknown|TestTerminalSaveTargetingHealth|TestTerminalSaveNoTargetFeedback' -count=1 -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   TestTerminalSaveTargetingOutcomeUnknown
--- PASS: TestTerminalSaveTargetingOutcomeUnknown (Ts)
=== RUN   TestTerminalSaveTargetingHealth
=== RUN   TestTerminalSaveTargetingHealth/deleted
=== RUN   TestTerminalSaveTargetingHealth/replaced
--- PASS: TestTerminalSaveTargetingHealth (Ts)
    --- PASS: TestTerminalSaveTargetingHealth/deleted (Ts)
    --- PASS: TestTerminalSaveTargetingHealth/replaced (Ts)
=== RUN   TestTerminalSaveNoTargetFeedback
--- PASS: TestTerminalSaveNoTargetFeedback (Ts)
PASS
ok  	github.com/chris/sqloid/internal/ui
```

Serialization: SerializeSQLQuery assembles exactly one standalone executable statement from the immutable complete HistoryState shared by internal/history and internal/result, delegating every identifier, fixed token, and literal to Issue #14's canonical atoms — there is no second literal serializer. The examples print representative statements containing difficult identifiers, quote-doubled and injection-looking TEXT, the MaxInt64 bound, SQL NULL, negative-zero REAL, and INSERT Value/NULL/Default-Omit choices; inspect the exact bytes and the single trailing semicolon.

```bash
cd /home/chris/sqloid && go test ./internal/export -run 'ExampleSerialize' -count=1 -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
=== RUN   ExampleSerializeSQLQuery_select
--- PASS: ExampleSerializeSQLQuery_select (Ts)
=== RUN   ExampleSerializeSQLQuery_update
--- PASS: ExampleSerializeSQLQuery_update (Ts)
=== RUN   ExampleSerializeSQLQuery_delete
--- PASS: ExampleSerializeSQLQuery_delete (Ts)
=== RUN   ExampleSerializeSQLQuery_insert
--- PASS: ExampleSerializeSQLQuery_insert (Ts)
=== RUN   ExampleSerializeSQLLiteral
--- PASS: ExampleSerializeSQLLiteral (Ts)
PASS
ok  	github.com/chris/sqloid/internal/export
```

The full exact-byte and round-trip suites pin every command shape, typed value edge (int64 boundaries, REAL integral identity, negative zero, exponent, subnormal, precision), empty/non-empty BLOB through the typed atom, and execute the exact bytes against modernc SQLite to prove semantic round trip.

```bash
cd /home/chris/sqloid && go test ./internal/export -run 'TestSerializeExactBytes|TestSerializeValueEdges|TestSerializeRoundTrip' -count=1 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
ok  	github.com/chris/sqloid/internal/export
```

Remaining ownership evidence: the sole canonical serializer lives in internal/querybuilder (Issue #14), the architecture checks forbid any private copy in internal/export, and the full suites stay green.

```bash
cd /home/chris/sqloid && grep -l 'RenderSQLLiteral' internal/export/*.go internal/querybuilder/sql_literal.go | sort && go test ./internal/result -run 'TestNoExporterPrivateResultRepresentation' -count=1 -v 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g'
```

```output
internal/export/sql_serialize.go
internal/querybuilder/sql_literal.go
=== RUN   TestNoExporterPrivateResultRepresentation
--- PASS: TestNoExporterPrivateResultRepresentation (Ts)
PASS
ok  	github.com/chris/sqloid/internal/result	0.002s
```

```bash
cd /home/chris/sqloid && go test ./... 2>&1 | grep -c '^ok' && go test ./internal/export ./internal/ui ./internal/history ./internal/result -count=1 2>&1 | sed -E 's/\(([0-9.]+)s\)/(Ts)/g; s/^(ok[[:space:]]+[^[:space:]]+)[[:space:]]+[0-9.]+s$/\1/'
```

```output
11
ok  	github.com/chris/sqloid/internal/export
ok  	github.com/chris/sqloid/internal/ui
ok  	github.com/chris/sqloid/internal/history
ok  	github.com/chris/sqloid/internal/result
```

The single delegation point is internal/export/sql_serialize.go calling querybuilder's RenderSQLLiteral (Issue #14's canonical renderer); the architecture check forbids any private literal or token copy in the exporter boundary. Loading saved SQL is unsupported (one-way export only), and the filesystem picker, atomic temp-file-plus-rename saves, and overwrite confirmation remain owned by later issues — this issue prepares exactly one immutable save target and nothing on disk. Full verification: 11 packages ok, with the export, ui, history, and result suites green above.
