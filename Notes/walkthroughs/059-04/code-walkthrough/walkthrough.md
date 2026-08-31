# Issue #059 Code Walkthrough: Percent-Encode Relative SQLite DSN Paths

*2026-08-31T18:54:07Z by Showboat 0.6.1*
<!-- showboat-id: a97932c2-afbb-463b-ab42-0597d52d8381 -->

Issue #59 (Notes/tasks/059-encode-relative-sqlite-dsns.md, Notes/PRD-sqloid.md §Startup validation and errors) fixes relative SQLite DSN construction in internal/connection/startup.go. Before this issue, mustFileURL placed raw relative path text directly into url.URL.Opaque, which url.URL.String() does not percent-encode. Reserved characters in filenames — '?', '#', spaces — therefore passed through unescaped and became query/fragment delimiters in the resulting file: URI, causing the SQLite URI parser to open a differently parsed filename (e.g. 'foo?bar.db' opened 'foo' with query 'bar.db'). Issue #59 percent-encodes relative paths through url.URL.EscapedPath() into Opaque, so reserved characters remain filename data while no '//' authority is invented for relative paths. Absolute paths are unchanged; mode=rw, the five-second busy timeout, validation order, diagnostics, and no-create behavior are all preserved.

The fix lives in mustFileURL, called by dsn, called by Open. dsn builds the file: URI with mode=rw and _busy_timeout=5000 query options; mustFileURL renders the path portion. The boundary between them is unchanged — only the path serialization inside mustFileURL differs.

```bash
grep -n 'func mustFileURL\|func dsn' internal/connection/startup.go
```

```output
289:func dsn(path string) string {
312:func mustFileURL(path string) url.URL {
```

```bash
sed -n '286,325p' internal/connection/startup.go
```

```output
// dsn builds the modernc.org/sqlite data source name for path: URI form so
// that mode=rw forbids creating a missing database, with the path percent-
// encoded so reserved characters such as '?' or '#' stay part of the filename.
func dsn(path string) string {
	u := mustFileURL(path)
	q := u.Query()
	q.Set("mode", "rw")
	// Applies to every physical connection the pool creates: busy handling is
	// per connection, so the five-second bound travels with the DSN itself.
	q.Set("_busy_timeout", strconv.Itoa(busyTimeoutMillis))
	u.RawQuery = q.Encode()
	return u.String()
}

// mustFileURL renders path as a file: URI whose query string can be extended;
// it only panics for inputs that cannot occur from normal callers.
//
// Relative paths render as opaque "file:<escaped-path>" URIs: the path is
// percent-encoded through url.URL.EscapedPath so reserved characters such as
// '?', '#', and spaces remain filename data rather than becoming query or
// fragment delimiters, and the escaped result is placed in Opaque so
// url.URL.String() does not promote the first path segment to a URI authority
// ("file://.wrangler/..."), which the SQLite URI parser rejects with "invalid
// uri authority" while diagnostics keep referring to the caller's relative
// path. Absolute paths use URL.Path, whose EscapedPath escaping is applied
// automatically by String() under the standard file:// authority form.
func mustFileURL(path string) url.URL {
	if filepath.IsAbs(path) {
		return url.URL{Scheme: "file", Path: path}
	}
	u := url.URL{Path: path}
	return url.URL{Scheme: "file", Opaque: u.EscapedPath()}
}

// classifyStatError maps an initial os.Stat failure onto a *StartupError,
// preserving the original cause unwrappable through the returned error. Only
// os.IsNotExist errors are classified as missing (Issue #58); EACCES/EPERM
// permission failures and any other non-not-existence stat cause are
// classified as unreadable so absence is never fabricated for a path that may
// exist but cannot be accessed.
```

The fix is two lines in mustFileURL's relative branch: instead of placing raw path text in Opaque, it first constructs a url.URL with Path set to the relative path, calls EscapedPath() to get Go's standard URL path escaping (which encodes '?', '#', and spaces as %3F, %23, %20 respectively), and places that escaped result in Opaque. This uses Go's URL representation deliberately — EscapedPath is the same escaping String() applies to URL.Path for absolute paths — so there is no manual pre-escaping and no double-encoding risk. Ordinary relative paths and Wrangler-style paths have no reserved characters, so EscapedPath leaves them unchanged.

The table-driven tests in internal/connection/dsn_escape_test.go cover mustFileURL, dsn, and real Open behavior for relative filenames containing '?', '#', spaces, combined reserved characters, an ordinary relative path, a dot-prefixed Wrangler-style path, and an absolute path.

```bash
sed -n '1,50p' internal/connection/dsn_escape_test.go
```

```output
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

```

```bash
sed -n '85,175p' internal/connection/dsn_escape_test.go
```

```output
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
```

```bash
go test -count=1 -v ./internal/connection/ -run 'TestMustFileURL|TestDSN|TestOpenRelativeReserved|TestOpenAbsoluteReserved' 2>&1
```

