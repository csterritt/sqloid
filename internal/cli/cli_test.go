package cli

import (
	"io"
	"os"
	"testing"
)

// recorder captures which handlers Main dispatched and with what arguments.
type recorder struct {
	sqlitePath string
	d1Called   bool
}

func (r *recorder) handlers() Handlers {
	return Handlers{
		SQLite: func(path string) error { r.sqlitePath = path; return nil },
		D1:     func() error { r.d1Called = true; return nil },
	}
}

func TestMainRouting(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStatus int
		wantSQLite string
		wantD1     bool
	}{
		{
			name:       "sqlite routes the file argument",
			args:       []string{"sqloid", "sqlite", "/tmp/example.db"},
			wantStatus: 0,
			wantSQLite: "/tmp/example.db",
		},
		{
			name:       "d1 routes with no arguments",
			args:       []string{"sqloid", "d1"},
			wantStatus: 0,
			wantD1:     true,
		},
		{
			name:       "missing sqlite argument is a usage failure",
			args:       []string{"sqloid", "sqlite"},
			wantStatus: 2,
		},
		{
			name:       "unexpected sqlite argument is a usage failure",
			args:       []string{"sqloid", "sqlite", "one.db", "two.db"},
			wantStatus: 2,
		},
		{
			name:       "unexpected d1 argument is a usage failure",
			args:       []string{"sqloid", "d1", "extra"},
			wantStatus: 2,
		},
		{
			name:       "unknown command is a usage failure",
			args:       []string{"sqloid", "bogus"},
			wantStatus: 2,
		},
		{
			name:       "help flag succeeds",
			args:       []string{"sqloid", "--help"},
			wantStatus: 0,
		},
		{
			name:       "short help flag succeeds",
			args:       []string{"sqloid", "-h"},
			wantStatus: 0,
		},
		{
			name:       "version flag succeeds",
			args:       []string{"sqloid", "--version"},
			wantStatus: 0,
		},
		{
			name:       "short version flag succeeds",
			args:       []string{"sqloid", "-v"},
			wantStatus: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recorder{}
			gotStatus := Main(tt.args, rec.handlers())

			if gotStatus != tt.wantStatus {
				t.Errorf("Main(%q) status = %d, want %d", tt.args, gotStatus, tt.wantStatus)
			}
			if rec.sqlitePath != tt.wantSQLite {
				t.Errorf("Main(%q) sqlite path = %q, want %q", tt.args, rec.sqlitePath, tt.wantSQLite)
			}
			if rec.d1Called != tt.wantD1 {
				t.Errorf("Main(%q) d1 called = %t, want %t", tt.args, rec.d1Called, tt.wantD1)
			}
		})
	}
}

// captureStdout redirects os.Stdout for the duration of f and returns what
// was written, so version output can be asserted exactly.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()

	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = w

	done := make(chan string)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()

	f()

	os.Stdout = original
	if err := w.Close(); err != nil {
		t.Fatalf("closing pipe writer: %v", err)
	}
	return <-done
}

func TestVersionOutput(t *testing.T) {
	for _, flag := range []string{"--version", "-v"} {
		t.Run(flag, func(t *testing.T) {
			rec := &recorder{}
			var status int
			got := captureStdout(t, func() {
				status = Main([]string{"sqloid", flag}, rec.handlers())
			})

			want := "sqloid " + Version + "\n"
			if got != want {
				t.Errorf("version output = %q, want %q", got, want)
			}
			if status != 0 {
				t.Errorf("version status = %d, want 0", status)
			}
			if rec.d1Called || rec.sqlitePath != "" {
				t.Errorf("version request dispatched a command: sqlite=%q d1=%t", rec.sqlitePath, rec.d1Called)
			}
		})
	}
}

func TestVersionOutputIsSilentOnSuccess(t *testing.T) {
	// A successful dispatch must add no CLI-authored output beyond the
	// version contract itself.
	rec := &recorder{}
	got := captureStdout(t, func() {
		Main([]string{"sqloid", "d1"}, rec.handlers())
	})
	if got != "" {
		t.Errorf("successful d1 dispatch wrote %q to stdout, want no output", got)
	}
}
