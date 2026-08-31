package connection

import (
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// createDatabaseEscaped creates a real SQLite database at path through a
// properly escaped file: URI with mode=rwc, so filenames containing reserved
// characters ('?', '#', spaces) are created as the intended file rather than
// being mis-parsed by the driver's URI handler. It works for both relative
// and absolute paths.
func createDatabaseEscaped(t *testing.T, path string) {
	t.Helper()

	u := mustFileURLEscaped(path)
	dsn := u.String() + "?mode=rwc"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open(dsn=%q) error = %v", dsn, err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT); INSERT INTO t VALUES (1, 'one')"); err != nil {
		t.Fatalf("creating fixture database at %q: %v", path, err)
	}
}

// mustFileURLEscaped renders path as a file: URL with the path percent-encoded
// via Go's EscapedPath, using Opaque for relative paths to avoid inventing a
// '//' authority and Path for absolute paths. It mirrors the production
// mustFileURL fix but is independent of it so fixture creation does not
// depend on the code under test.
func mustFileURLEscaped(path string) url.URL {
	if filepath.IsAbs(path) {
		return url.URL{Scheme: "file", Path: path}
	}
	escaped := (&url.URL{Path: path}).EscapedPath()
	return url.URL{Scheme: "file", Opaque: escaped}
}

// listFiles walks root recursively and returns the set of relative file paths.
func listFiles(t *testing.T, root string) map[string]bool {
	t.Helper()

	files := make(map[string]bool)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[rel] = true
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return files
}

// TestMustFileURLRelativeEscaping pins the Issue #59 contract that relative
// filesystem paths render as escaped file-URL path data without becoming a
// URL authority and without letting '?', '#', or spaces alter URL structure.
// Reserved characters must appear percent-encoded as URL path data, never as
// query/fragment delimiters, and relative file URLs must have no invented
// '//' authority.
func TestMustFileURLRelativeEscaping(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantEscaped string // exact escaped substring expected in the URL output
		badChars    string // raw reserved chars that must NOT appear unescaped
	}{
		{"question mark", "foo?bar.db", "foo%3Fbar.db", "?"},
		{"hash", "foo#bar.db", "foo%23bar.db", "#"},
		{"space", "foo bar.db", "foo%20bar.db", " "},
		{"combined reserved", "foo?bar#baz qux.db", "foo%3Fbar%23baz%20qux.db", "?# "},
		{"ordinary relative", "state/discovered.sqlite", "state/discovered.sqlite", "?# "},
		{"wrangler-style relative", ".wrangler/state/v3/d1/miniflare-D1DatabaseObject/x.sqlite", ".wrangler/state/v3/d1/miniflare-D1DatabaseObject/x.sqlite", "?# "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := mustFileURL(tt.path)
			if u.Scheme != "file" {
				t.Errorf("Scheme = %q, want %q", u.Scheme, "file")
			}
			if u.Host != "" {
				t.Errorf("Host = %q, want empty (no authority for relative path)", u.Host)
			}
			s := u.String()
			if strings.Contains(s, "//") {
				t.Errorf("String() = %q, must not contain // (no invented authority)", s)
			}
			if !strings.Contains(s, tt.wantEscaped) {
				t.Errorf("String() = %q, want escaped path data containing %q", s, tt.wantEscaped)
			}
			if strings.ContainsAny(s, tt.badChars) {
				t.Errorf("String() = %q, must not contain unescaped reserved chars %q", s, tt.badChars)
			}
		})
	}
}

// TestMustFileURLAbsolute pins absolute path handling as a regression: the
// path goes into URL.Path (escaped by EscapedPath), Opaque stays empty, and
// the standard file:// authority form is preserved.
func TestMustFileURLAbsolute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "abs.db")
	u := mustFileURL(path)
	if u.Scheme != "file" {
		t.Errorf("Scheme = %q, want %q", u.Scheme, "file")
	}
	if u.Path != path {
		t.Errorf("Path = %q, want %q", u.Path, path)
	}
	if u.Opaque != "" {
		t.Errorf("Opaque = %q, want empty for absolute path", u.Opaque)
	}
	s := u.String()
	if !strings.HasPrefix(s, "file://") {
		t.Errorf("String() = %q, want file:// prefix for absolute path", s)
	}
}

// TestDSNRelativeEscaping pins the Issue #59 contract that dsn produces a
// valid escaped mode=rw DSN for relative paths with reserved characters:
// the path portion is percent-encoded, no '//' authority is invented, and
// the real _busy_timeout and mode=rw query options are preserved.
func TestDSNRelativeEscaping(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantPath string // exact escaped path portion before the query delimiter
	}{
		{"question mark", "foo?bar.db", "file:foo%3Fbar.db"},
		{"hash", "foo#bar.db", "file:foo%23bar.db"},
		{"space", "foo bar.db", "file:foo%20bar.db"},
		{"combined reserved", "foo?bar#baz qux.db", "file:foo%3Fbar%23baz%20qux.db"},
		{"ordinary relative", "state/discovered.sqlite", "file:state/discovered.sqlite"},
		{"wrangler-style relative", ".wrangler/state/v3/d1/miniflare-D1DatabaseObject/x.sqlite", "file:.wrangler/state/v3/d1/miniflare-D1DatabaseObject/x.sqlite"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dsn(tt.path)
			pathPart, queryPart, found := strings.Cut(got, "?")
			if !found {
				t.Fatalf("dsn(%q) = %q, want a query delimiter ?", tt.path, got)
			}
			if pathPart != tt.wantPath {
				t.Errorf("dsn(%q) path portion = %q, want %q", tt.path, pathPart, tt.wantPath)
			}
			if strings.Contains(pathPart, "//") {
				t.Errorf("dsn path portion %q contains // (invented authority)", pathPart)
			}
			if strings.ContainsAny(pathPart, "?# ") {
				t.Errorf("dsn path portion %q contains unescaped reserved chars", pathPart)
			}
			if !strings.Contains(queryPart, "mode=rw") {
				t.Errorf("dsn(%q) query %q missing mode=rw", tt.path, queryPart)
			}
			if !strings.Contains(queryPart, "_busy_timeout=5000") {
				t.Errorf("dsn(%q) query %q missing _busy_timeout=5000", tt.path, queryPart)
			}
		})
	}
}

