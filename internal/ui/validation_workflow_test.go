// Scripted Bubble Tea coverage for Issue #21 Tasks 3–4: runnable base-context
// Enter opens the distinct pre-execution schema-validation workflow and issues
// a PRAGMA schema_version read before any execution command; an unchanged
// version reuses the exact cached catalog without issuing a catalog refresh;
// a changed version refreshes once and repairs only dependent builder state,
// focusing the first specific invalid reason when repair leaves the builder
// non-runnable. Ordinary refresh failures retain the stale cache behind the
// exact `Schema data is stale — retry or cancel` status plus
// `could not refresh: <cause>` with retry (fresh preparation identity) and
// cancel (exact builder restoration, no execution). Ctrl+W during an
// in-flight validation requests connection-scoped cancellation exactly once,
// renders exact `cancelling…` until settlement, starts no replacement
// request, and discards a late success as cancelled. Deletion/replacement
// health classifications take terminal precedence over every ordinary
// outcome. Validation appends neither query nor result history in any
// outcome except successful validation followed by the actual execution
// start; superseded preparation identities cannot mutate current state. A
// deterministic fake stands in for the Connection boundary so no database
// access runs here.

package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/history"
	qb "github.com/chris/sqloid/internal/querybuilder"
	"github.com/chris/sqloid/internal/schema"
)

// fakeVersionReader is the deterministic fake version-request boundary: it
// yields one queued outcome per ReadSchemaVersion call and counts every call
// so tests can prove exactly how many version requests were issued.
type fakeVersionReader struct {
	queued []schema.VersionAttempt
	calls  int
}

func (f *fakeVersionReader) ReadSchemaVersion() schema.VersionAttempt {
	f.calls++
	if len(f.queued) == 0 {
		return schema.NewVersionFailure(errors.New("unexpected extra version read"))
	}
	next := f.queued[0]
	f.queued = f.queued[1:]
	return next
}

func versionOK(v int64) schema.VersionAttempt { return schema.NewVersionOK(v) }
func versionDeleted() schema.VersionAttempt   { return schema.NewVersionDeleted() }
func versionReplaced() schema.VersionAttempt  { return schema.NewVersionReplaced() }
func versionFailed(c string) schema.VersionAttempt {
	return schema.NewVersionFailure(errors.New(c))
}

// validationModel returns a supported-size model with q as builder state, the
// given cached catalog, both fakes wired, and a fresh history store.
func validationModel(cat *schema.Catalog, reader *fakeVersionReader, fake *fakeRefresher, q qb.QueryBuilder) Model {
	m := modelWithQB(q)
	m.catalog = cat
	m.VersionReader = reader
	m.Refresher = fake
	m.History = history.NewStore()
	return m
}

// selectModel returns a runnable SELECT model over the whereUICatalog
// (version 17, users with id/email/note) with wildcard projection and a
// completed WHERE, ready for an Enter press.
func selectModel(reader *fakeVersionReader, fake *fakeRefresher) Model {
	return validationModel(whereUICatalog(), reader, fake, validSelectQB())
}

// enterRunnable presses Enter on runnable data and pumps the pre-execution
// lifecycle seam so the model opens the validation workflow; it returns the
// opened model plus the command carrying the schema-version request.
func enterRunnable(m Model) (Model, tea.Cmd) {
	m = focusField(m, commandFieldLabel) // a non-opener base field
	next, enterCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := next.(Model)
	if enterCmd == nil {
		panic("runnable Enter issued no pre-execution seam")
	}
	opened, cmd := mm.Update(enterCmd())
	return opened.(Model), cmd
}

// cancelledLateSettle constructs a settled message for an older preparation
// identity so tests can inject superseded and late completions directly.
func cancelledLateSettle(id uint64, r schema.Revalidation) ValidationSettledMsg {
	return ValidationSettledMsg{Preparation: id, Result: r}
}

