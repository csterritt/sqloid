// Scripted Bubble Tea coverage for Issue #13 Task 1: every Table-popup open
// issues a fresh main-schema catalog request before any refreshed result is
// presented; an ordinary refresh failure keeps the unchanged prior catalog —
// candidate identities, ordering, metadata, search state, and the selected
// builder table all survive — while the persistent status is exactly
// `Schema data is stale — retry or cancel` and the inline cause is exactly
// `could not refresh: <cause>`, appearing exactly once per rendered surface.
// Both indicators survive ordinary model updates and accepting a stale
// candidate, advancing to another builder field, or continuing the workflow
// toward execution is blocked. Retry, cancellation, and terminal override
// arrive with later tasks; a deterministic fake stands in for the Connection
// boundary so no database access runs here.

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

const failureCauseText = "lock busy"

// fakeRefresher is the deterministic fake Connection boundary: it yields one
// queued attempt per RefreshCatalog call so every test scripts exact
// outcomes; extra calls fail loudly instead of inventing data.
type fakeRefresher struct {
	queued []schema.Attempt
	calls  int
}

func (f *fakeRefresher) RefreshCatalog() schema.Attempt {
	f.calls++
	if len(f.queued) == 0 {
		return schema.NewFailure(errors.New("unexpected extra refresh"))
	}
	next := f.queued[0]
	f.queued = f.queued[1:]
	return next
}

func successAttempt(c *schema.Catalog) schema.Attempt { return schema.NewSuccess(c) }
func failAttempt(cause string) schema.Attempt         { return schema.NewFailure(errors.New(cause)) }

// pumpDrive sends msgs through Update and, whenever Update returns a command,
// executes it and feeds its settled message straight back through Update —
// exactly what the Bubble Tea runtime does between rendered frames.
func pumpDrive(m tea.Model, msgs ...tea.Msg) tea.Model {
	for _, msg := range msgs {
		next, cmd := m.Update(msg)
		m = next
		for cmd != nil {
			settled := cmd()
			next, cmd = m.Update(settled)
			m = next
			if settled == nil {
				break // guard against degenerate never-settling commands
			}
		}
	}
	return m
}

func popupVisibleIDs(m Model) []string {
	var out []string
	for _, c := range m.Popup.Visible() {
		out = append(out, c.ID)
	}
	return out
}

// tableReadyModel seeds the prior typed catalog and selects DELETE so the
// write-eligible prior list differs from SELECT's, then wires the fake
// Connection boundary before any popup exists.
func tableReadyModel(fake *fakeRefresher) Model {
	return tableReadyModelSeeded(fake, uiCatalog())
}

// tableReadyModelSeeded seeds an explicit prior catalog instance so tests can
// compare retained object identity verbatim against what they passed in.
func tableReadyModelSeeded(fake *fakeRefresher, prior *schema.Catalog) Model {
	m := drive(sized(New(), 80, 24), SchemaRefreshedMsg{Catalog: prior}, key('u')).(Model)
	if m.Focus != 1 || m.Fields[m.Focus].Label != tableFieldLabel {
		panic("tableReadyModel setup drifted: want Table focused")
	}
	m.Refresher = fake
	return m
}

