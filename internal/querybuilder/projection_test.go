// Pure table-driven tests for the SELECT wildcard and COUNT(*) projection
// path, per Issue #15 Task 1: empty-projection candidate ordering with `*`
// first/default and synthetic bare COUNT(*) second, conditional sentinel
// visibility, direct wildcard/sentinel transitions that reopen Column(s),
// named-column continuation to aggregate selection, and sentinel reappearance
// when removal empties the projection.
//
// These tests use typed visible columns from internal/schema only; there is no
// Bubble Tea dependency, no popup logic, and no SQL construction here. General
// ordered-editing and deduplication rules belong to Issue #16.

package querybuilder

import (
	"testing"

	"github.com/chris/sqloid/internal/schema"
)

// projFixture returns an ordinary SELECT table with two visible columns, one
// hidden column absent from candidate lists, and no duplicates.
func projFixture() QueryBuilder {
	obj := &schema.Object{
		Name:          "users",
		Kind:          schema.KindOrdinaryTable,
		WriteEligible: true,
		Rowid:         schema.RowidHas,
		Columns: []schema.Column{
			{Name: "id"},
			{Name: "email"},
			{Name: "created_at", Hidden: true},
		},
	}
	return selectBuilderFor(obj)
}

// selectBuilderFor returns a SELECT builder pointed at obj, reusable for
// edge-case catalogs built inline.
func selectBuilderFor(obj *schema.Object) QueryBuilder {
	return NewQuery().
		RefreshSchema(&schema.Catalog{Version: 3, Objects: []*schema.Object{obj}}).
		SelectCommand(CommandSelect).
		SelectTable(obj.Name)
}

func sameCandidate(a, b ProjectionCandidate) bool {
	return a.Kind == b.Kind && a.Column == b.Column
}

func sameEntries(a, b []ProjectionEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Kind != b[i].Kind || a[i].Column != b[i].Column || a[i].Aggregate != b[i].Aggregate {
			return false
		}
	}
	return true
}

// TestEmptyProjectionCandidatesOrderRequiresWildcardFirstCountStarSecondNamesAfter
// pins the empty Column(s) candidate derivation: wildcard `*` first (and thus
// the popup's default highlight), synthetic bare COUNT(*) second, then every
// visible column in Schema order — with hidden columns excluded entirely.
func TestEmptyProjectionCandidatesOrderRequiresWildcardFirstCountStarSecondNamesAfter(t *testing.T) {
	q := projFixture()
	got := q.ProjectionCandidates()
	want := []ProjectionCandidate{
		{Kind: ProjectionWildcard},
		{Kind: ProjectionCountStar},
		{Kind: ProjectionColumn, Column: "id"},
		{Kind: ProjectionColumn, Column: "email"},
	}
	if len(got) != len(want) {
		t.Fatalf("candidates=%v, want %v", got, want)
	}
	for i := range want {
		if !sameCandidate(got[i], want[i]) {
			t.Errorf("candidate[%d]=%+v, want %+v", i, got[i], want[i])
		}
	}
	displays := make([]string, len(got))
	for i, c := range got {
		displays[i] = c.Display()
	}
	wantDisplays := []string{"*", "COUNT(*)", "id", "email"}
	for i := range wantDisplays {
		if displays[i] != wantDisplays[i] {
			t.Errorf("display[%d]=%q, want %q", i, displays[i], wantDisplays[i])
		}
	}
}

// TestSentinelSelectionAddsDirectlyAndReopensWithoutAggregatePopup requires
// accepting bare COUNT(*) to append the dedicated sentinel identity straight
// away, return a transition that reopens Column(s), and never request a
// named-column aggregate choice.
func TestSentinelSelectionAddsDirectlyAndReopensWithoutAggregatePopup(t *testing.T) {
	q := projFixture()
	out := q.AcceptProjection(ProjectionCandidate{Kind: ProjectionCountStar})
	if !out.ReopenColumns {
		t.Error("sentinel acceptance did not request reopening Column(s)")
	}
	if out.PendingAggregate != nil {
		t.Errorf("sentinel requested aggregate selection: %+v", out.PendingAggregate)
	}
	want := []ProjectionEntry{{Kind: ProjectionCountStar}}
	if !sameEntries(out.Builder.ProjectionEntries(), want) {
		t.Errorf("entries=%v, want %v", out.Builder.ProjectionEntries(), want)
	}
	if out.Builder.Focus() != FieldColumns {
		t.Errorf("focus=%v, want %v", out.Builder.Focus(), FieldColumns)
	}
	if !q.ProjectionEmpty() {
		t.Error("acceptance mutated the source builder")
	}
}

