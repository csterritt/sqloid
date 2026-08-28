// Pure table-driven tests for the reusable guided WHERE predicate state, per
// Issue #17 Task 1 and the Query Grammar and SQL safety decisions in
// Notes/PRD-sqloid.md.
//
// Covered contracts: eligible columns derive from internal/schema metadata for
// the SELECT/UPDATE/DELETE consumers; the closed fixed operator set is offered
// identically for every column with no declared-type filtering; IS NULL and
// IS NOT NULL become complete immediately after operator selection, emit no
// placeholder and bind no parameter, and discard any stale value state; every
// other operator stays incomplete until value submission and then renders
// exactly one '?' placeholder with the universally parsed bound value at its
// concrete Go type — including typed `NULL` and empty input as TEXT and LIKE
// wildcards bound byte-for-byte; identifiers are safely quoted schema-derived
// atoms; the rendering/parameter contract is consumed unchanged by every WHERE
// consumer; every transition is immutable; and ineligible identities or
// invalid operators are rejected defensively without changing state.

package querybuilder

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/chris/sqloid/internal/schema"
)

// whereObject is one eligible typed object identity carrying a hidden column
// to prove eligibility filtering, an untyped column to prove that no declared
// type influences anything, and punctuation-shaped names to pin identifier
// quoting.
func whereObject() *schema.Object {
	return &schema.Object{
		Name:            "users",
		Kind:            schema.KindOrdinaryTable,
		WriteEligible:   true,
		Rowid:           schema.RowidHas,
		InsertableCount: 4,
		Columns: []schema.Column{
			{Name: "id", DeclaredType: "INTEGER"},
			{Name: "email", DeclaredType: "TEXT"},
			{Name: "note", DeclaredType: ""},
			{Name: `secret"x`, DeclaredType: "BLOB", Hidden: true},
		},
	}
}

// whereBuilder returns a builder with cmd chosen, users selected, and a
// refreshed catalog installed behind it.
func whereBuilder(cmd Command) QueryBuilder {
	q := NewQuery().SelectCommand(cmd).
		RefreshSchema(&schema.Catalog{Version: 3, Objects: []*schema.Object{whereObject()}})
	return q.SelectTable("users")
}

// TestWhereCandidatesDeriveFromSchemaPerCommand pins the eligible-column
// derivation for every WHERE consumer: visible columns of the selected object
// in declared order, identically for SELECT, UPDATE, and DELETE; empty with
// no table; never including hidden columns; and never offered to INSERT.
func TestWhereCandidatesDeriveFromSchemaPerCommand(t *testing.T) {
	want := []schema.Column{whereObject().Columns[0], whereObject().Columns[1], whereObject().Columns[2]}
	for _, cmd := range []Command{CommandSelect, CommandUpdate, CommandDelete} {
		got := whereBuilder(cmd).WhereCandidates()
		if !slices.Equal(got, want) {
			t.Errorf("%v: WhereCandidates()=%v, want %v", cmd, got, want)
		}
	}
	if got := whereBuilder(CommandInsert).WhereCandidates(); len(got) != 0 {
		t.Errorf("INSERT: WhereCandidates()=%v, want none", got)
	}
	noTable := NewQuery().SelectCommand(CommandSelect).RefreshSchema(
		&schema.Catalog{Version: 3, Objects: []*schema.Object{whereObject()}})
	if got := noTable.WhereCandidates(); len(got) != 0 {
		t.Errorf("no table: WhereCandidates()=%v, want none", got)
	}
}

