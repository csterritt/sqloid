// Deterministic array-of-objects JSON serialization for Issue #51, per the
// Export formats and values, Output names, Invalid UTF-8 TEXT, Export
// Module Design, and Testing Decisions decisions in Notes/PRD-sqloid.md.
// The serializer consumes an already immutable export capture payload —
// shared full-set deduplicated output names and typed rows from
// internal/result — and emits exactly one compact top-level JSON array with
// one object per retained row in ascending one-based logical-position
// order. Each object is written directly in column order rather than
// through an unordered map, so every object carries the shared deduplicated
// keys in identical left-to-right order and repeated serialization of one
// payload is byte-identical. Typed values follow the shared policy: INTEGER
// and finite REALs emit the shared raw result tokens, non-finite REALs emit
// the exact quoted strings "Inf", "-Inf", and "NaN", SQL NULL emits the
// JSON literal null, TEXT emits a standards-compliant escaped string
// (invalid UTF-8 was already normalized to one U+FFFD per maximal invalid
// sequence by internal/result), and BLOB bytes emit a standard base64
// string with the source bytes untouched. Capture metadata and warnings
// live outside the payload type entirely, so no warning, completeness, or
// outcome fact can ever become an object, property, key, or wrapper. The
// input is never mutated and no trailing whitespace or newline is emitted.

package export

import (
	"bytes"
	"encoding/base64"
	"math"

	"github.com/chris/sqloid/internal/result"
)

// JSON renders one immutable export payload as exact deterministic UTF-8
// JSON bytes: one top-level array containing one object per row in
// ascending logical-position order, each object emitting the payload's
// deduplicated names as keys in identical column order with typed values.
// The payload must be immutable while JSON runs (a Capture.Payload already
// is); the payload's names, positions, rows, and BLOB byte slices are
// never mutated.
func JSON(p Payload) []byte {
	var b bytes.Buffer
	b.WriteByte('[')
	for n, i := range ascendingPositions(p) {
		if n > 0 {
			b.WriteByte(',')
		}
		writeJSONObject(&b, p.Names, p.Rows[i])
	}
	b.WriteByte(']')
	return b.Bytes()
}

// writeJSONObject writes one row object: the deduplicated name for each
// column as an escaped JSON string key, a colon, and the typed JSON value,
// all in column order and separated by commas with no surrounding spaces.
// The row itself is only read, never reordered or modified.
func writeJSONObject(b *bytes.Buffer, names []string, row []result.Value) {
	b.WriteByte('{')
	for i, name := range names {
		if i > 0 {
			b.WriteByte(',')
		}
		writeJSONString(b, name)
		b.WriteByte(':')
		writeJSONValue(b, row[i])
	}
	b.WriteByte('}')
}

// writeJSONValue writes one typed value's JSON representation: JSON null
// for SQL NULL, the shared raw INTEGER token, the shared raw finite REAL
// token, the exact quoted non-finite policy strings, an escaped JSON
// string for TEXT, and a standard base64 JSON string for exact BLOB bytes.
func writeJSONValue(b *bytes.Buffer, v result.Value) {
	switch v.Kind {
	case result.KindInteger:
		b.WriteString(result.IntegerToken(v.Int))
	case result.KindReal:
		if math.IsInf(v.Float, 1) || math.IsInf(v.Float, -1) || math.IsNaN(v.Float) {
			// Pre-existing non-finite REALs cannot be JSON numbers; the
			// exact PRD policy tokens are emitted as quoted strings.
			writeJSONString(b, result.RealToken(v.Float))
			return
		}
		b.WriteString(result.RealToken(v.Float))
	case result.KindText:
		writeJSONString(b, v.Str)
	case result.KindBlob:
		writeJSONString(b, base64.StdEncoding.EncodeToString(v.Bytes))
	default: // result.KindNull
		b.WriteString("null")
	}
}

// writeJSONString writes one standards-compliant JSON string: surrounded by
// double quotes, with the double quote and reverse solidus escaped by a
// reverse solidus, and the control characters U+0000 through U+001F escaped
// through the short forms \b, \f, \n, \r, and \t or the \u00XX form. Every
// other byte is emitted verbatim: TEXT is already valid UTF-8 (invalid
// sequences became one U+FFFD each in internal/result), and keys are
// driver labels or deduplicated suffixes.
func writeJSONString(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			b.WriteString(`\"`)
		case c == '\\':
			b.WriteString(`\\`)
		case c == '\b':
			b.WriteString(`\b`)
		case c == '\f':
			b.WriteString(`\f`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c < 0x20:
			const hexDigits = "0123456789abcdef"
			b.WriteString(`\u00`)
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&0xF])
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
}
