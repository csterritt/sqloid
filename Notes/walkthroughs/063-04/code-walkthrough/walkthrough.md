# Issue #063 Code Walkthrough: Actionable Cause for Atomic-Save Short Writes

*2026-09-01T14:50:32Z by Showboat 0.6.1*
<!-- showboat-id: 91029f89-494b-4f43-8e91-3bfec60de2c6 -->

Issue #63 (Notes/tasks/063-report-short-write-cause.md, Notes/PRD-sqloid.md §Atomic saves, §Export Module Design, §Testing Decisions; user story 72) gives an atomic-save nil-error short write an actionable cause. Before this issue, when the single Write call in WriteAtomic returned (n, nil) with n < len(payload) — a short write reporting no error — the boundary constructed a StageError with a nil cause, producing the unactionable text 'save failed at write: <nil>' with no errors.Is match. Issue #63 converts that absent cause to io.ErrShortWrite before constructing the StageError, so the cause matches io.ErrShortWrite through errors.Is, the stage-qualified text names the short write ('save failed at write: short write'), and the existing pre-rename cleanup (best-effort temp close and removal) runs exactly as for a non-nil writer error. A non-nil writer error stays the cause unchanged; the short-write conversion applies only when the writer reported no error. Sync, the final close-as-success, and rename never run for either write-failure case; the existing destination is preserved byte-for-byte; a previously missing destination remains absent. No retry, no partial output exposure, no destination touch, and no UI retry/cancel identity change. Issue #64 builds on this persistence boundary and must preserve the demonstrated behavior. This walkthrough deterministically injects (n < len(payload), nil) through the Issue #53 fake filesystem and displays the StageWrite error, errors.Is(io.ErrShortWrite), actionable UI text, call ordering, absence of sync/rename, destination preservation, and temporary-file cleanup. It contrasts a non-nil write error and a complete successful write, then shows retry/cancel retaining the immutable save capture.

```bash
sed -n '183,220p' internal/export/save_write.go
```

```output
// never claimed. Success is reported only after the rename succeeded.
func WriteAtomic(fs SaveFS, path string, data []byte) error {
	dir, base := filepath.Dir(path), filepath.Base(path)
	f, err := fs.TempFile(dir, TempPattern(base))
	if err != nil {
		return &StageError{Stage: StageCreate, Err: err}
	}
	tmpName := fs.Name(f)
	cleanup := func() error {
		// Best-effort close for stages that leave the file open; the close
		// stage itself never closes twice.
		_ = f.Close()
		return fs.Remove(tmpName)
	}
	if n, err := f.Write(data); err != nil || n != len(data) {
		// Issue #63: a nil error with a short byte count is a failed write,
		// not a silent partial save. Convert it to io.ErrShortWrite so the
		// cause is actionable through errors.Is and the stage-qualified text
		// names the short write rather than <nil>. A non-nil writer error
		// stays the cause unchanged.
		cause := err
		if cause == nil {
			cause = io.ErrShortWrite
		}
		_ = cleanup()
		return &StageError{Stage: StageWrite, Err: cause}
	}
	if err := f.Sync(); err != nil {
		_ = cleanup()
		return &StageError{Stage: StageSync, Err: err}
	}
	if err := f.Close(); err != nil {
		_ = fs.Remove(tmpName)
		return &StageError{Stage: StageClose, Err: err}
	}
	if err := fs.Rename(tmpName, path); err != nil {
		_ = fs.Remove(tmpName)
		return &StageError{Stage: StageRename, Err: err}
```

The single-write boundary is the only change. When Write returns (n, nil) with n < len(data), cause is set to io.ErrShortWrite; when Write returns a non-nil error, that error stays the cause. Both cases route through the same cleanup() — best-effort Close plus Remove — before any Sync or Rename. The StageWrite attribution is unchanged from Issue #53.

```bash
sed -n '327,440p' internal/export/save_write_test.go
```