// columnedSelectQB returns runnable SELECT state whose projection is exactly
// the named columns in order (no wildcard, so appends are representable).
func columnedSelectQB(columns ...string) qb.QueryBuilder {
	q := qb.NewQuery().RefreshSchema(whereUICatalog()).
		SelectCommand(qb.CommandSelect).SelectTable("users")
	for _, c := range columns {
		q = q.CompleteProjectionAggregate(c, qb.AggregateValue).Builder
	}
	return completeWhereQB(q, "x")
}

// TestRunnableEnterOpensValidationWithNoHistory requires Enter on runnable
// data to open the validation workflow (distinct from execution), issue the
// schema-version request first, and render exact `validating…` while pending.
// The request runs before any execution command and no history exists yet.
func TestRunnableEnterOpensValidationWithNoHistory(t *testing.T) {
	reader := &fakeVersionReader{queued: []schema.VersionAttempt{versionOK(17)}}
	fake := &fakeRefresher{}
	m := selectModel(reader, fake)

	opened, cmd := enterRunnable(m)
	if !opened.validating {
		t.Fatal("runnable Enter did not open the validation workflow")
	}
	if !opened.validationPending {
		t.Error("validation pending flag not set while request is in flight")
	}
	if !opened.ActiveCancellable {
		t.Error("pending validation is not cancellable")
	}
	if reader.calls != 0 {
		// The command runs the request; Update itself must not issue work.
		t.Errorf("Update issued %d version reads outside the returned command", reader.calls)
	}
	if view := opened.View(); !strings.Contains(view, "validating…") {
		t.Errorf("pending validation view lacks exact validating status:\n%s", view)
	}
	if opened.History.Len() != 0 {
		t.Errorf("opening validation appended history (Len=%d)", opened.History.Len())
	}

	// Settle the request: unchanged version, cache reuse, execution route.
	settled := cmd()
	after, execCmd := opened.Update(settled)
	final := after.(Model)
	if execCmd == nil {
		t.Fatal("successful unchanged validation returned no execution route")
	}
	if final.validating || final.validationPending {
		t.Error("successful validation left the workflow open")
	}
	if fake.calls != 0 {
		t.Errorf("unchanged version issued %d catalog refreshes, want 0", fake.calls)
	}
	if _, ok := final.QB.SelectedTable(); !ok {
		t.Error("unchanged validation disturbed the selected table")
	}

	// The execution route appends query history exactly once at start.
	started, _ := final.Update(execCmd())
	if started.(Model).History.Len() != 1 {
		t.Errorf("successful validation then execution start: history Len=%d, want 1", started.(Model).History.Len())
	}
}