// TestFixedOperatorsClosedAndDeterministicOrder pins the exhaustive fixed
// operator set and its presentation order, unchanged by beginning a draft on
// any eligible column regardless of its declared type.
func TestFixedOperatorsClosedAndDeterministicOrder(t *testing.T) {
	want := []Operator{OpEq, OpNotEq, OpLt, OpLe, OpGt, OpGe, OpIsNull, OpIsNotNull, OpLike}
	for _, cmd := range []Command{CommandSelect, CommandUpdate, CommandDelete} {
		if got := whereBuilder(cmd).FixedOperators(); !slices.Equal(got, want) {
			t.Errorf("%v: FixedOperators()=%v, want %v", cmd, got, want)
		}
		for _, col := range whereBuilder(cmd).WhereCandidates() {
			begin, ok := whereBuilder(cmd).StartWhere(col.Name)
			if !ok {
				t.Fatalf("%v: StartWhere(%q) rejected", cmd, col.Name)
			}
			if got := begin.FixedOperators(); !slices.Equal(got, want) {
				t.Errorf("%v/%s: operators after begin=%v, want unchanged %v", cmd, col.Name, got, want)
			}
		}
	}
}

// TestNullOperatorsCompleteWithoutValueOrParameter requires IS NULL and
// IS NOT NULL to reach WhereComplete immediately after operator selection,
// render their exact predicate text with no placeholder, bind nothing, and
// discard any earlier submitted value state.
func TestNullOperatorsCompleteWithoutValueOrParameter(t *testing.T) {
	col := schema.Column{Name: "gone"}
	for _, tc := range []struct {
		op  Operator
		sql string
	}{{OpIsNull, `"gone" IS NULL`}, {OpIsNotNull, `"gone" IS NOT NULL`}} {
		p, ok := AbsentWhere().SelectColumn(col).ChooseOperator(OpEq)
		if !ok {
			t.Fatal("stale-value setup operator choice failed")
		}
		stale, ok := p.SubmitValue("stale junk")
		if !ok || stale.State() != WhereComplete {
			t.Fatal("stale setup did not produce a completed valued predicate")
		}
		done, ok := stale.ChooseOperator(tc.op)
		if !ok {
			t.Fatalf("choosing %v failed", tc.op)
		}
		if done.State() != WhereComplete {
			t.Errorf("after %v: State()=%v, want immediate WhereComplete", tc.op, done.State())
		}
		if _, hasVal := done.SubmittedValue(); hasVal {
			t.Errorf("after %v: stale value state survived", tc.op)
		}
		if done.SQL() != tc.sql {
			t.Errorf("after %v: SQL=%s, want %s", tc.op, done.SQL(), tc.sql)
		}
		if strings.Contains(done.SQL(), "?") {
			t.Errorf("after %v: SQL %q leaked a placeholder", tc.op, done.SQL())
		}
		if got := done.Params(); got != nil {
			t.Errorf("after %v: Params()=%v, want nil", tc.op, got)
		}
	}
}

// whereSubmitCases enumerate every universal value shape the guided entry
// must carry verbatim into its single bound parameter, per the universal
// parsing contract of Issue #14.
var whereSubmitCases = []struct {
	in        string
	boundType reflect.Type
}{
	{in: "42", boundType: reflect.TypeOf(int64(0))},
	{in: "-7", boundType: reflect.TypeOf(int64(0))},
	{in: "3.14", boundType: reflect.TypeOf(float64(0))},
	{in: "NULL", boundType: reflect.TypeOf("")},
	{in: "null", boundType: reflect.TypeOf("")},
	{in: "", boundType: reflect.TypeOf("")},
	{in: " x ", boundType: reflect.TypeOf("")},
	{in: "%", boundType: reflect.TypeOf("")},
	{in: "_", boundType: reflect.TypeOf("")},
	{in: "a%b_c%d", boundType: reflect.TypeOf("")},
	{in: "' OR '1'='1", boundType: reflect.TypeOf("")},
}

