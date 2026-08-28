// Scripted coverage for Issue #13 Tasks 3–4: the explicit retry and cancel
// lifecycle over the stale-schema flow, and the typed deletion/replacement
// health classifications taking terminal precedence over it. Retries route
// through the same Connection-backed refresh seam used by the initial open;
// repeated failures preserve stale candidates and affordances while updating
// exactly the inline cause; cancel restores the exact Table opener and
// pre-open state without continuing anything; terminal classification —
// injected as typed attempts, never matched from error strings — overrides
// the workflow, clears retry/cancel affordances, suppresses the stale
// status and ordinary causes, and rejects late or superseded refresh
// completions so no database work can revive the session.

package ui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	qb "github.com/chris/sqloid/internal/querybuilder"
	"github.com/chris/sqloid/internal/schema"
)

// expectDismissedView verifies both exact indicators are absent from the view
// and model while the terminal state owns the presentation.
func expectNoStaleIndicators(t *testing.T, name string, m Model) {
	t.Helper()
	view := m.View()
	if strings.Contains(view, StaleSchemaStatus) {
		t.Errorf("%s: view still shows stale status:\n%s", name, view)
	}
	if strings.Contains(view, "could not refresh") {
		t.Errorf("%s: view still shows an inline refresh cause:\n%s", name, view)
	}
	if m.SchemaStale() {
		t.Errorf("%s: model still reports stale schema", name)
	}
}

func expectIndicatorCount(t *testing.T, name string, m Model, want int) {
	t.Helper()
	if got := strings.Count(m.View(), StaleSchemaStatus); got != want {
		t.Errorf("%s: stale status appears %d times, want %d:\n%s", name, got, want, m.View())
	}
}

