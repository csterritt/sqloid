// Immutable typed limit-failure metadata tests for Issue #76, per the Cache
// and snapshot invariant and History Module Design decisions in
// Notes/PRD-sqloid.md. The typed LimitFailure kind and one-based position
// are preserved as immutable snapshot metadata facts, independent of the
// terminal outcome and byte-cap eviction disclosure. A no-failure snapshot
// keeps the limit-failure metadata absent.

package history

import (
	"testing"

	"github.com/chris/sqloid/internal/result"
)

// TestSnapshotMetadataCarriesTypedLimitFailure proves the constructor
// accepts the typed limit-failure kind and one-based position and records
// them exactly as supplied, independent of the terminal outcome.
func TestSnapshotMetadataCarriesTypedLimitFailure(t *testing.T) {
	cases := []struct {
		name        string
		kind        result.LimitKind
		position    int64
		outcome     TerminalOutcome
		wantOutcome TerminalOutcome
	}{
		{
			name:        "page failure with success outcome",
			kind:        result.KindPage,
			position:    25,
			outcome:     OutcomeSuccess,
			wantOutcome: OutcomeSuccess,
		},
		{
			name:        "value failure with failed outcome",
			kind:        result.KindValue,
			position:    15,
			outcome:     OutcomeFailed,
			wantOutcome: OutcomeFailed,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			meta, err := NewSnapshotMetadata(CacheFacts{}, Lifecycle{
				Outcome:              tc.outcome,
				LimitFailureKind:     tc.kind,
				LimitFailurePosition: tc.position,
			})
			if err != nil {
				t.Fatalf("NewSnapshotMetadata: %v", err)
			}
			if meta.LimitFailureKind != tc.kind {
				t.Errorf("LimitFailureKind = %v, want %v", meta.LimitFailureKind, tc.kind)
			}
			if meta.LimitFailurePosition != tc.position {
				t.Errorf("LimitFailurePosition = %d, want %d", meta.LimitFailurePosition, tc.position)
			}
			if meta.Outcome != tc.wantOutcome {
				t.Errorf("Outcome = %v, want %v", meta.Outcome, tc.wantOutcome)
			}
		})
	}
}

// TestSnapshotMetadataRejectsNonOneBasedLimitFailurePosition proves the
// constructor rejects a limit-failure position that is not one-based.
func TestSnapshotMetadataRejectsNonOneBasedLimitFailurePosition(t *testing.T) {
	_, err := NewSnapshotMetadata(CacheFacts{}, Lifecycle{
		Outcome:              OutcomeSuccess,
		LimitFailureKind:     result.KindPage,
		LimitFailurePosition: 0,
	})
	if err == nil {
		t.Fatal("constructor accepted zero limit-failure position, want error")
	}
}

// TestSnapshotMetadataNoLimitFailureStaysUnset proves a lifecycle with no
// limit-failure kind produces metadata with the limit-failure fields absent.
func TestSnapshotMetadataNoLimitFailureStaysUnset(t *testing.T) {
	meta, err := NewSnapshotMetadata(CacheFacts{}, Lifecycle{
		Outcome: OutcomeSuccess,
	})
	if err != nil {
		t.Fatalf("NewSnapshotMetadata: %v", err)
	}
	if meta.LimitFailureKind != 0 {
		t.Errorf("LimitFailureKind = %v, want 0 (unset)", meta.LimitFailureKind)
	}
	if meta.LimitFailurePosition != 0 {
		t.Errorf("LimitFailurePosition = %d, want 0 (unset)", meta.LimitFailurePosition)
	}
}

// TestSnapshotMetadataLimitFailureImmutableByValue proves a copied metadata
// value is independent of later mutation of the original: value semantics
// ensure later changes cannot reach the already-constructed snapshot.
func TestSnapshotMetadataLimitFailureImmutableByValue(t *testing.T) {
	meta, err := NewSnapshotMetadata(CacheFacts{}, Lifecycle{
		Outcome:              OutcomeSuccess,
		LimitFailureKind:     result.KindPage,
		LimitFailurePosition: 25,
	})
	if err != nil {
		t.Fatalf("NewSnapshotMetadata: %v", err)
	}
	copy := meta
	copy.LimitFailureKind = result.KindValue
	copy.LimitFailurePosition = 999
	if meta.LimitFailureKind != result.KindPage {
		t.Errorf("original LimitFailureKind changed by copy mutation: got %v want %v",
			meta.LimitFailureKind, result.KindPage)
	}
	if meta.LimitFailurePosition != 25 {
		t.Errorf("original LimitFailurePosition changed by copy mutation: got %d want 25",
			meta.LimitFailurePosition)
	}
}