// TestCancelledValidationAppendsNoHistory drives a successful validation,
// then failure and cancel paths: neither appends any history.
func TestCancelledValidationAppendsNoHistory(t *testing.T) {
	// Failed refresh, then cancel.
	reader := &fakeVersionReader{queued: []schema.VersionAttempt{versionOK(18)}}
	fake := &fakeRefresher{queued: []schema.Attempt{failAttempt("lock busy")}}
	m := selectModel(reader, fake)
	opened, vcmd := enterRunnable(m)
	failed, _ := opened.Update(vcmd().(ValidationSettledMsg))
	f := failed.(Model)
	if !f.validating || f.staleCause == "" {
		t.Fatal("ordinary refresh failure did not retain stale state inside validation")
	}
	cancelled, _ := f.Update(tea.KeyMsg{Type: tea.KeyEsc})
	c := cancelled.(Model)
	if c.validating || c.staleCause != "" {
		t.Error("cancel did not close the validation flow cleanly")
	}
	if c.History.Len() != 0 {
		t.Errorf("cancelled validation appended history (Len=%d)", c.History.Len())
	}

	// Invalid repair leaves non-runnable data and no history: the builder
	// projects only the column that vanishes in the refresh.
	reader2 := &fakeVersionReader{queued: []schema.VersionAttempt{versionOK(18)}}
	fake2 := &fakeRefresher{queued: []schema.Attempt{successAttempt(&schema.Catalog{Version: 18, Objects: whereUICatalog().Objects})}}
	m2 := selectModel(reader2, fake2)
	q2 := columnedSelectQB("note")
	m2 = validationModel(whereUICatalog(), reader2, fake2, q2)
	fresh := &schema.Catalog{Version: 18, Objects: []*schema.Object{{
		Name: "users", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
		Columns: []schema.Column{{Name: "id"}, {Name: "email"}},
	}}}
	fake2.queued = []schema.Attempt{successAttempt(fresh)}
	opened2, vcmd2 := enterRunnable(m2)
	repaired, repairCmd := opened2.Update(vcmd2().(ValidationSettledMsg))
	r := repaired.(Model)
	if repairCmd != nil {
		t.Error("repair leaving non-runnable data returned an execution route")
	}
	if label := r.Fields[r.Focus].Label; label != columnsFieldLabel {
		t.Errorf("focus after invalid repair = %q, want Column(s)", label)
	}
	if !strings.Contains(r.Fields[r.Focus].Content, qb.ReasonNoProjection) {
		t.Errorf("repaired view lacks the first specific reason %q:\n%s", qb.ReasonNoProjection, r.View())
	}
	if r.History.Len() != 0 {
		t.Errorf("invalid repair appended history (Len=%d)", r.History.Len())
	}
}

// TestChangedVersionRefreshesOnceAndPreservesUnrelatedState requires a
// changed version to refresh exactly once through the CatalogRefresher seam
// and repair only dependent state: the vanished column's projection entry is
// removed while the surviving entry and Limit are preserved, with the first
// specific reason focused.
func TestChangedVersionRefreshesOnceAndPreservesUnrelatedState(t *testing.T) {
	reader := &fakeVersionReader{queued: []schema.VersionAttempt{versionOK(18)}}
	fresh := &schema.Catalog{Version: 18, Objects: []*schema.Object{{
		Name: "users", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas,
		Columns: []schema.Column{{Name: "id"}, {Name: "email"}},
	}}}
	fake := &fakeRefresher{queued: []schema.Attempt{successAttempt(fresh)}}

	q := columnedSelectQB("email", "note").SetLimitInput("5")
	m := validationModel(whereUICatalog(), reader, fake, q)

	opened, vcmd := enterRunnable(m)
	settled := vcmd().(ValidationSettledMsg)
	if reader.calls != 1 {
		t.Fatalf("validation issued %d version reads, want 1", reader.calls)
	}
	repaired, execCmd := opened.Update(settled)
	r := repaired.(Model)
	if fake.calls != 1 {
		t.Errorf("changed version issued %d catalog refreshes, want 1", fake.calls)
	}
	// Surviving state preserved; dependent state cleared.
	entries := r.QB.ProjectionEntries()
	if len(entries) != 1 || entries[0].Column != "email" {
		t.Errorf("projection after repair = %v, want only email", entries)
	}
	if v, ok := r.QB.LimitValue(); !ok || v != 5 {
		t.Errorf("unrelated Limit not preserved: %d %v", v, ok)
	}
	// The repaired builder is runnable again: the execution route returns.
	if execCmd == nil {
		t.Fatal("repaired runnable state returned no execution route")
	}
}

