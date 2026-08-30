# Issue #31 — 64 MiB cache and oversized-value handling

*2026-08-28T17:53:52Z by Showboat 0.6.1*
<!-- showboat-id: 7e177f1b-cae8-47d6-9bc9-3f3e17b01971 -->

This walkthrough demonstrates the completed Issue #31 implementation: exact retained-payload accounting, the independent 64 MiB cache cap with opposite-end complete-row eviction, and the two distinct oversized-page/value failures with their exact messages. See Issue #31, the Cache and snapshot invariant of Notes/PRD-sqloid.md, and the wiki page Notes/wiki/byte-cap-oversized-values.md. Every block below is re-runnable from the repository root; each filters go-test output to its own deterministic lines.

```bash
cd "$(git rev-parse --show-toplevel)" && cat > internal/resultcache/zz_demo_test.go <<EOF
package resultcache

import (
	"bytes"
	"testing"

	"github.com/chris/sqloid/internal/result"
)

func TestDemoAccounting(t *testing.T) {
	blob := []byte{0x00, 0xFF, 0xDE, 0xAD, 0xBE, 0xEF}
	row := []result.Value{
		result.NewText("héllo 世界"),
		result.NewInteger(7),
		result.NewReal(1.5),
		result.NewNull(),
		result.NewBlob(blob),
	}
	for i, v := range row {
		t.Logf("value %d kind %-7s payload %d", i, v.Kind, ValuePayload(v))
	}
	t.Logf("RowPayload total = %d", RowPayload(row))
	c := New()
	c.Merge(Page{Start: 1, Rows: []Row{{Position: 1, Values: row}}}, Forward)
	t.Logf("cache PayloadBytes = %d", c.PayloadBytes())
	got := c.Rows()[0].Values[4]
	if got.Kind != result.KindBlob || !bytes.Equal(got.Bytes, blob) {
		t.Fatal("BLOB identity broken")
	}
	t.Logf("BLOB retained byte-for-byte with kind blob: %x", got.Bytes)
}
EOF
go test ./internal/resultcache -run TestDemoAccounting -count=1 -v 2>&1 | grep -E "value|payload|BLOB"; rm internal/resultcache/zz_demo_test.go
```

```output
    zz_demo_test.go:20: value 0 kind text    payload 13
    zz_demo_test.go:20: value 1 kind integer payload 8
    zz_demo_test.go:20: value 2 kind real    payload 8
    zz_demo_test.go:20: value 3 kind null    payload 0
    zz_demo_test.go:20: value 4 kind blob    payload 6
    zz_demo_test.go:30: BLOB retained byte-for-byte with kind blob: 00ffdeadbeef
```

The totals are exact: TEXT counts raw encoded bytes (13 for the multibyte string, never its display width), INTEGER and REAL cost exactly 8 bytes each, NULL costs 0, and the BLOB costs its exact byte length while surviving retention byte-for-byte with its distinct blob kind. Cache totals come from Cache.PayloadBytes, which excludes all model and metadata overhead per the Cache and snapshot invariant.

```bash
cd "$(git rev-parse --show-toplevel)" && cat > internal/resultcache/zz_demo_test.go <<EOF
package resultcache

import (
	"testing"

	"github.com/chris/sqloid/internal/result"
)

func TestDemoByteCap(t *testing.T) {
	half := int(MaxPayloadBytes / 2)
	c := New()
	c.Merge(Page{Start: 1, Rows: []Row{blobRow(1, half), blobRow(2, half)}}, Forward)
	c.Merge(Page{Start: 3, Rows: []Row{blobRow(3, half)}}, Forward)
	s, _ := c.Start()
	e, _ := c.End()
	t.Logf("forward crossing: retained %d..%d payload %d truncated=%v", s, e, c.PayloadBytes(), c.TruncatedByByteCap())
	c2 := New()
	c2.Merge(blobPage(2, 2, half), Forward)
	c2.Merge(Page{Start: 1, Rows: []Row{blobRow(1, half)}}, Backward)
	s2, _ := c2.Start()
	e2, _ := c2.End()
	t.Logf("backward crossing: retained %d..%d payload %d endKept=%d truncated=%v", s2, e2, c2.PayloadBytes(), e2, c2.TruncatedByByteCap())
	tiny := []Row{textRow(2, 4), textRow(3, 4)}
	c2.Merge(Page{Start: 2, Rows: tiny}, Backward)
	t.Logf("after falling below cap: payload %d truncated=%v", c2.PayloadBytes(), c2.TruncatedByByteCap())
	t.Logf("shared warning literal: %s", result.ByteCapWarning)
}
EOF
go test ./internal/resultcache -run TestDemoByteCap -count=1 -v 2>&1 | grep -E "crossing|falling|warning"; rm internal/resultcache/zz_demo_test.go
```

