# Issue #10 — early integration tracer (hardcoded SELECT *)

*2026-08-27T04:06:03Z by Showboat 0.6.1*
<!-- showboat-id: 325fd46f-9dba-43f7-aede-4ea79eed2d28 -->

Issue #10 de-risks the Bubble Tea ↔ Connection ↔ Schema stack before the real builder lands: one catalog-chosen object becomes a safely identifier-quoted hardcoded SELECT * and its typed rows or errors render in a minimal bordered grid. The path is disposable — Issue #22 must replace it, never extend it. First, the full test suite for every package the tracer touches:

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/schema ./internal/connection ./internal/ui | sed -E "s/[0-9]+\.[0-9]+s//g"
```

```output
ok  	github.com/chris/sqloid/internal/schema	
ok  	github.com/chris/sqloid/internal/connection	
ok  	github.com/chris/sqloid/internal/ui	
```

The fixture creates an ordinary table with INTEGER, TEXT, and NULL values plus a table whose cataloged name embeds a double quote. Create it and flow it through each module boundary in one run: ReadCatalog → ChooseTracerTarget → SelectAllSQL → RunTraceSelectAll → typed rows printed with their exact Go types, proving typed row transport (int64/string/nil) rather than stringified text:

```bash
cd /home/chris/sqloid && rm -f /tmp/tracer-demo.db && go run ./Notes/walkthroughs/010-06/code-walkthrough/_demoapp create /tmp/tracer-demo.db && go run ./Notes/walkthroughs/010-06/code-walkthrough/_demoapp show /tmp/tracer-demo.db 2>&1 | sed -n "1,10p"
```

```output
catalog version 2, objects: albums(ordinary-table) odd "name(ordinary-table)

selected "albums" -> SQL: SELECT * FROM "albums"
columns: [id title]
row 0: [1(int64), "one"(string)]
row 1: [2(int64), NULL(nil)]
row 2: [3(int64), "three"(string)]

selected "odd \"name" -> SQL: SELECT * FROM "odd ""name"
columns: [a]
```

Identifier safety is structural: SelectAllSQL only ever renders the cataloged name, double-quoted with embedded quotes doubled. The unusual identifier odd "name executed successfully above — the rendered SQL SELECT * FROM "odd ""name" is exactly what SQLite accepted. No user text can reach SQL anywhere in the tracer path.

```bash
cd /home/chris/sqloid && grep -n "func quoteIdentifier" -A 3 internal/schema/tracer.go && grep -rn "fmt.Sprintf\|strings.Join" internal/ui/tracer.go | wc -l
```

```output
53:func quoteIdentifier(s string) string {
54-	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
55-}
0
```

```bash
cd /home/chris/sqloid && rm -f /tmp/tracer-demo.db && go run ./Notes/walkthroughs/010-06/code-walkthrough/_demoapp create /tmp/tracer-demo.db && go run ./Notes/walkthroughs/010-06/code-walkthrough/_demoapp show /tmp/tracer-demo.db 2>&1 | sed -n "13,35p"
```

```output
=== rendered tracer grid (80x24 view) ===
╭──────────────────────────────────────────────────────────────────────────────╮
│tracer                                                                        │
│id | title                                                                    │
│1 | one                                                                       │
│2 |                                                                           │
│3 | three                                                                     │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
╰──────────────────────────────────────────────────────────────────────────────╯
╭──────────────────────────────────────────────────────────────────────────────╮
│                                                                              │
│> Command:                                                                    │
│                                                                              │
```

The settled tracer state replaces the placeholder content of the existing bordered results region only — the builder bar and footer of the Issue #8 shell remain intact below, with no row data leaking into builder fields. Now the basic-failure path: the fixture table is dropped after catalog selection, so execution alone fails; Connection settles a failed RequestResult with the wrapped cause preserved, and the model renders the typed error text without crashing or claiming any recovery:

```bash
cd /home/chris/sqloid && rm -f /tmp/tracer-demo2.db && go run ./Notes/walkthroughs/010-06/code-walkthrough/_demoapp create /tmp/tracer-demo2.db && go run ./Notes/walkthroughs/010-06/code-walkthrough/_demoapp show /tmp/tracer-demo2.db 2>&1 | sed -n "37,64p"
```

```output
 q quit   ? help                                                                

failed trace: outcome=failed result=<nil>
err=could not trace execute SELECT * FROM "albums": SQL logic error: no such table: albums (1)

=== rendered tracer error (80x24 view) ===
╭──────────────────────────────────────────────────────────────────────────────╮
│tracer                                                                        │
│could not trace execute SELECT * FROM "albums": SQL logic error: no such      │
│table: albums (1)                                                             │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
│                                                                              │
╰──────────────────────────────────────────────────────────────────────────────╯
╭──────────────────────────────────────────────────────────────────────────────╮
│                                                                              │
│> Command:                                                                    │
│                                                                              │
```

Scope call-outs: the tracer implements no builder, no popups, no pre-execution validation, no WHERE/GROUP BY/ORDER BY/LIMIT, no parameter binding, no paging or count request, no result cache, no query/result history, no cancellation UX beyond parent-context propagation, and no write paths of any kind. Tracer state is one isolated Model.Trace *TraceView field so Issue #22 can remove it wholesale — per Issue #10 and the Module Design / Testing Decisions sections of Notes/PRD-sqloid.md, the tracer must be replaced by Issue #22's first end-to-end SELECT, never extended into a production query path. Final full-suite verification plus vet/build:

```bash
cd /home/chris/sqloid && go vet ./... && go build ./... && gofmt -l internal cmd Notes/walkthroughs/010-06/code-walkthrough/_demoapp; echo VERIFIED
```

```output
VERIFIED
```