// TestRepairFocusesFirstInvalidReasonAndBlocksExecution requires a refresh
// that drops the whole table to clear the selection and focus Table with the
// first specific reason, and to return no execution route.
func TestRepairFocusesFirstInvalidReasonAndBlocksExecution(t *testing.T) {
	reader := &fakeVersionReader{queued: []schema.VersionAttempt{versionOK(18)}}
	fake := &fakeRefresher{queued: []schema.Attempt{successAttempt(&schema.Catalog{
		Version: 18,
		Objects: []*schema.Object{{Name: "logs", Kind: schema.KindView, Columns: []schema.Column{{Name: "line"}}}},
	})}}
	q := qb.NewQuery().RefreshSchema(whereUICatalog()).
		SelectCommand(qb.CommandDelete).SelectTable("users")
	m := validationModel(whereUICatalog(), reader, fake, q)
	// DELETE keeps tables only; users vanishes entirely in the refresh.
	opened, vcmd := enterRunnable(m)
	repaired, execCmd := opened.Update(vcmd().(ValidationSettledMsg))
	r := repaired.(Model)
	if execCmd != nil {
		t.Error("repair with invalidated object returned an execution route")
	}
	if _, ok := r.QB.SelectedTable(); ok {
		t.Error("invalidated table selection survived repair")
	}
	if label := r.Fields[r.Focus].Label; label != tableFieldLabel {
		t.Errorf("focus after repair = %q, want Table", label)
	}
	if r.History.Len() != 0 {
		t.Errorf("invalid repair appended history (Len=%d)", r.History.Len())
	}
}

// TestStaleValidationRetryUsesFreshIdentityAndCancelRestores covers the
// ordinary refresh-failure path: exact stale indicators, a retry that issues
// a fresh version read under a new preparation identity, an older
// superseded response discarded on arrival, and cancel restoring the builder
// without execution.
func TestStaleValidationRetryUsesFreshIdentityAndCancelRestores(t *testing.T) {
	reader := &fakeVersionReader{queued: []schema.VersionAttempt{
		versionOK(18), // initial: changed, refresh fails
		versionOK(17), // retry: unchanged again
	}}
	fake := &fakeRefresher{queued: []schema.Attempt{failAttempt("lock busy")}}
	m := selectModel(reader, fake)

	opened, vcmd := enterRunnable(m)
	failed, _ := opened.Update(vcmd().(ValidationSettledMsg))
	f := failed.(Model)
	if !f.schemaStale {
		t.Fatal("ordinary refresh failure did not enter stale state")
	}
	view := f.View()
	if !strings.Contains(view, StaleSchemaStatus) || !strings.Contains(view, "could not refresh: lock busy") {
		t.Errorf("stale validation view lacks exact indicators:\n%s", view)
	}

	// Retry issues a fresh version read (identity 2); its response settles
	// only after a superseded first-identity response is injected.
	retry, retryCmd := f.Update(tea.KeyMsg{Type: tea.KeyEnter})
	r := retry.(Model)
	if retryCmd == nil {
		t.Fatal("retry issued no request")
	}
	settled2 := retryCmd().(ValidationSettledMsg)
	if reader.calls != 2 {
		t.Fatalf("retry issued %d total version reads, want 2", reader.calls)
	}
	// A late response from the first preparation cannot mutate state.
	superseded := cancelledLateSettle(1, schema.Revalidation{Status: schema.RevalidateUnchanged, Catalog: whereUICatalog()})
	guarded, guardCmd := r.Update(superseded)
	g := guarded.(Model)
	if guardCmd != nil {
		t.Error("superseded response returned an execution route")
	}
	if !g.validating {
		t.Error("superseded response closed the active validation")
	}
	// The fresh response settles: unchanged, execution route returns.
	if settled2.Preparation != 2 {
		t.Fatalf("retry preparation identity = %d, want 2", settled2.Preparation)
	}
	after, execCmd := r.Update(settled2)
	a := after.(Model)
	if execCmd == nil {
		t.Fatal("successful retry returned no execution route")
	}
	if a.schemaStale {
		t.Error("successful retry left stale indicators active")
	}
}