```output
    zz_demo_test.go:16: forward crossing: retained 2..3 payload 67108864 truncated=true
    zz_demo_test.go:22: backward crossing: retained 1..2 payload 67108864 endKept=2 truncated=true
    zz_demo_test.go:25: after falling below cap: payload 33554440 truncated=true
    zz_demo_test.go:26: shared warning literal: Result truncated: 64 MiB cache limit
```

Crossing the cap forward evicts complete rows from the low end (rows 2..3 kept, exactly 64 MiB retained); crossing backward evicts the high end (rows 1..2 kept). The disclosure is persistent: after later navigation replaces the big rows and the payload falls far below the cap, TruncatedByByteCap stays true, and the disclosure renders the single shared literal "Result truncated: 64 MiB cache limit" from internal/result — the UI never rebuilds it.

```bash
cd "$(git rev-parse --show-toplevel)" && cat > internal/resultcache/zz_demo_test.go <<EOF
package resultcache

import (
	"errors"
	"testing"

	"github.com/chris/sqloid/internal/result"
)

func TestDemoPageCap(t *testing.T) {
	// One page of MaxPositions+5 tiny rows crosses only the position cap;
	// byte-cap metadata must stay false.
	c := New()
	rows := make([]Row, MaxPositions+5)
	for i := range rows {
		rows[i] = textRow(Position(i+1), 4)
	}
	c.Merge(Page{Start: 1, Rows: rows}, Forward)
	s, _ := c.Start()
	e, _ := c.End()
	t.Logf("position cap only: retained %d..%d payload %d byteTruncated=%v", s, e, c.PayloadBytes(), c.TruncatedByByteCap())

	// A page-envelope failure on top of prior retention: five rows of a
	// third of the cap each fail at the third position (row 7); the two
	// complete rows nearest the cache are admitted contiguously.
	c2 := New()
	c2.Merge(Page{Start: 1, Rows: []Row{blobRow(1, 8), blobRow(2, 8)}}, Forward)
	page := blobPage(3, 5, int(MaxPayloadBytes/3)+1)
	accepted, err := c2.Merge(page, Forward)
	var f *result.LimitFailure
	errors.As(err, &f)
	s2, _ := c2.Start()
	e2, _ := c2.End()
	t.Logf("page failure: accepted=%v err=%q kind=%v position=%d", accepted, err, f.Kind, f.Position)
	t.Logf("retained %d..%d payload %d byteTruncated=%v", s2, e2, c2.PayloadBytes(), c2.TruncatedByByteCap())
}
EOF
go test ./internal/resultcache -run TestDemoPageCap -count=1 -v 2>&1 | grep -E "position cap|page failure|retained"; rm internal/resultcache/zz_demo_test.go
```

```output
    zz_demo_test.go:21: position cap only: retained 6..10005 payload 40000 byteTruncated=false
    zz_demo_test.go:34: page failure: accepted=true err="result page exceeds the 64 MiB v1 limit at row 5" kind=page position=5
    zz_demo_test.go:35: retained 1..4 payload 44739260 byteTruncated=false
```

