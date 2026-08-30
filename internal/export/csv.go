// RFC 4180 CSV record serialization for Issue #50, per the Export formats
// and values and Export Module Design decisions in Notes/PRD-sqloid.md.
// The serializer consumes an already immutable export capture payload —
// shared full-set deduplicated output names and typed rows from
// internal/result — and emits UTF-8 CSV with exactly one header record,
// data records traversed by ascending one-based logical position, CRLF
// after every record, minimal quoting only for fields containing a comma,
// double quote, CR, or LF, and quote doubling inside quoted fields.
// Capture metadata and warnings live outside the payload type entirely,
// so no warning, completeness, or outcome fact can ever become a record,
// column, prefix, or comment. Repeated serialization of one payload is
// byte-identical, and the input is never mutated.

package export

import (
	"bytes"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/chris/sqloid/internal/result"
)

// CSV renders one immutable export payload as exact RFC 4180 UTF-8 bytes:
// one header record of the deduplicated output names, then one record per
// row in ascending logical-position order, each record CRLF-terminated.
// The payload must be immutable while CSV runs (a Capture.Payload already
// is); the payload's names, positions, rows, and BLOB byte slices are
// never mutated. Fields containing a comma, double quote, CR, or LF are
// quoted with embedded quotes doubled; every other field — including
// tab-bearing fields — is emitted verbatim and unquoted.
func CSV(p Payload) []byte {
	var b bytes.Buffer
	writeCSVRecord(&b, p.Names)
	for _, i := range ascendingPositions(p) {
		writeCSVRecord(&b, csvRowFields(p.Rows[i]))
	}
	return b.Bytes()
}

// ascendingPositions returns the row indexes of p ordered by ascending
// logical position without mutating p.Positions or p.Rows. Ties keep
// source order so serialization stays deterministic even for
// duplicate-valued positions.
func ascendingPositions(p Payload) []int {
	order := make([]int, len(p.Rows))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return p.Positions[order[a]] < p.Positions[order[b]]
	})
	return order
}

// csvRowFields converts one typed row into its CSV field strings under the
// shared typed-value policy: SQL NULL and empty TEXT both become the empty
// field (an accepted lossy limitation), INTEGER uses the shared
// result.IntegerToken,
// REAL uses the authoritative result.RealToken (finite shared tokens;
// non-finite values as Inf, -Inf, NaN), TEXT is preserved verbatim, and
// BLOB bytes become lowercase hexadecimal.
func csvRowFields(row []result.Value) []string {
	fields := make([]string, len(row))
	for i, v := range row {
		fields[i] = csvField(v)
	}
	return fields
}

// csvField renders one typed value's CSV field content.
func csvField(v result.Value) string {
	switch v.Kind {
	case result.KindNull:
		return ""
	case result.KindInteger:
		return result.IntegerToken(v.Int)
	case result.KindReal:
		return result.RealToken(v.Float)
	case result.KindText:
		return v.Str
	case result.KindBlob:
		return hex.EncodeToString(v.Bytes)
	default:
		return ""
	}
}

// writeCSVRecord writes one RFC 4180 record: each field minimally quoted
// (quote-doubling inside quoted fields) and the record terminated with
// CRLF.
func writeCSVRecord(b *bytes.Buffer, fields []string) {
	for i, f := range fields {
		if i > 0 {
			b.WriteByte(',')
		}
		writeCSVField(b, f)
	}
	b.WriteString("\r\n")
}

// writeCSVField writes one field: quoted with embedded quotes doubled
// exactly when the field contains a comma, double quote, CR, or LF;
// otherwise verbatim. Tabs alone never trigger quoting.
func writeCSVField(b *bytes.Buffer, f string) {
	if !strings.ContainsAny(f, ",\"\r\n") {
		b.WriteString(f)
		return
	}
	b.WriteByte('"')
	for i := 0; i < len(f); i++ {
		if f[i] == '"' {
			b.WriteByte('"')
		}
		b.WriteByte(f[i])
	}
	b.WriteByte('"')
}
