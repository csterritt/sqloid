// Guided WHERE popup integration for Issue #17: the builder's Where field
// opens the reusable column → operator → conditional value-entry sequence over
// the QueryBuilder predicate transitions. The UI never re-encodes operator or
// parsing behavior: column eligibility comes from WhereCandidates, the fixed
// operator list from FixedOperators, value-required versus no-value routing
// from the typed operator contract, and completion from CommitWhereDraft.
// Esc at any stage discards only the open draft, restoring a previously
// completed predicate and the exact opener focus without partial commits.

package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	qb "github.com/chris/sqloid/internal/querybuilder"
)

// applyWhereFlow installs a new builder snapshot from a WHERE-flow transition
// and keeps UI focus exactly on the guided flow's own Where field, whose
// identity never changes during column/operator/value staging regardless of
// the builder's next-required-field pointer.
func applyWhereFlow(m *Model, next qb.QueryBuilder) {
	m.applyBuilder(next)
	for i := range m.Fields {
		if m.Fields[i].Label == whereFieldLabel {
			m.setFocus(i)
			return
		}
	}
}

// whereFieldLabel is the field-bar label of the optional WHERE field shared by
// SELECT, UPDATE, and DELETE; it also names the opener identity for every
// stage of the guided flow so accept/cancel restore that exact focus.
const whereFieldLabel = "Where"

// wherePopupViewport caps how many candidates are visible at once in both
// WHERE popups; longer lists scroll within this window.
const wherePopupViewport = 8

// WhereTypedNullHint is the exact inline guidance rendered on every WHERE
// value prompt: typed `NULL` is TEXT under universal parsing, never SQL null.
const WhereTypedNullHint = "'NULL' binds as literal TEXT — use IS NULL / IS NOT NULL for SQL NULL"

// WhereNullHelpLines are the contextual help lines explaining ordinary
// comparison and LIKE SQL-null semantics and SQLite wildcard meaning.
func WhereNullHelpLines() []string {
	return []string{
		"Ordinary comparisons and LIKE do not match rows where the column IS NULL",
		"'%' and '_' keep their SQLite wildcard meaning inside LIKE values",
	}
}

// whereFocused reports whether the field bar currently has Where focused,
// guarding against suspension and open overlays like tableFocused.
func (m *Model) whereFocused() bool {
	if m.suspended || m.Popup != nil || m.ValuePrompt != nil || m.Focus < 0 || m.Focus >= len(m.Fields) {
		return false
	}
	return m.Fields[m.Focus].Label == whereFieldLabel
}

// beginWhereEdit opens the searchable eligible-column popup for the guided
// flow whenever Enter lands on the focused Where field. Candidates derive from
// QueryBuilder.WhereCandidates so refreshed schema identities flow through
// unchanged; no database work is issued.
func (m *Model) beginWhereEdit() {
	if !m.whereFocused() {
		return
	}
	m.installPopup(NewSearchablePopup(whereFieldLabel, whereColumnCandidates(m.QB)),
		whereColumnAcceptHook)
	m.Popup.SetViewportHeight(wherePopupViewport)
}

// whereColumnCandidates converts the builder's eligible column identities into
// popup rows keyed by declared name, in schema order.
func whereColumnCandidates(q qb.QueryBuilder) []PopupCandidate {
	cols := q.WhereCandidates()
	out := make([]PopupCandidate, 0, len(cols))
	for _, c := range cols {
		out = append(out, PopupCandidate{ID: c.Name, Display: c.Name})
	}
	return out
}

// operatorPopupCandidates lists the fixed operators by exact SQL token, in
// deterministic presentation order — the same identity used to accept them.
func operatorPopupCandidates() []string {
	var out []string
	for _, op := range qbNewQueryBuilderOperatorOrder() {
		tok, err := op.SQLToken()
		if err != nil {
			continue // unreachable for the closed set; defensive silence
		}
		out = append(out, tok)
	}
	return out
}

// qbNewQueryBuilderOperatorOrder fetches the closed operator set through a
// throwaway builder so the UI holds no copy of it.
func qbNewQueryBuilderOperatorOrder() []qb.Operator {
	return qb.QueryBuilder{}.FixedOperators()
}

// whereColumnAcceptHook commits one accepted column identity by beginning (or
// revising) the guided draft through StartWhere — which seeds same-column
// revisions with the prior completed choice — then moves to the scroll-only
// operator step.
func whereColumnAcceptHook(m *Model, id string) {
	next, ok := m.QB.StartWhere(id)
	if !ok {
		return
	}
	applyWhereFlow(m, next)
	openOperatorPopup(m)
}

// whereRestoreHighlight walks the fresh operator popup onto the draft's
// previously chosen operator when one exists, so revisiting restores the
// selection exactly rather than defaulting to the top row.
func whereRestoreHighlight(m *Model) {
	draft := m.QB.WhereDraft()
	op, ok := draft.ChosenOperator()
	if !ok {
		return
	}
	token, err := op.SQLToken()
	if err != nil {
		return
	}
	for i, cand := range operatorPopupCandidates() {
		if cand == token {
			for j := 0; j < i; j++ {
				m.Popup.Down()
			}
			return
		}
	}
}

