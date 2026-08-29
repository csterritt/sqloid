// Issue #31 byte-cap and over-limit shared definitions, per the Cache and
// snapshot invariant of Notes/PRD-sqloid.md. This package is the
// authoritative result metadata boundary: the exact truncation warning and
// the exact page/value over-limit messages are defined here once, and
// internal/ui, internal/resultcache, and later export flows render or return
// these definitions rather than rebuilding the literals.
package result

import "strconv"

// ByteCapWarning is the designated persistent result warning shown once
// byte-cap eviction has occurred. It is metadata presented in the results
// header/status and export flow only; it never becomes a row or data value.
const ByteCapWarning = "Result truncated: 64 MiB cache limit"

// LimitKind distinguishes the two Issue #31 over-limit failure kinds. They
// are never conflated: KindPage is a fetched page whose retained rows
// collectively exceed the 64 MiB v1 cache envelope, and KindValue is one
// value exceeding the connection-local 64 MiB SQLite length limit.
type LimitKind int

const (
	// KindPage is the page-envelope failure at cache admission.
	KindPage LimitKind = iota + 1
	// KindValue is the connection-local oversized-value failure at the
	// SQLite scan boundary.
	KindValue
)

// String renders the limit-failure kind name for tests and diagnostics.
func (k LimitKind) String() string {
	if k == KindValue {
		return "value"
	}
	return "page"
}

// LimitFailure is one typed Issue #31 over-limit failure carrying its exact
// kind and the one-based absolute logical result position of the first
// failing row. It implements error, so the exact user-facing message is
// produced in exactly one place: Error.
type LimitFailure struct {
	Kind     LimitKind
	Position int64
}

// Error returns the exact shared message for this failure: `result page
// exceeds the 64 MiB v1 limit at row N` or `result value exceeds the 64 MiB
// v1 limit at row N`, with N the one-based logical result position.
func (f *LimitFailure) Error() string {
	return LimitFailureMessage(f.Kind, f.Position)
}

// LimitFailureMessage returns the exact shared over-limit message for the
// given kind and one-based logical result position. All layers render this
// definition; none rebuild the wording.
func LimitFailureMessage(kind LimitKind, position int64) string {
	if kind == KindValue {
		return "result value exceeds the 64 MiB v1 limit at row " + strconv.FormatInt(position, 10)
	}
	return "result page exceeds the 64 MiB v1 limit at row " + strconv.FormatInt(position, 10)
}