// TestSentinelHiddenWhileAnyEntryExistsAndRestoredWhenProjectionEmpties
// requires the bare sentinel to disappear from the candidate list once any
// projection entry is committed and to reappear in second position after
// removal returns the projection to empty.
func TestSentinelHiddenWhileAnyEntryExistsAndRestoredWhenProjectionEmpties(t *testing.T) {
	q := projFixture()
	out := q.AcceptProjection(ProjectionCandidate{Kind: ProjectionCountStar})
	q = out.Builder
	q = q.CompleteProjectionAggregate("id", AggregateValue).Builder
	nonempty := q.ProjectionCandidates()
	if len(nonempty) != 2 {
		t.Fatalf("nonempty candidates=%v, want exactly the two named columns", nonempty)
	}
	for _, c := range nonempty {
		if c.Kind == ProjectionCountStar || c.Kind == ProjectionWildcard {
			t.Errorf("sentinel/wildcard leaked into nonempty candidates: %+v", c)
		}
	}
	emptied := q.RemoveProjection(len(q.ProjectionEntries()) - 1)
	emptied = emptied.RemoveProjection(len(emptied.ProjectionEntries()) - 1)
	if !emptied.ProjectionEmpty() {
		t.Fatal("removals did not empty the projection")
	}
	back := emptied.ProjectionCandidates()
	if len(back) < 2 || !sameCandidate(back[0], ProjectionCandidate{Kind: ProjectionWildcard}) ||
		!sameCandidate(back[1], ProjectionCandidate{Kind: ProjectionCountStar}) {
		t.Errorf("emptied candidates=%v, want wildcard first and COUNT(*) second", back)
	}
}

// TestWildcardSelectedDirectlyAndIsSoleProjection requires accepting the
// wildcard to commit it directly and to leave no further candidates: the
// wildcard is the entire projection and cannot coexist with entries.
func TestWildcardSelectedDirectlyAndIsSoleProjection(t *testing.T) {
	q := projFixture()
	out := q.AcceptProjection(ProjectionCandidate{Kind: ProjectionWildcard})
	want := []ProjectionEntry{{Kind: ProjectionWildcard}}
	if !sameEntries(out.Builder.ProjectionEntries(), want) {
		t.Fatalf("entries=%v, want %v", out.Builder.ProjectionEntries(), want)
	}
	if out.ReopenColumns || out.PendingAggregate != nil {
		t.Errorf("wildcard reopened=%v pending=%+v, want neither", out.ReopenColumns, out.PendingAggregate)
	}
	if got := out.Builder.ProjectionCandidates(); len(got) != 0 {
		t.Errorf("wildcard left candidates=%v, want none", got)
	}
}

// TestNamedColumnContinuesToAggregateSelectionFromEmptyAndPopulated requires
// accepting a named column — whether the projection starts empty or already
// holds an entry — to preserve the chosen column identity and hand back a
// pending aggregate choice instead of committing immediately.
func TestNamedColumnContinuesToAggregateSelectionFromEmptyAndPopulated(t *testing.T) {
	populated := func() QueryBuilder {
		q := projFixture()
		return q.AcceptProjection(ProjectionCandidate{Kind: ProjectionCountStar}).Builder
	}
	for _, start := range []struct {
		name    string
		builder func() QueryBuilder
	}{
		{"empty", projFixture},
		{"populated", populated},
	} {
		q := start.builder().AcceptProjection(
			ProjectionCandidate{Kind: ProjectionColumn, Column: "email"})
		if q.PendingAggregate == nil ||
			!sameCandidate(*q.PendingAggregate, ProjectionCandidate{Kind: ProjectionColumn, Column: "email"}) {
			t.Errorf("%s: pending=%+v, want named identity email", start.name, q.PendingAggregate)
		}
		if q.ReopenColumns {
			t.Errorf("%s: named acceptance reopened Column(s)", start.name)
		}
		if !sameEntries(q.Builder.ProjectionEntries(), start.builder().ProjectionEntries()) {
			t.Errorf("%s: named acceptance committed early: %v", start.name, q.Builder.ProjectionEntries())
		}
		done := q.Builder.CompleteProjectionAggregate("email", AggAvg)
		if !done.ReopenColumns {
			t.Errorf("%s: aggregate completion did not reopen Column(s)", start.name)
		}
		want := []ProjectionEntry{{Kind: ProjectionColumn, Column: "email", Aggregate: AggAvg}}
		if start.name == "populated" {
			want = append([]ProjectionEntry{{Kind: ProjectionCountStar}}, want...)
		}
		if !sameEntries(done.Builder.ProjectionEntries(), want) {
			t.Errorf("%s: entries=%v, want %v", start.name, done.Builder.ProjectionEntries(), want)
		}
	}
}

