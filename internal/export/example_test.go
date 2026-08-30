// Executable documentation for the Issue #48 standalone SQL serializer:
// representative statements containing difficult identifiers, quote-doubled
// and injection-looking TEXT, signed int64 boundaries, REAL edges, SQL NULL,
// BLOB payloads, UPDATE assignments, and INSERT Value/NULL/Default-Omit
// choices. Each example pins the exact statement bytes, including the single
// trailing semicolon, exactly as SaveTarget serialization emits them.

package export_test

import (
	"fmt"
	"math"

	"github.com/chris/sqloid/internal/export"
	qb "github.com/chris/sqloid/internal/querybuilder"
)

// updateState composes one UPDATE with the given SET assignments in order.
func updateSet(sets ...qb.HistorySetAssignment) qb.HistoryState {
	return qb.HistoryState{Command: qb.CommandUpdate, Table: "users", TableSet: true, Sets: sets}
}

// setVal builds one SET assignment with a parsed value.
func setVal(col string, v qb.Value) qb.HistorySetAssignment {
	return qb.HistorySetAssignment{Column: col, Choice: qb.SetChoiceValue, HasValue: true, Value: v}
}

// setNull returns one SQL-NULL SET choice.
func setNull(col string) qb.HistorySetAssignment {
	return qb.HistorySetAssignment{Column: col, Choice: qb.SetChoiceNull}
}

// withWhere composes a completed `=` predicate onto the state.
func withWhere(s qb.HistoryState, col string, v qb.Value) qb.HistoryState {
	s.WhereSet, s.WhereColumn, s.WhereOperator = true, col, qb.OpEq
	s.WhereHasValue = true
	s.WhereValue = v
	return s
}

// serialize calls export.SerializeSQLQuery, panicking on the impossible
// error of these complete examples.
func mustSerialize(state qb.HistoryState) string {
	s, err := export.SerializeSQLQuery(state)
	if err != nil {
		panic(err)
	}
	return s
}

// ExampleSerializeSQLQuery_select prints a representative SELECT with
// difficult schema-derived identifiers and the single trailing semicolon.
func ExampleSerializeSQLQuery_select() {
	state := qb.HistoryState{Command: qb.CommandSelect, Table: `schema"table`, TableSet: true,
		Projection: []qb.HistoryProjectionEntry{
			{Kind: qb.ProjectionColumn, Column: `we"ird`},
			{Kind: qb.ProjectionColumn, Column: "SELECT"},
			{Kind: qb.ProjectionColumn, Column: ""},
		},
	}
	fmt.Print(mustSerialize(state))
	// Output: SELECT "we""ird", "SELECT", "" FROM "schema""table";
}

// ExampleSerializeSQLQuery_update prints one qualified UPDATE with preserved
// SET order, an unsubmitted-value-free assignment set, and WHERE value.
func ExampleSerializeSQLQuery_update() {
	state := withWhere(updateSet(
		setNull("b"),
		setVal("email", qb.Value{Kind: qb.KindText, Text: `x'); DROP TABLE "users"; --`}),
		setVal("id", qb.Value{Kind: qb.KindInteger, Int: math.MaxInt64}),
	), "id", qb.Value{Kind: qb.KindInteger, Int: 5})
	fmt.Print(mustSerialize(state))
	// Output: UPDATE "users" SET "b" = NULL, "email" = 'x''); DROP TABLE "users"; --', "id" = 9223372036854775807 WHERE "id" = 5;
}

// ExampleSerializeSQLQuery_delete prints one unqualified DELETE (targets
// every row) and one qualified DELETE side by side.
func ExampleSerializeSQLQuery_delete() {
	unqualified := qb.HistoryState{Command: qb.CommandDelete, Table: `logs`, TableSet: true}
	qualified := withWhere(qb.HistoryState{Command: qb.CommandDelete, Table: "users", TableSet: true},
		"email", qb.Value{Kind: qb.KindText, Text: `a'b`})
	fmt.Print(mustSerialize(unqualified), "\n", mustSerialize(qualified))
	// Output:
	// DELETE FROM "logs";
	// DELETE FROM "users" WHERE "email" = 'a''b';
}

// ExampleSerializeSQLQuery_insert prints one INSERT with Value, NULL, and
// Default/Omit choices in schema prompt order plus REAL and BLOB edges.
func ExampleSerializeSQLQuery_insert() {
	state := qb.HistoryState{Command: qb.CommandInsert, Table: "t", TableSet: true,
		Inserts: []qb.HistoryInsertColumn{
			{Column: "a", Choice: qb.InsertChoiceValue, HasValue: true, Value: qb.Value{Kind: qb.KindText, Text: `it's "q"`}},
			{Column: "b", Choice: qb.InsertChoiceNull},
			{Column: "c", Choice: qb.InsertChoiceValue, HasValue: true, Value: qb.Value{Kind: qb.KindReal, Real: math.Copysign(0, -1)}},
			{Column: "d", Choice: qb.InsertChoiceOmit},
		},
	}
	fmt.Print(mustSerialize(state))
	// Output: INSERT INTO "t" ("a", "b", "c") VALUES ('it''s "q"', NULL, -0.0);
}

// ExampleSerializeSQLLiteral_blob prints empty and non-empty BLOB payloads
// through the typed atom, uppercase X and lowercase hex included.
func ExampleSerializeSQLLiteral() {
	for _, payload := range [][]byte{{}, {0x00, 0xDE, 0xAD, 0xFF}} {
		l, err := export.SerializeSQLLiteral(qb.Literal{Kind: qb.LiteralBlob, Blob: payload})
		if err != nil {
			fmt.Println("err:", err)
			continue
		}
		fmt.Print(l, " ")
	}
	// Output: X'' X'00deadff'
}
