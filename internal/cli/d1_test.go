package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chris/sqloid/internal/d1"
)

// zeroCandidateStderr is the exact two-line diagnostic required by Issue #4
// and the D1 discovery section of Notes/PRD-sqloid.md for every zero-candidate
// outcome (missing, unreadable, empty, or candidate-free directory).
var zeroCandidateStderr = d1.ErrNoCandidate.Error() + "\n" +
	"Expected " + d1.Dir + "; your Wrangler version may use a different local-state layout. Use sqloid sqlite <file> to open the database explicitly.\n"

// multipleCandidatesStderr is the exact single-line diagnostic required by
// Issue #4 for more than one eligible candidate; it deliberately carries no
// expected-path or explicit-open hint.
const multipleCandidatesStderr = "There is more than one SQLite database in .wrangler\n"

// TestRunD1DiscoveryFailureMapsExactDiagnosticAndSkipsOpener proves that each
// typed internal/d1 failure outcome maps to its exact Issue #4 diagnostic and
// that the shared opener is never invoked on any discovery failure.
func TestRunD1DiscoveryFailureMapsExactDiagnosticAndSkipsOpener(t *testing.T) {
	tests := []struct {
		name      string
		discover  func() (string, error)
		wantError string
		wantLines int
		wantHint  bool
	}{
		{
			name:      "zero candidates yields the exact two lines",
			discover:  func() (string, error) { return "", d1.ErrNoCandidate },
			wantError: strings.TrimSuffix(zeroCandidateStderr, "\n"),
			wantLines: 2,
			wantHint:  true,
		},
		{
			name:      "multiple candidates yields the exact single line",
			discover:  func() (string, error) { return "", d1.ErrMultipleCandidates },
			wantError: strings.TrimSuffix(multipleCandidatesStderr, "\n"),
			wantLines: 1,
			wantHint:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opened := false
			err := runD1With(tt.discover, func(path string) error {
				opened = true
				return nil
			})
			if err == nil {
				t.Fatal("runD1With() = nil error, want the mapped discovery diagnostic")
			}
			if err.Error() != tt.wantError {
				t.Errorf("diagnostic = %q, want exactly %q", err.Error(), tt.wantError)
			}
			if lines := strings.Count(err.Error(), "\n") + 1; lines != tt.wantLines {
				t.Errorf("diagnostic line count = %d, want %d", lines, tt.wantLines)
			}
			hasHint := strings.Contains(err.Error(), d1.Dir) && strings.Contains(err.Error(), "sqloid sqlite <file>")
			if hasHint != tt.wantHint {
				t.Errorf("hint present = %v, want %v (diagnostic %q)", hasHint, tt.wantHint, err.Error())
			}
			if opened {
				t.Error("shared opener was invoked on a discovery failure, want no opener call")
			}
		})
	}
}

