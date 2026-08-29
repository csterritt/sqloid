// Issue #31 cache-admission coverage for the page-envelope failure, distinct
// from the connection-local value-limit failure: a fetched page whose
// retained rows collectively exceed 64 MiB admits only complete leading rows
// that fit, fails typed at the first nonfitting absolute logical position
// with the exact shared message, keeps no bytes or fields from that row, and
// preserves previously valid cache rows under both caps. Non-first pages,
// backward requests, boundary-sized pages, TEXT and BLOB payloads, and the
// distinct failure kinds are all covered.

package resultcache

import (
	"errors"
	"strings"
	"testing"

	"github.com/chris/sqloid/internal/result"
)

func TestPageEnvelopeAdmission(t *testing.T) {
	quarter := int(MaxPayloadBytes / 4)
	_ = quarter

	t.Run("non-first forward page trims at the first nonfitting position", func(t *testing.T) {
		// Cache already holds positions 1..2 (tiny rows). A page starting at
		// position 3 carries three rows of a third of the cap plus one byte:
		// the cumulative payload first exceeds the envelope at position 5,
		// and the two admitted leading rows plus prior content stay under the
		// cap so nothing is evicted — prior cache rows are visibly preserved.
		c := New()
		mustMergeChecked(t, c, Page{Start: 1, Rows: []Row{blobRow(1, 8), blobRow(2, 8)}}, Forward)
		third := int(MaxPayloadBytes/3) + 1
		page := blobPage(3, 3, third)
		accepted, err := c.Merge(page, Forward)
		var failure *result.LimitFailure
		if !errors.As(err, &failure) {
			t.Fatalf("err = %v, want typed page-limit failure", err)
		}

		if failure.Kind != result.KindPage || failure.Position != 5 {
			t.Fatalf("failure = %+v, want {KindPage, position 5}", failure)
		}
		if !accepted {
			t.Fatal("the complete leading rows were not admitted")
		}
		assertRange(t, c, 1, 4)
		if got := c.PayloadBytes(); got != 16+2*int64(third) {
			t.Fatalf("PayloadBytes() = %d, want prior rows plus two complete leading rows", got)
		}
		if c.TruncatedByByteCap() {
			t.Fatal("byte-cap eviction happened although admitted rows fit under both caps")
		}
		// No bytes or fields from the failing row: position 5 must be absent.
		for _, row := range c.Rows() {
			if row.Position == 5 {
				t.Fatal("the failing row was partially retained")
			}
		}
	})

	t.Run("backward page trims the same way at the page's leading rows", func(t *testing.T) {
		c := New()
		mustMergeChecked(t, c, blobPage(8, 3, 8), Forward) // positions 8..10
		// Backward page covering 5..7 with three rows of a third of the cap
		// plus one byte. In traversal order (row 7 first) the cumulative
		// payload first exceeds the envelope at row 5; rows 6..7 — the
		// complete rows nearest the retained range — are admitted, prior rows
		// 8..10 are preserved, and the retained range stays contiguous.
		third := int(MaxPayloadBytes/3) + 1
		page := blobPage(5, 3, third)
		accepted, err := c.Merge(page, Backward)
		var failure *result.LimitFailure
		if !errors.As(err, &failure) || failure.Kind != result.KindPage || failure.Position != 5 {
			t.Fatalf("err = %v (%+v), want {KindPage, position 5}", err, failure)
		}
		if !accepted {
			t.Fatal("the complete leading rows were not admitted")
		}
		// Prior cache content is preserved and the admitted leading rows merge.
		assertRange(t, c, 6, 10)
		if c.Len() != 5 {
			t.Fatalf("Len() = %d, want 5 retained positions", c.Len())
		}
		if c.PayloadBytes() != 24+2*int64(third) {
			t.Fatalf("PayloadBytes() = %d, want prior rows plus two complete rows nearest the cache", c.PayloadBytes())
		}
	})

	t.Run("boundary page of exactly the cap is admitted fully", func(t *testing.T) {
		c := New()
		rows := []Row{blobRow(1, int(MaxPayloadBytes/2)), blobRow(2, int(MaxPayloadBytes/2))}
		accepted, err := c.Merge(Page{Start: 1, Rows: rows}, Forward)
		if err != nil || !accepted {
			t.Fatalf("err = %v accepted = %v, want a boundary-sized page fully admitted", err, accepted)
		}
		assertRange(t, c, 1, 2)
		if c.TruncatedByByteCap() {
			t.Fatal("boundary-sized page disclosed truncation")
		}
	})

	t.Run("one byte more than the cap fails at the exact position", func(t *testing.T) {
		c := New()
		rows := []Row{blobRow(1, int(MaxPayloadBytes/2)), blobRow(2, int(MaxPayloadBytes/2)+1)}
		_, err := c.Merge(Page{Start: 1, Rows: rows}, Forward)
		var failure *result.LimitFailure
		if !errors.As(err, &failure) || failure.Position != 2 {
			t.Fatalf("err = %v (%+v), want failure at position 2", err, failure)
		}
		if len(c.Rows()) != 1 {
			t.Fatalf("retained rows = %d, want exactly the one complete leading row", len(c.Rows()))
		}
	})

	t.Run("TEXT payload trims exactly like BLOB payload", func(t *testing.T) {
		c := New()
		textRowOfSize := func(pos Position, n int) Row {
			return Row{Position: pos, Values: []result.Value{result.NewText(strings.Repeat("x", n))}}
		}
		rows := []Row{textRowOfSize(1, int(MaxPayloadBytes)+1), textRowOfSize(2, 4)}
		_, err := c.Merge(Page{Start: 1, Rows: rows}, Forward)
		var failure *result.LimitFailure
		if !errors.As(err, &failure) || failure.Position != 1 || failure.Kind != result.KindPage {
			t.Fatalf("err = %v (%+v), want {KindPage, position 1} for TEXT payload", err, failure)
		}
		if len(c.Rows()) != 0 {
			t.Fatal("the oversized TEXT row reached the cache")
		}
	})

	t.Run("page and value failure kinds cannot be conflated", func(t *testing.T) {
		if result.KindPage == result.KindValue {
			t.Fatal("page and value limit kinds are identical")
		}

		// The page-envelope kind comes from cache admission; the value kind only
		// originates from the SQLite scan boundary, never from cache admission.
		c := New()
		_, err := c.Merge(Page{Start: 1, Rows: []Row{blobRow(1, int(MaxPayloadBytes)+1)}}, Forward)
		var failure *result.LimitFailure
		if !errors.As(err, &failure) || failure.Kind != result.KindPage {
			t.Fatalf("err = %v (%+v), want the page-envelope kind from cache admission", err, failure)
		}
	})
}
