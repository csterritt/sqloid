// Page-request SQL construction (Issue #25): the minimal extension of the
// safe SELECT rendering path for adjacent vertical paging. PageSQL renders
// the complete page statement — the base SELECT with any user ORDER BY kept
// byte-for-byte, the single eligible `ORDER BY rowid` fallback applied when
// there is no user ORDER BY, and an exact integer LIMIT/OFFSET range clamped
// around the user's logical Limit so a page can never read beyond it. No
// navigation orchestration, cache behavior, or new ordering guarantee lives
// here.

package querybuilder

import (
	"strconv"
	"strings"
)

// PageSQL renders the page-request statement for the range of at most
// pageLimit logical rows starting at logical offset offset (0-based). The
// rendered LIMIT is the page limit clamped to the remaining user Limit, so
// offsets at or beyond the user's Limit yield an empty string instead of a
// statement that reads past it. An empty result also covers every
// non-runnable SELECT — the authoritative RunnableReport gates the shared
// renderSelectCore seam (Issue #66) — and invalid ranges (pageLimit < 1 or
// offset < 0); it never means a partially valid query. Range validation stays
// independent of builder validity. The user's entered Limit text is never
// interpolated: only accepted integers render, canonically.
func (q QueryBuilder) PageSQL(pageLimit, offset int64) string {
	if pageLimit < 1 || offset < 0 {
		return ""
	}
	parts, ok := renderSelectCore(q, true)
	if !ok {
		return ""
	}
	limit := pageLimit
	if userLimit, has := q.LimitValue(); has {
		remaining := userLimit - offset
		if remaining <= 0 {
			return "" // the page would read beyond the user's logical Limit
		}
		if remaining < limit {
			limit = remaining
		}
	}
	parts = append(parts,
		"LIMIT "+strconv.FormatInt(limit, 10),
		"OFFSET "+strconv.FormatInt(offset, 10))
	return strings.Join(parts, " ")
}

// PageParams returns this snapshot's page-request bound parameters in the
// same deterministic order as SelectParams (currently only a completed WHERE
// value). The page statement's parameter count always matches SelectParams:
// LIMIT/OFFSET render as literal integers, never parameters.
func (q QueryBuilder) PageParams() []any {
	return q.SelectParams()
}
