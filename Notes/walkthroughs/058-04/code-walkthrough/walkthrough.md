# Issue #058 Code Walkthrough: Classify Stat Permission Errors as Unreadable

*2026-08-31T17:55:14Z by Showboat 0.6.1*
<!-- showboat-id: d097aff5-34e3-4d03-862e-bddcede1c54e -->

Issue #58 (Notes/tasks/058-classify-stat-permission-errors.md, Notes/PRD-sqloid.md §Startup validation and errors, §CLI behavior) refines the initial path-stat classification in internal/connection/startup.go. Before this issue, every os.Stat failure was classified as FailureMissing, so a permission-denied stat (EACCES/EPERM from the file or a denied parent directory traversal) or an unrelated stat error (EIO, ELOOP) was silently reported as 'no such file or directory' — fabricating absence for a path that may exist but cannot be accessed. Issue #58 introduces the classifyStatError seam so only os.IsNotExist errors remain missing while EACCES/EPERM and other non-not-existence causes are classified as FailureUnreadable with preserved unwrappable causes.

The classification seam and cause-aware rendering live in internal/connection/startup.go. classifyStatError is the narrow function boundary that tests exercise directly with constructed errors — no filesystem permissions required. unreadableDetail renders the FailureUnreadable message cause-aware: EACCES/EPERM render as 'permission denied', other errnos render verbatim, nil cause defaults to 'permission denied'.

```bash
grep -n 'func classifyStatError\|func unreadableDetail' internal/connection/startup.go
```

```output
134:func unreadableDetail(cause error) string {
320:func classifyStatError(path string, err error) *StartupError {
```

```bash
sed -n '128,147p' internal/connection/startup.go
```

```output
// unreadableDetail maps a stat- or read-time accessibility failure onto its
// message fragment. EACCES and EPERM (including *os.PathError wrapping) render
// as "permission denied"; any other preserved errno or raw cause renders
// verbatim so unrelated stat failures stay actionable rather than masquerading
// as permission denial. A nil cause defaults to "permission denied" to keep the
// pre-Issue #58 rendering for bare StartupError values.
func unreadableDetail(cause error) string {
	var errno syscall.Errno
	if errors.As(cause, &errno) {
		if errno == syscall.EACCES || errno == syscall.EPERM {
			return "permission denied"
		}
		return errno.Error()
	}
	if cause != nil {
		return cause.Error()
	}
	return "permission denied"
}

```

```bash
sed -n '320,326p' internal/connection/startup.go
```

```output
func classifyStatError(path string, err error) *StartupError {
	if os.IsNotExist(err) {
		return &StartupError{Path: path, Kind: FailureMissing, Cause: err}
	}
	return &StartupError{Path: path, Kind: FailureUnreadable, Cause: err}
}

```

The table-driven test in internal/connection/stat_classify_test.go exercises the four mandated stat-boundary cases through the classifyStatError seam. Each case asserts the typed Kind, cause preservation via errors.Is/errors.As, and the exact rendered line — all without depending on the test user's filesystem permissions.

```bash
sed -n '1,60p' internal/connection/stat_classify_test.go
```

