// Package result is Sqloid's shared, UI-independent representation of one
// SELECT result: typed SQLite values, original driver output labels, and the
// deterministic presentation policies from Notes/PRD-sqloid.md — full-set
// output-name deduplication, the exact finite REAL token, visible grid
// control-character symbols, and maximal invalid UTF-8 replacement with
// warning metadata. Both internal/ui's frozen-header grid and the future
// CSV/JSON export packages must consume this one seam rather than copy
// representation logic.
//
// The package is independent of Bubble Tea, database-driver concrete types,
// and exporter formats: FromDriver converts only the plain driver value set
// (nil, int64, float64, string, []byte) once at the boundary, and every other
// consumer works on the typed values here. Generated SQL and driver column
// metadata are never altered; deduplication applies only to display and
// export names.
package result

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Kind identifies the SQLite storage class of one result value. Kinds are
// never collapsed: INTEGER 1, REAL 1.0, and TEXT "1.0" carry distinct kinds
// for their whole lifetime.
type Kind int

const (
	// KindNull is SQL NULL. It is the zero kind so a zero Value is a typed
	// NULL rather than an invalid value.
	KindNull Kind = iota
	// KindInteger is a signed 64-bit integer.
	KindInteger
	// KindReal is a float64 REAL value, finite or not; non-finite rendering
	// policy is owned by Issue #23 and deliberately not this package.
	KindReal
	// KindText is decoded TEXT, verbatim except for maximal invalid UTF-8
	// sequences, which become exactly one U+FFFD each at conversion time.
	KindText
	// KindBlob is an exact BLOB payload, retained byte-for-byte.
	KindBlob
)

// String renders the kind name used in tests and diagnostics.
func (k Kind) String() string {
	switch k {
	case KindNull:
		return "null"
	case KindInteger:
		return "integer"
	case KindReal:
		return "real"
	case KindText:
		return "text"
	case KindBlob:
		return "blob"
	default:
		return fmt.Sprintf("Kind(%d)", int(k))
	}
}

// TabSymbol is the designated visible replacement for a tab in grid-facing
// TEXT rendering.
const TabSymbol = "⇥"

// NewlineSymbol is the designated visible replacement for a newline in
// grid-facing TEXT rendering.
const NewlineSymbol = "⏎"

// NullDisplay is the designated visible grid representation of SQL NULL.
const NullDisplay = "(NULL)"

// Value is one typed result cell. Exactly the field named by Kind is
// meaningful. Values are immutable after construction; NewBlob copies its
// payload so the retained value never aliases caller storage.
type Value struct {
	Kind  Kind
	Int   int64   // KindInteger
	Float float64 // KindReal
	Str   string  // KindText (decoded)
	Bytes []byte  // KindBlob (exact retained payload)
}

// NewNull returns a typed SQL NULL value.
func NewNull() Value { return Value{Kind: KindNull} }

// NewInteger returns a typed INTEGER value.
func NewInteger(v int64) Value { return Value{Kind: KindInteger, Int: v} }

// NewReal returns a typed REAL value.
func NewReal(v float64) Value { return Value{Kind: KindReal, Float: v} }

// NewText returns a typed TEXT value carrying s verbatim; callers convert
// from driver strings through FromDriver or DecodeText, not this constructor.
func NewText(s string) Value { return Value{Kind: KindText, Str: s} }

// NewBlob returns a typed BLOB value whose Bytes are a fresh copy of payload.
func NewBlob(payload []byte) Value {
	return Value{Kind: KindBlob, Bytes: append([]byte(nil), payload...)}
}

// Display returns the shared presentation token for this value as used by
// the frozen grid header's cells and by future exporters: INTEGER uses
// strconv formatting, finite REAL uses RealToken, TEXT uses the decoded
// string transformed through the visible control-character symbols, BLOB
// renders exactly `[BLOB n bytes]`, and NULL renders NullDisplay. The render
// seam never coerces a value into another type: numeric-looking TEXT stays
// text and non-finite REALs keep the REAL kind.
func (v Value) Display() string {
	switch v.Kind {
	case KindNull:
		return NullDisplay
	case KindInteger:
		return strconv.FormatInt(v.Int, 10)
	case KindReal:
		return RealToken(v.Float)
	case KindText:
		return GridText(v.Str)
	case KindBlob:
		return fmt.Sprintf("[BLOB %d bytes]", len(v.Bytes))
	default:
		return fmt.Sprintf("(invalid kind %d)", int(v.Kind))
	}
}

// RealToken returns the exact PRD finite-REAL token: the shortest
// round-tripping 'g'-format float64 representation, with ".0" appended
// exactly when the token contains none of '.', 'e', or 'E' so REAL identity
// survives for values such as 1.0, -0.0, and 1e+20. The token is
// locale-independent by construction of strconv.
func RealToken(v float64) string {
	token := strconv.FormatFloat(v, 'g', -1, 64)
	if !strings.ContainsAny(token, ".eE") {
		token += ".0"
	}
	return token
}

