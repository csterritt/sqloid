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
			}
			if se.Path != path {
				t.Errorf("Path = %q, want %q", se.Path, path)
			}
			if se.Kind != tt.wantKind {
				t.Errorf("Kind = %d (%s), want %d (%s)", se.Kind, se.Kind, tt.wantKind, tt.wantKind)
			}
			if tt.notKind != 0 && se.Kind == tt.notKind {
				t.Errorf("Kind = %d (%s), must not be %d (%s)", se.Kind, se.Kind, tt.notKind, tt.notKind)
			}
			if se.Cause != tt.statErr {
				t.Errorf("Cause = %v, want the original error %v", se.Cause, tt.statErr)
			}

			// The cause must stay inspectable through the *StartupError chain.
			if tt.checkIs != nil && !errors.Is(se, tt.checkIs) {
				t.Errorf("errors.Is(se, %v) = false, want true (cause not preserved through wrapping)", tt.checkIs)
			}
			if tt.checkAsPathError {
				var pe *os.PathError
				if !errors.As(se, &pe) {
					t.Errorf("errors.As(se, *os.PathError) = false, want true (PathError wrapping not preserved)")
				}
			}

			msg := se.Error()
			if strings.ContainsAny(msg, "\n\r") {
				t.Errorf("diagnostic %q contains a newline; startup diagnostics are one line", msg)
			}
			if tt.wantExact != "" {
				if msg != tt.wantExact {
					t.Errorf("Error() = %q, want exactly %q", msg, tt.wantExact)
				}
			} else if tt.wantSub != "" && !strings.Contains(msg, tt.wantSub) {
				t.Errorf("Error() = %q, want it to contain %q", msg, tt.wantSub)
			}
		})
	}
}

// TestClassifyStatErrorDoesNotFabricateAbsence pins that a non-not-existence
// stat error never renders the missing-file diagnostic, so unrelated failures
// are not silently relabeled as missing.
func TestClassifyStatErrorDoesNotFabricateAbsence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "target.db")

	unrelated := &os.PathError{Op: "stat", Path: path, Err: syscall.EIO}
	se := classifyStatError(path, unrelated)

	if se.Kind == FailureMissing {
		t.Errorf("unrelated stat error classified as missing; must not fabricate absence")
	}
	if strings.Contains(se.Error(), "no such file or directory") {
		t.Errorf("unrelated stat error rendered the missing-file diagnostic %q; must not", se.Error())
	}
}