// TestZeroVisibleColumnsStillOfferWildcardAndSentinel requires an object with
// no visible columns to keep deriving exactly the two synthetic candidates.
func TestZeroVisibleColumnsStillOfferWildcardAndSentinel(t *testing.T) {
	empty := &schema.Object{Name: "bare", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas}
	q := selectBuilderFor(empty).SelectTable(empty.Name)
	got := q.ProjectionCandidates()
	if len(got) != 2 || !sameCandidate(got[0], ProjectionCandidate{Kind: ProjectionWildcard}) ||
		!sameCandidate(got[1], ProjectionCandidate{Kind: ProjectionCountStar}) {
		t.Errorf("zero-column candidates=%v, want [*, COUNT(*)]", got)
	}
}

// TestSentinelLikeColumnNamesKeepDistinctIdentity requires real columns named
// `*` and `COUNT(*)` to remain named-column identities whose display text
// collides with the synthetic labels while their identity does not.
func TestSentinelLikeColumnNamesKeepDistinctIdentity(t *testing.T) {
	tricky := &schema.Object{
		Name: "tricky", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
		Columns: []schema.Column{{Name: "*"}, {Name: "COUNT(*)"}},
	}
	q := selectBuilderFor(tricky)
	got := q.ProjectionCandidates()
	if len(got) != 4 {
		t.Fatalf("candidates=%d (%v), want four", len(got), got)
	}
	if got[0].Kind != ProjectionWildcard || got[0].Display() != "*" {
		t.Errorf("candidate[0]=%+v, want wildcard displaying *", got[0])
	}
	if got[1].Kind != ProjectionCountStar || got[1].Display() != "COUNT(*)" {
		t.Errorf("candidate[1]=%+v, want sentinel displaying COUNT(*)", got[1])
	}
	if got[2].Kind != ProjectionColumn || got[2].Column != "*" || got[2].Display() != "*" {
		t.Errorf("candidate[2]=%+v, want named column * displaying *", got[2])
	}
	if got[3].Kind != ProjectionColumn || got[3].Column != "COUNT(*)" || got[3].Display() != "COUNT(*)" {
		t.Errorf("candidate[3]=%+v, want named column COUNT(*) displaying COUNT(*)", got[3])
	}
	if sameCandidate(got[0], got[2]) || sameCandidate(got[1], got[3]) {
		t.Error("display-colliding candidates compared identical")
	}
}

// TestWildcardAggregatesAreUnrepresentableByConstruction requires MIN(*),
// MAX(*), AVG(*), and SUM(*) to be impossible: the wildcard aggregate
// requests are rejected outright, and no supported aggregate ever yields an
// aggregate-over-wildcard candidate or entry.
func TestWildcardAggregatesAreUnrepresentableByConstruction(t *testing.T) {
	aggs := []struct {
		name string
		agg  Aggregate
	}{
		{"MIN", AggMin}, {"MAX", AggMax},
		{"AVG", AggAvg}, {"SUM", AggSum},
		{"COUNT", AggCount}, {"Value", AggregateValue},
	}
	for _, tc := range aggs {
		before := projFixture()
		out := before.CompleteProjectionAggregate("", tc.agg)
		if !sameEntries(out.Builder.ProjectionEntries(), before.ProjectionEntries()) {
			t.Errorf("%s(\"\" ) produced entries %v from empty state", tc.name, out.Builder.ProjectionEntries())
		}
		if out.ReopenColumns || out.PendingAggregate != nil {
			t.Errorf("%s(\"\") opened=%v pending=%+v, want neither", tc.name, out.ReopenColumns, out.PendingAggregate)
		}
		wildcardDone := before.AcceptProjection(ProjectionCandidate{Kind: ProjectionWildcard}).Builder
		done := wildcardDone.CompleteProjectionAggregate("*", tc.agg)
		if !sameEntries(done.Builder.ProjectionEntries(), wildcardDone.ProjectionEntries()) {
			t.Errorf("%s over wildcard changed entries to %v", tc.name, done.Builder.ProjectionEntries())
		}
	}
	q := projFixture()
	for _, c := range q.ProjectionCandidates() {
		if c.Display() != "*" && containsWildcardAggregateToken(c.Display()) {
			t.Errorf("unexpected aggregate-on-wildcard display %q", c.Display())
		}
	}
}