Two distinct over-limit behaviors in one block: a page larger than MaxPositions crosses only the position cap (payload 40000 bytes, byteTruncated=false), while an oversized page envelope fails typed at the first nonfitting position — "result page exceeds the 64 MiB v1 limit at row 5" — admitting only the two complete leading rows contiguously with prior cache rows preserved, and no bytes from row 5.

```bash
cd "$(git rev-parse --show-toplevel)" && cat > internal/connection/zz_demo_test.go <<EOF
package connection

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/chris/sqloid/internal/result"
)

func TestDemoValueLimit(t *testing.T) {
	// Fixture written through a plain modernc connection (no limits) so the
	// oversized value can exist: row 1 is exactly 64 MiB (the boundary,
	// which must succeed), row 2 is 64 MiB + 1 byte.
	path := t.TempDir() + "/demo.db"
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE blobs (id INTEGER PRIMARY KEY, b BLOB)"); err != nil {
		t.Fatal(err)
	}
	stmt, _ := db.Prepare("INSERT INTO blobs (id, b) VALUES (?, ?)")
	for _, spec := range []struct {
		id   int64
		size int
	}{{1, int(sqlMaxLengthBytes)}, {2, int(sqlMaxLengthBytes) + 1}} {
		payload := make([]byte, spec.size)
		for i := range payload {
			payload[i] = byte(1)
		}
		if _, err := stmt.Exec(spec.id, payload); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	under, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer under.Close()

	page, res := under.RunFirstPage(context.Background(), "SELECT id, b FROM blobs ORDER BY id", nil)
	var f *result.LimitFailure
	if !errors.As(res.Err, &f) || f.Kind != result.KindValue {
		t.Fatalf("err = %v, want typed value failure", res.Err)
	}
	t.Logf("outcome=%v position=%d message=%q", res.Outcome, f.Position, res.Err)
	t.Logf("earlier complete rows retained: %d (row 1, %d exact bytes)", len(page.Rows), len(page.Rows[0][1].Bytes))
	t.Logf("failing row 2 contributed nothing, no partial row exposed")
}
EOF
go test ./internal/connection -run TestDemoValueLimit -count=1 -v 2>&1 | grep -E "outcome|earlier|failing"; rm internal/connection/zz_demo_test.go
```

```output
    zz_demo_test.go:50: outcome=failed position=2 message="result value exceeds the 64 MiB v1 limit at row 2"
    zz_demo_test.go:51: earlier complete rows retained: 1 (row 1, 67108864 exact bytes)
    zz_demo_test.go:52: failing row 2 contributed nothing, no partial row exposed
```

The connection-local value limit fires at the SQLite scan boundary with the exact shared message and the one-based absolute logical position. Row 1 (exactly 64 MiB, the boundary) is retained with all 67108864 bytes byte-for-byte; the oversized row contributes nothing — no partial row, no bytes. ExecutePage takes the page OFFSET so a non-first page reports absolute positions the same way. The full test suites for the three packages finish the demonstration:

```bash
cd "$(git rev-parse --show-toplevel)" && go test ./internal/resultcache ./internal/connection ./internal/ui > /tmp/sqloid-31-demo.txt 2>&1 && echo ALL-THREE-PACKAGES-PASS || cat /tmp/sqloid-31-demo.txt | tail -20
```

```output
ALL-THREE-PACKAGES-PASS
```

All three suites pass. Summary of the Issue #31 contracts demonstrated: exact per-type payload accounting with byte-for-byte BLOB identity; the independent 64 MiB cap evicting complete rows at the traversal-opposite end alongside the 10,000-position cap with contiguous retained ranges; persistent typed truncated-by-byte-cap disclosure via the shared literal "Result truncated: 64 MiB cache limit"; and the two distinct over-limit failures — page envelope at cache admission and connection-local value at the SQLite scan boundary — each with its exact message, one-based absolute logical position, complete leading rows, preserved prior retention, and the no-partial-row guarantee. References: Issue #31, Notes/PRD-sqloid.md (Cache and snapshot invariant, Errors and cancellation bounds, Testing Decisions), Notes/wiki/byte-cap-oversized-values.md.
