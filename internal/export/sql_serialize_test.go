// Standalone SQL serialization coverage for Issue #48, per the Query save
// targeting decision in Notes/PRD-sqloid.md. Every identifier, fixed SQL
// token, and INTEGER/REAL/TEXT/NULL/BLOB literal delegates to Issue #14's
// canonical atoms — there is no second literal serializer — and the result
// is exactly one standalone executable statement with exactly one trailing
// semicolon. Exact-byte tests pin the deterministic structure and ordering;
// modernc SQLite round trips prove every statement executes and preserves
// the typed value semantics.

package export

import (
	"database/sql"
	"math"
	"strings"
	"testing"

	qb "github.com/chris/sqloid/internal/querybuilder"

	_ "modernc.org/sqlite"
)

// selState returns a complete SELECT history state with the given projection
// entries and table.
func sel(table string, entries ...qb.HistoryProjectionEntry) qb.HistoryState {
	return qb.HistoryState{Command: qb.CommandSelect, Table: table, TableSet: true, Projection: entries}
}

// colEntry returns one plain column projection entry.
func colEntry(name string) qb.HistoryProjectionEntry {
	return qb.HistoryProjectionEntry{Kind: qb.ProjectionColumn, Column: name}
}

// aggEntry returns one aggregated column projection entry.
func aggEntry(name string, agg qb.Aggregate) qb.HistoryProjectionEntry {
	return qb.HistoryProjectionEntry{Kind: qb.ProjectionColumn, Column: name, Aggregate: agg}
}

// deleteState returns an unqualified DELETE state.
func deleteState(table string) qb.HistoryState {
	return qb.HistoryState{Command: qb.CommandDelete, Table: table, TableSet: true}
}

// textSet returns one submitted TEXT Value SET assignment.
func textSet(col, text string) qb.HistorySetAssignment {
	return qb.HistorySetAssignment{Column: col, Choice: qb.SetChoiceValue, HasValue: true,
		Value: qb.Value{Kind: qb.KindText, Text: text}}
}

// updateState builds an UPDATE with the given SET assignments in order.
func updateSets(table string, sets ...qb.HistorySetAssignment) qb.HistoryState {
	return qb.HistoryState{Command: qb.CommandUpdate, Table: table, TableSet: true, Sets: sets}
}

// nullSet returns one SQL-NULL SET choice.
func nullSet(col string) qb.HistorySetAssignment {
	return qb.HistorySetAssignment{Column: col, Choice: qb.SetChoiceNull}
}

// intSet returns one INTEGER Value SET assignment.
func intSet(col string, v int64) qb.HistorySetAssignment {
	return qb.HistorySetAssignment{Column: col, Choice: qb.SetChoiceValue, HasValue: true, Value: intValue(v)}
}

// realSet returns one REAL Value SET assignment.
func realSet(col string, v float64) qb.HistorySetAssignment {
	return qb.HistorySetAssignment{Column: col, Choice: qb.SetChoiceValue, HasValue: true, Value: realValue(v)}
}

// insertState builds an INSERT with the given per-column prompts.
func insertState(table string, inserts ...qb.HistoryInsertColumn) qb.HistoryState {
	return qb.HistoryState{Command: qb.CommandInsert, Table: table, TableSet: true, Inserts: inserts}
}

// insertValue returns one submitted Value prompt.
func insertValue(col string, v qb.Value) qb.HistoryInsertColumn {
	return qb.HistoryInsertColumn{Column: col, Choice: qb.InsertChoiceValue, HasValue: true, Value: v}
}

// insertNull returns one SQL-NULL prompt choice.
func insertNull(col string) qb.HistoryInsertColumn {
	return qb.HistoryInsertColumn{Column: col, Choice: qb.InsertChoiceNull}
}

// textValue returns a parsed TEXT universal value.
func textValue(s string) qb.Value { return qb.Value{Kind: qb.KindText, Text: s} }

// intValue returns an INTEGER value.
func intValue(v int64) qb.Value { return qb.Value{Kind: qb.KindInteger, Int: v} }

// realValue returns a REAL value.
func realValue(v float64) qb.Value { return qb.Value{Kind: qb.KindReal, Real: v} }

// whereText commits a `col op 'text'` predicate on the state.
func whereEqText(s qb.HistoryState, col, text string) qb.HistoryState {
	s.WhereSet, s.WhereColumn, s.WhereOperator = true, col, qb.OpEq
	s.WhereHasValue = true
	s.WhereValue = qb.Value{Kind: qb.KindText, Text: text}
	return s
}

