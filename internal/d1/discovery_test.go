package d1

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// wranglerDir is the exact working-directory-relative directory that
// Discovery searches, mirrored from discovery.go so a typo in either fails.
const wranglerDir = ".wrangler/state/v3/d1/miniflare-D1DatabaseObject"

// writeCandidate creates an empty file at dir/name; eligibility is purely a
// name-based filesystem scan, so the content never matters here.
func writeCandidate(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

// setup switches the test into cwd and writes files under it relative to the
// given root-relative names (empty string entries create nothing).
func chdirInto(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	return dir
}

func TestDiscoverSoleCandidate(t *testing.T) {
	chdirInto(t)
	writeCandidate(t, wranglerDir, "abc123.sqlite")

	got, err := Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v, want sole candidate", err)
	}
	want := filepath.Join(wranglerDir, "abc123.sqlite")
	if got != want {
		t.Errorf("Discover() = %q, want %q", got, want)
	}
}

func TestDiscoverZeroCandidates(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T)
	}{
		{
			name:  "directory absent entirely",
			setup: func(t *testing.T) {},
		},
		{
			name: "directory exists but empty",
			setup: func(t *testing.T) {
				writeCandidate(t, wranglerDir, ".keep")
				if err := os.Remove(filepath.Join(wranglerDir, ".keep")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "only metadata file",
			setup: func(t *testing.T) {
				writeCandidate(t, wranglerDir, "state-metadata.sqlite")
			},
		},
		{
			name: "only sidecar files",
			setup: func(t *testing.T) {
				writeCandidate(t, wranglerDir, "abc123.sqlite-wal")
				writeCandidate(t, wranglerDir, "abc123.sqlite-shm")
			},
		},
		{
			name: "only wrong-case extension",
			setup: func(t *testing.T) {
				writeCandidate(t, wranglerDir, "ABC.SQLITE")
				writeCandidate(t, wranglerDir, "abc.SQLite")
			},
		},
		{
			name: "only nested sqlite",
			setup: func(t *testing.T) {
				writeCandidate(t, filepath.Join(wranglerDir, "nested"), "deep.sqlite")
			},
		},
		{
			name: "alternate layout directory only",
			setup: func(t *testing.T) {
				writeCandidate(t, ".wrangler/state/v3/wrangler-state/v2/d1/miniflare-D1DatabaseObject", "alt.sqlite")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chdirInto(t)
			tt.setup(t)

			path, err := Discover()
			if !errors.Is(err, ErrNoCandidate) {
				t.Errorf("Discover() error = %v, want ErrNoCandidate", err)
			}
			if path != "" {
				t.Errorf("Discover() path = %q, want empty on zero candidates", path)
			}
		})
	}
}

func TestDiscoverMultipleCandidates(t *testing.T) {
	tests := []struct {
		name  string
		names []string
	}{
		{name: "two plain candidates", names: []string{"a.sqlite", "b.sqlite"}},
		{name: "uppercase Metadata does not match lowercase rule", names: []string{"a.sqlite", "B-Metadata.sqlite"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chdirInto(t)
			for _, n := range tt.names {
				writeCandidate(t, wranglerDir, n)
			}

			path, err := Discover()
			if !errors.Is(err, ErrMultipleCandidates) {
				t.Errorf("Discover() error = %v, want ErrMultipleCandidates", err)
			}
			if path != "" {
				t.Errorf("Discover() path = %q, want empty on multiple candidates", path)
			}
		})
	}
}

func TestDiscoverExclusionsStillLeaveSoleCandidate(t *testing.T) {
	tests := []struct {
		name      string
		ignored   []string // excluded by metadata/sidecar/nested/alternate rules
		candidate string   // the single eligible name
	}{
		{
			name:      "metadata and sidecars ignored",
			ignored:   []string{"db-metadata.sqlite", "db-METADATA-marker", "x.sqlite-wal", "x.sqlite-shm"},
			candidate: "real.sqlite",
		},
		{
			name:      "uppercase metadata substring is not the lowercase exclusion",
			ignored:   []string{"y-metadata-y.sqlite"},
			candidate: "MetadataOnly.sqlite",
		},
		{
			name:      "nested ignored while top-level survives",
			ignored:   []string{},
			candidate: "top.sqlite",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := chdirInto(t)
			writeCandidate(t, wranglerDir, tt.candidate)
			writeCandidate(t, filepath.Join(wranglerDir, "nested"), "deep.sqlite")
			writeCandidate(t, ".wrangler/state/v3/d1/miniflare-D1DatabaseObject-alternate", "alt-layout.sqlite")
			_ = dir

			got, err := Discover()
			if err != nil {
				t.Fatalf("Discover() error = %v, want %q", err, tt.candidate)
			}
			want := filepath.Join(wranglerDir, tt.candidate)
			if got != want {
				t.Errorf("Discover() = %q, want %q", got, want)
			}
		})
	}
}