// containsWildcardAggregateToken reports whether s spells any forbidden
// AGGREGATE(*) form such as MIN(*) or SUM(*).
func containsWildcardAggregateToken(s string) bool {
	for _, fn := range []string{"MIN(", "MAX(", "AVG(", "SUM("} {
		if len(s) > len(fn) && s[:len(fn)] == fn && s[len(s)-1:] == ")" &&
			s[len(fn):len(s)-1] == "*" {
			return true
		}
	}
	return false
}

// TestRemovalIgnoresOutOfRangeIndexes pins the removal guard so scripted
// callers cannot corrupt state through bad indexes.
func TestRemovalIgnoresOutOfRangeIndexes(t *testing.T) {
	q := projFixture()
	for _, idx := range []int{-1, 0, 5} {
		next := q.RemoveProjection(idx)
		if !next.ProjectionEmpty() || next.Focus() != q.Focus() {
			t.Errorf("RemoveProjection(%d) moved empty state or focus", idx)
		}
	}
}

// TestInsertionOrderPreservedAcrossAggregatesOnSharedColumns requires the
// ordered Issue #16 projection to retain every committed entry exactly in
// selection order across Value, Count, Min, Max, Avg, and Sum — permitting
// the same column under distinct aggregates and distinct columns under the
// same aggregate, with the bare sentinel coexisting as the first entry.
func TestInsertionOrderPreservedAcrossAggregatesOnSharedColumns(t *testing.T) {
	q := projFixture()
	steps := []ProjectionEntry{
		{Kind: ProjectionCountStar},
		{Kind: ProjectionColumn, Column: "email"},
		{Kind: ProjectionColumn, Column: "id", Aggregate: AggCount},
		{Kind: ProjectionColumn, Column: "email", Aggregate: AggMin},
		{Kind: ProjectionColumn, Column: "id", Aggregate: AggMax},
		{Kind: ProjectionColumn, Column: "email", Aggregate: AggAvg},
		{Kind: ProjectionColumn, Column: "id", Aggregate: AggSum},
	}
	for i, step := range steps {
		before := q
		if step.Kind != ProjectionCountStar {
			q = q.CompleteProjectionAggregate(step.Column, step.Aggregate).Builder
		} else {
			q = q.AcceptProjection(ProjectionCandidate{Kind: ProjectionCountStar}).Builder
		}
		want := append([]ProjectionEntry(nil), steps[:i+1]...)
		if got := q.ProjectionEntries(); !sameEntries(got, want) {
			t.Fatalf("step %d: entries=%v, want %v", i, got, want)
		}
		if got := before.ProjectionEntries(); !sameEntries(got, steps[:i]) {
			t.Errorf("step %d: mutated the source builder: %v", i, got)
		}
		if q.Focus() != FieldColumns {
			t.Errorf("step %d: focus=%v, want %v", i, q.Focus(), FieldColumns)
		}
	}
	// Distinct columns may share one aggregate: email and id both Min.
	shared := projFixture()
	shared = shared.CompleteProjectionAggregate("email", AggMin).Builder
	shared = shared.CompleteProjectionAggregate("id", AggMin).Builder
	want := []ProjectionEntry{
		{Kind: ProjectionColumn, Column: "email", Aggregate: AggMin},
		{Kind: ProjectionColumn, Column: "id", Aggregate: AggMin},
	}
	if got := shared.ProjectionEntries(); !sameEntries(got, want) {
		t.Errorf("shared-aggregate entries=%v, want %v", got, want)
	}
}

