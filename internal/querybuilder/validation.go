// Runnable-state validation boundary (Issue #18): every downstream rule —
// grouping, ORDER BY, and LIMIT as this issue adds them — reports through one
// first-invalid contract so later runnable reporting needs no per-feature
// plumbing. Rule order is fixed: grouping, then ORDER BY, then LIMIT.

package querybuilder

// FieldIdentity constants name the builder domain an InvalidIssue belongs to.
// They are plain strings so the report carries directly into user-facing copy.
const (
	FieldIdentityColumns = "Column(s)"
	FieldIdentityTable   = "Table"
	FieldIdentityGroupBy = "GROUP BY"
	FieldIdentityOrderBy = "ORDER BY"
	FieldIdentityLimit   = "Limit"
)

// InvalidIssue describes the first blocking problem found in one builder
// snapshot: which field owns it and its exact user-facing reason wording.
type InvalidIssue struct {
	Field  string // one of the FieldIdentity constants
	Reason string // exact reason text asserted by tests and shown verbatim
}