```output

// hasCall reports whether any recorded boundary call starts with prefix.
func (f *saveFakeFS) hasCall(prefix string) bool {
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

// TestWriteAtomicShortWriteNilErrorIsErrShortWrite requires Issue #63's
// nil-error short write to surface as a typed write-stage error whose cause
// is io.ErrShortWrite with an actionable message rather than <nil>. Sync,
// the final close-as-success, and rename never run; the partially written
// destination-local temporary file is closed and removed; an existing
// destination remains byte-for-byte unchanged; a previously missing
// destination remains absent. Complete-write and non-nil-error rows are
// retained as controls to prove the short-write boundary is distinct.
func TestWriteAtomicShortWriteNilErrorIsErrShortWrite(t *testing.T) {
	data := []byte("payload")
	cases := []struct {
		name            string
		setup           func(f *saveFakeFS)
		existing        bool
		wantSuccess     bool
		wantErrIs       error
		wantErrNotIs    error
		wantErrContains string
	}{
		{
			name:            "short-write-nil-error-existing-destination",
			setup:           func(f *saveFakeFS) { f.shortWriteNilAfter = 3 },
			existing:        true,
			wantErrIs:       io.ErrShortWrite,
			wantErrContains: "short write",
		},
		{
			name:            "short-write-nil-error-missing-destination",
			setup:           func(f *saveFakeFS) { f.shortWriteNilAfter = 3 },
			existing:        false,
			wantErrIs:       io.ErrShortWrite,
			wantErrContains: "short write",
		},
		{
			name:        "complete-write-control",
			setup:       func(f *saveFakeFS) {},
			existing:    true,
			wantSuccess: true,
		},
		{
			name:         "non-nil-write-error-control",
			setup:        func(f *saveFakeFS) { f.failWriteAfter = 3 },
			existing:     true,
			wantErrNotIs: io.ErrShortWrite,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSaveFakeFS()
			if tc.existing {
				f.exists["/work/out.csv"] = true
				f.contents["/work/out.csv"] = []byte("original bytes")
			}
			tc.setup(f)
			err := WriteAtomic(f, "/work/out.csv", data)
			if tc.wantSuccess {
				if err != nil {
					t.Fatalf("complete write failed: %v", err)
				}
				if got := f.contents["/work/out.csv"]; !bytes.Equal(got, data) {
					t.Fatalf("destination bytes = %q, want %q", got, data)
				}
				return
			}
			var stageErr *StageError
			if !errors.As(err, &stageErr) || stageErr.Stage != StageWrite {
				t.Fatalf("err = %v, want StageError(StageWrite)", err)
			}
			if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
				t.Fatalf("errors.Is(err, %v) = false, want true; err = %v", tc.wantErrIs, err)
			}
			if tc.wantErrNotIs != nil && errors.Is(err, tc.wantErrNotIs) {
				t.Fatalf("errors.Is(err, %v) = true, want false; err = %v", tc.wantErrNotIs, err)
			}
			if tc.wantErrContains != "" {
				if msg := err.Error(); !strings.Contains(msg, tc.wantErrContains) {
					t.Fatalf("err text = %q, want it to contain %q", msg, tc.wantErrContains)
				}
				if msg := err.Error(); strings.Contains(msg, "<nil>") {
					t.Fatalf("err text contains <nil> rather than an actionable cause: %q", msg)
				}
			}
			// Sync, the final close-as-success, and rename never run for a
			// write-stage failure.
			if f.hasCall("sync:") {
				t.Fatalf("sync ran after a write-stage failure: %q", f.calls)
			}
			if f.hasCall("rename:") {
				t.Fatalf("rename ran after a write-stage failure: %q", f.calls)
			}
			// The partially written destination-local temporary file is
			// closed and removed.
			tempName := f.createdTempName()
			if tempName == "" {
				t.Fatal("no temporary artifact was created")
			}
			if _, ok := f.contents[tempName]; ok {
				t.Fatalf("temporary artifact %q was not removed", tempName)
			}
			removes := 0
			for _, c := range f.calls {
				if strings.HasPrefix(c, "remove:") {
					removes++
```

Running the export test proves the short-write nil-error case returns a StageWrite error whose cause matches io.ErrShortWrite with actionable text, while the complete-write and non-nil-error control rows behave unchanged.

```bash
go test ./internal/export/ -run '^TestWriteAtomicShortWriteNilErrorIsErrShortWrite$' -v -count=1
```

