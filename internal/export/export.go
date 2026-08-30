// Package export is the exporter-facing boundary over Sqloid's shared
// UI-independent result representation (Issue #47): it hands future CSV/JSON
// writers (Issues #50/#51) the same full-set deduplicated output names and
// typed value tokens the frozen grid consumes, straight from
// internal/result's single definition sites. This package owns no
// representation logic of its own — no name suffixing, no numeric token
// formatting, no UTF-8 normalization — and no format serialization yet;
// format-specific quoting and non-finite policies belong to the later
// format packages.
package export

import (
	"github.com/chris/sqloid/internal/result"
)

// OutputNames returns the page's full-set deduplicated output names in
// column order, exactly as the frozen grid header renders them, via
// result.Page.HeaderNames. The page's original driver labels and every
// stored value are left untouched.
func OutputNames(p result.Page) []string { return p.HeaderNames() }

// CellToken returns the shared typed cell token for v, exactly as the grid
// renders it, via result.Value.Display. Exporters must not infer a value's
// type from this token: use the Value's Kind (INTEGER 1, REAL 1.0, and TEXT
// "1.0" tokenize differently or identically without ever sharing a kind).
func CellToken(v result.Value) string { return v.Display() }
