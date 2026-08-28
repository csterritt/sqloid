// SELECT LIMIT state (Issue #18): one entered representation kept verbatim
// beside its optional accepted integer, parsed with a strict bounded rule —
// only empty input means an unbounded logical result, and only canonical
// base-10 integer text from 1 through the signed-int64 maximum is accepted.
// Zero, negatives, signs, whitespace, decimal/exponent/hex forms, nonnumeric
// text, and overflow all classify invalid with one exact user-facing reason;
// no secondary UI validator exists.

package querybuilder

import "strconv"

// LimitInvalidReason is the exact invalid feedback for every rejected Limit
// input category; tests assert this wording verbatim.
const LimitInvalidReason = "Limit must be an integer from 1 to 9223372036854775807"

// SetLimitInput records text as the entered Limit representation and re-parses
// it in place. Empty input clears to the unbounded logical result (no clause,
// nothing accepted); any other value either stores its exact int64 when within
// [1, 9223372036854775807] or stays unaccepted so FirstInvalidIssue can report
// the exact reason. The entered bytes are always preserved verbatim for
// correction and history comparison — including valid inputs.
func (q QueryBuilder) SetLimitInput(text string) QueryBuilder {
	next := q
	next.limitInput = text
	next.limitVal, next.limitHas = parseLimitText(text)
	return next
}

// LimitInput returns the entered representation byte-for-byte as last set.
func (q QueryBuilder) LimitInput() string { return q.limitInput }

// LimitValue reports the accepted integer and whether one exists. Invalid or
// empty input never reports a value: empty is unbounded by definition, and
// invalid input must never be bound or interpolated anywhere.
func (q QueryBuilder) LimitValue() (int64, bool) { return q.limitVal, q.limitHas }

// parseLimitText applies the closed v1 grammar: one nonempty run of ASCII
// digits whose value parses into uint64 without overflow and lies in
// [1, MaxInt64]. Leading zeros are tolerated at entry but always render
// canonically; everything else — empty aside — is invalid by classification.
func parseLimitText(text string) (int64, bool) {
	if text == "" {
		return 0, false
	}
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return 0, false
		}
	}
	u, err := strconv.ParseUint(text, 10, 64)
	if err != nil || u < 1 || u > 1<<63-1 {
		return 0, false
	}
	return int64(u), true
}

// validateLimit reports the exact field/reason pair whenever a nonempty
// entered representation was not accepted.
func validateLimit(q QueryBuilder) (InvalidIssue, bool) {
	if q.limitInput == "" || q.limitHas {
		return InvalidIssue{}, false
	}
	return InvalidIssue{FieldIdentityLimit, LimitInvalidReason}, true
}

// renderLimit renders the canonical numeric LIMIT clause when an integer was
// accepted; empty or unaccepted input renders nothing at all.
func renderLimit(q QueryBuilder) string {
	v, ok := q.LimitValue()
	if !ok {
		return ""
	}
	return "LIMIT " + strconv.FormatInt(v, 10)
}
