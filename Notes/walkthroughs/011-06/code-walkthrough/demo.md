# Issue #11 — command and table selection lifecycle

*2026-08-27T05:54:21Z by Showboat 0.6.1*
<!-- showboat-id: 33310311-93ce-457d-9900-c99b3956baa4 -->

Issue #11 delivers the first production QueryBuilder slice: at startup the bordered results region shows exactly `Select a command (S/U/D/I) to begin` with no frozen header, range, or count; Command holds focus, one plain letter selects or replaces the command and advances focus to Table; every replacement clears downstream state; and internal/schema metadata alone decides whether a selected object survives — views are SELECT-only, ordinary and virtual tables stay write candidates. Reference: Notes/issues/011-command-and-table-selection-lifecycle.md and the Builder and Display Interaction / Builder lifecycle / QueryBuilder Module Design sections of Notes/PRD-sqloid.md. First, the packages involved:

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/querybuilder ./internal/ui | sed -E 's/[0-9]+\.[0-9]+s//g'
```

```output
ok  	github.com/chris/sqloid/internal/querybuilder	
ok  	github.com/chris/sqloid/internal/ui	
```

The disposable _demoapp drives the real Bubble Tea model with scripted messages (no database involved): it sizes to 80x24, dumps builder state, and prints the first rows of the rendered view. Startup: Command focused on the single Command field, no table selected, and the exact idle prompt in the results region:

```bash
cd /home/chris/sqloid && go run ./Notes/walkthroughs/011-06/code-walkthrough/_demoapp 2>/dev/null | sed -n '1,13p'
```

```output

=== startup idle ===
UI focus=0 fields: [Command=""] selectedTable=<none> downstreamGen=0 eligibles=[]
 1 ╭──────────────────────────────────────────────────────────────────────────────╮
 2 │Select a command (S/U/D/I) to begin                                           │
 3 │                                                                              │
 4 │                                                                              │
 5 │                                                                              │
 6 │                                                                              │
 7 │                                                                              │
 8 │                                                                              │
 9 │                                                                              │
10 │                                                                              │
```

One plain letter selects a command. Pressing S replaces nothing (first selection, downstreamGen 0→1) and advances both the builder's next-required-field and UI focus to the new Table field:

```bash
cd /home/chris/sqloid && go run ./Notes/walkthroughs/011-06/code-walkthrough/_demoapp 2>/dev/null | sed -n '15,30p'
```

```output
12 │                                                                              │

=== press S — SELECT chosen, focus advances to Table ===
UI focus=1 fields: [Command="SELECT"] [Table=""] selectedTable=<none> downstreamGen=1 eligibles=[]
 1 ╭──────────────────────────────────────────────────────────────────────────────╮
 2 │Select a command (S/U/D/I) to begin                                           │
 3 │                                                                              │
 4 │                                                                              │
 5 │                                                                              │
 6 │                                                                              │
 7 │                                                                              │
 8 │                                                                              │
 9 │                                                                              │
10 │                                                                              │
11 │                                                                              │
12 │                                                                              │
```

Shift+Tab revisits Command, where U/D/I one-key replacements apply: each bumps downstreamGen again — every downstream command-specific field below Table is discarded on any replacement. With no catalog refreshed yet the eligible list stays empty; keys still behave identically:

```bash
cd /home/chris/sqloid && go run ./Notes/walkthroughs/011-06/code-walkthrough/_demoapp 2>/dev/null | sed -n '32,72p' | grep -v '^ 1 \|^ 2 \|^ 3 \|^ 4 \|^ 5 \|^ 6 \|^ 7 \|^ 8 \|^ 9 \|^10 \|^11 \|^12 '
```

```output
=== Shift+Tab back to Command, press U / D / I replacing the command ===
UI focus=1 fields: [Command="UPDATE"] [Table=""] selectedTable=<none> downstreamGen=2 eligibles=[]
UI focus=1 fields: [Command="DELETE"] [Table=""] selectedTable=<none> downstreamGen=3 eligibles=[]
UI focus=1 fields: [Command="INSERT"] [Table=""] selectedTable=<none> downstreamGen=4 eligibles=[]

```

Eligibility is Schema-owned. Injecting a fixture catalog (virtual logs_fts, ordinary users, view vw_summary) through SchemaRefreshedMsg makes SELECT offer all three objects, including the SELECT-only view:

```bash
cd /home/chris/sqloid && go run ./Notes/walkthroughs/011-06/code-walkthrough/_demoapp 2>/dev/null | sed -n '/refresh schema/,/switch to INSERT/p' | grep UI\ focus
```

```output
UI focus=1 fields: [Command="SELECT"] [Table=""] selectedTable="vw_summary" downstreamGen=5 eligibles=[logs_fts(virtual-table) users(ordinary-table) vw_summary(view)]
```

With the view selected as table, switching to INSERT clears the selection, keeps Table focused, and repopulates the eligible list with exactly the two write candidates — views never appear under write commands:

```bash
cd /home/chris/sqloid && go run ./Notes/walkthroughs/011-06/code-walkthrough/_demoapp 2>/dev/null | sed -n '/switch to INSERT/,+1p' | head -2
```

```output
=== switch to INSERT: view cleared, Table focused, eligible write tables listed ===
UI focus=1 fields: [Command="INSERT"] [Table=""] selectedTable=<none> downstreamGen=6 eligibles=[logs_fts(virtual-table) users(ordinary-table)]
```

The retention rule is generic eligibility: an ordinary table survives UPDATE→DELETE untouched (note downstreamGen still bumps — clearing happened to downstream state even though the table was retained):

```bash
cd /home/chris/sqloid && go run ./Notes/walkthroughs/011-06/code-walkthrough/_demoapp 2>/dev/null | sed -n '/retain eligible ordinary/,+1p' | head -2
```

```output
=== retain eligible ordinary table across UPDATE -> DELETE ===
UI focus=1 fields: [Command="DELETE"] [Table="users"] selectedTable="users" downstreamGen=8 eligibles=[logs_fts(virtual-table) users(ordinary-table)]
```

The idle state is pinned in tests: exact prompt, no result-only decoration, unchanged layout arithmetic at every supported baseline size, and a settled-tracer presentation that can never be confused with it. The focused test suites enumerate the whole contract:

The idle state is pinned in tests: exact prompt, no result-only decoration, unchanged layout arithmetic at every supported baseline size, and a settled-tracer presentation that can never be confused with it. Both focused suites pass:

```bash
cd /home/chris/sqloid && go test ./internal/querybuilder ./internal/ui | tr -d '\t' | sed -E 's/[[:space:]]*$//'
```

```output
ok  github.com/chris/sqloid/internal/querybuilder(cached)
ok  github.com/chris/sqloid/internal/ui(cached)
```

gofmt-clean and vet-clean across everything touched:

```bash
cd /home/chris/sqloid && gofmt -l internal/querybuilder internal/ui Notes/walkthroughs/011-06/code-walkthrough/_demoapp; echo "gofmt-clean: $?" && go vet ./internal/querybuilder ./internal/ui && echo 'vet: clean'
```

```output
gofmt-clean: 0
vet: clean
```