// staleFixture opens the Table popup, lets its forced refresh fail with
// `lock busy`, and returns the model in the resulting stale state.
func staleFixture(t *testing.T, fake *fakeRefresher) Model {
	t.Helper()
	stale := pumpDrive(tableReadyModel(fake), tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if !stale.SchemaStale() {
		t.Fatal("failed refresh did not enter the stale state")
	}
	if stale.Popup == nil || !stale.Popup.Open() {
		t.Fatal("failed refresh lost the open popup")
	}
	assertStalePopupView(t, stale)
	return stale
}

// assertStalePopupView checks that one failed-refresh result renders each
// exact indicator exactly once on the open-popup surface.
func assertStalePopupView(t *testing.T, stale Model) {
	t.Helper()
	want := staleCauseMessage(errors.New(failureCauseText))
	view := stale.View()
	if got := strings.Count(view, StaleSchemaStatus); got != 1 {
		t.Errorf("persistent stale status appears %d times in the open-popup view, want exactly once:\n%s", got, view)
	}
	if !strings.Contains(view, want) {
		t.Errorf("view lacks inline cause %q:\n%s", want, view)
	}
}

// TestTableOpenIssuesFreshCatalogRequestPerOpen requires each Table-popup open
// to issue one fresh main-schema catalog request; the settled result, not the
// pre-open presentation, replaces the offered candidates afterward.
func TestTableOpenIssuesFreshCatalogRequestPerOpen(t *testing.T) {
	fake := &fakeRefresher{}
	fresh := &schema.Catalog{Version: 8, Objects: []*schema.Object{
		{Name: "events", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas},
	}}
	fake.queued = append(fake.queued, successAttempt(fresh))

	opened := pumpDrive(tableReadyModel(fake), tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if fake.calls != 1 {
		t.Fatalf("one open issued %d catalog requests, want exactly one", fake.calls)
	}

	settled := pumpDrive(opened).(Model)
	if got := popupVisibleIDs(settled); !slices.Equal(got, []string{"events"}) {
		t.Errorf("after success candidates=%v, want exactly the freshly refreshed [events]", got)
	}
	for _, o := range settled.QB.EligibleTables() {
		if o.Name == "events" && o != fresh.Objects[0] {
			t.Error("refreshed catalog objects were copied instead of installed verbatim")
		}
	}
}

// TestTableReopenAlwaysIssuesFreshRequest drives open → settle → Esc → reopen
// and requires the second open to run another full request cycle, replacing
// what the previous refresh had presented.
func TestTableReopenAlwaysIssuesFreshRequest(t *testing.T) {
	fake := &fakeRefresher{}
	first := uiCatalog()
	second := &schema.Catalog{Version: 9, Objects: []*schema.Object{
		{Name: "renamed_users", Kind: schema.KindOrdinaryTable, WriteEligible: true, Rowid: schema.RowidHas},
	}}
	fake.queued = append(fake.queued, successAttempt(first), successAttempt(second))

	m := pumpDrive(tableReadyModel(fake), tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyEsc}, tea.KeyMsg{Type: tea.KeyEnter}).(Model)

	if fake.calls != 2 {
		t.Fatalf("two opens issued %d requests, want a fresh one per open", fake.calls)
	}
	settled := pumpDrive(m).(Model)
	if got := popupVisibleIDs(settled); !slices.Equal(got, []string{"renamed_users"}) {
		t.Errorf("second-open candidates=%v, want exactly the newly refreshed list", got)
	}
}

// TestFailedRefreshRetainsStalePopupWithExactIndicators is the core stale
// contract: unchanged candidate identities/order/metadata plus both exact
// indicators rendered exactly once, surviving ordinary model updates.
func TestFailedRefreshRetainsStalePopupWithExactIndicators(t *testing.T) {
	fake := &fakeRefresher{queued: []schema.Attempt{failAttempt(failureCauseText)}}
	prior := uiCatalog()
	m := tableReadyModelSeeded(fake, prior)
	if name, ok := m.QB.SelectedTable(); ok {
		t.Fatalf("setup drift: unexpected pre-selection (%q,%v)", name, ok)
	}
	m.QB = m.QB.SelectTable("users")
	m.Fields = builderFields(m.QB)
	stale := pumpDrive(m, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	assertStalePopupView(t, stale)

	wantCandidates := []string{"logs_fts", "users"} // write-eligible prior list for UPDATE
	if got := popupVisibleIDs(stale); !slices.Equal(got, wantCandidates) {
		t.Errorf("stale popup shows %v, want the prior %v", got, wantCandidates)
	}
	eligible := stale.QB.EligibleTables()
	if !slices.Equal(namesOf(eligible), []string{"logs_fts", "users"}) {
		t.Fatalf("eligible list changed after failed refresh: %v", namesOf(eligible))
	}
	for i, o := range eligible {
		if o != prior.Objects[i] {
			t.Errorf("eligible[%d]=%p, want prior object %q retained verbatim",
				i, o, prior.Objects[i].Name)
		}
	}
	if name, ok := stale.QB.SelectedTable(); !ok || name != "users" {
		t.Errorf("selected table (%q,%v) changed during stale flow, want users kept", name, ok)
	}

	// Search state stays usable across the failure, then returns to full view.
	searched := drive(stale, key('e')).(Model)
	if searched.Popup.Search != "e" {
		t.Errorf("search text after typing = %q, want \"e\"", searched.Popup.Search)
	}
	if got := popupVisibleIDs(searched); !slices.Equal(got, []string{"users"}) {
		t.Errorf("'e' did not filter stale candidates as expected: %v", got)
	}
	clearSearch := drive(searched,
		tea.KeyMsg{Type: tea.KeyBackspace}, tea.KeyMsg{Type: tea.KeyBackspace}).(Model)
	if clearSearch.Popup.Search != "" || !slices.Equal(popupVisibleIDs(clearSearch), wantCandidates) {
		t.Errorf("backspacing twice left search=%q visible=%v",
			clearSearch.Popup.Search, popupVisibleIDs(clearSearch))
	}

	// Both indicators survive ordinary model updates (resize cycles, typing).
	survivor := drive(clearSearch,
		tea.WindowSizeMsg{Width: 100, Height: 30},
		tea.WindowSizeMsg{Width: 80, Height: 24},
		key('z'), tea.KeyMsg{Type: tea.KeyBackspace}).(Model)
	if !survivor.SchemaStale() {
		t.Fatal("ordinary updates cleared the stale state")
	}
	postView := survivor.View()
	if got := strings.Count(postView, StaleSchemaStatus); got != 1 {
		t.Errorf("after updates status appears %d times, want exactly once:\n%s", got, postView)
	}
	if !strings.Contains(postView, staleCauseMessage(errors.New(failureCauseText))) {
		t.Error("inline cause vanished after ordinary model updates")
	}
	if survivor.View() != postView {
		t.Error("rendering mutated across repeated views")
	}
}

// TestStaleStateBlocksAcceptanceContinuationAndNavigation proves that while
// the stale flow is active no unsafe path forward exists: Enter cannot accept
// a candidate, builder navigation cannot advance to another field, and the
// model reports execution gating through its blocked-continuation state.
func TestStaleStateBlocksAcceptanceContinuationAndNavigation(t *testing.T) {
	fake := &fakeRefresher{queued: []schema.Attempt{failAttempt(failureCauseText)}}
	stale := staleFixture(t, fake)

	afterEnter := drive(stale, tea.KeyMsg{Type: tea.KeyEnter}).(Model)
	if afterEnter.Popup == nil || !afterEnter.Popup.Open() {
		t.Fatal("Enter accepted despite stale schema data")
	}
	if _, ok := afterEnter.QB.SelectedTable(); ok {
		t.Error("stale Enter committed a selected table through the builder")
	}
	if afterEnter.Focus != 1 {
		t.Errorf("acceptance moved UI focus to %d", afterEnter.Focus)
	}

	tabbed := drive(afterEnter, tea.KeyMsg{Type: tea.KeyTab}, tea.KeyMsg{Type: tea.KeyDown}).(Model)
	if tabbed.Focus != 1 || tabbed.QB.Focus() != qb.FieldTable {
		t.Errorf("navigation advanced under stale flow: ui=%d builder=%v", tabbed.Focus, tabbed.QB.Focus())
	}
	if tabbed.Popup == nil || !tabbed.Popup.Open() {
		t.Error("navigation broke the stale popup gate")
	}

	if !stale.ContinuationBlocked() {
		t.Error("stale flow must block downstream continuation and execution")
	}
	if alive := drive(tabbed, tea.KeyMsg{Type: tea.KeyEsc}).(Model); alive.SchemaStale() || alive.Popup != nil || alive.Focus != 1 {
		t.Errorf("Esc failed to end the stale flow via cancel: stale=%v popup=%v focus=%d",
			alive.SchemaStale(), alive.Popup, alive.Focus)
	}
}
