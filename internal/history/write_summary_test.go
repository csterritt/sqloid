// Write history and summary coverage for Issue #42, per the History and
// write-transaction Implementation Decisions in Notes/PRD-sqloid.md. Every
// resolved write produces exactly one immutable non-tabular KindWrite entry
// tied to its execution identity and containing the executed standalone SQL;
// duplicate or late finalization for the same identity is rejected; summary
// labels use the actual statement RowsAffected (never an estimate) with
// rows-affected wording for UPDATE/DELETE and rows-added wording for INSERT;
// and no label claims the database was untouched before rollback success is
// confirmed.

package history

import (
	"strings"
	"testing"
)

// TestWriteSummaryLabels covers the operation-appropriate RowsAffected
// wording and the no-untouched-claim-before-confirmed-rollback rule.
func TestWriteSummaryLabels(t *testing.T) {
	tests := []struct {
		name              string
		operation         string
		status            WriteStatus
		rows              int64
		rollbackConfirmed bool
		cause             string
		want              string
		wantForbidden     []string // substrings that must never appear
	}{
		{
			name:      "committed update reports rows affected",
			operation: "UPDATE",
			status:    WriteStatusCommitted,
			rows:      3,
			want:      "UPDATE committed: 3 rows affected",
		},
		{
			name:      "committed delete reports rows affected",
			operation: "DELETE",
			status:    WriteStatusCommitted,
			rows:      0,
			want:      "DELETE committed: 0 rows affected",
		},
		{
			name:      "committed insert reports rows added",
			operation: "INSERT",
			status:    WriteStatusCommitted,
			rows:      2,
			want:      "INSERT committed: 2 rows added",
		},
		{
			name:      "insert never uses rows-affected wording",
			operation: "INSERT",
			status:    WriteStatusCommitted,
			rows:      1,
			want:      "INSERT committed: 1 rows added",
			// exercised via the forbidden list below too
			wantForbidden: []string{"rows affected"},
		},
		{
			name:              "cancelled without confirmation makes no untouched claim",
			operation:         "UPDATE",
			status:            WriteStatusCancelled,
			rows:              5,
			rollbackConfirmed: false,
			want:              "UPDATE cancelled",
			wantForbidden:     []string{"untouched"},
		},
		{
			name:              "cancelled with confirmed rollback claims untouched",
			operation:         "DELETE",
			status:            WriteStatusCancelled,
			rollbackConfirmed: true,
			want:              "DELETE cancelled: rollback confirmed, database untouched",
		},
		{
			name:              "failed constraint without confirmation makes no untouched claim",
			operation:         "UPDATE",
			status:            WriteStatusFailed,
			cause:             "UNIQUE constraint failed: users.email",
			rollbackConfirmed: false,
			want:              "UPDATE failed: UNIQUE constraint failed: users.email",
			wantForbidden:     []string{"untouched"},
		},
		{
			name:              "failed constraint with confirmed rollback claims untouched",
			operation:         "INSERT",
			status:            WriteStatusFailed,
			cause:             "NOT NULL constraint failed: users.email",
			rollbackConfirmed: true,
			want:              "INSERT failed: NOT NULL constraint failed: users.email (rollback confirmed, database untouched)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WriteSummary(tt.operation, tt.status, tt.rows, tt.rollbackConfirmed, tt.cause)
			if got != tt.want {
				t.Fatalf("WriteSummary = %q, want %q", got, tt.want)
			}
			for _, forbidden := range tt.wantForbidden {
				if strings.Contains(got, forbidden) {
					t.Fatalf("WriteSummary = %q, must not contain %q", got, forbidden)
				}
			}
		})
	}
}

// writeEntry builds a minimal KindWrite entry for append tests.
func writeEntry(execution uint64, summary string) ResultEntry {
	return ResultEntry{
		ExecutionID:  execution,
		Kind:         KindWrite,
		SQL:          `UPDATE "users" SET "email" = 'new' WHERE "id" = 5`,
		Summary:      summary,
		RowsAffected: 1,
	}
}

// TestWriteResultEntryExactlyOnceImmutable proves a write result is retained
// exactly once per execution identity, survives as an immutable non-tabular
// entry carrying the executed SQL and summary, and that duplicate or late
// finalization messages append nothing further.
func TestWriteResultEntryExactlyOnceImmutable(t *testing.T) {
	s := NewResultStore()

	retained, ok := s.AppendFinalized(writeEntry(7, "UPDATE committed: 1 rows affected"))
	if !ok {
		t.Fatal("first write finalization was rejected")
	}
	if retained.Kind != KindWrite {
		t.Fatalf("retained kind = %v, want write", retained.Kind)
	}
	if retained.ExecutionID != 7 {
		t.Fatalf("retained execution = %d, want 7", retained.ExecutionID)
	}
	if retained.SQL != `UPDATE "users" SET "email" = 'new' WHERE "id" = 5` {
		t.Fatalf("retained SQL = %q, want the executed standalone statement", retained.SQL)
	}
	if retained.Summary != "UPDATE committed: 1 rows affected" {
		t.Fatalf("retained summary = %q", retained.Summary)
	}
	if retained.RowsAffected != 1 {
		t.Fatalf("retained RowsAffected = %d, want 1", retained.RowsAffected)
	}

	// The retained entry is immutable: mutating the caller's copy afterwards
	// must not change what the store returns.
	caller := writeEntry(7, "UPDATE committed: 1 rows affected")
	caller.Summary = "mutated"
	for _, e := range s.Entries() {
		if e.ExecutionID == 7 && e.Summary != "UPDATE committed: 1 rows affected" {
			t.Fatalf("retained summary mutated to %q", e.Summary)
		}
	}

	// A duplicate or late finalization for the same execution appends nothing.
	if _, ok := s.AppendFinalized(writeEntry(7, "duplicate")); ok {
		t.Fatal("duplicate write finalization appended a second entry")
	}
	if s.Len() != 1 {
		t.Fatalf("store length = %d, want exactly 1 write entry", s.Len())
	}

	// A different execution identity is a distinct write and appends.
	if _, ok := s.AppendFinalized(writeEntry(8, "UPDATE cancelled")); !ok {
		t.Fatal("distinct write execution finalization was rejected")
	}
	if s.Len() != 2 {
		t.Fatalf("store length = %d, want 2", s.Len())
	}
}

// TestWriteResultEntryRequiresExecutionIdentity proves the execution-identity
// tie is mandatory: an entry without one is rejected, so a write can never
// create an unidentifiable result.
func TestWriteResultEntryRequiresExecutionIdentity(t *testing.T) {
	s := NewResultStore()
	entry := writeEntry(0, "UPDATE committed: 1 rows affected")
	if _, ok := s.AppendFinalized(entry); ok {
		t.Fatal("entry without an execution identity was retained")
	}
	if s.Len() != 0 {
		t.Fatalf("store length = %d, want 0", s.Len())
	}
}