```output
package connection

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// TestClassifyStatError is the Issue #58 table-driven contract around the
// initial path-stat boundary: EACCES/EPERM (including *os.PathError wrapping)
// produce *StartupError with FailureUnreadable, the original cause stays
// inspectable through errors.Is/errors.As, and the rendered line is exactly
// "<path>: permission denied". Only os.IsNotExist errors retain
// FailureMissing and the missing-file diagnostic. Unrelated stat errors are
// not silently relabeled missing: they receive an actionable non-missing
// classification with the cause preserved. The seam is classifyStatError, so
// no test depends on the test user's filesystem permissions.
func TestClassifyStatError(t *testing.T) {
	const path = "/test/db.sqlite"

	tests := []struct {
		name      string
		statErr   error
		wantKind  FailureKind
		wantExact string // exact Error() when non-empty
		wantSub   string // substring that must appear when wantExact is empty
		notKind   FailureKind
		// checkIs is the sentinel that errors.Is must resolve through the
		// returned *StartupError's preserved cause chain.
		checkIs error
		// checkAs is true when the cause must be unwrappable as *os.PathError.
		checkAsPathError bool
	}{
		{
			name:             "EACCES wrapped in PathError is unreadable",
			statErr:          &os.PathError{Op: "stat", Path: path, Err: syscall.EACCES},
			wantKind:         FailureUnreadable,
			wantExact:        path + ": permission denied",
			checkIs:          syscall.EACCES,
			checkAsPathError: true,
		},
		{
			name:             "EPERM wrapped in PathError is unreadable",
			statErr:          &os.PathError{Op: "stat", Path: path, Err: syscall.EPERM},
			wantKind:         FailureUnreadable,
			wantExact:        path + ": permission denied",
			checkIs:          syscall.EPERM,
			checkAsPathError: true,
		},
		{
			name:      "fs.ErrNotExist wrapped in PathError remains missing",
			statErr:   &os.PathError{Op: "stat", Path: path, Err: fs.ErrNotExist},
			wantKind:  FailureMissing,
			wantExact: path + ": no such file or directory",
			checkIs:   fs.ErrNotExist,
		},
```

```bash
sed -n '60,100p' internal/connection/stat_classify_test.go
```

```output
		},
		{
			name:      "raw syscall.ENOENT remains missing",
			statErr:   syscall.ENOENT,
			wantKind:  FailureMissing,
			wantExact: path + ": no such file or directory",
			checkIs:   syscall.ENOENT,
		},
		{
			name:             "unrelated EIO wrapped in PathError is not missing",
			statErr:          &os.PathError{Op: "stat", Path: path, Err: syscall.EIO},
			wantKind:         FailureUnreadable,
			notKind:          FailureMissing,
			wantSub:          path,
			checkIs:          syscall.EIO,
			checkAsPathError: true,
		},
		{
			name:             "unrelated ELOOP wrapped in PathError is not missing",
			statErr:          &os.PathError{Op: "stat", Path: path, Err: syscall.ELOOP},
			wantKind:         FailureUnreadable,
			notKind:          FailureMissing,
			wantSub:          path,
			checkIs:          syscall.ELOOP,
			checkAsPathError: true,
		},
		{
			name:     "bare non-sentinel error is not missing",
			statErr:  errors.New("some opaque stat failure"),
			wantKind: FailureUnreadable,
			notKind:  FailureMissing,
			wantSub:  path,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			se := classifyStatError(path, tt.statErr)

			if se == nil {
				t.Fatal("classifyStatError returned nil, want *StartupError")
```

Running the stat-boundary classification test verifies all cases pass: EACCES/EPERM are unreadable with 'permission denied', fs.ErrNotExist/ENOENT remain missing, and unrelated EIO/ELOOP/bare errors are unreadable but never relabeled missing.

```bash
go test -count=1 -v ./internal/connection/ -run '^TestClassifyStatError' 2>&1 | grep -E 'RUN|PASS|FAIL|ok '
```