// TestSerializeExactBytes pins the exact standalone statement bytes for
// every supported command shape: deterministic builder ordering, quoted
// schema-derived identifiers, typed literals from Issue #14's atoms, and
// exactly one trailing semicolon.
func TestSerializeExactBytes(t *testing.T) {
	cases := []struct {
		name string
		s    qb.HistoryState
		want string
	}{
		{
			name: "select plain columns in commit order",
			s:    sel("users", colEntry("id"), colEntry("email")),
			want: `SELECT "id", "email" FROM "users";`,
		},
		{
			name: "select wildcard with embedded-quote table",
			s:    sel(`t"1`, qb.HistoryProjectionEntry{Kind: qb.ProjectionWildcard}),
			want: `SELECT * FROM "t""1";`,
		},
		{
			name: "select where group order limit",
			s: qb.HistoryState{Command: qb.CommandSelect, Table: "t", TableSet: true,
				Projection: []qb.HistoryProjectionEntry{{Kind: qb.ProjectionColumn, Column: "a"}},
				WhereSet:   true, WhereColumn: "b", WhereOperator: qb.OpGe, WhereHasValue: true,
				WhereValue: qb.Value{Kind: qb.KindInteger, Int: 3},
				Groups:     []string{"a", `b"q`},
				OrderSet:   true, OrderExpression: "order-column:score", OrderDirection: qb.DirDesc,
				LimitHas: true, LimitValue: 7,
			},
			want: `SELECT "a" FROM "t" WHERE "b" >= 3 GROUP BY "a", "b""q" ORDER BY "score" DESC LIMIT 7;`,
		},
		{
			name: "select order by aggregate expression",
			s: qb.HistoryState{
				Command: qb.CommandSelect, Table: "t", TableSet: true,
				Projection: []qb.HistoryProjectionEntry{{Kind: qb.ProjectionColumn, Column: "amount", Aggregate: qb.AggSum}},
				OrderSet:   true, OrderExpression: "order-aggregate:amount:SUM", OrderDirection: qb.DirAsc,
			},
			want: `SELECT SUM("amount") FROM "t" ORDER BY SUM("amount") ASC;`,
		},
		{
			name: "select order by count star",
			s: qb.HistoryState{
				Command: qb.CommandSelect, Table: "t", TableSet: true,
				Projection: []qb.HistoryProjectionEntry{{Kind: qb.ProjectionCountStar}},
				OrderSet:   true, OrderExpression: "order-count-star", OrderDirection: qb.DirDesc,
			},
			want: `SELECT COUNT(*) FROM "t" ORDER BY COUNT(*) DESC;`,
		},
		{
			name: "select is not null where",
			s: qb.HistoryState{Command: qb.CommandSelect, Table: "t", TableSet: true,
				Projection: []qb.HistoryProjectionEntry{{Kind: qb.ProjectionColumn, Column: "a"}},
				WhereSet:   true, WhereColumn: "note", WhereOperator: qb.OpIsNotNull,
			},
			want: `SELECT "a" FROM "t" WHERE "note" IS NOT NULL;`,
		},
		{
			name: "unqualified update keeps SET order",
			s:    updateSets("users", nullSet("b"), textSet("a", `it's`)),
			want: `UPDATE "users" SET "b" = NULL, "a" = 'it''s';`,
		},
		{
			name: "qualified update with where value",
			s: qb.HistoryState{
				Command: qb.CommandUpdate, Table: "users", TableSet: true,
				Sets:     []qb.HistorySetAssignment{textSet("email", "new")},
				WhereSet: true, WhereColumn: "id", WhereOperator: qb.OpEq, WhereHasValue: true,
				WhereValue: intValue(5),
			},
			want: `UPDATE "users" SET "email" = 'new' WHERE "id" = 5;`,
		},
		{
			name: "unqualified delete",
			s:    deleteState("t"),
			want: `DELETE FROM "t";`,
		},
		{
			name: "qualified delete with like",
			s: qb.HistoryState{Command: qb.CommandDelete, Table: "t", TableSet: true,
				WhereSet: true, WhereColumn: "c", WhereOperator: qb.OpLike, WhereHasValue: true,
				WhereValue: qb.Value{Kind: qb.KindText, Text: "x%"},
			},
			want: `DELETE FROM "t" WHERE "c" LIKE 'x%';`,
		},
		{
			name: "insert keeps Value, NULL, and omitted choices",
			s: insertState("t",
				qb.HistoryInsertColumn{Column: "a", Choice: qb.InsertChoiceValue, HasValue: true, Value: qb.Value{Kind: qb.KindText, Text: `o'reilly`}},
				qb.HistoryInsertColumn{Column: "b", Choice: qb.InsertChoiceNull},
				qb.HistoryInsertColumn{Column: "c", Choice: qb.InsertChoiceOmit},
			),
			want: `INSERT INTO "t" ("a", "b") VALUES ('o''reilly', NULL);`,
		},
		{
			name: "all-omit insert renders default values",
			s:    insertState("t", qb.HistoryInsertColumn{Column: "a", Choice: qb.InsertChoiceOmit}),
			want: `INSERT INTO "t" DEFAULT VALUES;`,
		},
		{
			name: "difficult schema-derived identifiers",
			s:    sel(`schema"table`, colEntry(`we"ird`), colEntry("SELECT"), colEntry("")),
			want: `SELECT "we""ird", "SELECT", "" FROM "schema""table";`,
		},
		{
			name: "empty and injection-looking text",
			s:    updateSets("t", textSet("a", `x'); DROP TABLE "t"; --`)),
			want: `UPDATE "t" SET "a" = 'x''); DROP TABLE "t"; --';`,
		},
		{
			name: "max int64",
			s:    updateSets("t", intSet("a", math.MaxInt64)),
			want: `UPDATE "t" SET "a" = 9223372036854775807;`,
		},
		{
			name: "real integral identity",
			s:    updateSets("t", realSet("a", 5)),
			want: `UPDATE "t" SET "a" = 5.0;`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SerializeSQLQuery(tc.s)
			if err != nil {
				t.Fatalf("SerializeSQLQuery returned %v", err)
			}
			if got != tc.want {
				t.Fatalf("serialized bytes:\n got %q\nwant %q", got, tc.want)
			}
			// Exactly one trailing semicolon and no placeholder anywhere.
			if !strings.HasSuffix(got, ";") || strings.HasSuffix(got, ";;") {
				t.Fatalf("statement must carry exactly one trailing semicolon: %q", got)
			}
			if strings.Contains(got, "?") {
				t.Fatalf("placeholder leaked into standalone SQL: %q", got)
			}
		})
	}
}

