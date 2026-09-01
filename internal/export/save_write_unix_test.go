//go:build unix

// Real-filesystem race-safe save tests for Issue #64 on Linux/macOS. The
// fake-filesystem tests in save_write_test.go cover the portable contract;
// these tests exercise the real OSSaveFS with temporary files and
// deterministic barriers (no sleeps) between inspection and persistence:
// an unconfirmed save raced by external creation preserves the raced file;
// a confirmed replacement raced by external replacement or removal
// preserves the changed file; unchanged confirmed state replaces
// atomically. Issue #63 short-write behavior is retained on both paths.

package export

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestOSSaveFSNoReplaceRacedByExternalCreation inspects a missing
// destination, creates it externally before persistence, and requires the
// no-replace path to preserve the raced file and return a typed
// DestinationExistsError.
func TestOSSaveFSNoReplaceRacedByExternalCreation(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "out.csv")
	fs := OSSaveFS{}

	state, err := InspectDestination(fs, dst)
	if err != nil || state.Status != DestinationNew {
		t.Fatalf("inspect = %v err %v, want DestinationNew", state, err)
	}

	// Deterministic barrier: create the destination externally.
	raced := []byte("external content")
	if err := os.WriteFile(dst, raced, 0o644); err != nil {
		t.Fatalf("external create failed: %v", err)
	}

	err = WriteAtomic(fs, dst, []byte("payload"), state, IntentNoReplace)
	var existsErr *DestinationExistsError
	if !errors.As(err, &existsErr) {
		t.Fatalf("err = %v, want *DestinationExistsError", err)
	}
	// The raced file is preserved byte-for-byte.
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("raced file unreadable: %v", err)
	}
	if !bytes.Equal(got, raced) {
		t.Fatalf("raced file changed: %q, want %q", got, raced)
	}
}

// TestOSSaveFSNoReplaceSuccess inspects a missing destination and requires
// the no-replace path to create it atomically with the exact captured
// bytes.
func TestOSSaveFSNoReplaceSuccess(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "out.csv")
	fs := OSSaveFS{}

	state, err := InspectDestination(fs, dst)
	if err != nil || state.Status != DestinationNew {
		t.Fatalf("inspect = %v err %v, want DestinationNew", state, err)
	}

	data := []byte("exact captured bytes\r\n")
	if err := WriteAtomic(fs, dst, data, state, IntentNoReplace); err != nil {
		t.Fatalf("no-replace save failed: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("destination unreadable: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("destination bytes = %q, want %q", got, data)
	}
}

// TestOSSaveFSReplaceRacedByExternalReplacement inspects an existing
// destination, replaces it externally before persistence, and requires the
// replace path to preserve the changed file and return a typed
// DestinationChangedError.
func TestOSSaveFSReplaceRacedByExternalReplacement(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "out.csv")
	fs := OSSaveFS{}

	original := []byte("original content")
	if err := os.WriteFile(dst, original, 0o644); err != nil {
		t.Fatalf("setup write failed: %v", err)
	}
	state, err := InspectDestination(fs, dst)
	if err != nil || state.Status != DestinationExisting {
		t.Fatalf("inspect = %v err %v, want DestinationExisting", state, err)
	}

	// Deterministic barrier: replace the destination externally.
	changed := []byte("external replacement content")
	if err := os.WriteFile(dst, changed, 0o644); err != nil {
		t.Fatalf("external replace failed: %v", err)
	}

	err = WriteAtomic(fs, dst, []byte("payload"), state, IntentReplace)
	var changedErr *DestinationChangedError
	if !errors.As(err, &changedErr) {
		t.Fatalf("err = %v, want *DestinationChangedError", err)
	}
	// The changed file is preserved byte-for-byte.
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("changed file unreadable: %v", err)
	}
	if !bytes.Equal(got, changed) {
		t.Fatalf("changed file overwritten: %q, want %q", got, changed)
	}
	// No temp file leaked.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("destination dir unreadable: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("dir has %d entries, want 1 (no temp leak)", len(entries))
	}
}