// GridText returns the grid-facing rendering of already-decoded TEXT: tabs
// and newlines become the designated visible symbols. It performs no UTF-8
// replacement — FromDriver owns decoding — and never changes any other
// byte or rune.
func GridText(s string) string {
	if !strings.ContainsAny(s, "\t\n") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\t':
			b.WriteString(TabSymbol)
		case '\n':
			b.WriteString(NewlineSymbol)
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// DecodeText decodes raw TEXT bytes into their stored representation: each
// maximal invalid UTF-8 byte sequence (per Unicode's maximal-subpart rule)
// becomes exactly one U+FFFD, and the boolean reports whether any
// replacement occurred so callers can set warning metadata without touching
// row order or count. Valid UTF-8 returns the input unchanged with false.
func DecodeText(s string) (string, bool) {
	if utf8.ValidString(s) {
		return s, false
	}
	var b strings.Builder
	b.Grow(len(s))
	replaced := false
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r != utf8.RuneError {
			b.WriteString(s[i : i+size])
			i += size
			continue
		}
		n := maximalSubpart(s[i:])
		b.WriteRune(utf8.RuneError)
		i += n
		replaced = true
	}
	return b.String(), replaced
}

// maximalSubpart returns the length of the maximal subpart of an ill-formed
// UTF-8 sequence starting at s[0], per Unicode Table 3-7. The sequence
// beginning at s[0] is already known to be invalid.
func maximalSubpart(s string) int {
	c := s[0]
	var lo, hi byte
	size := 2
	switch {
	case c >= 0xC2 && c <= 0xDF:
		lo, hi = 0x80, 0xBF
	case c == 0xE0:
		lo, hi = 0xA0, 0xBF
	case c >= 0xE1 && c <= 0xEC:
		lo, hi = 0x80, 0xBF
	case c == 0xED:
		lo, hi = 0x80, 0x9F
	case c >= 0xEE && c <= 0xEF:
		lo, hi = 0x80, 0xBF
	case c == 0xF0:
		lo, hi = 0x90, 0xBF
		size = 3
	case c >= 0xF1 && c <= 0xF3:
		lo, hi = 0x80, 0xBF
		size = 3
	case c == 0xF4:
		lo, hi = 0x80, 0x8F
		size = 3
	default:
		// Continuation bytes, C0/C1, and F5–FF: a one-byte ill-formed
		// subpart.
		return 1
	}
	if len(s) < size || s[1] < lo || s[1] > hi {
		return 1
	}
	if size == 2 {
		return 2
	}
	if len(s) < 3 || s[2] < 0x80 || s[2] > 0xBF {
		return 2
	}
	return 3
}

// DeduplicateNames applies the PRD full-set output-name rule to the original
// output-name set: walking left to right, the first occurrence of a name
// keeps it unchanged, and each later duplicate receives the lowest `_2`,
// `_3`, … suffix that collides with neither any already-final name nor any
// original name. Generated SQL and driver metadata are never altered; the
// result is a fresh slice. Empty labels and pre-suffixed labels follow the
// same rule, so collision chains resolve deterministically.
func DeduplicateNames(names []string) []string {
	final := make([]string, len(names))
	taken := make(map[string]bool, len(names))
	for _, n := range names {
		taken[n] = true
	}
	seen := make(map[string]bool, len(names))
	for i, n := range names {
		if !seen[n] {
			seen[n] = true
			final[i] = n
			continue
		}
		for k := 2; ; k++ {
			candidate := n + "_" + strconv.Itoa(k)
			if !taken[candidate] {
				final[i] = candidate
				taken[candidate] = true
				break
			}
		}
	}
	return final
}

// Page is one eagerly fetched SELECT result page in the shared
// representation: original driver labels in order, typed rows in result
// order, and the invalid-UTF warning metadata. Rows and their values are
// owned by the Page; callers must not mutate them. Raw typed values are
// preserved separately from rendering — Display happens only on demand.
type Page struct {
	// Columns holds the original driver output labels exactly as returned;
	// deduplicated display/export names come from HeaderNames.
	Columns []string
	// Rows holds every row's typed values in column order.
	Rows [][]Value
	// InvalidUTF reports that at least one TEXT value required maximal
	// invalid-UTF-8 replacement. It is metadata only: row and column counts
	// are unchanged and BLOB bytes are never affected.
	InvalidUTF bool
}

// HeaderNames returns the full-set deduplicated output names for this page
// via DeduplicateNames, as rendered by the frozen grid header and reused by
// future CSV headers and JSON keys.
func (p Page) HeaderNames() []string { return DeduplicateNames(p.Columns) }

// FromDriver converts one driver-scanned result into a Page exactly once at
// the Connection boundary: nil becomes typed NULL, int64 INTEGER, float64
// REAL, string TEXT (decoded through DecodeText, so maximal invalid UTF-8
// sequences become one U+FFFD each and set Page.InvalidUTF), and []byte BLOB
// (copied byte-for-byte, never decoded or transformed). It panics on any
// other driver value because that would silently coerce types.
func FromDriver(columns []string, rows [][]any) Page {
	page := Page{Columns: append([]string(nil), columns...), Rows: make([][]Value, len(rows))}
	for i, row := range rows {
		values := make([]Value, len(row))
		for j, raw := range row {
			switch v := raw.(type) {
			case nil:
				values[j] = NewNull()
			case int64:
				values[j] = NewInteger(v)
			case float64:
				values[j] = NewReal(v)
			case string:
				decoded, replaced := DecodeText(v)
				values[j] = NewText(decoded)
				if replaced {
					page.InvalidUTF = true
				}
			case []byte:
				values[j] = NewBlob(v)
			default:
				panic(fmt.Sprintf("result: unsupported driver value type %T", raw))
			}
		}
		page.Rows[i] = values
	}
	return page
}

// UTFWarning is the designated persistent result warning for TEXT values
// whose maximal invalid UTF-8 byte sequences were replaced by U+FFFD. It is
// metadata presented in the results status/header only; it never becomes a
// row, column, or data value.
const UTFWarning = "invalid UTF-8 replaced with U+FFFD"
