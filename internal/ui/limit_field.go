// LIMIT field integration for Issue #18: the builder's Limit field opens the
// universal text-entry prompt (Issues #14 and #17 seam), submits the verbatim
// buffer through the QueryBuilder's bounded LIMIT parser, and shows the
// builder's exact invalid reason verbatim — this layer keeps no validator of
// its own. Cancel restores the prior committed representation untouched.

package ui

import (
	qb "github.com/chris/sqloid/internal/querybuilder"
)

// limitFieldLabel is the field-bar label of the LIMIT field; it also names
// the prompt opener identity so accept/cancel restore that exact focus.
const limitFieldLabel = "Limit"

// limitFocused reports whether the field bar currently has Limit focused,
// guarding against suspension and open overlays like tableFocused.
func (m *Model) limitFocused() bool {
	if m.suspended || m.Popup != nil || m.ValuePrompt != nil || m.Focus < 0 || m.Focus >= len(m.Fields) {
		return false
	}
	return m.Fields[m.Focus].Label == limitFieldLabel
}

// beginLimitPrompt opens the universal text entry over the focused Limit
// field, seeded byte-for-byte with the currently entered representation so
// revision restores it exactly and cancel preserves the prior value.
func (m *Model) beginLimitPrompt() {
	if !m.limitFocused() {
		return
	}
	m.ValuePrompt = NewValuePrompt(limitFieldLabel, "row limit", m.QB.LimitInput())
}

// limitPromptAccepted submits the entered representation verbatim through
// SetLimitInput. Empty input commits as the unbounded logical result; valid
// input stores its canonical integer; invalid input keeps the entered text
// and reports the builder's exact reason through the rendered field.
func limitPromptAccepted(m *Model, text string) {
	m.applyBuilder(m.QB.SetLimitInput(text))
	refocusField(m, limitFieldLabel)
}

// limitPromptCancelled discards the open draft: the builder's committed
// representation was never changed by opening the prompt, so cancel restores
// it untouched.
func limitPromptCancelled(m *Model) {}

// clearLimitField removes the whole Limit value — entered representation and
// any accepted integer — from the focused base Limit field.
func clearLimitField(m *Model) {
	m.applyBuilder(m.QB.SetLimitInput(""))
	refocusField(m, limitFieldLabel)
}

// limitFieldContent renders the entered representation verbatim; when the
// builder rejects it, the exact QueryBuilder reason is appended so the
// user-facing feedback is QueryBuilder-owned and shown verbatim.
func limitFieldContent(q qb.QueryBuilder) string {
	input := q.LimitInput()
	if input == "" {
		return ""
	}
	if _, ok := q.LimitValue(); ok {
		return input
	}
	return input + " — " + qb.LimitInvalidReason
}
