package connection

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	_ "modernc.org/sqlite"
)

// createDatabase creates a real SQLite database at path through the pinned
// driver so opener tests exercise genuine files rather than byte fixtures.
func createDatabase(t *testing.T, path string) {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open(%q) error = %v", path, err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT); INSERT INTO t VALUES (1, 'one')"); err != nil {
		t.Fatalf("creating fixture database: %v", err)
	}
}

// snapshot records the observable filesystem state of a target so tests can
// prove pre-open validation neither created nor modified it.
type snapshot struct {
	exists bool
	size   int64
	mode   os.FileMode
	mtime  int64
	body   []byte
}

func takeSnapshot(t *testing.T, path string) snapshot {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return snapshot{exists: false}
		}
		t.Fatalf("stat(%q) error = %v", path, err)
	}
	body, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, syscall.EACCES) && !errors.Is(err, syscall.EISDIR) {
		t.Fatalf("read(%q) error = %v", path, err)
	}
	return snapshot{
		exists: true,
		size:   info.Size(),
		mode:   info.Mode().Perm(),
		mtime:  info.ModTime().UnixNano(),
		body:   body,
	}
}

func requireUnchanged(t *testing.T, before snapshot, path string) {
	t.Helper()

	after := takeSnapshot(t, path)
	if !snapshotEqual(before, after) {
		t.Fatalf("target %q was modified: before %+v, after %+v", path, before, after)
	}
}

// TestPreOpenValidation is the table-driven startup-validation contract:
// checks run in the mandated order existence → readable → header, with
// structured failure classes, and never create or modify the target.
func TestPreOpenValidation(t *testing.T) {
	validDir := t.TempDir()

	tests := []struct {
		name      string
		setup     func(t *testing.T) string
		wantKind  FailureKind
		wantMsg   string
		skipOnErr error // errno to skip on (e.g. root bypasses file modes)
	}{
		{
			name: "missing file fails existence check without creating it",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "absent.db")
			},
			wantKind: FailureMissing,
			wantMsg:  "no such file or directory",
		},
		{
			name: "directory is rejected as not a database",
			setup: func(t *testing.T) string {
				return validDir
			},
			wantKind: FailureNotADatabase,
			wantMsg:  "not a SQLite database",
		},
		{
			name: "invalid header text file is rejected as not a database",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "text.db")
				if err := os.WriteFile(path, []byte("not a database at all"), 0o644); err != nil {
					t.Fatal(err)
				}
				return path
			},
			wantKind: FailureNotADatabase,
			wantMsg:  "not a SQLite database",
		},
		{
			name: "short file that cannot hold a header is rejected as not a database",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "short.db")
				if err := os.WriteFile(path, []byte(sqliteHeader[:5]), 0o644); err != nil {
					t.Fatal(err)
				}
				return path
			},
			wantKind: FailureNotADatabase,
			wantMsg:  "not a SQLite database",
		},
		{
			name: "header corrupted in final bytes is rejected",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "corrupt.db")
				bad := append([]byte{}, sqliteHeader...)
				bad[15] = 'X'
				if err := os.WriteFile(path, bad, 0o644); err != nil {
					t.Fatal(err)
				}
				return path
			},
			wantKind: FailureNotADatabase,
			wantMsg:  "not a SQLite database",
		},
		{
			name: "unreadable invalid-header file reports readability before header",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "unreadable.db")
				if err := os.WriteFile(path, []byte("junk"), 0o000); err != nil {
					t.Fatal(err)
				}
				return path
			},
			wantKind:  FailureUnreadable,
			wantMsg:   "permission denied",
			skipOnErr: syscall.EACCES,
		},
		{
			name: "unreadable valid-header file still fails readability first",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "locked.db")
				if err := os.WriteFile(path, []byte(sqliteHeader), 0o000); err != nil {
					t.Fatal(err)
				}
				return path
			},
			wantKind:  FailureUnreadable,
			wantMsg:   "permission denied",
			skipOnErr: syscall.EACCES,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)

			before := takeSnapshot(t, path)
			db, err := Open(path)

			if tt.skipOnErr != nil && errors.Is(err, tt.skipOnErr) && os.Geteuid() == 0 {
				t.Skipf("running as root; mode-based unreadability cannot be exercised")
			}
			if err == nil {
				defer db.Close()
				t.Fatalf("Open(%q) succeeded, want %v failure class", path, tt.wantKind)
			}
			var startErr *StartupError
			if !errors.As(err, &startErr) {
				t.Fatalf("Open(%q) error = %T (%v), want *StartupError", path, err, err)
			}
			if startErr.Kind != tt.wantKind {
				t.Errorf("Open(%q) kind = %d (%s), want %d (%s)", path, startErr.Kind, startErr.Kind, tt.wantKind, tt.wantKind)
			}
			if startErr.Path != path {
				t.Errorf("StartupError.Path = %q, want %q", startErr.Path, path)
			}
			msg := startErr.Error()
			if got := msg; len(tt.wantMsg) > 0 && !containsOnce(got, tt.wantMsg) {
				t.Errorf("Open(%q) message = %q, want exactly one occurrence of %q", path, msg, tt.wantMsg)
			}
			requireUnchanged(t, before, path)
		})
	}
}

// snapshotEqual compares observable filesystem state.
func snapshotEqual(a, b snapshot) bool {
	return a.exists == b.exists && a.size == b.size && a.mode == b.mode &&
		a.mtime == b.mtime
}

// containsOnce reports whether s contains sub exactly once, supporting the
// one-line diagnostic requirement.
func containsOnce(s, sub string) bool {
	first := indexOf(s, sub)
	return first >= 0 && indexOf(s[first+1:], sub) < 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
