// Pure table-driven tests for SELECT LIMIT state (Issue #18 Task 5): the
// entered representation kept verbatim beside its optional accepted integer,
// empty input meaning the unbounded logical result, base-10 acceptance in
// [1, 9223372036854775807] with canonical rendering, and every invalid
// category — zero, negatives, signs, whitespace, decimal/exponent/hex forms,
// nonnumeric text, overflow — classified with the exact required reason.
// No universal REAL/TEXT coercion is reused: LIMIT owns one closed grammar.

package querybuilder

import (
	"testing"
)

// limitFixture returns a SELECT builder with users selected, mirroring the
// grouping fixture's shape so LIMIT composes over the same stable base.
func limitFixture() QueryBuilder { return groupFixture() }

// limitCase drives one table row: set the entered text, then assert the
// acceptance and validity contracts at the QueryBuilder boundary.
type limitCase struct {
	name      string
	input     string
	wantVal   int64  // accepted value when wantOK
	wantOK    bool   // whether an integer was accepted
	unbounded bool   // valid but no integer accepted (empty input)
	wantSQL   string // expected SelectSQL; "" means no LIMIT clause expected
}

func runLimitCase(t *testing.T, tc limitCase) {
	t.Helper()
	q := limitFixture()
	q = commitProjection(t, q, "id", AggregateValue)
	q = commitProjection(t, q, "email", AggregateValue)
	q = q.SetLimitInput(tc.input)
	if got := q.LimitInput(); got != tc.input {
		t.Fatalf("%s: LimitInput()=%q, want the entered representation verbatim %q",
			tc.name, got, tc.input)
	}
	val, ok := q.LimitValue()
	if ok != tc.wantOK || (ok && val != tc.wantVal) {
		t.Fatalf("%s: LimitValue()=(%d,%v), want (%d,%v)", tc.name, val, ok, tc.wantVal, tc.wantOK)
	}
	issue, invalid := q.FirstInvalidIssue()
	if tc.wantOK || tc.unbounded {
		if invalid {
			t.Fatalf("%s: unexpected first-invalid %+v", tc.name, issue)
		}
	} else {
		if !invalid || issue.Field != FieldIdentityLimit || issue.Reason != LimitInvalidReason {
			t.Fatalf("%s: first-invalid=(%+v), want Limit/%s", tc.name, issue, LimitInvalidReason)
		}
	}
	if tc.wantSQL == "" {
		if sql := q.SelectSQL(); len(sql) > 0 && containsLimit(sql) {
			t.Fatalf("%s: SelectSQL()=%q unexpectedly carries a LIMIT clause", tc.name, sql)
		}
		return
	}
	if got := q.SelectSQL(); got != tc.wantSQL {
		t.Fatalf("%s: SelectSQL()=%q, want %q", tc.name, got, tc.wantSQL)
	}
}

// containsLimit reports whether a rendered statement contains the LIMIT
// keyword; used when no clause is expected.
func containsLimit(sql string) bool {
	for i := 0; i+len("LIMIT ") <= len(sql); i++ {
		if sql[i:i+len("LIMIT ")] == "LIMIT " {
			return true
		}
	}
	return false
}

func TestLimitAcceptedRangeAndCanonicalRendering(t *testing.T) {
	cases := []limitCase{
		{name: "empty means unbounded", input: "", unbounded: true, wantSQL: ""},
		{name: "one", input: "1", wantVal: 1, wantOK: true, wantSQL: `SELECT "id", "email" FROM "users" LIMIT 1`},
		{name: "signed-int64 maximum", input: "9223372036854775807", wantVal: 9223372036854775807, wantOK: true,
			wantSQL: `SELECT "id", "email" FROM "users" LIMIT 9223372036854775807`},
		{name: "leading zeros render canonically", input: "007", wantVal: 7, wantOK: true,
			wantSQL: `SELECT "id", "email" FROM "users" LIMIT 7`},
	}
	for _, tc := range cases {
		runLimitCase(t, tc)
	}
}

func TestLimitInvalidCategoriesReportExactReason(t *testing.T) {
	cases := []limitCase{
		{name: "zero", input: "0"},
		{name: "zero padded", input: "000"},
		{name: "negative", input: "-3"},
		{name: "leading plus", input: "+5"},
		{name: "whitespace prefix", input: " 5"},
		{name: "whitespace suffix", input: "5 "},
		{name: "decimal form", input: "5.5"},
		{name: "exponent form", input: "1e3"},
		{name: "hex form", input: "0x10"},
		{name: "nonnumeric text", input: "many"},
		{name: "signed-int64 overflow", input: "9223372036854775808"},
		{name: "extremely long input", input: "1" + "000000000000000000000000000000000000000"},
	}
	for _, tc := range cases {
		tc.wantOK = false
		runLimitCase(t, tc)
	}
}

func TestLimitRevisionFromValidToInvalidAndEmpty(t *testing.T) {
	q := limitFixture().SetLimitInput("5")
	if v, ok := q.LimitValue(); !ok || v != 5 {
		t.Fatalf("setup LimitValue()=(%d,%v), want (5,true)", v, ok)
	}
	// Revision to an invalid representation preserves the entered text and
	// the exact reason, never the previously accepted integer.
	q = q.SetLimitInput("9223372036854775808")
	if got := q.LimitInput(); got != "9223372036854775808" {
		t.Fatalf("LimitInput()=%q, want the new invalid text preserved", got)
	}
	if _, ok := q.LimitValue(); ok {
		t.Fatal("revision to invalid input kept the prior accepted value")
	}
	issue, invalid := q.FirstInvalidIssue()
	if !invalid || issue.Field != FieldIdentityLimit || issue.Reason != LimitInvalidReason {
		t.Fatalf("first-invalid=(%+v), want Limit/%s", issue, LimitInvalidReason)
	}
	// Revision back to empty returns to the unbounded logical result.
	q = q.SetLimitInput("")
	if got := q.LimitInput(); got != "" {
		t.Fatalf("LimitInput()=%q, want empty", got)
	}
	if _, ok := q.LimitValue(); ok {
		t.Fatal("empty input reported an accepted value")
	}
	if issue, invalid := q.FirstInvalidIssue(); invalid {
		t.Fatalf("empty Limit reported %+v", issue)
	}
	if sql := q.SelectSQL(); containsLimit(sql) {
		t.Fatalf("SelectSQL()=%q, want no LIMIT clause", sql)
	}
}

func TestLimitRenderingComposesWithGroupingAndOrdering(t *testing.T) {
	q := limitFixture()
	q = commitProjection(t, q, "id", AggCount)
	q = commitProjection(t, q, "email", AggregateValue)
	q, _ = q.AcceptGroupColumn("email")
	q, _ = q.AcceptOrderBy("order-column:email")
	q = q.SetOrderDirection(DirDesc)
	q = q.SetLimitInput("10")
	want := `SELECT COUNT("id"), "email" FROM "users" GROUP BY "email" ORDER BY "email" DESC LIMIT 10`
	if got := q.SelectSQL(); got != want {
		t.Fatalf("SelectSQL()=%q, want %q", got, want)
	}
	// Invalid input renders nothing and never interpolates the entered text.
	q = q.SetLimitInput("-1")
	if got := q.SelectSQL(); containsLimit(got) {
		t.Fatalf("SelectSQL()=%q carried invalid limit input", got)
	}
	if params := q.SelectParams(); len(params) != 0 {
		t.Fatalf("limit introduced params %v", params)
	}
}