// TestExactDuplicateNamedPairIsRejectedNoOp requires an exact repeated
// (column, aggregate) pair — including the zero Value aggregate — to be an
// unchanged no-op: no reordering, no replacement, no reopen, and no focus
// change, while later distinct entries still append normally afterward.
func TestExactDuplicateNamedPairIsRejectedNoOp(t *testing.T) {
	q := projFixture()
	q = q.CompleteProjectionAggregate("email", AggAvg).Builder
	q = q.CompleteProjectionAggregate("id", AggregateValue).Builder
	for _, dup := range []struct {
		column string
		agg    Aggregate
	}{
		{"email", AggAvg},
		{"id", AggregateValue},
	} {
		out := q.CompleteProjectionAggregate(dup.column, dup.agg)
		if !sameEntries(out.Builder.ProjectionEntries(), q.ProjectionEntries()) {
			t.Errorf("duplicate (%s,%v) changed entries to %v", dup.column, dup.agg, out.Builder.ProjectionEntries())
		}
		if out.ReopenColumns {
			t.Errorf("duplicate (%s,%v) requested reopening Column(s)", dup.column, dup.agg)
		}
		if out.PendingAggregate != nil {
			t.Errorf("duplicate (%s,%v) requested aggregate selection: %+v", dup.column, dup.agg, out.PendingAggregate)
		}
		if out.Builder.Focus() != q.Focus() {
			t.Errorf("duplicate (%s,%v) changed focus %v -> %v", dup.column, dup.agg, q.Focus(), out.Builder.Focus())
		}
	}
	next := q.CompleteProjectionAggregate("id", AggMin).Builder
	want := []ProjectionEntry{
		{Kind: ProjectionColumn, Column: "email", Aggregate: AggAvg},
		{Kind: ProjectionColumn, Column: "id"},
		{Kind: ProjectionColumn, Column: "id", Aggregate: AggMin},
	}
	if got := next.ProjectionEntries(); !sameEntries(got, want) {
		t.Errorf("entries after rejected duplicates=%v, want %v", got, want)
	}
}

// TestDuplicateSentinelTransitionDirectlyIsNoOp requires a direct bare
// COUNT(*) transition outside the conditional UI path — whether the sentinel
// is already present or any other entry owns the projection — to be an
// unchanged no-op.
func TestDuplicateSentinelTransitionDirectlyIsNoOp(t *testing.T) {
	once := projFixture()
	once = once.AcceptProjection(ProjectionCandidate{Kind: ProjectionCountStar}).Builder
	out := once.AcceptProjection(ProjectionCandidate{Kind: ProjectionCountStar})
	if !sameEntries(out.Builder.ProjectionEntries(), once.ProjectionEntries()) {
		t.Errorf("duplicate sentinel changed entries to %v", out.Builder.ProjectionEntries())
	}
	if out.ReopenColumns || out.PendingAggregate != nil {
		t.Errorf("duplicate sentinel reopened=%v pending=%+v, want neither", out.ReopenColumns, out.PendingAggregate)
	}
	if out.Builder.Focus() != once.Focus() {
		t.Errorf("duplicate sentinel changed focus %v -> %v", once.Focus(), out.Builder.Focus())
	}
	// The sentinel also stays committed while a named entry follows, and the
	// duplicate-sentinel no-op must not disturb that coexistence.
	mixed := once.CompleteProjectionAggregate("email", AggMin).Builder
	out = mixed.AcceptProjection(ProjectionCandidate{Kind: ProjectionCountStar})
	if !sameEntries(out.Builder.ProjectionEntries(), mixed.ProjectionEntries()) {
		t.Errorf("duplicate sentinel over mixed entries produced %v", out.Builder.ProjectionEntries())
	}
}