// TestCancelRestoresContextWithoutExecution requires Esc during stale
// validation to close the flow, restore runnable builder context, and issue
// no execution.
func TestCancelRestoresContextWithoutExecution(t *testing.T) {
	reader := &fakeVersionReader{queued: []schema.VersionAttempt{versionOK(18)}}
	fake := &fakeRefresher{queued: []schema.Attempt{failAttempt("lock busy")}}
	m := selectModel(reader, fake)
	opened, vcmd := enterRunnable(m)
	failed, _ := opened.Update(vcmd().(ValidationSettledMsg))
	f := failed.(Model)
	cancelled, cancelCmd := f.Update(tea.KeyMsg{Type: tea.KeyEsc})
	c := cancelled.(Model)
	if cancelCmd != nil {
		t.Error("cancel returned a command")
	}
	if c.validating || c.schemaStale {
		t.Error("cancel left validation or stale state active")
	}
	if c.QB.RunnableReport().Runnable == false {
		t.Error("cancel disturbed builder state")
	}
	if c.History.Len() != 0 {
		t.Errorf("cancelled validation appended history (Len=%d)", c.History.Len())
	}
}

// TestCtrlWRequestsCancellationOnceShowsCancellingUntilSettlement covers the
// Ctrl+W contract: exactly one cancellation request, exact `cancelling…`
// rendering until settlement, no replacement request started while
// cancelling, and a late success discarded as cancelled.
func TestCtrlWRequestsCancellationOnceShowsCancellingUntilSettlement(t *testing.T) {
	reader := &fakeVersionReader{queued: []schema.VersionAttempt{versionOK(18)}}
	fake := &fakeRefresher{queued: []schema.Attempt{failAttempt("locked again")}}
	m := selectModel(reader, fake)

	opened, vcmd := enterRunnable(m)
	failed, _ := opened.Update(vcmd().(ValidationSettledMsg))
	f := failed.(Model)

	// Retry, then hold the response so the request is in flight.
	retry, retryCmd := f.Update(tea.KeyMsg{Type: tea.KeyEnter})
	r := retry.(Model)
	if retryCmd == nil {
		t.Fatal("retry issued no request")
	}
	held := retryCmd().(ValidationSettledMsg)

	before, wCmd := r.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	// The model dispatches cancellation through its owned CancelCommand.
	if wCmd == nil {
		t.Fatal("Ctrl+W issued no cancellation")
	}
	msg := wCmd()
	if _, ok := msg.(CancelValidationMsg); !ok {
		t.Fatalf("cancellation message = %T, want CancelValidationMsg", msg)
	}
	c := before.(Model)
	if !c.validationCancelling {
		t.Fatal("model did not enter the cancelling state")
	}
	// Second Ctrl+W must not dispatch another cancellation.
	_, again := c.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	if again != nil {
		t.Error("second Ctrl+W issued another cancellation")
	}
	// Enter must not start a replacement request while cancelling.
	if _, enterCmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter}); enterCmd != nil {
		t.Error("Enter started a replacement request during cancellation")
	}
	// Exact `cancelling…` renders until settlement.
	if view := c.View(); !strings.Contains(view, "cancelling…") {
		t.Errorf("cancelling view lacks exact cancelling… status:\n%s", view)
	}
	// Late success after cancellation is discarded as cancelled.
	late, lateCmd := c.Update(held)
	l := late.(Model)
	if lateCmd != nil {
		t.Error("late success after cancellation returned an execution route")
	}
	if l.validating || l.validationCancelling {
		t.Error("settled cancellation did not close the workflow")
	}
	if l.History.Len() != 0 {
		t.Errorf("late success after cancellation appended history (Len=%d)", l.History.Len())
	}
	if l.schemaStale {
		t.Error("cancellation retained stale indicators")
	}
}

