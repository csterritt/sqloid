// Accepted-quit write finalization coverage for Issue #43, per the History
// and write-cancellation Testing Decisions in Notes/PRD-sqloid.md. Every
// accepted quit that resolves a write finalizes exactly one immutable
// KindWrite result entry: AppendFinalized rejects a second entry for the
// same execution, Newest selects the just-finalized entry, and each definite
// or unknown outcome carries the operation-appropriate label without ever
// claiming the database was untouched before confirmed rollback — or claiming
// persistence for any non-committed outcome.

package history

import (
	"strings"
	"testing"
)

// writeQuitEntry builds the non-tabular write entry shape the UI's accepted
// quit finalization appends for one resolved write.
func writeQuitEntry(execID uint64, status WriteStatus, rows int64, rollbackConfirmed bool, cause string) ResultEntry {
	return ResultEntry{
		ExecutionID:  execID,
		Kind:         KindWrite,
		SQL:          `UPDATE "users" SET "email" = 'new'`,
		Summary:      WriteSummary("UPDATE", status, rows, rollbackConfirmed, cause),
		RowsAffected: rows,
	}
}

func TestResultStoreQuitFinalizesWriteEntryOnce(t *testing.T) {
	s := NewResultStore()

	retained, ok := s.AppendFinalized(writeQuitEntry(7, WriteStatusCancelled, 0, true, ""))
	if !ok {
		t.Fatal("first quit finalization was rejected")
	}
	if retained.Kind != KindWrite || retained.ExecutionID != 7 {
		t.Fatalf("retained entry kind=%v execution=%d, want write/7", retained.Kind, retained.ExecutionID)
	}
	if got := WriteSummary("UPDATE", WriteStatusCancelled, 0, true, ""); !strings.Contains(got, "untouched") {
		t.Fatalf("confirmed-rollback summary %q made no untouched claim", got)
	}

	// A replayed duplicate finalization for the same execution is a no-op:
	// the original entry stays newest and unchanged.
	replay, ok := s.AppendFinalized(writeQuitEntry(7, WriteStatusCancelled, 0, true, ""))
	if ok {
		t.Fatal("duplicate quit finalization appended a second entry")
	}
	if replay.ID != 0 {
		t.Fatalf("rejected duplicate returned a retained entry %v", replay)
	}
	newest, ok := s.Newest()
	if !ok || newest.ID != retained.ID || newest.ExecutionID != 7 {
		t.Fatalf("Newest after replay = %+v ok=%v, want the original entry", newest, ok)
	}
}

func TestResultStoreQuitEntriesNeverOverclaimOutcomes(t *testing.T) {
	cases := []struct {
		name       string
		entry      ResultEntry
		wantPhrase string
		forb       []string
	}{
		{"committed", writeQuitEntry(1, WriteStatusCommitted, 3, false, ""), "rows affected", []string{"untouched"}},
		{"unconfirmed rollback", writeQuitEntry(2, WriteStatusFailed, 0, false, "commit failed"), "commit failed", []string{"untouched", "committed"}},
		{"unconfirmed cancel", writeQuitEntry(3, WriteStatusCancelled, 0, false, ""), "cancelled", []string{"untouched"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tc.entry.Summary, tc.wantPhrase) {
				t.Fatalf("summary %q lacks %q", tc.entry.Summary, tc.wantPhrase)
			}
			for _, f := range tc.forb {
				if strings.Contains(tc.entry.Summary, f) {
					t.Fatalf("summary %q overclaims %q the resolution never proved", tc.entry.Summary, f)
				}
			}
		})
	}
}