// TestEveryOperatorOnEveryColumnViaBuilder walks each WHERE consumer's whole
// eligible-column list across every fixed operator: value-taking operators
// must remain incomplete until submission and then render exactly one '?'
// with the exact parsed bound value and concrete bound type, with no entered
// representation interpolated anywhere.
func TestEveryOperatorOnEveryColumnViaBuilder(t *testing.T) {
	valueOps := []Operator{OpEq, OpNotEq, OpLt, OpLe, OpGt, OpGe, OpLike}
	for _, cmd := range []Command{CommandSelect, CommandUpdate, CommandDelete} {
		for _, col := range whereBuilder(cmd).WhereCandidates() {
			begin, ok := whereBuilder(cmd).StartWhere(col.Name)
			if !ok {
				t.Fatalf("%v: StartWhere(%q) rejected", cmd, col.Name)
			}
			if draft := begin.WhereDraft(); draft.State() != WhereColumnChosen {
				t.Errorf("%v/%s: fresh draft state=%v, want WhereColumnChosen", cmd, col.Name, draft.State())
			} else if got, ok := draft.Column(); !ok || got != col {
				t.Errorf("%v/%s: draft column=(%v,%v), want %v", cmd, col.Name, got, ok, col)
			}
			for _, op := range valueOps {
				awaiting, ok := begin.WhereDraft().ChooseOperator(op)
				if !ok {
					t.Fatalf("%v/%s: choosing %v failed", cmd, col.Name, op)
				}
				if awaiting.State() != WhereAwaitingValue {
					t.Errorf("%v/%s/%v: state=%v, want WhereAwaitingValue", cmd, col.Name, op, awaiting.State())
				}
				if awaiting.SQL() != "" {
					t.Errorf("%v/%s/%v: SQL=%q before submission, want empty", cmd, col.Name, op, awaiting.SQL())
				}
				token, err := op.SQLToken()
				if err != nil {
					t.Fatalf("token lookup failed: %v", err)
				}
				wantSQL := quoteForTest(col.Name) + " " + token + " ?"
				for _, sc := range whereSubmitCases {
					done, ok := awaiting.SubmitValue(sc.in)
					if !ok {
						t.Fatalf("%v/%s/%v: submitting %q failed", cmd, col.Name, op, sc.in)
					}
					if done.State() != WhereComplete {
						t.Errorf("%v/%s/%v input %q: State()=%v, want WhereComplete", cmd, col.Name, op, sc.in, done.State())
					}
					if done.SQL() != wantSQL {
						t.Errorf("%v/%s/%v input %q: SQL=%s, want %s", cmd, col.Name, op, sc.in, done.SQL(), wantSQL)
					}
					params := done.Params()
					if len(params) != 1 {
						t.Fatalf("%v/%s/%v input %q: bound %d params, want exactly 1", cmd, col.Name, op, sc.in, len(params))
					}
					v := ParseValue(sc.in)
					if reflect.TypeOf(params[0]) != sc.boundType ||
						!reflect.DeepEqual(params[0], v.ParamValue()) {
						t.Errorf("%v/%s/%v input %q: param %#v (%T), want %#v (%T)",
							cmd, col.Name, op, sc.in, params[0], params[0], v.ParamValue(), v.ParamValue())
					}
					if v.Kind == KindText && sc.in != "" && strings.Contains(done.SQL(), sc.in) {
						t.Errorf("%v/%s/%v input %q: entered text leaked into SQL %s", cmd, col.Name, op, sc.in, done.SQL())
					}
				}
			}
		}
	}
}

// quoteForTest wraps an identifier through the production atom quoter so
// tests never reimplement quoting rules.
func quoteForTest(name string) string {
	return ColumnIdentifier(schema.Column{Name: name})
}