// newWranglerDir creates the expected Wrangler D1 directory relative to the
// current working directory (the caller must have t.Chdir'd first) unless it
// is meant to stay missing.
func newWranglerDir(t *testing.T, mkdir bool) string {
	t.Helper()
	if !mkdir {
		return d1.Dir
	}
	if err := os.MkdirAll(d1.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return d1.Dir
}

// snapshotDir walked the working directory in earlier drafts; walkFiles below
// is the retained helper.

// TestRunD1PassesSoleCandidateUnchangedToSharedOpener proves the Issue #3
// handoff contract: the single candidate from internal/d1 reaches the shared
// internal/connection opener verbatim, and there is no D1-specific opening
// or validation path between them.
func TestRunD1PassesSoleCandidateUnchangedToSharedOpener(t *testing.T) {
	discovered := filepath.Join(".wrangler/state/v3/d1/miniflare-D1DatabaseObject", "abc123.sqlite")

	var opened string
	err := runD1With(
		func() (string, error) { return discovered, nil },
		func(path string) error { opened = path; return nil },
	)
	if err != nil {
		t.Fatalf("runD1With() error = %v", err)
	}
	if opened != discovered {
		t.Errorf("opener received %q, want the sole candidate %q unchanged", opened, discovered)
	}
}

// writeFile creates a file that must be ignored by discovery; content is
// irrelevant because eligibility is purely name-based.
func writeFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestD1EndToEndOpensSoleDiscoveredCandidate runs `sqloid d1` through Main on
// a mixed filesystem fixture: one eligible candidate plus metadata, sidecar,
// wrong-case, nested, and alternate-layout files that must be ignored.
// Successful startup must stay silent (exit 0, no output).
func TestD1EndToEndOpensSoleDiscoveredCandidate(t *testing.T) {
	t.Chdir(t.TempDir())
	dir := ".wrangler/state/v3/d1/miniflare-D1DatabaseObject"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	createFixture(t, filepath.Join(dir, "abc123.sqlite"))

	// Every name here is excluded by exactly one documented rule.
	for _, name := range []string{
		"state-metadata.sqlite",        // lowercase metadata substring
		"db-metadata-notes.sqlite-shm", // metadata sidecar
		"ABC.SQLITE",                   // case-sensitive extension
		"abc123.sqlite-wal",            // -wal sidecar of the candidate
		"abc123.sqlite-shm",            // -shm sidecar of the candidate
	} {
		writeFile(t, dir, name)
	}
	nestedDir := filepath.Join(dir, "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, nestedDir, "deep.sqlite") // nested files are never searched

	altLayout := ".wrangler/state/v3/wrangler-state/v2/d1/miniflare-D1DatabaseObject"
	if err := os.MkdirAll(altLayout, 0o755); err != nil {
		t.Fatal(err)
	}
	createFixture(t, filepath.Join(altLayout, "alternate.sqlite")) // alternate layouts are never searched

	stdout, stderr, status := runD1(t)
	if status != 0 {
		t.Fatalf("status = %d, stderr = %q, want silent success (0)", status, stderr)
	}
	if stdout != "" || stderr != "" {
		t.Errorf("successful startup wrote stdout=%q stderr=%q, want silence", stdout, stderr)
	}
}

// TestD1DiscoveryFailureProcessBehavior runs `sqloid d1` through Main against
// every Issue #4 failure fixture and asserts the exact diagnostic on stderr,
// exit status 1, silence on stdout, and that nothing was created anywhere in
// the working directory.
func TestD1DiscoveryFailureProcessBehavior(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission-based fixtures cannot be exercised")
	}

	tests := []struct {
		name       string
		mkdir      bool                           // create the Wrangler directory at all
		plant      func(t *testing.T, dir string) // optional fixture contents
		unreadable bool                           // chmod the directory to 0o000
		wantStderr string
	}{
		{
			name:       "missing directory",
			mkdir:      false,
			wantStderr: zeroCandidateStderr,
		},
		{
			name:       "empty directory",
			mkdir:      true,
			wantStderr: zeroCandidateStderr,
		},
		{
			name:  "candidate-free directory",
			mkdir: true,
			plant: func(t *testing.T, dir string) {
				writeFile(t, dir, "state-metadata.sqlite") // only an excluded name
			},
			wantStderr: zeroCandidateStderr,
		},
		{
			name:  "multiple candidates",
			mkdir: true,
			plant: func(t *testing.T, dir string) {
				writeFile(t, dir, "first.sqlite")
				writeFile(t, dir, "second.sqlite")
			},
			wantStderr: multipleCandidatesStderr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			root, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			dir := newWranglerDir(t, tt.mkdir)
			if tt.plant != nil {
				tt.plant(t, dir)
			}
			before := walkFiles(t, root)

			if tt.unreadable {
				if err := os.Chmod(dir, 0o000); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.Chmod(dir, 0o755) })
			}

			stdout, stderr, status := runD1(t)
			if tt.unreadable {
				// Restore access so the no-creation snapshot can be taken.
				os.Chmod(dir, 0o755)
			}

			if status != 1 {
				t.Errorf("status = %d, want 1", status)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want silence", stdout)
			}
			if stderr != tt.wantStderr {
				t.Errorf("stderr = %q (%d bytes), want exactly %q", stderr, len(stderr), tt.wantStderr)
			}
			if after := walkFiles(t, root); !sameFileSet(before, after) {
				t.Errorf("working directory changed from %v to %v; a failed discovery must create no database", before, after)
			}
		})
	}
}

// unreadableDirectory pins that an unwrappable-permission failure of the
// Wrangler directory itself produces the same zero-candidate two lines.
func TestD1DiscoveryUnreadableDirectoryProcessBehavior(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission-based fixtures cannot be exercised")
	}
	t.Chdir(t.TempDir())
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := newWranglerDir(t, true)
	before := walkFiles(t, root)
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	stdout, stderr, status := runD1(t)
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if status != 1 {
		t.Errorf("status = %d, want 1", status)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want silence", stdout)
	}
	if stderr != zeroCandidateStderr {
		t.Errorf("stderr = %q, want exactly %q", stderr, zeroCandidateStderr)
	}
	if after := walkFiles(t, root); !sameFileSet(before, after) {
		t.Errorf("working directory changed from %v to %v; a failed discovery must create no database", before, after)
	}
}

// walkFiles records the path of every regular file under root so equality of
// two snapshots proves neither target databases nor stray files appeared.
func walkFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return files
}

// sameFileSet compares two directory snapshots ignoring order.
func sameFileSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, p := range a {
		counts[p]++
	}
	for _, p := range b {
		counts[p]--
		if counts[p] < 0 {
			return false
		}
	}
	return true
}

// runD1 executes `sqloid d1` through Main with the production D1 handler
// composition while capturing both streams, mirroring runStartup in
// startup_test.go.
func runD1(t *testing.T) (stdout, stderr string, status int) {
	savedOut, savedErr := os.Stdout, os.Stderr
	defer func() { os.Stdout, os.Stderr = savedOut, savedErr }()
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = outW, errW

	status = Main([]string{"sqloid", "d1"}, Handlers{D1: RunD1})

	outW.Close()
	errW.Close()

	var outBuf, errBuf bytes.Buffer
	io.Copy(&outBuf, outR)
	io.Copy(&errBuf, errR)
	return outBuf.String(), errBuf.String(), status
}