```output
=== RUN   TestMustFileURLRelativeEscaping
=== RUN   TestMustFileURLRelativeEscaping/question_mark
=== RUN   TestMustFileURLRelativeEscaping/hash
=== RUN   TestMustFileURLRelativeEscaping/space
=== RUN   TestMustFileURLRelativeEscaping/combined_reserved
=== RUN   TestMustFileURLRelativeEscaping/ordinary_relative
=== RUN   TestMustFileURLRelativeEscaping/wrangler-style_relative
--- PASS: TestMustFileURLRelativeEscaping (0.00s)
    --- PASS: TestMustFileURLRelativeEscaping/question_mark (0.00s)
    --- PASS: TestMustFileURLRelativeEscaping/hash (0.00s)
    --- PASS: TestMustFileURLRelativeEscaping/space (0.00s)
    --- PASS: TestMustFileURLRelativeEscaping/combined_reserved (0.00s)
    --- PASS: TestMustFileURLRelativeEscaping/ordinary_relative (0.00s)
    --- PASS: TestMustFileURLRelativeEscaping/wrangler-style_relative (0.00s)
=== RUN   TestMustFileURLAbsolute
--- PASS: TestMustFileURLAbsolute (0.00s)
=== RUN   TestDSNRelativeEscaping
=== RUN   TestDSNRelativeEscaping/question_mark
=== RUN   TestDSNRelativeEscaping/hash
=== RUN   TestDSNRelativeEscaping/space
=== RUN   TestDSNRelativeEscaping/combined_reserved
=== RUN   TestDSNRelativeEscaping/ordinary_relative
=== RUN   TestDSNRelativeEscaping/wrangler-style_relative
--- PASS: TestDSNRelativeEscaping (0.00s)
    --- PASS: TestDSNRelativeEscaping/question_mark (0.00s)
    --- PASS: TestDSNRelativeEscaping/hash (0.00s)
    --- PASS: TestDSNRelativeEscaping/space (0.00s)
    --- PASS: TestDSNRelativeEscaping/combined_reserved (0.00s)
    --- PASS: TestDSNRelativeEscaping/ordinary_relative (0.00s)
    --- PASS: TestDSNRelativeEscaping/wrangler-style_relative (0.00s)
=== RUN   TestDSNAbsolutePath
--- PASS: TestDSNAbsolutePath (0.00s)
=== RUN   TestOpenRelativeReservedCharacterFilenames
=== RUN   TestOpenRelativeReservedCharacterFilenames/question_mark
=== RUN   TestOpenRelativeReservedCharacterFilenames/hash
=== RUN   TestOpenRelativeReservedCharacterFilenames/space
=== RUN   TestOpenRelativeReservedCharacterFilenames/combined_reserved
=== RUN   TestOpenRelativeReservedCharacterFilenames/ordinary_relative
=== RUN   TestOpenRelativeReservedCharacterFilenames/wrangler-style_relative
--- PASS: TestOpenRelativeReservedCharacterFilenames (0.05s)
    --- PASS: TestOpenRelativeReservedCharacterFilenames/question_mark (0.01s)
    --- PASS: TestOpenRelativeReservedCharacterFilenames/hash (0.01s)
    --- PASS: TestOpenRelativeReservedCharacterFilenames/space (0.01s)
    --- PASS: TestOpenRelativeReservedCharacterFilenames/combined_reserved (0.01s)
    --- PASS: TestOpenRelativeReservedCharacterFilenames/ordinary_relative (0.01s)
    --- PASS: TestOpenRelativeReservedCharacterFilenames/wrangler-style_relative (0.01s)
=== RUN   TestOpenAbsoluteReservedCharacterFilename
--- PASS: TestOpenAbsoluteReservedCharacterFilename (0.01s)
PASS
ok  	github.com/chris/sqloid/internal/connection	0.067s
```

Now we exercise the shipped TUI binary (made executable by Issue #57) against real SQLite files with reserved-character relative filenames. For each filename we create the database, snapshot the directory, start the TUI under a PTY with a timeout, and verify no alternate file was created. The TUI starts successfully (the builder renders) and exits cleanly when sent q+Enter.

```bash
go build -o /tmp/sqloid ./cmd/sqloid && echo 'build ok'
```

```output
build ok
```

The following Go program uses the project's creack/pty dependency (the same library as cmd/sqloid/pty_integration_test.go from Issue #57) to run the shipped binary under a pseudo-terminal for each reserved-character filename. It creates the database, snapshots the directory, starts the TUI, waits for the builder's 'Command' field to render, sends q+Enter to quit, and checks that no alternate file was created.

```bash
rm -rf /tmp/sqloid_walkthrough && go run /tmp/pty_smoke/main.go
```

```output
=== question mark: foo?bar.db === builder=true exit=0 newFiles=[] fileExists=true
=== hash: foo#bar.db === builder=true exit=0 newFiles=[] fileExists=true
=== space: foo bar.db === builder=true exit=0 newFiles=[] fileExists=true
=== combined reserved: foo?bar#baz qux.db === builder=true exit=0 newFiles=[] fileExists=true
=== wrangler-style: .wrangler/state/v3/d1/miniflare-D1DatabaseObject/x.sqlite === builder=true exit=0 newFiles=[] fileExists=true
=== absolute: /tmp/sqloid_walkthrough/abs_test.db === builder=true exit=0 newFiles=[] fileExists=true
```

Every reserved-character relative filename opens through the shipped TUI binary: the builder renders (proving the database was opened read-write through the escaped DSN), the process exits cleanly with status 0, and no alternate file is created in any case. The absolute path case is pinned as a regression. This confirms the Issue #59 fix end-to-end through the production composition root (Issue #57): connection.Open → session.Compose → tea.NewProgram → builder render → q+Enter quit → exit 0.

Cross-references: Issue #57 (production TUI composition and binary smoke path), Issue #59 (percent-encode relative SQLite DSN paths), Notes/PRD-sqloid.md §Startup validation and errors, §Connection Module Design.