```output
=== RUN   TestWriteAtomicShortWriteNilErrorIsErrShortWrite
=== RUN   TestWriteAtomicShortWriteNilErrorIsErrShortWrite/short-write-nil-error-existing-destination
=== RUN   TestWriteAtomicShortWriteNilErrorIsErrShortWrite/short-write-nil-error-missing-destination
=== RUN   TestWriteAtomicShortWriteNilErrorIsErrShortWrite/complete-write-control
=== RUN   TestWriteAtomicShortWriteNilErrorIsErrShortWrite/non-nil-write-error-control
--- PASS: TestWriteAtomicShortWriteNilErrorIsErrShortWrite (0.00s)
    --- PASS: TestWriteAtomicShortWriteNilErrorIsErrShortWrite/short-write-nil-error-existing-destination (0.00s)
    --- PASS: TestWriteAtomicShortWriteNilErrorIsErrShortWrite/short-write-nil-error-missing-destination (0.00s)
    --- PASS: TestWriteAtomicShortWriteNilErrorIsErrShortWrite/complete-write-control (0.00s)
    --- PASS: TestWriteAtomicShortWriteNilErrorIsErrShortWrite/non-nil-write-error-control (0.00s)
PASS
ok  	github.com/chris/sqloid/internal/export	0.002s
```

A deterministic demonstration program injects the three write outcomes through the Issue #53 fake filesystem and prints the StageError, errors.Is(io.ErrShortWrite), the full boundary call ordering, whether sync/rename ran, the destination contents, and temp cleanup — for the nil-error short write, a non-nil write error, and a complete successful write.

```bash
go run Notes/walkthroughs/063-04/code-walkthrough/demo.go
```

```output
=== nil-error short write (n=3, nil) — existing destination ===
  error:              save failed at write: short write
  errors.Is(ErrShortWrite): true
  StageError.Stage:   write
  boundary calls:     ["create:/work" "tempname:/work/.temp-1" "write:/work/.temp-1:7" "close:/work/.temp-1" "remove:/work/.temp-1"]
  sync ran:           false
  rename ran:         false
  temp created:       /work/.temp-1
  temp removed:       true
  destination bytes:  "original bytes" (preserved=true)

=== nil-error short write (n=3, nil) — missing destination ===
  error:              save failed at write: short write
  errors.Is(ErrShortWrite): true
  StageError.Stage:   write
  boundary calls:     ["create:/work" "tempname:/work/.temp-1" "write:/work/.temp-1:7" "close:/work/.temp-1" "remove:/work/.temp-1"]
  sync ran:           false
  rename ran:         false
  temp created:       /work/.temp-1
  temp removed:       true
  destination absent: true

=== non-nil write error (n=3, error) — existing destination ===
  error:              save failed at write: injected write failure
  errors.Is(ErrShortWrite): false
  StageError.Stage:   write
  boundary calls:     ["create:/work" "tempname:/work/.temp-1" "write:/work/.temp-1:7" "close:/work/.temp-1" "remove:/work/.temp-1"]
  sync ran:           false
  rename ran:         false
  temp created:       /work/.temp-1
  temp removed:       true
  destination bytes:  "original bytes" (preserved=true)

=== complete write (n=len, nil) — existing destination ===
  error:              <nil>
  errors.Is(ErrShortWrite): false
  boundary calls:     ["create:/work" "tempname:/work/.temp-1" "write:/work/.temp-1:7" "sync:/work/.temp-1" "close:/work/.temp-1" "rename:/work/.temp-1->/work/out.csv"]
  sync ran:           true
  rename ran:         true
  temp created:       /work/.temp-1
  temp removed:       true
  destination bytes:  "payload" (preserved=false)

```

The three cases are clearly distinct:
1. nil-error short write (n=3, nil): error text is 'save failed at write: short write' (not '<nil>'), errors.Is(io.ErrShortWrite) is true, StageWrite, no sync/rename, temp closed and removed, existing destination preserved byte-for-byte, missing destination remains absent.
2. non-nil write error (n=3, error): error text carries the injected writer error, errors.Is(io.ErrShortWrite) is false, same StageWrite and same cleanup — the short-write conversion does not apply.
3. complete write (n=len, nil): no error, sync and rename both run, destination receives the exact payload — the short-write boundary does not affect complete writes.

The call ordering for both write-failure cases is identical: create → tempname → write → close (cleanup) → remove. Sync and rename are absent. The complete-write case adds sync → close (success) → rename.

The UI test drives the same nil-error short write through the full Bubble Tea save flow: destination inspection, overwrite confirmation, write command, and inline failure with retry/cancel — proving the typed failure surfaces with the captured destination and payload intact.

```bash
sed -n '495,560p' internal/ui/save_write_test.go
```