// TestTypedNullCompletesThroughBuilder end-to-end requires typed `NULL`
// submitted on '=' to commit as a TEXT-bound parameter at the consumer
// boundary: identical shape to any other TEXT value and retaining its
// concrete string type rather than becoming SQL null.
func TestTypedNullCompletesThroughBuilder(t *testing.T) {
	b := whereBuilder(CommandDelete)
	begin, ok := b.StartWhere("email")
	if !ok {
		t.Fatal("begin rejected")
	}
	draft, _ := begin.WhereDraft().ChooseOperator(OpEq)
	done, ok := draft.SubmitValue("NULL")
	if !ok {
		t.Fatal("submit failed")
	}
	next, ok := begin.ApplyWhereDraft(done).CommitWhereDraft()
	if !ok {
		t.Fatal("commit rejected a complete draft")
	}
	p := next.WherePredicate()
	if p.SQL() != `"email" = ?` {
		t.Errorf("committed SQL=%s, want \"email\" = ?", p.SQL())
	}
	params := next.WhereParams()
	if len(params) != 1 {
		t.Fatalf("committed %d params, want 1", len(params))
	}
	if s, isText := params[0].(string); !isText || s != "NULL" {
		t.Errorf("bound param %#v (%T), want the verbatim TEXT \"NULL\"", params[0], params[0])
	}
}

// TestLikeWildcardsBoundVerbatimAbsentFromSQL requires '%' and '_' LIKE text
// to bind byte-for-byte with no escaping or interpolation and no appearance
// in the executable SQL text, preserving SQLite's wildcard meaning.
func TestLikeWildcardsBoundVerbatimAbsentFromSQL(t *testing.T) {
	for _, in := range []string{"%", "_", "50%", "_attern"} {
		p, ok := AbsentWhere().SelectColumn(schema.Column{Name: "name"}).ChooseOperator(OpLike)
		if !ok {
			t.Fatal("LIKE choice failed")
		}
		done, ok := p.SubmitValue(in)
		if !ok {
			t.Fatalf("submitting %q failed", in)
		}
		if done.SQL() != `"name" LIKE ?` {
			t.Errorf("input %q: SQL=%s, want \"name\" LIKE ?", in, done.SQL())
		}
		params := done.Params()
		if len(params) != 1 || params[0] != in {
			t.Errorf("input %q: params=%v, want byte-for-byte [%q]", in, params, in)
		}
		if strings.Contains(done.SQL(), in) {
			t.Errorf("input %q: wildcard text appeared in SQL %s", in, done.SQL())
		}
	}
}

// TestPredicateStatesStructurallyDistinct pins that absent, mid-selection
// (column chosen), awaiting-value, and complete are distinguishable through
// the typed accessors alone — no booleans or recomputation required.
func TestPredicateStatesStructurallyDistinct(t *testing.T) {
	col := schema.Column{Name: "note"}

	absent := AbsentWhere()
	if absent.State() != WhereAbsent {
		t.Fatalf("absent state=%v, want WhereAbsent", absent.State())
	}
	if _, ok := absent.Column(); ok {
		t.Error("absent exposed a column")
	}
	if _, ok := absent.ChosenOperator(); ok {
		t.Error("absent exposed an operator")
	}
	if _, ok := absent.SubmittedValue(); ok {
		t.Error("absent exposed a value")
	}
	if absent.SQL() != "" || absent.Params() != nil {
		t.Error("absent rendered output")
	}

	chosen := absent.SelectColumn(col)
	if chosen.State() != WhereColumnChosen {
		t.Errorf("column-chosen state=%v, want WhereColumnChosen", chosen.State())
	}
	if got, ok := chosen.Column(); !ok || got.Name != "note" {
		t.Errorf("chosen column=(%v,%v), want note", got, ok)
	}
	if _, ok := chosen.ChosenOperator(); ok {
		t.Error("column-chosen exposed an operator")
	}

	awaiting, _ := chosen.ChooseOperator(OpGe)
	if awaiting.State() != WhereAwaitingValue {
		t.Errorf("awaiting state=%v, want WhereAwaitingValue", awaiting.State())
	}
	if _, ok := awaiting.SubmittedValue(); ok {
		t.Error("awaiting exposed a submitted value")
	}

	complete, _ := awaiting.SubmitValue("-9223372036854775808")
	if complete.State() != WhereComplete {
		t.Errorf("complete state=%v, want WhereComplete", complete.State())
	}
	v, ok := complete.SubmittedValue()
	if !ok || v.Kind != KindInteger || v.Int != -9223372036854775808 {
		t.Errorf("submitted value=(%#v,%v), want min-int64 INTEGER", v, ok)
	}
}

