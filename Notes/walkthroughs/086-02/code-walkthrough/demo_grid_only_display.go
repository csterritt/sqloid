package main

// Issue #86 walkthrough demonstration: trace one typed matrix containing
// TEXT with tabs/newlines, NULL, BLOB, finite REAL, and non-finite REAL
// through grid display (Value.Display) and CSV/JSON serialization
// (export.CSV/export.JSON), proving the grid alone uses visible control
// symbols, (NULL), and [BLOB n bytes], while exporters inspect Kind and
// typed payload fields for their format-specific bytes. Reference: Issue
// #86 and Notes/PRD-sqloid.md (Grid rendering/cache, Export formats and
// values).

import (
	"bytes"
	"fmt"
	"math"

	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/result"
)

func main() {
	// One typed matrix exercising every kind-relevant policy: TEXT with an
	// embedded tab and newline, SQL NULL, a BLOB with non-UTF-8 bytes, a
	// finite REAL, and the three non-finite REAL tokens.
	matrix := [][]result.Value{{
		result.NewText("line1\tcol\t2\nline2"),
		result.NewNull(),
		result.NewBlob([]byte{0x00, 0xFF, 0xE0, 0xA0, 0x6F, 0x6B}),
		result.NewReal(1.0),
		result.NewReal(math.Inf(1)),
		result.NewReal(math.Inf(-1)),
		result.NewReal(math.NaN()),
	}}
	names := []string{"text", "null", "blob", "finite_real", "pos_inf", "neg_inf", "nan"}
	positions := []int64{1, 2, 3, 4, 5, 6, 7}

	fmt.Println("=== Grid display: Value.Display (grid presentation policy) ===")
	fmt.Printf("headers: %v\n", names)
	for i, row := range matrix {
		cells := make([]string, len(row))
		for j, v := range row {
			cells[j] = v.Display()
		}
		fmt.Printf("row %d: %v\n", i+1, cells)
	}

	fmt.Println()
	fmt.Println("=== Exporters inspect Kind and typed payload fields directly ===")
	payload := export.Payload{Names: names, Positions: positions, Rows: matrix}

	fmt.Println("--- CSV (export.CSV) ---")
	csvOut := export.CSV(payload)
	fmt.Print(string(csvOut))

	fmt.Println("--- JSON (export.JSON) ---")
	jsonOut := export.JSON(payload)
	// Pretty-print the single-row JSON object for readability.
	fmt.Println(formatJSONIndent(jsonOut))

	fmt.Println("=== Kind/payload inspection summary (what exporters see) ===")
	for j, v := range matrix[0] {
		switch v.Kind {
		case result.KindNull:
			fmt.Printf("col %d (%s): Kind=Null -> CSV empty field, JSON null\n", j+1, names[j])
		case result.KindInteger:
			fmt.Printf("col %d (%s): Kind=Integer Int=%d\n", j+1, names[j], v.Int)
		case result.KindReal:
			fmt.Printf("col %d (%s): Kind=Real Float=%v token=%q (CSV textual, JSON raw or quoted if non-finite)\n", j+1, names[j], v.Float, result.RealToken(v.Float))
		case result.KindText:
			fmt.Printf("col %d (%s): Kind=Text Str=%q (CSV/JSON receive raw bytes, no visible symbols)\n", j+1, names[j], v.Str)
		case result.KindBlob:
			fmt.Printf("col %d (%s): Kind=Blob Bytes=%d (CSV lowercase hex, JSON base64)\n", j+1, names[j], len(v.Bytes))
		}
	}
}

// formatJSONIndent re-indents a compact single-line JSON array-of-objects for
// readability. It is a walkthrough display helper, not production code.
func formatJSONIndent(b []byte) string {
	var out bytes.Buffer
	indent := 0
	for _, c := range b {
		switch c {
		case '{', '[':
			out.WriteByte(c)
			indent++
			out.WriteByte('\n')
			writeIndent(&out, indent)
		case '}', ']':
			indent--
			out.WriteByte('\n')
			writeIndent(&out, indent)
			out.WriteByte(c)
		case ',':
			out.WriteByte(c)
			out.WriteByte('\n')
			writeIndent(&out, indent)
		case ':':
			out.WriteByte(':')
			out.WriteByte(' ')
		default:
			out.WriteByte(c)
		}
	}
	return out.String()
}

func writeIndent(b *bytes.Buffer, n int) {
	for i := 0; i < n; i++ {
		b.WriteString("  ")
	}
}