// TestSerializeValueEdges pins the exact literal bytes of typed value edges
// through Issue #14's canonical atoms, delegated via Value.Literal.
func TestSerializeValueEdges(t *testing.T) {
	cases := []struct {
		name string
		s    qb.HistoryState
		want string
	}{
		{name: "min int64", s: updateSets("t", intSet("a", math.MinInt64)), want: `UPDATE "t" SET "a" = -9223372036854775808;`},
		{name: "max int64", s: updateSets("t", intSet("a", math.MaxInt64)), want: `UPDATE "t" SET "a" = 9223372036854775807;`},
		{name: "real integral identity", s: updateSets("t", realSet("a", 5)), want: `UPDATE "t" SET "a" = 5.0;`},
		{name: "negative zero", s: updateSets("t", realSet("a", math.Copysign(0, -1))), want: `UPDATE "t" SET "a" = -0.0;`},
		{name: "exponent", s: updateSets("t", realSet("a", 1e300)), want: `UPDATE "t" SET "a" = 1e+300;`},
		{name: "subnormal", s: updateSets("t", realSet("a", 5e-324)), want: `UPDATE "t" SET "a" = 5e-324;`},
		{name: "precision edge", s: updateSets("t", realSet("a", 0.1)), want: `UPDATE "t" SET "a" = 0.1;`},
		{name: "empty text", s: updateSets("t", textSet("a", "")), want: `UPDATE "t" SET "a" = '';`},
		{name: "quote doubled text", s: updateSets("t", textSet("a", `it's "quoted"`)), want: `UPDATE "t" SET "a" = 'it''s "quoted"';`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SerializeSQLQuery(tc.s)
			if err != nil {
				t.Fatalf("SerializeSQLQuery returned %v", err)
			}
			if got != tc.want {
				t.Fatalf("serialized bytes:\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

// TestSerializeRoundTrip executes serialized statements against modernc
// SQLite and proves the typed value semantics survive the round trip:
// quote-doubled TEXT, SQL NULL, signed int64 boundaries, REAL shortest
// round-trip identity, and empty and non-empty BLOB payloads. Every
// statement carries exactly one trailing semicolon and one statement.
func TestSerializeRoundTrip(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	schema := `CREATE TABLE "t" ("a" TEXT, "b" INTEGER, "c" REAL, "d" BLOB, "e" INTEGER);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("schema: %v", err)
	}

	stmt, err := SerializeSQLQuery(insertState("t",
		insertValue("a", textValue(`it's "q"`)),
		insertValue("b", intValue(math.MinInt64)),
		insertValue("c", realValue(1e300)),
		insertNull("d"),
		qb.HistoryInsertColumn{Column: "e", Choice: qb.InsertChoiceOmit},
	))
	if err != nil {
		t.Fatalf("serialize insert: %v", err)
	}
	if stmt, want := stmt, `INSERT INTO "t" ("a", "b", "c", "d") VALUES ('it''s "q"', -9223372036854775808, 1e+300, NULL);`; stmt != want {
		t.Fatalf("serialized insert:\n got %q\nwant %q", stmt, want)
	}
	if _, err := db.Exec(stmt); err != nil {
		t.Fatalf("serialized statement did not execute: %v", err)
	}
	var gotText string
	var gotInt int64
	var gotReal float64
	var gotNull any
	var gotOmitted any
	if err := db.QueryRow(`SELECT "a", "b", "c", "d", "e" FROM "t"`).Scan(&gotText, &gotInt, &gotReal, &gotNull, &gotOmitted); err != nil {
		t.Fatalf("round trip read: %v", err)
	}
	if gotText != `it's "q"` {
		t.Fatalf("TEXT round trip = %q", gotText)
	}
	if gotInt != math.MinInt64 {
		t.Fatalf("INTEGER round trip = %d", gotInt)
	}
	if gotReal != 1e300 {
		t.Fatalf("REAL round trip = %v", gotReal)
	}
	if gotNull != nil {
		t.Fatalf("NULL round trip = %v, want SQL NULL", gotNull)
	}
	if gotOmitted != nil {
		t.Fatalf("omitted column round trip = %v, want SQL NULL default", gotOmitted)
	}
}

// TestSerializeRoundTripEdges exercises the remaining semantic edges through
// SQLite: difficult schema-derived identifiers, unqualified and qualified
// UPDATE/DELETE effects on real rows, and empty and non-empty BLOB literals
// rendered through the typed atom.
func TestSerializeRoundTripEdges(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	schema := `CREATE TABLE "schema""table" ("we""ird" TEXT, "id" INTEGER, "n" BLOB);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("schema: %v", err)
	}

	// Difficult schema-derived identifiers round trip: the SELECT over the
	// quoted table and columns executes byte-for-byte.
	selectStmt, err := SerializeSQLQuery(sel(`schema"table`, colEntry(`we"ird`)))
	if err != nil {
		t.Fatalf("serialize select: %v", err)
	}
	rows, err := db.Query(selectStmt)
	if err != nil {
		t.Fatalf("difficult-identifier select failed: %v", err)
	}
	rows.Close()

	// Seed two rows; the unqualified UPDATE targets every row.
	for i := 1; i <= 2; i++ {
		seed, err := SerializeSQLQuery(insertState(`schema"table`, insertValue("id", intValue(int64(i)))))
		if err != nil {
			t.Fatalf("serialize seed: %v", err)
		}
		if _, err := db.Exec(seed); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	unqualified, err := SerializeSQLQuery(updateSets(`schema"table`, textSet(`we"ird`, "kept")))
	if err != nil {
		t.Fatalf("serialize unqualified update: %v", err)
	}
	if unqualified != `UPDATE "schema""table" SET "we""ird" = 'kept';` {
		t.Fatalf("unqualified update = %q", unqualified)
	}
	if _, err := db.Exec(unqualified); err != nil {
		t.Fatalf("unqualified update did not execute: %v", err)
	}

	// A qualified DELETE removes exactly the matching rows.
	qualified := deleteState(`schema"table`)
	qualified.WhereSet, qualified.WhereColumn, qualified.WhereOperator = true, "id", qb.OpEq
	qualified.WhereHasValue = true
	qualified.WhereValue = intValue(1)
	qualifiedDelete, err := SerializeSQLQuery(qualified)
	if err != nil {
		t.Fatalf("serialize delete: %v", err)
	}
	if qualifiedDelete != `DELETE FROM "schema""table" WHERE "id" = 1;` {
		t.Fatalf("qualified delete = %q", qualifiedDelete)
	}
	if _, err := db.Exec(qualifiedDelete); err != nil {
		t.Fatalf("delete round trip: %v", err)
	}

	// BLOB literals reach the statement only through the typed atom; empty
	// and non-empty payloads both round trip byte-for-byte.
	blobCases := []struct {
		name    string
		payload []byte
	}{
		{name: "empty blob", payload: []byte{}},
		{name: "non-empty blob", payload: []byte{0x00, 0xDE, 0xAD, 0xFF}},
	}
	for _, tc := range blobCases {
		t.Run(tc.name, func(t *testing.T) {
			literal, err := SerializeSQLLiteral(qb.Literal{Kind: qb.LiteralBlob, Blob: tc.payload})
			if err != nil {
				t.Fatalf("serialize blob literal: %v", err)
			}
			statement := `UPDATE "schema""table" SET "n" = ` + literal + ` WHERE "id" = 2;`
			if _, err := db.Exec(statement); err != nil {
				t.Fatalf("blob update did not execute: %v", err)
			}
			var got []byte
			if err := db.QueryRow(`SELECT "n" FROM "schema""table" WHERE "id" = 2`).Scan(&got); err != nil {
				t.Fatalf("blob read: %v", err)
			}
			if string(got) != string(tc.payload) {
				t.Fatalf("BLOB round trip = %v, want %v", got, tc.payload)
			}
		})
	}
}