// TestPredicateTransitionsAreImmutable preserves the receiver across every
// transition: each snapshot keeps its prior structural state untouched.
func TestPredicateTransitionsAreImmutable(t *testing.T) {
	col := schema.Column{Name: "note"}
	start := AbsentWhere()

	chosen := start.SelectColumn(col)
	if start.State() != WhereAbsent {
		t.Error("SelectColumn mutated its receiver")
	}
	awaiting, ok := chosen.ChooseOperator(OpLt)
	if !ok {
		t.Fatal("op choose failed")
	}
	if chosen.State() != WhereColumnChosen || start.State() != WhereAbsent {
		t.Error("an intermediate snapshot was mutated")
	}
	done, _ := awaiting.SubmitValue("5")
	if awaiting.State() != WhereAwaitingValue {
		t.Error("SubmitValue mutated the awaiting snapshot")
	}
	if done.Params()[0] != int64(5) {
		t.Errorf("completed param=%v, want int64(5)", done.Params()[0])
	}
}

// TestWhereIdentifiersAreSafelyQuoted requires schema-derived column
// identities to render as safely quoted atoms in every completed predicate,
// whatever keyword- or punctuation-shaped names they carry.
func TestWhereIdentifiersAreSafelyQuoted(t *testing.T) {
	cases := []struct {
		name    string
		op      Operator
		suffix  string
		submits bool
	}{
		{name: `tricky""name`, op: OpIsNull, suffix: " IS NULL"},
		{name: "'; DROP TABLE users--", op: OpEq, suffix: " = ?", submits: true},
		{name: "select", op: OpIsNotNull, suffix: " IS NOT NULL"},
		{name: "?", op: OpLike, suffix: " LIKE ?", submits: true},
	}
	for _, tc := range cases {
		p, ok := AbsentWhere().SelectColumn(schema.Column{Name: tc.name}).ChooseOperator(tc.op)
		if !ok {
			t.Fatalf("case %q operator choice failed", tc.name)
		}
		out := p
		if tc.submits {
			out, ok = p.SubmitValue("1")
			if !ok {
				t.Fatalf("case %q submit failed", tc.name)
			}
		}
		if out.SQL() != quoteForTest(tc.name)+tc.suffix {
			t.Errorf("input %q: SQL=%s, want quoted atom plus %q", tc.name, out.SQL(), tc.suffix)
		}
	}
}

// TestWhereContractConsumedUnchangedByAllConsumers requires the identical
// committed predicate to render identical SQL and parameters whether reached
// through the SELECT, UPDATE, or DELETE consumer boundary.
func TestWhereContractConsumedUnchangedByAllConsumers(t *testing.T) {
	var sqls [3]string
	var params [3][]any
	for i, cmd := range []Command{CommandSelect, CommandUpdate, CommandDelete} {
		b := whereBuilder(cmd)
		begin, ok := b.StartWhere("email")
		if !ok {
			t.Fatal("start rejected")
		}
		draft, _ := begin.WhereDraft().ChooseOperator(OpGe)
		done, _ := draft.SubmitValue("2.5")
		final, ok := begin.ApplyWhereDraft(done).CommitWhereDraft()
		if !ok {
			t.Fatal("commit rejected")
		}
		sqls[i] = final.WherePredicate().SQL()
		params[i] = final.WhereParams()
	}
	if sqls[0] != sqls[1] || sqls[1] != sqls[2] {
		t.Errorf("consumer SQL diverged: %q / %q / %q", sqls[0], sqls[1], sqls[2])
	}
	for i, p := range params {
		if len(p) != 1 || !reflect.DeepEqual(p[0], 2.5) {
			t.Errorf("consumer %d params=%v, want [2.5]", i, p)
		}
	}
}

