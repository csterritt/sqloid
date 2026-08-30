// Outcome-unknown non-tabular entry coverage for Issue #45, per the Writes
// and commit boundary decisions in Notes/PRD-sqloid.md. When a write's
// rollback or commit cannot be resolved, finalization appends exactly one
// immutable non-tabular outcome-unknown entry — kind, operation, table, the
// executed standalone SQL, the commit-versus-rollback phase, the driver
// error, and the optional statement RowsAffected with wording that
// explicitly says it does not prove persistence — and Newest selects it.
// AppendFinalized rejects duplicates for the same execution, and no entry
// ever claims the database was committed, rolled back, or untouched.

package history

import (
	"strings"
	"testing"
)

// unknownEntry builds the non-tabular outcome-unknown entry shape the UI's
// settlement finalization appends for one unresolved write.
func unknownEntry(execID uint64, operation, table string, phase UnknownPhase, cause string, rows int64, rowsKnown bool) ResultEntry {
	return ResultEntry{
		ExecutionID:  execID,
		Kind:         KindOutcomeUnknown,
		Operation:    operation,
		Table:        table,
		SQL:          `UPDATE "users" SET "email" = 'new'`,
		Phase:        phase,
		Summary:      WriteUnknownSummary(operation, phase, cause, rows, rowsKnown),
		RowsAffected: rows,
	}
}

func TestOutcomeUnknownEntryRetainsUnresolvedFacts(t *testing.T) {
	s := NewResultStore()

	retained, ok := s.AppendFinalized(unknownEntry(7, "UPDATE", "users", UnknownPhaseCommit, "disk I/O error", 3, true))
	if !ok {
		t.Fatal("outcome-unknown finalization was rejected")
	}
	if retained.Kind != KindOutcomeUnknown {
		t.Errorf("entry kind = %v, want outcome-unknown", retained.Kind)
	}
	if retained.ExecutionID != 7 {
		t.Errorf("entry execution = %d, want 7", retained.ExecutionID)
	}
	if retained.Operation != "UPDATE" || retained.Table != "users" {
		t.Errorf("entry operation/table = %q/%q, want UPDATE/users", retained.Operation, retained.Table)
	}
	if retained.Phase != UnknownPhaseCommit {
		t.Errorf("entry phase = %v, want commit", retained.Phase)
	}
	if retained.SQL != `UPDATE "users" SET "email" = 'new'` {
		t.Errorf("entry SQL = %q, want the executed standalone statement", retained.SQL)
	}
	if !strings.Contains(retained.Summary, "disk I/O error") {
		t.Errorf("summary %q lost the driver error", retained.Summary)
	}
	if !strings.Contains(retained.Summary, "does not prove persistence") {
		t.Errorf("summary %q lacks the non-proving rows-affected wording", retained.Summary)
	}
	for _, forbidden := range []string{"committed", "untouched", "rollback confirmed", "rows added"} {
		if strings.Contains(retained.Summary, forbidden) {
			t.Errorf("summary %q claims %q, which the unresolved resolution never proved", retained.Summary, forbidden)
		}
	}

	newest, ok := s.Newest()
	if !ok || newest.ID != retained.ID || newest.ExecutionID != 7 {
		t.Fatalf("Newest = %+v ok=%v, want the just-appended outcome-unknown entry", newest, ok)
	}

	// A duplicate or late settlement for the same execution appends nothing.
	if _, ok := s.AppendFinalized(unknownEntry(7, "UPDATE", "users", UnknownPhaseCommit, "disk I/O error", 3, true)); ok {
		t.Fatal("duplicate outcome-unknown finalization appended a second entry")
	}
	if s.Len() != 1 {
		t.Fatalf("retained entries = %d, want exactly one", s.Len())
	}
}

func TestOutcomeUnknownEntryRollbackPhase(t *testing.T) {
	s := NewResultStore()
	retained, ok := s.AppendFinalized(unknownEntry(9, "DELETE", "users", UnknownPhaseRollback, "constraint trigger failed", 0, false))
	if !ok {
		t.Fatal("outcome-unknown finalization was rejected")
	}
	if retained.Phase != UnknownPhaseRollback {
		t.Errorf("entry phase = %v, want rollback", retained.Phase)
	}
	if !strings.Contains(retained.Summary, "rollback did not resolve") {
		t.Errorf("summary %q lacks the unresolved-rollback phase wording", retained.Summary)
	}
}

func TestOutcomeUnknownSummaryPhasesAndRows(t *testing.T) {
	cases := []struct {
		name      string
		operation string
		phase     UnknownPhase
		cause     string
		rows      int64
		rowsKnown bool
		want      string
	}{
		{
			name:      "unresolved commit with rows does not prove persistence",
			operation: "UPDATE",
			phase:     UnknownPhaseCommit,
			cause:     "disk I/O error",
			rows:      3,
			rowsKnown: true,
			want:      "UPDATE outcome unknown: the commit did not resolve (disk I/O error); the statement reported 3 rows affected, which does not prove persistence",
		},
		{
			name:      "unresolved commit without a row count makes no persistence claim",
			operation: "DELETE",
			phase:     UnknownPhaseCommit,
			cause:     "commit failed",
			want:      "DELETE outcome unknown: the commit did not resolve (commit failed); the final database state is not proven",
		},
		{
			name:      "unresolved rollback with rows does not prove persistence",
			operation: "INSERT",
			phase:     UnknownPhaseRollback,
			rows:      1,
			rowsKnown: true,
			want:      "INSERT outcome unknown: the rollback did not resolve; the statement reported 1 rows affected, which does not prove persistence",
		},
		{
			name:      "unresolved rollback without a row count makes no persistence claim",
			operation: "INSERT",
			phase:     UnknownPhaseRollback,
			want:      "INSERT outcome unknown: the rollback did not resolve; the final database state is not proven",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := WriteUnknownSummary(tc.operation, tc.phase, tc.cause, tc.rows, tc.rowsKnown)
			if got != tc.want {
				t.Errorf("summary =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestUnknownPhaseString(t *testing.T) {
	cases := []struct {
		phase UnknownPhase
		want  string
	}{
		{UnknownPhaseCommit, "commit"},
		{UnknownPhaseRollback, "rollback"},
		{UnknownPhase(99), "UnknownPhase(99)"},
	}
	for _, tc := range cases {
		if got := tc.phase.String(); got != tc.want {
			t.Errorf("UnknownPhase(%d).String() = %q, want %q", int(tc.phase), got, tc.want)
		}
	}
}

func TestOutcomeUnknownKindString(t *testing.T) {
	if got := KindOutcomeUnknown.String(); got != "outcome-unknown" {
		t.Errorf("KindOutcomeUnknown.String() = %q, want %q", got, "outcome-unknown")
	}
}
