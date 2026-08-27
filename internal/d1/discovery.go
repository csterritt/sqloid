// Package d1 discovers local Wrangler D1 database candidates for Issue #3
// and the D1 discovery section of Notes/PRD-sqloid.md. It is a pure
// filesystem scan: it never opens SQLite, validates headers, or owns any
// connection; the sole candidate path is handed to internal/cli and then to
// internal/connection unchanged.
package d1

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Dir is the exact working-directory-relative directory inspected by
// Discover. Per the PRD there is deliberately no recursive or alternate-
// layout search: an absent directory simply yields no candidates.
const Dir = ".wrangler/state/v3/d1/miniflare-D1DatabaseObject"

// ErrNoCandidate reports that the Wrangler D1 directory is absent or contains
// no eligible candidate. internal/cli renders the diagnostic; this package
// carries only the typed outcome.
var ErrNoCandidate = errors.New("no candidate database found in .wrangler")

// ErrMultipleCandidates reports that more than one eligible candidate exists
// in the Wrangler D1 directory, so exactly one cannot be selected.
var ErrMultipleCandidates = errors.New("there is more than one SQLite database in .wrangler")

// Discover inspects only the immediate entries of Dir relative to the process
// working directory and applies the exact PRD candidate rules:
//
//   - a case-sensitive ".sqlite" filename extension,
//   - exclusion of names containing the lowercase substring "metadata",
//   - exclusion of "-wal"/"-shm" sidecar names.
//
// Exactly one eligible candidate returns its joined path unchanged from the
// scan. Zero candidates return ErrNoCandidate and multiple candidates return
// ErrMultipleCandidates, both with an empty path, as typed outcomes for
// internal/cli; diagnostics for them are defined by Issue #4.
func Discover() (string, error) {
	entries, err := os.ReadDir(Dir)
	if err != nil {
		return "", ErrNoCandidate
	}

	var candidates []string
	for _, entry := range entries {
		if eligible(entry.Name()) {
			candidates = append(candidates, entry.Name())
		}
	}

	switch len(candidates) {
	case 0:
		return "", ErrNoCandidate
	case 1:
		return filepath.Join(Dir, candidates[0]), nil
	default:
		return "", ErrMultipleCandidates
	}
}

// eligible reports whether name satisfies every candidate rule in one place:
// exact case-sensitive ".sqlite" suffix, no lowercase "metadata" substring,
// and not a "-wal"/"-shm" sidecar of an adjacent database file.
func eligible(name string) bool {
	if strings.Contains(name, "metadata") {
		return false
	}
	if strings.HasSuffix(name, "-wal") || strings.HasSuffix(name, "-shm") {
		return false
	}
	return strings.HasSuffix(name, ".sqlite")
}