// TestDefensiveRejections guards the builder seams: no table, INSERT, an
// unknown or hidden column identity, invalid operators, submissions outside
// the awaiting-value state, committing or canceling with no draft, and idle
// queries are all refused without changing any builder state.
func TestDefensiveRejections(t *testing.T) {
	// No table selected yet.
	if _, ok := NewQuery().SelectCommand(CommandSelect).StartWhere("id"); ok {
		t.Error("BeginWhere succeeded without a selected table")
	}
	// INSERT owns no WHERE.
	if _, ok := whereBuilder(CommandInsert).StartWhere("id"); ok {
		t.Error("INSERT began a WHERE draft")
	}
	// Unknown and hidden columns are rejected identities.
	b := whereBuilder(CommandDelete)
	for _, bad := range []string{"nope", `secret"x`, ""} {
		if _, ok := b.StartWhere(bad); ok {
			t.Errorf("StartWhere(%q) accepted an ineligible identity", bad)
		}
	}
	if b.HasWhere() {
		t.Error("rejected starts left committed WHERE state behind")
	}

	// Invalid operators are refused and leave the draft usable.
	begin, _ := b.StartWhere("id")
	draft := begin.WhereDraft()
	for _, bad := range []Operator{0, 10} {
		if _, ok := draft.ChooseOperator(bad); ok {
			t.Errorf("invalid operator %d accepted", int(bad))
		}
	}
	if _, ok := draft.ChooseOperator(OpEq); !ok {
		t.Fatal("a valid operator was rejected after invalid attempts")
	}
	if _, ok := draft.ChosenOperator(); ok {
		t.Error("the unmutated draft gained an operator defensively")
	}

	// Submission is meaningful only while awaiting a value.
	fresh := AbsentWhere().SelectColumn(schema.Column{Name: "id"})
	if _, ok := fresh.SubmitValue("1"); ok {
		t.Error("submission accepted while column-chosen")
	}
	if _, ok := AbsentWhere().SubmitValue("1"); ok {
		t.Error("submission accepted while absent")
	}
	nullP, _ := fresh.ChooseOperator(OpIsNull)
	if _, ok := nullP.SubmitValue("1"); ok {
		t.Error("submission accepted after a no-value operator")
	}

	// Commit requires a complete draft; an incomplete one commits nothing.
	halfDone, _ := fresh.ChooseOperator(OpEq)
	next, ok := begin.ApplyWhereDraft(halfDone).CommitWhereDraft()
	if ok || next.HasWhere() {
		t.Error("an incomplete draft committed")
	}

	// Without ever beginning, cancel, commit, and draft queries stay inert.
	idle := whereBuilder(CommandSelect)
	cancelled := idle.CancelWhereDraft()
	if cancelled.HasWhere() || cancelled.WhereDrafting() {
		t.Error("idle cancel changed state")
	}
	if _, ok := idle.CommitWhereDraft(); ok {
		t.Error("idle commit succeeded")
	}
	if d := idle.WhereDraft(); d.State() != WhereAbsent {
		t.Errorf("idle draft=%v, want WhereAbsent", d.State())
	}
	// A command replacement discards committed WHERE state downstream.
	first, _ := idle.StartWhere("id")
	fv, _ := first.WhereDraft().ChooseOperator(OpIsNull)
	committed, ok := first.ApplyWhereDraft(fv).CommitWhereDraft()
	if !ok || !committed.HasWhere() {
		t.Fatal("setup commitment failed")
	}
	cleared := committed.SelectCommand(CommandUpdate)
	if cleared.HasWhere() || cleared.WhereDrafting() {
		t.Error("command replacement kept downstream WHERE state")
	}
}
