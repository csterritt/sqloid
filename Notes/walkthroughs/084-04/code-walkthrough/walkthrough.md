# Issue #084 Code Walkthrough: Remove the Unused Rowid Enum Value

*2026-09-03T22:59:38Z by Showboat 0.6.1*
<!-- showboat-id: 8a50b54d-442c-42b4-ab52-3e7b0c94e281 -->

Issue #84 (Notes/tasks/084-remove-unused-rowid-enum-value.md, Notes/PRD-sqloid.md Schema scope and Schema metadata decisions) removes the unused `RowidApplicable` constant from `internal/schema/schema.go` and aligns the `RowidCapability` enum with the PRD's three-value capability set `{has-rowid, without-rowid, not-applicable}` and the project's other schema enums (`ObjectKind`, `RefreshStatus`, `RevalidateStatus`). Before the edit, a behavioral safety net in `internal/schema/catalog_test.go` pinned the exact `String()` output for the three meaningful values, the zero (unset) sentinel, and a representative unknown value, plus the kind/write-eligibility/rowid-capability classifications for an ordinary rowid table, a WITHOUT ROWID table, a virtual table, and a view. The edit itself only removes the obsolete identifier and types the constant block at `iota + 1`; it does not touch `BuildCatalog` classification, rowid-shadow detection, write eligibility, ordering, or any serialized/user-facing name. Reference: Issue #84 and Notes/PRD-sqloid.md. All artifacts are under Notes/walkthroughs/084-04/code-walkthrough/.

## The typed three-value constant declaration

The `RowidCapability` block now exposes only the three PRD-defined typed values. `RowidHas` is explicitly typed as `RowidCapability` beginning at `iota + 1`, the remaining constants inherit the type, and the zero value is reserved as an unset/unknown sentinel that `BuildCatalog` never assigns. This mirrors `ObjectKind`, `RefreshStatus`, and `RevalidateStatus`, which all start at `iota + 1` so zero is not a settled value.

```bash
sed -n '/\/\/ RowidCapability classifies whether/,/^)/p' /home/chris/sqloid/internal/schema/schema.go
```

```output
// RowidCapability classifies whether a rowid column may be addressed on an
// object, per the Schema metadata decision {has-rowid, without-rowid,
// not-applicable}. The zero value is not a meaningful capability: it is an
// unset/unknown sentinel that BuildCatalog never assigns.
type RowidCapability int

const (
	// RowidHas means the object supports ORDER BY / addressing by rowid,
	// including the case of an undeclared alias column.
	RowidHas RowidCapability = iota + 1
	// RowidWithout means the object is a WITHOUT ROWID table or otherwise
	// cannot be ordered by rowid but still accepts writes.
	RowidWithout
	// RowidNotApplicable means rowid ordering never applies to this kind of
	// object, such as views and virtual tables.
	RowidNotApplicable
)
```

## Exact strings and zero/unknown diagnostics

The `String()` method renders the three meaningful capabilities with their PRD names; the zero value and any unknown out-of-range value fall through to a typed `RowidCapability(%d)` diagnostic. The behavioral test pins all five.

```bash
sed -n '/\/\/ String renders the human-facing name of the capability/,/^}/p' /home/chris/sqloid/internal/schema/schema.go
```

```output
// String renders the human-facing name of the capability used in tests and
// diagnostics.
func (c RowidCapability) String() string {
	switch c {
	case RowidHas:
		return "has-rowid"
	case RowidWithout:
		return "without-rowid"
	case RowidNotApplicable:
		return "not-applicable"
	default:
		return fmt.Sprintf("RowidCapability(%d)", int(c))
	}
}
```

## Catalog classifications did not move

The focused classification safety net (`TestBuildCatalogRowidClassificationsLocked`) runs catalog fixtures for an ordinary rowid table, a WITHOUT ROWID table, a virtual table, and a view, asserting kind, write eligibility, and rowid capability are unchanged by the enum cleanup.

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/schema/ -run 'TestBuildCatalogRowidClassificationsLocked' -v
```

```output
=== RUN   TestBuildCatalogRowidClassificationsLocked
=== RUN   TestBuildCatalogRowidClassificationsLocked/ordinary_rowid_table
=== RUN   TestBuildCatalogRowidClassificationsLocked/WITHOUT_ROWID_table
=== RUN   TestBuildCatalogRowidClassificationsLocked/virtual_table
=== RUN   TestBuildCatalogRowidClassificationsLocked/view
--- PASS: TestBuildCatalogRowidClassificationsLocked (0.00s)
    --- PASS: TestBuildCatalogRowidClassificationsLocked/ordinary_rowid_table (0.00s)
    --- PASS: TestBuildCatalogRowidClassificationsLocked/WITHOUT_ROWID_table (0.00s)
    --- PASS: TestBuildCatalogRowidClassificationsLocked/virtual_table (0.00s)
    --- PASS: TestBuildCatalogRowidClassificationsLocked/view (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/schema	0.002s
```

The string/diagnostic safety net (`TestRowidCapabilityStrings`) pins the three meaningful names plus the zero and unknown diagnostics.

```bash
cd /home/chris/sqloid && go test -count=1 ./internal/schema/ -run 'TestRowidCapabilityStrings' -v
```

```output
=== RUN   TestRowidCapabilityStrings
--- PASS: TestRowidCapabilityStrings (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/schema	0.002s
```

## `RowidApplicable` is absent

A repository-wide search of Go source confirms the obsolete identifier is gone from production code; the only remaining mention is this walkthrough's own prose and a test comment describing the cleanup.

```bash
cd /home/chris/sqloid && rg -n 'RowidApplicable' --glob '*.go' || echo 'no matches in Go source'
```

```output
rg: failed to read the file specified in RIPGREP_CONFIG_PATH: /Users/chris/.config/ripgrep/config.rc: No such file or directory (os error 2)
internal/schema/catalog_test.go:328:// RowidApplicable cleanup so the enum edit cannot move a classification.
```

The single remaining mention is a descriptive comment in `internal/schema/catalog_test.go` (line 328) naming the cleanup; no code references the removed constant.

## Focused and repository-wide verification

The focused `internal/schema` package passes, and the repository-wide vet, test, and build all succeed so any compile-time use of the removed symbol would have been caught.

```bash
cd /home/chris/sqloid && go vet ./... && go test -count=1 ./internal/schema/ && go build ./...
```

```output
ok  	github.com/chris/sqloid/internal/schema	0.077s
```

## Documentation check

Task 3 reviewed the schema pages under `Notes/wiki` (`schema-catalog.md`, `source-code.md`, `unit-tests.md`, `schema-validation-workflow.md`, `index.md`, `log.md`). No page names `RowidApplicable` or claims a four-value capability model; every page already describes the three-value set `{has-rowid, without-rowid, not-applicable}`. No documentation change was required, so the wiki, index, and dated ingest log are left untouched. Reference: Issue #84 and Notes/PRD-sqloid.md.
