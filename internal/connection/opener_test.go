package connection

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	_ "modernc.org/sqlite"
)

// journalMode reads PRAGMA journal_mode from the raw driver for comparison.
func journalMode(t *testing.T, path string) string {
	t.Helper()

	db, err := sql.Open("sqlite", dsnReadOnly(path))
	if err != nil {
		t.Fatalf("opening %q to read journal mode: %v", path, err)
	}
	defer db.Close()
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("reading journal mode of %q: %v", path, err)
	}
	return mode
}

// dsnReadOnly builds a mode=ro DSN so inspection never mutates fixtures.
func dsnReadOnly(path string) string {
	u := mustFileURL(path)
	q := u.Query()
	q.Set("mode", "ro")
	u.RawQuery = q.Encode()
	return u.String()
}

func TestOpenValidDatabaseReadWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "valid.db")
	createDatabase(t, path)

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	defer db.Close()

	var name string
	if err := db.SQL.QueryRow("SELECT v FROM t WHERE id = 1").Scan(&name); err != nil {
		t.Fatalf("querying opened database: %v", err)
	}
	if name != "one" {
		t.Errorf("probed value = %q, want %q", name, "one")
	}
}

func TestOpenPreservesJournalMode(t *testing.T) {
	walPath := filepath.Join(t.TempDir(), "wal.db")
	deletePath := filepath.Join(t.TempDir(), "delete.db")
	for _, tc := range []struct {
		path     string
		toWal    bool
		wantMode string
	}{
		{path: walPath, toWal: true, wantMode: "wal"},
		{path: deletePath, wantMode: "delete"},
	} {
		t.Run(tc.wantMode, func(t *testing.T) {
			createDatabase(t, tc.path)
			if tc.toWal {
				setJournalMode(t, tc.path, "wal")
			}
			before := journalMode(t, tc.path)

			db, err := Open(tc.path)
			if err != nil {
				t.Fatalf("Open(%q) error = %v", tc.path, err)
			}
			defer db.Close()
			var after string
			if err := db.SQL.QueryRow("PRAGMA journal_mode").Scan(&after); err != nil {
				t.Fatalf("reading journal mode through opener: %v", err)
			}

			if before != tc.wantMode || after != before {
				t.Errorf("journal mode before = %q, after = %q; both must be %q", before, after, tc.wantMode)
			}
		})
	}
}

func setJournalMode(t *testing.T, path, mode string) {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("PRAGMA journal_mode=" + mode); err != nil {
		t.Fatal(err)
	}
}

func TestOpenNonWritableDatabaseIsPermissionDenied(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readonly.db")
	createDatabase(t, path)
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	before := takeSnapshot(t, path)

	_, err := Open(path)
	if os.Geteuid() == 0 && errors.Is(err, syscall.EACCES) {
		t.Skip("running as root; unwritable modes cannot be exercised")
	}
	if err == nil {
		t.Fatal("Open succeeded on a non-writable database, want permission-denied startup failure")
	}
	msg := err.Error()
	want := "cannot open database read-write: " + path + ": permission denied"
	if msg != want {
		t.Errorf("error =\n%q\nwant exactly\n%q", msg, want)
	}
	requireUnchanged(t, before, path)
}

func TestReadWriteDetailClassification(t *testing.T) {
	tests := []struct {
		name      string
		cause     error
		wantTrail string // text following "cannot open database read-write: <path>: "
	}{
		{
			name:      "EACCES renders permission denied",
			cause:     &os.PathError{Op: "open", Path: "/x", Err: syscall.EACCES},
			wantTrail: "permission denied",
		},
		{
			name:      "EPERM renders permission denied",
			cause:     &os.PathError{Op: "open", Path: "/x", Err: syscall.EPERM},
			wantTrail: "permission denied",
		},
		{
			name:      "EROFS renders read-only file system",
			cause:     &os.PathError{Op: "open", Path: "/x", Err: syscall.EROFS},
			wantTrail: "read-only file system",
		},
		{
			name:      "raw driver causes are preserved verbatim",
			cause:     errors.New("some opaque driver failure"),
			wantTrail: "some opaque driver failure",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &StartupError{Path: "/db/here.db", Kind: FailureReadWrite, Cause: tt.cause}
			got := e.Error()
			want := "cannot open database read-write: /db/here.db: " + tt.wantTrail
			if got != want {
				t.Errorf("Error() = %q, want %q", got, want)
			}
		})
	}
}

func TestStartupErrorsKeepOneLineMessages(t *testing.T) {
	messages := []string{
		(&StartupError{Path: "/p/m", Kind: FailureMissing}).Error(),
		(&StartupError{Path: "/p/u", Kind: FailureUnreadable}).Error(),
		(&StartupError{Path: "/p/h", Kind: FailureNotADatabase}).Error(),
		(&StartupError{Path: "/p/r", Kind: FailureReadWrite, Cause: errors.New("cause")}).Error(),
	}
	for _, m := range messages {
		if strings.ContainsAny(m, "\n\r") {
			t.Errorf("diagnostic %q contains a newline; startup diagnostics are one line", m)
		}
	}
}

// TestOpenRelativePath pins the Issue #3 end-to-end requirement that a
// working-directory-relative discovered path opens read-write through the
// same mode=rw flow as absolute paths, without the URI parser rejecting a
// path segment as an authority and without touching the caller's path text.
func TestOpenRelativePath(t *testing.T) {
	t.Chdir(t.TempDir())
	rel := filepath.Join("state", "discovered.sqlite")
	if err := os.MkdirAll("state", 0o755); err != nil {
		t.Fatal(err)
	}
	createDatabase(t, rel)

	db, err := Open(rel)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", rel, err)
	}
	defer db.Close()

	var got string
	if err := db.SQL.QueryRow("SELECT v FROM t WHERE id = 1").Scan(&got); err != nil {
		t.Fatalf("querying opened database: %v", err)
	}
}