// TestWildcardReplacesWholeListAtomicallyAndIsSole requires wildcard selection
// at any point to clear every prior named and sentinel entry in one step and
// become the sole projection, rejecting any append beside it until removal
// empties the projection again.
func TestWildcardReplacesWholeListAtomicallyAndIsSole(t *testing.T) {
	q := projFixture()
	q = q.AcceptProjection(ProjectionCandidate{Kind: ProjectionCountStar}).Builder
	q = q.CompleteProjectionAggregate("id", AggCount).Builder
	q = q.CompleteProjectionAggregate("email", AggMin).Builder
	out := q.AcceptProjection(ProjectionCandidate{Kind: ProjectionWildcard})
	want := []ProjectionEntry{{Kind: ProjectionWildcard}}
	if got := out.Builder.ProjectionEntries(); !sameEntries(got, want) {
		t.Fatalf("wildcard replacement entries=%v, want %v", got, want)
	}
	if out.ReopenColumns || out.PendingAggregate != nil {
		t.Errorf("wildcard replacement reopened=%v pending=%+v, want neither", out.ReopenColumns, out.PendingAggregate)
	}
	if out.Builder.Focus() != FieldColumns {
		t.Errorf("wildcard replacement focus=%v, want %v", out.Builder.Focus(), FieldColumns)
	}
	// Nothing may append beside the wildcard until a valid transition removes it.
	guards := out.Builder
	if g := guards.AcceptProjection(ProjectionCandidate{Kind: ProjectionCountStar}); !sameEntries(g.Builder.ProjectionEntries(), want) {
		t.Errorf("sentinel beside wildcard produced %v", g.Builder.ProjectionEntries())
	}
	if g := guards.CompleteProjectionAggregate("id", AggMin); !sameEntries(g.Builder.ProjectionEntries(), want) {
		t.Errorf("named append beside wildcard produced %v", g.Builder.ProjectionEntries())
	} else if g.ReopenColumns {
		t.Error("named append beside wildcard requested reopening Column(s)")
	}
	// Re-accepting the wildcard while it is already sole leaves one entry.
	again := guards.AcceptProjection(ProjectionCandidate{Kind: ProjectionWildcard})
	if !sameEntries(again.Builder.ProjectionEntries(), want) {
		t.Errorf("re-accepted wildcard produced %v", again.Builder.ProjectionEntries())
	}
	// Removal empties the projection and restores the empty candidate sequence.
	emptied := out.Builder.RemoveProjection(len(out.Builder.ProjectionEntries()) - 1)
	if !emptied.ProjectionEmpty() {
		t.Fatalf("removing the wildcard left %v", emptied.ProjectionEntries())
	}
	back := emptied.ProjectionCandidates()
	if len(back) < 2 || !sameCandidate(back[0], ProjectionCandidate{Kind: ProjectionWildcard}) ||
		!sameCandidate(back[1], ProjectionCandidate{Kind: ProjectionCountStar}) {
		t.Errorf("emptied candidates=%v, want wildcard first and COUNT(*) second", back)
	}
}

// TestMalformedIdentitiesCannotBypassInvariants requires unknown candidate
// kinds and junk-carrying sentinel/wildcard identities to be harmless no-ops,
// and a real column literally named COUNT(*) to keep a distinct identity so
// it can coexist with the committed bare sentinel.
func TestMalformedIdentitiesCannotBypassInvariants(t *testing.T) {
	q := projFixture()
	before := q.ProjectionEntries()
	for _, kind := range []ProjectionKind{0, ProjectionKind(7)} {
		out := q.AcceptProjection(ProjectionCandidate{Kind: kind})
		if !sameEntries(out.Builder.ProjectionEntries(), before) {
			t.Errorf("kind %d changed entries to %v", kind, out.Builder.ProjectionEntries())
		}
		if out.ReopenColumns || out.PendingAggregate != nil {
			t.Errorf("kind %d reopened=%v pending=%+v, want neither", kind, out.ReopenColumns, out.PendingAggregate)
		}
		if out.Builder.Focus() != q.Focus() {
			t.Errorf("kind %d changed focus %v -> %v", kind, q.Focus(), out.Builder.Focus())
		}
	}
	// Junk carried on synthetic identities must not enter committed state.
	wc := q.AcceptProjection(ProjectionCandidate{Kind: ProjectionWildcard, Column: "junk"}).Builder
	want := []ProjectionEntry{{Kind: ProjectionWildcard}}
	if got := wc.ProjectionEntries(); !sameEntries(got, want) || got[0].Column != "" {
		t.Errorf("junk-carrying wildcard produced %v", got)
	}
	sent := q.AcceptProjection(ProjectionCandidate{Kind: ProjectionCountStar, Column: "junk"}).Builder
	want = []ProjectionEntry{{Kind: ProjectionCountStar}}
	if got := sent.ProjectionEntries(); !sameEntries(got, want) || got[0].Column != "" {
		t.Errorf("junk-carrying sentinel produced %v", got)
	}
	// A real column named COUNT(*) is a named identity: it coexists with the
	// committed bare sentinel instead of collapsing into it.
	tricky := &schema.Object{
		Name: "tricky", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
		Columns: []schema.Column{{Name: "COUNT(*)"}},
	}
	tq := selectBuilderFor(tricky)
	tq = tq.AcceptProjection(ProjectionCandidate{Kind: ProjectionCountStar}).Builder
	tq = tq.CompleteProjectionAggregate("COUNT(*)", AggregateValue).Builder
	want = []ProjectionEntry{{Kind: ProjectionCountStar}, {Kind: ProjectionColumn, Column: "COUNT(*)"}}
	if got := tq.ProjectionEntries(); !sameEntries(got, want) {
		t.Errorf("sentinel plus named COUNT(*) produced %v, want %v", got, want)
	}
}