// TestOSSaveFSReplaceRacedByExternalRemoval inspects an existing
// destination, removes it externally before persistence, and requires the
// replace path to return a typed DestinationChangedError with Missing=true.
func TestOSSaveFSReplaceRacedByExternalRemoval(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "out.csv")
	fs := OSSaveFS{}

	original := []byte("original content")
	if err := os.WriteFile(dst, original, 0o644); err != nil {
		t.Fatalf("setup write failed: %v", err)
	}
	state, err := InspectDestination(fs, dst)
	if err != nil || state.Status != DestinationExisting {
		t.Fatalf("inspect = %v err %v, want DestinationExisting", state, err)
	}

	// Deterministic barrier: remove the destination externally.
	if err := os.Remove(dst); err != nil {
		t.Fatalf("external remove failed: %v", err)
	}

	err = WriteAtomic(fs, dst, []byte("payload"), state, IntentReplace)
	var changedErr *DestinationChangedError
	if !errors.As(err, &changedErr) {
		t.Fatalf("err = %v, want *DestinationChangedError", err)
	}
	if !changedErr.Missing {
		t.Fatal("changed error does not report missing")
	}
	// No temp file leaked.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("destination dir unreadable: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("dir has %d entries, want 0 (no temp leak)", len(entries))
	}
}

// TestOSSaveFSReplaceUnchangedStateSucceeds inspects an existing
// destination and requires the replace path to replace it atomically when
// the state remains unchanged.
func TestOSSaveFSReplaceUnchangedStateSucceeds(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "out.csv")
	fs := OSSaveFS{}

	original := []byte("original content")
	if err := os.WriteFile(dst, original, 0o644); err != nil {
		t.Fatalf("setup write failed: %v", err)
	}
	state, err := InspectDestination(fs, dst)
	if err != nil || state.Status != DestinationExisting {
		t.Fatalf("inspect = %v err %v, want DestinationExisting", state, err)
	}

	data := []byte("new payload content")
	if err := WriteAtomic(fs, dst, data, state, IntentReplace); err != nil {
		t.Fatalf("unchanged replacement failed: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("destination unreadable: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("replacement bytes = %q, want %q", got, data)
	}
	// No temp file leaked.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("destination dir unreadable: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("dir has %d entries, want 1 (no temp leak)", len(entries))
	}
}

// TestOSSaveFSNoReplaceAndReplaceStagingInDestinationDirectory requires
// both paths to stage in the destination directory: the no-replace path
// creates the destination itself, and the replace path creates a temp file
// in the destination's own directory.
func TestOSSaveFSNoReplaceAndReplaceStagingInDestinationDirectory(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	fs := OSSaveFS{}

	// No-replace: destination created in the destination directory.
	dstNew := filepath.Join(sub, "new.csv")
	state, _ := InspectDestination(fs, dstNew)
	if err := WriteAtomic(fs, dstNew, []byte("new"), state, IntentNoReplace); err != nil {
		t.Fatalf("no-replace failed: %v", err)
	}
	if _, err := os.Stat(dstNew); err != nil {
		t.Fatalf("no-replace destination not in destination dir: %v", err)
	}

	// Replace: temp file created in the destination directory.
	dstExisting := filepath.Join(sub, "existing.csv")
	if err := os.WriteFile(dstExisting, []byte("old"), 0o644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	state, _ = InspectDestination(fs, dstExisting)
	if err := WriteAtomic(fs, dstExisting, []byte("new"), state, IntentReplace); err != nil {
		t.Fatalf("replace failed: %v", err)
	}
	got, err := os.ReadFile(dstExisting)
	if err != nil {
		t.Fatalf("replace destination unreadable: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("replace bytes = %q, want %q", got, "new")
	}
	// No temp leaked in the destination directory.
	entries, err := os.ReadDir(sub)
	if err != nil {
		t.Fatalf("destination dir unreadable: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("destination dir has %d entries, want 2 (no temp leak)", len(entries))
	}
}
