package result

import "testing"

// TestCountStateHeaderPresentation covers the exact PRD count wording
// (Issues #24 and #57/#59): the pending state counts rows, success without a
// user Limit renders exactly `Result count: N`, success with a user Limit
// renders exactly `Result count: N (after Limit M)`, and failure renders
// exactly `Count unavailable`. The wording never implies a table size or a
// pre-Limit count.
func TestCountStateHeaderPresentation(t *testing.T) {
	tests := []struct {
		name  string
		state CountState
		want  string
	}{
		{
			name:  "pending counts rows",
			state: CountState{Status: CountPending},
			want:  "Counting rows…",
		},
		{
			name:  "success without limit",
			state: CountState{Status: CountSuccess, Total: 42},
			want:  "Result count: 42",
		},
		{
			name:  "success with limit",
			state: CountState{Status: CountSuccess, Total: 42, HasLimit: true, Limit: 10},
			want:  "Result count: 42 (after Limit 10)",
		},
		{
			name:  "success with limit and fewer counted rows",
			state: CountState{Status: CountSuccess, Total: 7, HasLimit: true, Limit: 100},
			want:  "Result count: 7 (after Limit 100)",
		},
		{
			name:  "count unavailable",
			state: CountState{Status: CountUnavailable},
			want:  "Count unavailable",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.state.Header(); got != tc.want {
				t.Errorf("CountState.Header() = %q, want exactly %q", got, tc.want)
			}
		})
	}
}

// TestCountStateZeroValueIsNotPresented covers that a zero-value CountState
// (no count request issued at all) reports no header text, so callers render
// it only from explicit state rather than inferring from row counts.
func TestCountStateZeroValueIsNotPresented(t *testing.T) {
	if got := (CountState{}).Header(); got != "" {
		t.Errorf("zero-value CountState.Header() = %q, want empty", got)
	}
}