// TestRemoveLatestProjectionRemovesOnlyLatestPreservingOrder requires the
// immutable remove-latest transition to delete exactly the last committed
// entry per call — walking backward through named entries and the bare
// sentinel — to leave every earlier entry and its insertion order intact,
// and to be an unchanged no-op (entries, focus, candidates) when empty.
// Removing a sole wildcard empties the projection, restoring the Issue #15
// empty candidate sequence. A rejected duplicate is never treated as
// removable state.
func TestRemoveLatestProjectionRemovesOnlyLatestPreservingOrder(t *testing.T) {
	q := projFixture()
	steps := []ProjectionEntry{
		{Kind: ProjectionCountStar},
		{Kind: ProjectionColumn, Column: "id", Aggregate: AggMin},
		{Kind: ProjectionColumn, Column: "email"},
	}
	for i, step := range steps {
		if step.Kind == ProjectionCountStar {
			q = q.AcceptProjection(ProjectionCandidate{Kind: ProjectionCountStar}).Builder
		} else {
			q = q.CompleteProjectionAggregate(step.Column, step.Aggregate).Builder
		}
		if got := len(q.ProjectionEntries()); got != i+1 {
			t.Fatalf("setup step %d: %d entries", i, got)
		}
	}
	for len(q.ProjectionEntries()) > 0 {
		next := q.RemoveLatestProjection()
		want := q.ProjectionEntries()[:len(q.ProjectionEntries())-1]
		if got := next.ProjectionEntries(); !sameEntries(got, want) {
			t.Fatalf("removal produced %v, want %v", got, want)
		}
		if next.Focus() != FieldColumns {
			t.Errorf("removal moved focus to %v, want %v preserved", next.Focus(), FieldColumns)
		}
		before := append([]ProjectionEntry(nil), want...)
		q = next
		if got := q.ProjectionEntries(); !sameEntries(got, before) {
			t.Fatalf("removal mutated state again: %v vs %v", got, before)
		}
	}
	if !q.ProjectionEmpty() || q.ProjectionCandidates() == nil {
		t.Fatalf("emptied projection: entries=%v candidates=%v", q.ProjectionEntries(), q.ProjectionCandidates())
	}
	// Empty removal is an exact unchanged no-op.
	same := q.RemoveLatestProjection()
	if !sameEntries(same.ProjectionEntries(), q.ProjectionEntries()) ||
		same.Focus() != q.Focus() ||
		len(same.ProjectionCandidates()) != len(q.ProjectionCandidates()) {
		t.Error("empty removal changed state")
	}
	// Wildcard removal produces empty state with restored candidates.
	w := projFixture().AcceptProjection(ProjectionCandidate{Kind: ProjectionWildcard}).Builder
	empty := w.RemoveLatestProjection()
	if !empty.ProjectionEmpty() {
		t.Errorf("wildcard removal left %v", empty.ProjectionEntries())
	}
	back := empty.ProjectionCandidates()
	if len(back) < 2 || !sameCandidate(back[0], ProjectionCandidate{Kind: ProjectionWildcard}) ||
		!sameCandidate(back[1], ProjectionCandidate{Kind: ProjectionCountStar}) {
		t.Errorf("post-wildcard candidates=%v, want wildcard first and COUNT(*) second", back)
	}
	// Rejected duplicates are not removable history: duplicating a pair must
	// not grow the list, so removals cannot walk into rejection artifacts.
	dup := projFixture().CompleteProjectionAggregate("email", AggMin).Builder
	dup = dup.CompleteProjectionAggregate("email", AggMin).Builder
	if got := dup.ProjectionEntries(); len(got) != 1 {
		t.Errorf("rejected duplicate left %d removable entries: %v", len(got), got)
	}
}