// TestTerminalOverridesValidation covers deletion/replacement taking
// terminal precedence: an injected terminal state suppresses Enter
// entirely (no request before work), and a version read classifying as
// deleted transitions to the exact terminal presentation, rejecting late
// completions.
func TestTerminalOverridesValidation(t *testing.T) {
	// Terminal state set before Enter: no request may be issued.
	reader := &fakeVersionReader{queued: []schema.VersionAttempt{versionOK(17)}}
	fake := &fakeRefresher{}
	m := selectModel(reader, fake)
	m.enterTerminal(TerminalDeleted)
	guarded, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Error("terminal state allowed a validation request")
	}
	if reader.calls != 0 {
		t.Errorf("terminal state allowed %d version reads", reader.calls)
	}
	if g := guarded.(Model); g.validating {
		t.Error("terminal state allowed validation to open")
	}

	// Version read classifies as deleted: terminal presentation wins.
	reader2 := &fakeVersionReader{queued: []schema.VersionAttempt{versionDeleted()}}
	m2 := selectModel(reader2, fake)
	opened2, vcmd2 := enterRunnable(m2)
	settled, cmd3 := opened2.Update(vcmd2().(ValidationSettledMsg))
	s := settled.(Model)
	if cmd3 != nil {
		t.Error("terminal settlement returned an execution route")
	}
	if s.terminalState != TerminalDeleted {
		t.Fatalf("deleted version read produced %v, want terminal deleted", s.terminalState)
	}
	if view := s.View(); !strings.Contains(view, DeletedSessionEndedMessage) {
		t.Errorf("terminal view lacks exact session-ended message:\n%s", view)
	}
	// A late completion cannot revive the session.
	late := cancelledLateSettle(s.validationAttempt, schema.Revalidation{Status: schema.RevalidateUnchanged, Catalog: whereUICatalog()})
	final, finalCmd := s.Update(late)
	if finalCmd != nil {
		t.Error("late completion after terminal returned a command")
	}
	if f := final.(Model); f.validating {
		t.Error("late completion reopened validation after terminal")
	}
}

// TestReplacedVersionReadIsTerminal covers the replacement classification on
// the version read itself.
func TestReplacedVersionReadIsTerminal(t *testing.T) {
	reader := &fakeVersionReader{queued: []schema.VersionAttempt{versionReplaced()}}
	m := selectModel(reader, &fakeRefresher{})
	opened, vcmd := enterRunnable(m)
	settled, execCmd := opened.Update(vcmd().(ValidationSettledMsg))
	s := settled.(Model)
	if execCmd != nil {
		t.Error("terminal replacement returned an execution route")
	}
	if s.terminalState != TerminalReplaced {
		t.Fatalf("replaced version read produced %v, want terminal replaced", s.terminalState)
	}
	if view := s.View(); !strings.Contains(view, ReplacedSessionEndedMessage) {
		t.Errorf("terminal view lacks exact session-ended message:\n%s", view)
	}
}

// TestPostValidationRaceIsOrdinaryExecutionError requires DDL after a settled
// validation to leave the successful outcome untouched: the execution route
// stands, no re-validation is issued, and the later changed version is
// discovered on the next request, not retroactively.
func TestPostValidationRaceIsOrdinaryExecutionError(t *testing.T) {
	reader := &fakeVersionReader{queued: []schema.VersionAttempt{versionOK(17)}}
	m := selectModel(reader, &fakeRefresher{})
	opened, vcmd := enterRunnable(m)
	after, execCmd := opened.Update(vcmd().(ValidationSettledMsg))
	a := after.(Model)
	if execCmd == nil {
		t.Fatal("unchanged validation returned no execution route")
	}
	// External DDL races after settlement: the settled outcome must not
	// retroactively change and the execution route must still start exactly
	// once, with the history append happening at execution start.
	started, _ := a.Update(execCmd())
	final := started.(Model)
	if final.History.Len() != 1 {
		t.Fatalf("history Len=%d, want exactly one execution-start append", final.History.Len())
	}
	// No further version reads were issued: the race is ordinary execution
	// territory, not validation.
	if reader.calls != 1 {
		t.Errorf("post-validation DDL issued %d extra version reads, want 0", reader.calls-1)
	}
}