```output
=== RUN   TestClassifyStatError
=== RUN   TestClassifyStatError/EACCES_wrapped_in_PathError_is_unreadable
=== RUN   TestClassifyStatError/EPERM_wrapped_in_PathError_is_unreadable
=== RUN   TestClassifyStatError/fs.ErrNotExist_wrapped_in_PathError_remains_missing
=== RUN   TestClassifyStatError/raw_syscall.ENOENT_remains_missing
=== RUN   TestClassifyStatError/unrelated_EIO_wrapped_in_PathError_is_not_missing
=== RUN   TestClassifyStatError/unrelated_ELOOP_wrapped_in_PathError_is_not_missing
=== RUN   TestClassifyStatError/bare_non-sentinel_error_is_not_missing
--- PASS: TestClassifyStatError (0.00s)
    --- PASS: TestClassifyStatError/EACCES_wrapped_in_PathError_is_unreadable (0.00s)
    --- PASS: TestClassifyStatError/EPERM_wrapped_in_PathError_is_unreadable (0.00s)
    --- PASS: TestClassifyStatError/fs.ErrNotExist_wrapped_in_PathError_remains_missing (0.00s)
    --- PASS: TestClassifyStatError/raw_syscall.ENOENT_remains_missing (0.00s)
    --- PASS: TestClassifyStatError/unrelated_EIO_wrapped_in_PathError_is_not_missing (0.00s)
    --- PASS: TestClassifyStatError/unrelated_ELOOP_wrapped_in_PathError_is_not_missing (0.00s)
    --- PASS: TestClassifyStatError/bare_non-sentinel_error_is_not_missing (0.00s)
=== RUN   TestClassifyStatErrorDoesNotFabricateAbsence
--- PASS: TestClassifyStatErrorDoesNotFabricateAbsence (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/connection	0.002s
```

The CLI binary demonstrates the same classification at the process level. A stat-time EACCES (parent directory not traversable) and a read-open EACCES (file mode 0o000) both produce exactly one stderr line '<path>: permission denied' with status 1. A genuinely missing path produces '<path>: no such file or directory' with status 1 and no file creation. The directory and invalid-header cases retain their unchanged 'not a SQLite database' classification — Issue #58 does not reorder or alter those diagnostics.

```bash
cd /tmp && rm -rf sqloid-demo-dir && mkdir sqloid-demo-dir && cd sqloid-demo-dir && echo '--- stat-time EACCES: parent dir not traversable (chmod 000 on dir) ---' && mkdir denied && printf 'SQLite format 3\x00' > denied/lock.db && chmod 000 denied && /tmp/sqloid-demo sqlite denied/lock.db 2>&1; echo "exit=$?" && chmod 755 denied && echo && echo '--- read-open EACCES: file mode 0o000 (stat succeeds, os.Open fails) ---' && chmod 000 denied/lock.db && /tmp/sqloid-demo sqlite denied/lock.db 2>&1; echo "exit=$?" && echo && echo '--- genuinely missing path ---' && /tmp/sqloid-demo sqlite genuinely-missing.db 2>&1; echo "exit=$?" && echo && echo '--- no file was created ---' && ls && echo && echo '--- directory (unchanged not-a-database) ---' && mkdir adir && /tmp/sqloid-demo sqlite adir 2>&1; echo "exit=$?" && echo && echo '--- invalid header (unchanged not-a-database) ---' && printf 'not a database' > text.db && /tmp/sqloid-demo sqlite text.db 2>&1; echo "exit=$?"
```

```output
--- stat-time EACCES: parent dir not traversable (chmod 000 on dir) ---
denied/lock.db: permission denied
exit=1

--- read-open EACCES: file mode 0o000 (stat succeeds, os.Open fails) ---
denied/lock.db: permission denied
exit=1

--- genuinely missing path ---
genuinely-missing.db: no such file or directory
exit=1

--- no file was created ---
denied

--- directory (unchanged not-a-database) ---
adir: not a SQLite database
exit=1

--- invalid header (unchanged not-a-database) ---
text.db: not a SQLite database
exit=1
```

Both stat-time EACCES (parent directory not traversable) and read-open EACCES (file mode 0o000) produce the same exact '<path>: permission denied' line with status 1 — the FailureUnreadable classification is unified across both boundaries. The genuinely missing path produces '<path>: no such file or directory' with no file creation. The directory and invalid-header cases retain their unchanged 'not a SQLite database' classification. Issue #58's change is surgical: only the initial stat classification was refined, preserving the full validation order (existence/stat -> readability -> header -> read-write open -> probe) and every existing distinct classification. Cross-reference: Issue #58, Notes/PRD-sqloid.md §Startup validation and errors, §CLI behavior, user stories 3 and 7.
