# Issue #9 — schema catalog and table eligibility

*2026-08-27T01:17:06Z by Showboat 0.6.1*
<!-- showboat-id: b2acf59b-9684-4f68-b0e4-5e98b6c79a95 -->

Issue #9 implements the Schema scope and Schema metadata decisions from Notes/PRD-sqloid.md: a UI-independent schema package that catalogs main-schema objects (ordinary tables, virtual tables, views), reports write eligibility, rowid capability and declared-rowid shadowing, and carries columns with declared type, visibility, and insertability from PRAGMA table_xinfo — excluding sqlite_% and _cf_METADATA — while internal/connection gathers every read through its shared request boundary. Full test suite first:

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/schema ./internal/connection | sed -E "s/[0-9]+\.[0-9]+s//g"
```

```output
ok  	github.com/chris/sqloid/internal/schema	
ok  	github.com/chris/sqloid/internal/connection	
```

The fixture below creates every object kind the contract must handle — an ordinary table, an fts5 virtual table (whose five shadow tables SQLite itself registers as ordinary tables), a view, a WITHOUT ROWID table, a declared-rowid shadowing table, an AUTOINCREMENT table (auto-creating sqlite_sequence), D1's _cf_METADATA, and a generated-columns mix. A small throwaway driver program opens the database through connection.Open and prints the catalog contract:

Fixture creation (drop-in helper under _demoapp, outside the production source tree) and the full catalog through connection.Open — every object kind, write eligibility, rowid capability, shadowing, and insertable-column counts. Note both reserved objects exist in the fixture database yet neither sqlite_sequence nor _cf_METADATA appears:

```bash
cd /home/chris/sqloid && rm -f /tmp/schema-fix.db && go run ./Notes/walkthroughs/009-06/code-walkthrough/_demoapp create /tmp/schema-fix.db && go run ./Notes/walkthroughs/009-06/code-walkthrough/_demoapp show /tmp/schema-fix.db
```

```output
schema_version = 13
cataloged objects: 12

album_notes_fts          virtual-table   write=true  rowid=not-applicable shadow=false insertableCols=1
album_notes_fts_config   ordinary-table  write=true  rowid=without-rowid  shadow=false insertableCols=2
album_notes_fts_content  ordinary-table  write=true  rowid=has-rowid      shadow=false insertableCols=2
album_notes_fts_data     ordinary-table  write=true  rowid=has-rowid      shadow=false insertableCols=2
album_notes_fts_docsize  ordinary-table  write=true  rowid=has-rowid      shadow=false insertableCols=2
album_notes_fts_idx      ordinary-table  write=true  rowid=without-rowid  shadow=false insertableCols=3
albums                   ordinary-table  write=true  rowid=has-rowid      shadow=false insertableCols=2
big_auto                 ordinary-table  write=true  rowid=has-rowid      shadow=false insertableCols=1
generated_mix            ordinary-table  write=true  rowid=has-rowid      shadow=false insertableCols=1
kv_no_rowid              ordinary-table  write=true  rowid=without-rowid  shadow=false insertableCols=2
recent                   view            write=false rowid=not-applicable shadow=false insertableCols=0
shadowed_rowid           ordinary-table  write=true  rowid=has-rowid      shadow=true  insertableCols=2
```

Reserved-object exclusions are meaningful: the fixture genuinely contains both, and only the catalog excludes them:

```bash
cd /home/chris/sqloid && sqlite3 /tmp/schema-fix.db "SELECT name FROM main.sqlite_master WHERE name LIKE 'sqlite_%' OR name = '_cf_METADATA'" 2>/dev/null || go run ./Notes/walkthroughs/009-06/code-walkthrough/_demoapp show /tmp/schema-fix.db | grep -c 'sqlite_sequence\|_cf_METADATA'
```

```output
sqlite_sequence
_cf_METADATA
```

(The grep count 0 proves neither appears in the typed catalog while both exist in the database — the catalog_integration_test.go asserts existence directly against main.sqlite_master.) Column metadata: declared types pass through as pure metadata, virtual-table hidden columns and generated columns are noninsertable, and the view's columns are SELECT-only even though table_xinfo lists them:

```bash
cd /home/chris/sqloid && go run ./Notes/walkthroughs/009-06/code-walkthrough/_demoapp columns /tmp/schema-fix.db
```

```output
albums           id               type="INTEGER" hidden=false insertable=true
albums           title            type="TEXT"    hidden=false insertable=true
album_notes_fts  title            type=""        hidden=false insertable=true
album_notes_fts  album_notes_fts  type=""        hidden=true  insertable=false
album_notes_fts  rank             type=""        hidden=true  insertable=false
kv_no_rowid      code             type="TEXT"    hidden=false insertable=true
kv_no_rowid      v                type="TEXT"    hidden=false insertable=true
shadowed_rowid   rowid            type="TEXT"    hidden=false insertable=true
shadowed_rowid   n                type="INTEGER" hidden=false insertable=true
recent           id               type="INTEGER" hidden=false insertable=false
recent           title            type="TEXT"    hidden=false insertable=false
generated_mix    a                type="INTEGER" hidden=false insertable=true
generated_mix    b                type="INTEGER" hidden=true  insertable=false
generated_mix    c                type="INTEGER" hidden=true  insertable=false
```

Determinism (identical repeated reads) and refresh/drop behavior: a DDL change raises PRAGMA schema_version and the dropped object vanishes from the refreshed catalog — the exact mechanism pre-execution schema validation (Issue #21) will key on:

```bash
cd /home/chris/sqloid && go run ./Notes/walkthroughs/009-06/code-walkthrough/_demoapp determinism /tmp/schema-fix.db && go run ./Notes/walkthroughs/009-06/code-walkthrough/_demoapp drop-refresh /tmp/schema-fix.db
```

```output
schema_version = 13; repeated read deep-equal: true
version before = 13 after DDL refresh = 14 big_auto present = false
```

UI independence and no type-specific input behavior are structural: internal/schema imports no driver, Bubble Tea, internal/ui, or internal/querybuilder, and declared types are carried verbatim but consumed by nothing:

```bash
cd /home/chris/sqloid && echo 'import blocks of internal/schema production code:' && sed -n '/^import (/,/^)/p' internal/schema/schema.go internal/schema/catalog.go && echo && echo 'go list -deps of internal/schema containing ui/driver/querybuilder packages:' && go list -deps ./internal/schema | grep -E 'bubbletea|lipgloss|querybuilder|internal/ui$|modernc' || echo 'none'
```

```output
import blocks of internal/schema production code:
import (
	"sort"
	"strings"
)

go list -deps of internal/schema containing ui/driver/querybuilder packages:
none
```

The dependency graph confirms it: internal/schema pulls in nothing but the standard library — no driver, no Bubble Tea/Lip Gloss, no internal/ui, and no internal/querybuilder — while DeclaredType exists only as metadata (populated from table_xinfo and consumed by nothing). Closing verification pass:

```bash
cd /home/chris/sqloid && gofmt -l internal/schema internal/connection | wc -l && go vet ./... && go test ./... 2>&1 | grep -v '^ok' ; rm -f /tmp/schema-fix.db; echo VERIFIED
```

```output
0
VERIFIED
```