// openOperatorPopup installs the scroll-only fixed-operator popup seeded from
// the current draft state. Every documented operator appears for every column;
// ordering and membership follow QueryBuilder's closed set.
func openOperatorPopup(m *Model) {
	rows := make([]PopupCandidate, 0, len(operatorPopupCandidates()))
	for _, tok := range operatorPopupCandidates() {
		rows = append(rows, PopupCandidate{ID: tok, Display: tok})
	}
	m.installPopup(NewScrollOnlyPopup(whereFieldLabel, rows), whereOperatorAcceptHook)
	m.Popup.SetViewportHeight(wherePopupViewport)
	whereRestoreHighlight(m)
}

// whereOperatorAcceptHook routes one accepted operator by its transition
// result rather than any UI-local knowledge: no-value operators complete
// immediately, value-taking ones open Issue #14's universal entry seeded with
// the restored entered representation when revision preserved it.
func whereOperatorAcceptHook(m *Model, id string) {
	chosen, ok := parseOperatorToken(id)
	if !ok {
		return
	}
	pred, ok := m.QB.WhereDraft().ChooseOperator(chosen)
	if !ok {
		return
	}
	applied := m.QB.ApplyWhereDraft(pred)
	applyWhereFlow(m, applied)
	if chosen.TakesValue() {
		openWhereValuePrompt(m, applied)
		return
	}
	commitWhereDraftAndClose(m, applied)
}

// parseOperatorToken maps an accepted SQL token back to its typed operator via
// the builder's own set; unknown tokens are rejected defensively.
func parseOperatorToken(token string) (qb.Operator, bool) {
	for _, op := range qbNewQueryBuilderOperatorOrder() {
		if tok, err := op.SQLToken(); err == nil && tok == token {
			return op, true
		}
	}
	return 0, false
}

// stagedPredicateLabels renders a human label describing the staged draft for
// the value prompt heading.
func stagedPredicateLabel(m *Model) string {
	draft := m.QB.WhereDraft()
	head := ""
	if col, ok := draft.Column(); ok {
		head = col.Name + " "
	}
	if op, ok := draft.ChosenOperator(); ok {
		if tok, err := op.SQLToken(); err == nil {
			head += tok
		}
	}
	return head
}

// openWhereValuePrompt opens the universal text entry over the focused Where
// field, seeded byte-for-byte with the draft's restored input when an earlier
// completion is being revised on the same column.
func openWhereValuePrompt(m *Model, applied qb.QueryBuilder) {
	seed := ""
	draftCol, hasDraftCol := applied.WhereDraft().Column()
	if !hasDraftCol {
		m.ValuePrompt = NewValuePrompt(whereFieldLabel, stagedPredicateLabel(m), seed)
		return
	}
	// Revision on the same column restores the exact prior entered
	// representation from the untouched commitment.
	if prior := applied.WherePredicate(); prior.State() == qb.WhereComplete {
		if col, ok := prior.Column(); ok && col == draftCol {
			if entered, ok2 := prior.Entered(); ok2 {
				seed = entered
			}
		}
	}
	m.ValuePrompt = NewValuePrompt(whereFieldLabel, stagedPredicateLabel(m), seed)
}

// handleValuePromptKey routes one key message into the open universal entry:
// Enter submits the verbatim buffer to the owner hook, Esc cancels the whole
// open draft — restoring any prior completion untouched — and other keys go
// to the prompt itself.
func (m Model) handleValuePromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		opener := m.ValuePrompt.Opener
		text := m.ValuePrompt.Buffer()
		m.closeValuePrompt()
		if opener == limitFieldLabel {
			limitPromptAccepted(&m, text)
		} else {
			whereValueAccepted(&m, text)
		}
		m.adjustScroll()
		return m, nil
	case tea.KeyEsc:
		opener := m.ValuePrompt.Opener
		m.closeValuePrompt()
		if opener == limitFieldLabel {
			limitPromptCancelled(&m)
		} else {
			whereValueCancelled(&m)
		}
		m.adjustScroll()
		return m, nil
	}
	m.ValuePrompt.HandleKey(msg)
	m.adjustScroll()
	return m, nil
}

// closeValuePrompt removes the open entry without touching focus restoration;
// callers either restore it explicitly through applyBuilder or keep focus.
func (m *Model) closeValuePrompt() { m.ValuePrompt = nil }

// commitWhereDraftAndClose promotes a structurally complete draft through
// CommitWhereDraft — the sole completion boundary shared by SELECT, UPDATE,
// and DELETE consumers — leaving focus on the Where field whose label keeps
// rendering the committed predicate.
func commitWhereDraftAndClose(m *Model, b qb.QueryBuilder) {
	next, ok := b.CommitWhereDraft()
	if ok {
		applyWhereFlow(m, next)
	}
}

// whereValueAccepted submits the entered representation through the builder's
// universal parser: typed `NULL`, empty input, and LIKE wildcards stay verbatim
// bound values because this layer adds no parsing or normalization of its own.
func whereValueAccepted(m *Model, text string) {
	submitted, ok := m.QB.WhereDraft().SubmitValue(text)
	if !ok {
		// The draft vanished underneath (e.g. downstream clear): drop silently.
		return
	}
	commitWhereDraftAndClose(m, m.QB.ApplyWhereDraft(submitted))
}

// whereValueCancelled cancels the open draft entirely so no partial commit can
// survive; a previously completed predicate remains intact for later restore.
func whereValueCancelled(m *Model) {
	if m.QB.WhereDrafting() {
		applyWhereFlow(m, m.QB.CancelWhereDraft())
	}
}