```output
// retry/cancel behavior — never a success message. The displayed failure
// text names the actionable short-write cause rather than <nil>.
func TestSaveShortWriteNilErrorShowsInlineFailureWithErrShortWrite(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{}
	s := newSaveFlowFakeFS()
	s.existing["/work/r.sql"] = true
	s.contents["/work/r.sql"] = []byte("SELECT 1;")
	s.shortWriteNilAfter = 3
	_, p := savePickerFlowModel(t, f, s)
	baseline := openerFingerprint(p)
	reached := submitDestination(t, p, "r")
	failed := confirmSave(t, reached)
	if failed.overwriteOpen {
		t.Fatal("failure left the confirmation open")
	}
	if failed.saveFailure == "" {
		t.Fatal("short write did not show an inline failure")
	}
	if !strings.Contains(failed.saveFailure, "short write") {
		t.Fatalf("inline failure text = %q, want it to name the short-write cause", failed.saveFailure)
	}
	if strings.Contains(failed.saveFailure, "<nil>") {
		t.Fatalf("inline failure text contains <nil> rather than an actionable cause: %q", failed.saveFailure)
	}
	if failed.saveCompletedPath != "" {
		t.Fatal("short-write failure claimed a completed destination")
	}
	// The captured destination and payload are retained for retry.
	if failed.saveCapture == nil || failed.saveCapture.path != "/work/r.sql" {
		t.Fatal("captured destination was discarded after short-write failure")
	}
	want := "SELECT * FROM \"items\";"
	if string(failed.saveCapture.payload) != want {
		t.Fatalf("captured payload = %q, want %q", failed.saveCapture.payload, want)
	}
	// The existing destination is byte-for-byte preserved.
	if got := s.contents["/work/r.sql"]; string(got) != "SELECT 1;" {
		t.Fatalf("existing destination changed: %q", got)
	}
	// No leaked temporary artifact.
	if name := s.createdTempName(); name != "" {
		if _, ok := s.contents[name]; ok {
			t.Fatalf("temporary artifact %q not cleaned up", name)
		}
	}
	// Retry clears the injected boundary and writes the same captured copy.
	s.shortWriteNilAfter = -1
	retried, cmd := pressKey(failed, tea.KeyMsg{Type: tea.KeyEnter})
	final, _ := runSaveCmds(t, retried, cmd)
	if got := s.contents["/work/r.sql"]; string(got) != want {
		t.Fatalf("retry bytes = %q, want the same captured copy %q", got, want)
	}
	if final.saveCompletedPath != "/work/r.sql" {
		t.Fatal("successful retry did not complete")
	}
	if fp := openerFingerprint(final); fp != baseline {
		t.Errorf("retry completion fingerprint drifted:\n%s\nvs\n%s", fp, baseline)
	}
}

func TestSaveFailureCancelRestoresOpenerExactly(t *testing.T) {
	f := pickerNewFakeFS()
	f.dirs["/work"] = []pickerFakeEntry{}
	s := newSaveFlowFakeFS()
	s.failRename = errors.New("busy")
```

The UI test drives the same nil-error short write through the full Bubble Tea save flow: destination inspection, overwrite confirmation, write command, and inline failure with retry/cancel — proving the typed failure surfaces with the captured destination and payload intact.

```bash
go test ./internal/ui/ -run '^TestSaveShortWriteNilErrorShowsInlineFailureWithErrShortWrite$' -v -count=1 2>&1 | sed 's/[0-9]\+\.[0-9]\+s/TIME/'
```

```output
=== RUN   TestSaveShortWriteNilErrorShowsInlineFailureWithErrShortWrite
--- PASS: TestSaveShortWriteNilErrorShowsInlineFailureWithErrShortWrite (TIME)
PASS
ok  	github.com/chris/sqloid/internal/ui	TIME
```

The UI test proves the inline failure retains the immutable save capture (destination '/work/r.sql' and payload 'SELECT * FROM "items";') so Enter/y retry writes the same captured copy after the injected boundary is cleared, and the existing destination is preserved byte-for-byte throughout. The failure text contains 'short write' (not '<nil>'), no success message is shown, and the retry completion fingerprint matches the baseline opener. Issue #64 builds on this persistence boundary and must preserve the demonstrated short-write detection, io.ErrShortWrite cause, StageWrite attribution, pre-rename cleanup, destination preservation, and retry/cancel behavior. Cross-references: Issues #53, #63, and #64; Notes/PRD-sqloid.md §Atomic saves, §Export Module Design, §Testing Decisions; user story 72.