// staleWithCause produces a stale popup whose reported cause is exactly the
// given text.
func staleWithCause(t *testing.T, cause string) Model {
	t.Helper()
	fake := &fakeRefresher{queued: []schema.Attempt{failAttempt(cause)}}
	stale := pumpDrive(tableReadyModel(fake), tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if got := stale.staleCauseForTest(); got != staleCauseMessage(errors.New(cause)) {
		t.Fatalf("fixture: cause=%q, want %q", got, staleCauseMessage(errors.New(cause)))
	}
	return stale
}

func (m Model) staleCauseForTest() string { return m.staleCause }

// TestRetryIssuesNewRequestAndKeepsExactIndicatorsWhilePending drives retry on
// a stale popup: it fires a fresh catalog request, keeps the unchanged stale
// catalog and both exact indicators until the attempt settles, and gates
// duplicate retries while one is outstanding.
func TestRetryIssuesNewRequestAndKeepsExactIndicatorsWhilePending(t *testing.T) {
	fake := &fakeRefresher{queued: []schema.Attempt{
		failAttempt("lock busy"),
		successAttempt(uiCatalog()), // retry eventually succeeds
	}}
	stale := pumpDrive(tableReadyModel(fake), tea.KeyMsg{Type: tea.KeyEnter}).(Model)

	next, cmd := stale.Update(RetrySchemaRefreshMsg{})
	retried := next.(Model)
	if cmd == nil {
		t.Fatal("retry issued no catalog request")
	}
	if retried.QB.EligibleTables()[0] != stale.QB.EligibleTables()[0] {
		t.Error("retry mutated the retained catalog before settling")
	}
	expectIndicatorCount(t, "retry-pending", retried, 1)
	again, cmd2 := retried.Update(RetrySchemaRefreshMsg{})
	if cmd2 != nil {
		t.Error("duplicate retry started a second concurrent request")
	}
	var _ = again

	settledMsg := cmd()
	m, _ := retried.Update(settledMsg)
	final := m.(Model)
	expectIndicatorCount(t, "post-settle", final, 0)
}

// TestSuccessfulRetryClearsIndicatorsAtomicallyAndContinuesPopup drives a
// failing open, one retry succeeding against a changed catalog: stale status
// and cause vanish together, the refreshed eligible list replaces the popup's
// offered candidates, and Enter accepts from the refreshed data.
func TestSuccessfulRetryClearsIndicatorsAtomicallyAndContinuesPopup(t *testing.T) {
	fake := &fakeRefresher{queued: []schema.Attempt{
		failAttempt("lock busy"),
		successAttempt(&schema.Catalog{Version: 12, Objects: []*schema.Object{
			{Name: "orders", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas},
		}}),
	}}
	stale := pumpDrive(tableReadyModel(fake), tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	wantCause := staleCauseMessage(errors.New("lock busy"))
	if got := stale.staleCauseForTest(); got != wantCause {
		t.Fatalf("setup: cause=%q, want %q", got, wantCause)
	}
	retryM, retryCmd := stale.Update(RetrySchemaRefreshMsg{})
	if retryCmd == nil {
		t.Fatal("retry issued no catalog request")
	}
	finalM, _ := retryM.Update(retryCmd())
	m := finalM.(Model)
	expectNoStaleIndicators(t, "successful-retry", m)
	if m.ContinuationBlocked() {
		t.Error("successful retry left continuation blocked")
	}
	if m.Popup == nil || !m.Popup.Open() {
		t.Fatal("successful retry did not continue the Table popup")
	}
	if got := popupVisibleIDs(m); !slices.Equal(got, []string{"orders"}) {
		t.Errorf("continued popup offers %v, want refreshed [orders]", got)
	}
	accepted := drive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	name, ok := accepted.QB.SelectedTable()
	if !ok || name != "orders" {
		t.Errorf("acceptance after retry committed (%q,%v), want orders", name, ok)
	}
	if accepted.Focus != 1 || accepted.Fields[accepted.Focus].Label != tableFieldLabel {
		t.Errorf("acceptance restored focus=%d (%q), want exact Table opener",
			accepted.Focus, accepted.Fields[accepted.Focus].Label)
	}
}

// TestRepeatedFailureRetainsCandidatesAndUpdatesCauseOnly scripts a failed
// open followed by a retry failing with a different cause: the prior catalog
// stays put, the inline cause becomes exactly that attempt's cause, and
// retry/cancel remain available afterwards.
func TestRepeatedFailureRetainsCandidatesAndUpdatesCauseOnly(t *testing.T) {
	prior := uiCatalog()
	fake := &fakeRefresher{queued: []schema.Attempt{
		failAttempt("lock busy"),
		failAttempt("database disk image is malformed"),
	}}
	m := tableReadyModelSeeded(fake, prior)
	stale := pumpDrive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	before := slices.Clone(popupVisibleIDs(stale))

	retried, cmd := stale.Update(RetrySchemaRefreshMsg{})
	r := retried.(Model)
	if cmd == nil {
		t.Fatal("second attempt issued no catalog request")
	}
	settled, _ := r.Update(cmd())
	got := settled.(Model)

	if !slices.Equal(popupVisibleIDs(got), before) {
		t.Errorf("repeated failure changed candidates %v -> %v", before, popupVisibleIDs(got))
	}
	for i, o := range got.QB.EligibleTables() {
		if o != prior.Objects[i] {
			t.Errorf("retained object %p differs from prior snapshot entry %p", o, prior.Objects[i])
		}
	}
	expectIndicatorCount(t, "repeated-failure", got, 1)
	if want := staleCauseMessage(errors.New("database disk image is malformed")); got.staleCauseForTest() != want {
		t.Errorf("inline cause=%q, want exactly the latest attempt's %q", got.staleCauseForTest(), want)
	}
	if _, cmd := got.Update(RetrySchemaRefreshMsg{}); cmd == nil {
		t.Error("repeated failure did not leave retry available")
	}
}

// TestCancelClosesFlowAndRestoresExactPreOpenState drives cancel on a stale
// popup opened from Table: only the stale flow closes — the popup disappears,
// the exact pre-open builder/catalog state (command DELETE, selected users,
// eligible lists) and the Table opener focus are restored verbatim, and no
// continuation or execution starts.
func TestCancelClosesFlowAndRestoresExactPreOpenState(t *testing.T) {
	prior := uiCatalog()
	fake := &fakeRefresher{queued: []schema.Attempt{failAttempt("lock busy")}}
	pre := tableReadyModelSeeded(fake, prior)
	pre.QB = pre.QB.SelectTable("users")
	pre.Fields = builderFields(pre.QB)

	stale := pumpDrive(pre, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	canceled, _ := stale.Update(CancelStaleRefreshMsg{})
	after := canceled.(Model)

	if after.Popup != nil {
		t.Error("cancel left the popup installed")
	}
	expectNoStaleIndicators(t, "cancel", after)
	if !after.ContinuationBlocked() == false {
		t.Log("continuation restored") // cancel reopens the workflow
	}
	if after.ContinuationBlocked() {
		t.Error("cancel left downstream continuation blocked with no stale flow active")
	}
	name, selected := after.QB.SelectedTable()
	if !selected || after.Fields[after.Focus].Content != "users" {
		t.Errorf("pre-open selection lost: table=(%q,%v) content=%q", name, selected,
			after.Fields[after.Focus].Content)
	}
	if after.QB.Command() != qb.CommandUpdate {
		t.Errorf("command changed during stale flow: %v", after.QB.Command())
	}
	for i, o := range after.QB.EligibleTables() {
		if o != prior.Objects[i] {
			t.Errorf("catalog snapshot replaced on cancel: entry %d=%p want prior %p", i, o, prior.Objects[i])
		}
	}
	if after.Focus != 1 || after.Fields[after.Focus].Label != tableFieldLabel {
		t.Errorf("cancel restored focus=%d (%q), want exact Table opener",
			after.Focus, after.Fields[after.Focus].Label)
	}
	if after.Trace != nil && after.Trace.Settled && after.Trace.Grid != nil {
		t.Error("cancel executed database work: tracer grid settled")
	}
}

// terminalFixture lands the stale flow into one terminal classification by
// injecting a typed Connection health outcome as the very first refresh result.
func terminalFixture(t *testing.T, status schema.RefreshStatus) Model {
	t.Helper()
	fake := &fakeRefresher{queued: []schema.Attempt{schema.NewTerminal(status)}}
	m := tableReadyModel(fake)
	return pumpDrive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
}

// TestDeletionOverridesStaleWorkflowImmediately asserts that a typed
// connection deletion outcome replaces the whole stale workflow with the exact
// terminal state before any work completes.
func TestDeletionOverridesStaleWorkflowImmediately(t *testing.T) {
	end := terminalFixture(t, schema.RefreshDeleted)
	if end.TerminalState() != TerminalDeleted {
		t.Fatalf("deletion produced terminal state %v", end.TerminalState())
	}
	view := end.View()
	if view != DeletedSessionEndedMessage {
		t.Errorf("terminal view =\n%s\nwant exactly %q", view, DeletedSessionEndedMessage)
	}
	if end.Popup != nil {
		t.Error("terminal deletion left a popup installed")
	}
	expectNoStaleIndicators(t, "terminal-deletion", end)
	if _, cmd := end.Update(RetrySchemaRefreshMsg{}); cmd != nil {
		t.Error("deletion kept a retry affordance alive")
	}
	if _, cmd := end.Update(CancelStaleRefreshMsg{}); cmd != nil {
		t.Error("deletion kept a cancel affordance alive")
	}
	if !end.ContinuationBlocked() {
		t.Error("terminal state must block all continuation")
	}
	opened := drive(end, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if opened.Popup != nil || opened.View() != DeletedSessionEndedMessage {
		t.Error("database work became available again under terminal state")
	}
}

// TestReplacementOverridesStaleWorkflowImmediately mirrors deletion through
// the replacement classification.
func TestReplacementOverridesStaleWorkflowImmediately(t *testing.T) {
	end := terminalFixture(t, schema.RefreshReplaced)
	if end.TerminalState() != TerminalReplaced {
		t.Fatalf("replacement produced terminal state %v", end.TerminalState())
	}
	if view := end.View(); view != ReplacedSessionEndedMessage {
		t.Errorf("terminal view =\n%s\nwant exactly %q", view, ReplacedSessionEndedMessage)
	}
	expectNoStaleIndicators(t, "terminal-replacement", end)
}

// TestReclassifiedErrorBeatsOrdinaryCause injects an ordinary failure whose
// settle carries a typed replacement classification instead of the raw cause:
// the classification wins and no `could not refresh` line renders.
func TestReclassifiedErrorBeatsOrdinaryCause(t *testing.T) {
	fake := &fakeRefresher{queued: []schema.Attempt{
		failAttempt(failureCauseText),
		failAttempt("lock busy"),
	}}
	stale := pumpDrive(tableReadyModel(fake), tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	retryM, retryCmd := stale.Update(RetrySchemaRefreshMsg{})
	if retryCmd == nil {
		t.Fatal("retry issued no request to reclassify")
	}
	settled, okm := retryCmd().(SchemaRefreshSettledMsg)
	if !okm {
		t.Fatalf("retry command yielded unexpected message %#v", retryCmd())
	}
	settled.Result = schema.NewTerminal(schema.RefreshReplaced) // post-error reclassification
	ended, _ := retryM.Update(settled)
	final := ended.(Model)
	if final.View() != ReplacedSessionEndedMessage {
		t.Errorf("reclassification view=\n%s\nwant exact terminal message", final.View())
	}
	expectNoStaleIndicators(t, "reclassified-terminal", final)
}

// TestLateResultsRejectedAfterCancelAndTerminal proves superseded identities
// and post-terminal completions cannot mutate state: an in-flight retry's
// success delivered after cancel is dropped without installing anything, and
// deliveries arriving once deletion terminated the session leave the exact
// terminal presentation and retained catalog untouched.
func TestLateResultsRejectedAfterCancelAndTerminal(t *testing.T) {
	ghost := &schema.Catalog{Version: 99, Objects: []*schema.Object{
		{Name: "ghost", Kind: schema.KindOrdinaryTable, WriteEligible: true},
	}}

	fake := &fakeRefresher{queued: []schema.Attempt{failAttempt("lock busy")}}
	stale := pumpDrive(tableReadyModel(fake), tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	retried, cmd := stale.Update(RetrySchemaRefreshMsg{})
	r := retried.(Model)
	retryID := r.currentAttemptForTest()
	var _ = cmd // simulated still in flight: never settled directly

	canceledM, _ := r.Update(CancelStaleRefreshMsg{})
	canceled := canceledM.(Model)
	superseded := SchemaRefreshSettledMsg{Attempt: retryID, Result: schema.NewSuccess(ghost)}
	finalM, _ := canceled.Update(superseded)
	final := finalM.(Model)
	if final.Popup != nil || final.SchemaStale() || strings.Contains(final.View(), StaleSchemaStatus) {
		t.Errorf("late success after cancel mutated the closed flow: popup=%v stale=%v", final.Popup != nil, final.SchemaStale())
	}
	for _, o := range final.QB.EligibleTables() {
		if o.Name == "ghost" {
			t.Error("late success installed the ghost catalog through a canceled flow")
		}
	}

	end := terminalFixture(t, schema.RefreshDeleted)
	before := namesOf(end.QB.EligibleTables())
	last, _ := end.Update(SchemaRefreshSettledMsg{
		Attempt: end.currentAttemptForTest(), Result: schema.NewSuccess(ghost)})
	done := last.(Model)
	if view := done.View(); view != DeletedSessionEndedMessage {
		t.Errorf("late delivery escaped the terminal state, view:\n%s", view)
	}
	if !slices.Equal(namesOf(done.QB.EligibleTables()), before) {
		t.Errorf("late delivery replaced the catalog: %v -> %v", before, namesOf(done.QB.EligibleTables()))
	}
}

func (m Model) currentAttemptForTest() uint64 { return m.refreshAttempt }
