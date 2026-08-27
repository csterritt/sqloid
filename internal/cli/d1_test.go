package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

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