// TestDSNAbsolutePath pins absolute path DSN construction as a regression:
// the file:// authority form is used, the path is escaped, and mode=rw with
// _busy_timeout=5000 is preserved.
func TestDSNAbsolutePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "abs.db")
	got := dsn(path)
	if !strings.HasPrefix(got, "file://") {
		t.Errorf("dsn(%q) = %q, want file:// prefix for absolute path", path, got)
	}
	pathPart, queryPart, found := strings.Cut(got, "?")
	if !found {
		t.Fatalf("dsn(%q) = %q, want a query delimiter ?", path, got)
	}
	if !strings.Contains(pathPart, path) {
		t.Errorf("dsn path portion %q does not contain %q", pathPart, path)
	}
	if !strings.Contains(queryPart, "mode=rw") {
		t.Errorf("dsn query %q missing mode=rw", queryPart)
	}
	if !strings.Contains(queryPart, "_busy_timeout=5000") {
		t.Errorf("dsn query %q missing _busy_timeout=5000", queryPart)
	}
}

// TestOpenRelativeReservedCharacterFilenames is the Issue #59 end-to-end
// contract: working-directory-relative SQLite filenames containing '?', '#',
// spaces, and combined reserved characters open read-write through the same
// mode=rw flow as ordinary paths. For each fixture the intended existing
// database opens, a known row is selected, a write reaches that same file,
// and no differently parsed or newly created path appears.
func TestOpenRelativeReservedCharacterFilenames(t *testing.T) {
	tests := []struct {
		name string
		rel  string
	}{
		{"question mark", "foo?bar.db"},
		{"hash", "foo#bar.db"},
		{"space", "foo bar.db"},
		{"combined reserved", "foo?bar#baz qux.db"},
		{"ordinary relative", "state/discovered.sqlite"},
		{"wrangler-style relative", ".wrangler/state/v3/d1/miniflare-D1DatabaseObject/x.sqlite"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			dir := filepath.Dir(tt.rel)
			if dir != "." {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			createDatabaseEscaped(t, tt.rel)

			before := listFiles(t, ".")

			db, err := Open(tt.rel)
			if err != nil {
				t.Fatalf("Open(%q) error = %v", tt.rel, err)
			}

			var got string
			if err := db.SQL.QueryRow("SELECT v FROM t WHERE id = 1").Scan(&got); err != nil {
				t.Fatalf("querying opened database: %v", err)
			}
			if got != "one" {
				t.Errorf("probed value = %q, want %q", got, "one")
			}

			if _, err := db.SQL.Exec("INSERT INTO t VALUES (2, 'two')"); err != nil {
				t.Fatalf("writing to database: %v", err)
			}
			if err := db.Close(); err != nil {
				t.Fatalf("closing database: %v", err)
			}

			after := listFiles(t, ".")
			for f := range after {
				if !before[f] {
					t.Errorf("Open(%q) created unexpected new file: %s", tt.rel, f)
				}
			}

			db2, err := Open(tt.rel)
			if err != nil {
				t.Fatalf("re-Open(%q) error = %v", tt.rel, err)
			}
			defer db2.Close()
			var got2 string
			if err := db2.SQL.QueryRow("SELECT v FROM t WHERE id = 2").Scan(&got2); err != nil {
				t.Fatalf("querying re-opened database: %v", err)
			}
			if got2 != "two" {
				t.Errorf("written value = %q, want %q (write did not reach %q)", got2, "two", tt.rel)
			}
		})
	}
}

// TestOpenAbsoluteReservedCharacterFilename pins the absolute-path regression
// for Issue #59: an absolute path with a reserved character opens read-write,
// the known row is selected, a write reaches the same file, and no alternate
// file is created.
func TestOpenAbsoluteReservedCharacterFilename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "abs?bar.db")
	createDatabaseEscaped(t, path)

	before := listFiles(t, dir)

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}

	var got string
	if err := db.SQL.QueryRow("SELECT v FROM t WHERE id = 1").Scan(&got); err != nil {
		t.Fatalf("querying opened database: %v", err)
	}
	if got != "one" {
		t.Errorf("probed value = %q, want %q", got, "one")
	}

	if _, err := db.SQL.Exec("INSERT INTO t VALUES (2, 'two')"); err != nil {
		t.Fatalf("writing to database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing database: %v", err)
	}

	after := listFiles(t, dir)
	for f := range after {
		if !before[f] {
			t.Errorf("Open(%q) created unexpected new file: %s", path, f)
		}
	}

	db2, err := Open(path)
	if err != nil {
		t.Fatalf("re-Open(%q) error = %v", path, err)
	}
	defer db2.Close()
	var got2 string
	if err := db2.SQL.QueryRow("SELECT v FROM t WHERE id = 2").Scan(&got2); err != nil {
		t.Fatalf("querying re-opened database: %v", err)
	}
	if got2 != "two" {
		t.Errorf("written value = %q, want %q (write did not reach %q)", got2, "two", path)
	}
}
