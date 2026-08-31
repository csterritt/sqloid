package cli

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/connection"
	"github.com/chris/sqloid/internal/session"

	_ "modernc.org/sqlite"
)

// createFixture builds a real SQLite database so success-path tests exercise
// the full validation, opening, and probing pipeline.
func createFixture(t *testing.T, path string) {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open(%q) error = %v", path, err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t (id INTEGER)"); err != nil {
		t.Fatalf("creating fixture: %v", err)
	}
}

// runStartup runs `sqloid sqlite <path>` through Main with the production
// session handler (injected with a no-op program runner so no TTY is
// required) while capturing both streams.
func runStartup(t *testing.T, path string) (stdout, stderr string, status int) {
	t.Helper()

	savedOut, savedErr := os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = outW, errW

	status = Main([]string{"sqloid", "sqlite", path}, Handlers{SQLite: func(path string) error {
		return session.RunSQLiteWith(path, noopStartupRunner, nil)
	}})

	os.Stdout, os.Stderr = savedOut, savedErr
	outW.Close()
	errW.Close()

	var outBuf, errBuf bytes.Buffer
	if _, err := outBuf.ReadFrom(outR); err != nil {
		t.Fatal(err)
	}
	if _, err := errBuf.ReadFrom(errR); err != nil {
		t.Fatal(err)
	}
	return outBuf.String(), errBuf.String(), status
}

// noopStartupRunner is the injected program runner for startup tests: it
// returns immediately without touching the TTY, proving the open → compose →
// close lifecycle succeeds silently.
func noopStartupRunner(_ tea.Model) (tea.Model, error) { return nil, nil }

func lineCount(s string) int { return len(strings.Split(strings.TrimSuffix(s, "\n"), "\n")) }

// TestStartupFailuresRenderOneLineOnStderr pins the Issue #2 CLI contract:
// every file-validation/open startup failure prints exactly one stderr line —
// the documented diagnostic — writes nothing to stdout, and exits 1.
func TestStartupFailuresRenderOneLineOnStderr(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; unreadable/non-writable fixtures cannot be exercised")
	}

	tests := []struct {
		name  string
		setup func(t *testing.T) (path string, wantLine string)
	}{
		{
			name: "missing file",
			setup: func(t *testing.T) (string, string) {
				path := filepath.Join(t.TempDir(), "absent.db")
				return path, path + ": no such file or directory"
			},
		},
		{
			name: "unreadable file",
			setup: func(t *testing.T) (string, string) {
				path := filepath.Join(t.TempDir(), "unreadable.db")
				if err := os.WriteFile(path, []byte("junk"), 0o000); err != nil {
					t.Fatal(err)
				}
				return path, path + ": permission denied"
			},
		},
		{
			name: "directory",
			setup: func(t *testing.T) (string, string) {
				dir := t.TempDir()
				return dir, dir + ": not a SQLite database"
			},
		},
		{
			name: "invalid header",
			setup: func(t *testing.T) (string, string) {
				path := filepath.Join(t.TempDir(), "text.db")
				if err := os.WriteFile(path, []byte("not a database"), 0o644); err != nil {
					t.Fatal(err)
				}
				return path, path + ": not a SQLite database"
			},
		},
		{
			name: "non-writable database",
			setup: func(t *testing.T) (string, string) {
				path := filepath.Join(t.TempDir(), "locked.db")
				createFixture(t, path)
				if err := os.Chmod(path, 0o444); err != nil {
					t.Fatal(err)
				}
				return path, "cannot open database read-write: " + path + ": permission denied"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, wantLine := tt.setup(t)

			stdout, stderr, status := runStartup(t, path)

			if status != 1 {
				t.Errorf("startup failure status = %d, want 1", status)
			}
			if stdout != "" {
				t.Errorf("startup failure wrote %q to stdout, want silence", stdout)
			}
			if stderr == "" || lineCount(stderr) != 1 || strings.TrimSuffix(stderr, "\n") != wantLine {
				t.Errorf("stderr = %q (%d lines), want exactly one line %q", stderr, lineCount(stderr), wantLine)
			}
		})
	}
}

// TestSuccessfulStartupIsSilent requires a valid database to open with no
// output at all and exit status 0.
func TestSuccessfulStartupIsSilent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ok.db")
	createFixture(t, path)

	stdout, stderr, status := runStartup(t, path)
	if status != 0 {
		t.Errorf("valid database status = %d (stderr %q), want 0", status, stderr)
	}
	if stdout != "" || stderr != "" {
		t.Errorf("successful startup wrote stdout=%q stderr=%q, want silence", stdout, stderr)
	}
}

// TestStartupFailureKeepsStructuredCause guarantees classification stays
// inspectable by internal/cli rather than being flattened to text.
func TestStartupFailureKeepsStructuredCause(t *testing.T) {
	_, err := connection.Open(filepath.Join(t.TempDir(), "missing.db"))
	var se *connection.StartupError
	if !errors.As(err, &se) {
		t.Fatalf("error = %T, want *connection.StartupError", err)
	}
}
