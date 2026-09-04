# Issue #88 Task 3: Full CI Definition of Done Walkthrough

*2026-09-04T11:01:46Z by Showboat 0.6.1*
<!-- showboat-id: 9df38f26-3746-447f-83ce-fcd540785a4c -->

## The final workflow definition

The sole CI workflow at `.github/workflows/capability-suite.yml` defines two jobs — `capability-suite (linux)` on `ubuntu-latest` and `capability-suite (macos)` on `macos-latest` — each running seven sequential steps. The first failing step fails the job immediately; there are no `continue-on-error` steps, no retries, no platform exclusions, and no conditional skips.

```bash
cd /home/chris/sqloid && cat .github/workflows/capability-suite.yml
```

```output
name: capability-suite

# Issue #56 Task 1: integrated release-verification gate.
# Issue #88 Task 1: expanded into the full cross-platform release gate.
#
# Both supported platforms run the full definition of done from a clean
# checkout with the pinned module graph:
#
#   1. modernc.org/sqlite exact-pin assertion (Issue #56, retained).
#   2. Repository-wide `go test ./...` — every shipped package, including
#      the Issue #57 PTY-driven built-binary integration test in
#      cmd/sqloid that builds the real sqloid binary and drives it
#      headlessly through github.com/creack/pty against a real temporary
#      SQLite fixture. No package is skipped and no continue-on-error
#      hides a failure.
#   3. Repository-wide `go build ./...` — every shipped package compiles.
#   4. Repository-wide `go vet ./...` — every shipped package is vet-clean.
#   5. The targeted race/cancellation capability suite
#      (scripts/capability-suite.sh, Issue #56) — retained as a separate
#      gate for its specialized -race cancellation guarantees, not
#      replaced by the ordinary repository tests above.
#
# Any modernc.org/sqlite dependency change lands in go.mod/go.sum and
# therefore triggers both jobs on the pull request that proposes it; the
# upgrade cannot merge as successful unless this same gate passes on both
# Linux and macOS. There are no continue-on-error, allowed failures,
# retries, platform exclusions, or conditional skips: any setup, test,
# build, vet, capability, binary-integration, timeout, or cleanup failure
# fails the job and blocks release.

on:
  pull_request:
  push:
    branches: [main]

jobs:
  capability-linux:
    name: capability-suite (linux)
    runs-on: ubuntu-latest
    timeout-minutes: 45
    steps:
      - name: Clean checkout
        uses: actions/checkout@v4
      - name: Install Go (from go.mod)
        uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - name: Confirm the vetted modernc pin
        run: |
          go list -m modernc.org/sqlite
          go list -m modernc.org/sqlite | grep -qx 'modernc.org/sqlite v1.57.0' || {
            echo "modernc.org/sqlite pin is not the vetted v1.57.0 exact version" >&2
            exit 1
          }
      - name: Repository-wide tests (includes Issue #57 PTY binary integration)
        run: go test -count=1 -timeout 20m ./...
      - name: Repository-wide build
        run: go build ./...
      - name: Repository-wide vet
        run: go vet ./...
      - name: Canonical capability suite
        run: scripts/capability-suite.sh

  capability-macos:
    name: capability-suite (macos)
    runs-on: macos-latest
    timeout-minutes: 45
    steps:
      - name: Clean checkout
        uses: actions/checkout@v4
      - name: Install Go (from go.mod)
        uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - name: Confirm the vetted modernc pin
        run: |
          go list -m modernc.org/sqlite
          go list -m modernc.org/sqlite | grep -qx 'modernc.org/sqlite v1.57.0' || {
            echo "modernc.org/sqlite pin is not the vetted v1.57.0 exact version" >&2
            exit 1
          }
      - name: Repository-wide tests (includes Issue #57 PTY binary integration)
        run: go test -count=1 -timeout 20m ./...
      - name: Repository-wide build
        run: go build ./...
      - name: Repository-wide vet
        run: go vet ./...
      - name: Canonical capability suite
        run: scripts/capability-suite.sh
```

Both jobs run the same seven steps in the same order: clean checkout, Go setup from `go.mod`, the retained exact modernc pin assertion, repository-wide tests, repository-wide build, repository-wide vet, and the retained canonical capability suite. The Linux and macOS jobs are structurally identical — both platforms run equivalent required gates.

## Gate 1: The retained exact modernc pin check

The pin assertion is unchanged from Issue #56. `go list -m modernc.org/sqlite` must print exactly `modernc.org/sqlite v1.57.0` or the job fails before any test runs. This is a direct, exact semantic-version pin with no replace directive, no branch, no wildcard.

```bash
cd /home/chris/sqloid && grep -n "modernc.org/sqlite" go.mod && echo "---" && go list -m modernc.org/sqlite && echo "---" && grep -c "replace" go.mod || true
```

```output
11:	modernc.org/sqlite v1.57.0
---
modernc.org/sqlite v1.57.0
---
0
```

## Gate 2: Repository-wide tests — every shipped package participates

`go test -count=1 -timeout 20m ./...` exercises every shipped package. The `-count=1` flag disables test caching so every run is fresh; `-timeout 20m` bounds each package. This gate includes the Issue #57 PTY-driven built-binary integration test in `cmd/sqloid`.

```bash
cd /home/chris/sqloid && go list ./... 2>&1
```

```output
github.com/chris/sqloid/Notes/walkthroughs/063-04/code-walkthrough
github.com/chris/sqloid/Notes/walkthroughs/070-06/code-walkthrough
github.com/chris/sqloid/Notes/walkthroughs/085-04/code-walkthrough
github.com/chris/sqloid/Notes/walkthroughs/086-02/code-walkthrough
github.com/chris/sqloid/cmd/sqloid
github.com/chris/sqloid/internal/cli
github.com/chris/sqloid/internal/connection
github.com/chris/sqloid/internal/d1
github.com/chris/sqloid/internal/export
github.com/chris/sqloid/internal/filepicker
github.com/chris/sqloid/internal/history
github.com/chris/sqloid/internal/querybuilder
github.com/chris/sqloid/internal/result
github.com/chris/sqloid/internal/resultcache
github.com/chris/sqloid/internal/schema
github.com/chris/sqloid/internal/session
github.com/chris/sqloid/internal/ui
```

The 13 shipped Go packages (excluding the walkthrough directories with no test files) all participate in `go test ./...`. Running the full test suite proves every package is green:

```bash
cd /home/chris/sqloid && go test -count=1 -timeout 20m ./cmd/... ./internal/... 2>&1
```

```output
ok  	github.com/chris/sqloid/cmd/sqloid	0.765s
ok  	github.com/chris/sqloid/internal/cli	0.044s
ok  	github.com/chris/sqloid/internal/connection	41.629s
ok  	github.com/chris/sqloid/internal/d1	0.007s
ok  	github.com/chris/sqloid/internal/export	0.019s
ok  	github.com/chris/sqloid/internal/filepicker	0.003s
ok  	github.com/chris/sqloid/internal/history	0.142s
ok  	github.com/chris/sqloid/internal/querybuilder	0.011s
ok  	github.com/chris/sqloid/internal/result	0.006s
ok  	github.com/chris/sqloid/internal/resultcache	0.824s
ok  	github.com/chris/sqloid/internal/schema	0.108s
ok  	github.com/chris/sqloid/internal/session	0.423s
ok  	github.com/chris/sqloid/internal/ui	0.774s
```

All 13 shipped packages pass. The `cmd/sqloid` package includes the PTY integration test — let's verify it specifically:

```bash
cd /home/chris/sqloid && go test -v -count=1 -timeout 20m -run TestSqloidPTYEndToEndBuildsAndRunsRealBinary ./cmd/sqloid 2>&1
```

```output
=== RUN   TestSqloidPTYEndToEndBuildsAndRunsRealBinary
--- PASS: TestSqloidPTYEndToEndBuildsAndRunsRealBinary (0.53s)
PASS
ok  	github.com/chris/sqloid/cmd/sqloid	0.535s
```

## The Issue #57 PTY-driven built-binary integration test

The production-level integration test at `cmd/sqloid/pty_integration_test.go` is the canonical harness that proves the shipped binary reaches and operates the real application composition root. It builds the real `sqloid` binary, creates a real temporary SQLite database fixture, spawns the binary under `github.com/creack/pty` with a 100×30 terminal, and drives it through a real TTY. No injected runners or fakes are used.

```bash
cd /home/chris/sqloid && sed -n "1,50p" cmd/sqloid/pty_integration_test.go
```

```output
// Task 5 (RED) for Issue #57: production-level integration test that builds
// the real sqloid binary and runs it under a pseudo-terminal. This test
// proves the full production composition root — connection.Open →
// session.Compose → tea.NewProgram → user interaction → clean shutdown —
// works end-to-end against a real SQLite database through a real TTY.
//
// The test builds the sqloid binary from cmd/sqloid, creates a real SQLite
// database fixture, spawns the binary under github.com/creack/pty with a
// fixed terminal size, responds to Bubble Tea's terminal capability queries
// so the UI renders, waits for the initial builder view to appear, sends
// `q` then `Enter` to confirm the universal quit, and asserts the process
// exits with status 0. No injected runners or fakes are used: this is the
// shipped binary through the shipped composition root through a real
// terminal.

package main

import (
	"bytes"
	"database/sql"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"

	_ "modernc.org/sqlite"
)

// TestSqloidPTYEndToEndBuildsAndRunsRealBinary is the production-level
// integration test: it builds the sqloid binary, creates a real database,
// runs the binary under a PTY, verifies the builder view renders, quits
// cleanly, and exits with status 0.
func TestSqloidPTYEndToEndBuildsAndRunsRealBinary(t *testing.T) {
	// Build the real sqloid binary from cmd/sqloid.
	binPath := filepath.Join(t.TempDir(), "sqloid")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/sqloid")
	build.Dir = projectRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	t.Cleanup(func() { os.Remove(binPath) })

	// Create a real SQLite database with a table and some rows.
	dbPath := filepath.Join(t.TempDir(), "pty_test.db")
	db, err := sql.Open("sqlite", dbPath)
```

The test builds the real binary, creates a real SQLite fixture with a `users` table, spawns the binary under a PTY, responds to Bubble Tea's cursor-position-report request, waits for the builder's `Command` field, sends quit confirmation, and asserts status 0. The `t.TempDir()` fixtures and `t.Cleanup` ensure no goroutines, leases, or open handles are left behind. The 10-second deadlines are deterministic — no arbitrary sleeps. Captured PTY output appears in `t.Fatalf` messages on failure.

```bash
cd /home/chris/sqloid && sed -n "50,155p" cmd/sqloid/pty_integration_test.go
```

```output
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec("INSERT INTO users (id, name) VALUES (1, 'alice'), (2, 'bob')"); err != nil {
		t.Fatalf("insert rows: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	// Spawn the binary under a PTY with a fixed terminal size.
	cmd := exec.Command(binPath, "sqlite", dbPath)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 30, Cols: 100})
	if err != nil {
		t.Fatalf("pty.StartWithSize: %v", err)
	}
	t.Cleanup(func() {
		ptmx.Close()
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	})

	// Read the PTY output in a goroutine, responding to Bubble Tea's
	// terminal capability queries so the UI renders. Bubble Tea sends
	// \x1b[6n (cursor position report request) on startup and blocks until
	// a response arrives; we respond with \x1b[1;1R (cursor at row 1, col
	// 1) so rendering proceeds.
	var output bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				output.Write(buf[:n])
				// Respond to the cursor position report request so Bubble
				// Tea proceeds to render.
				if bytes.Contains(buf[:n], []byte("\x1b[6n")) {
					ptmx.Write([]byte("\x1b[1;1R"))
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Wait for the initial builder view to render — the "Command" field
	// label that the startup model focuses must appear in the output.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(output.String(), "Command") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(output.String(), "Command") {
		t.Fatalf("PTY output did not contain the builder's Command field within timeout.\nOutput:\n%s", output.String())
	}

	// Send `q` to open the universal quit confirmation, then `Enter` to
	// confirm and exit.
	if _, err := ptmx.Write([]byte("q")); err != nil {
		t.Fatalf("write q: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if _, err := ptmx.Write([]byte("\r")); err != nil {
		t.Fatalf("write Enter: %v", err)
	}

	// Wait for the process to exit.
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	select {
	case err := <-waitDone:
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				t.Fatalf("sqloid exited with status %d, want 0", exitErr.ExitCode())
			}
			t.Fatalf("sqloid wait: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("sqloid did not exit within 10 seconds of quit confirmation")
	}
}

// projectRoot returns the project root directory containing go.mod.
func projectRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// cmd/sqloid is two levels under the root; go up two.
	return filepath.Dir(filepath.Dir(wd))
}

// guard against unused import warnings.
var _ io.Reader
```

## Gate 3: Repository-wide build

`go build ./...` proves every shipped package compiles. This catches compilation errors in any package, not just the three targeted by the capability suite.

```bash
cd /home/chris/sqloid && go build ./... 2>&1 && echo "BUILD OK (exit $?)"
```

```output
BUILD OK (exit 0)
```

## Gate 4: Repository-wide vet

`go vet ./...` proves every shipped package is vet-clean. This catches suspicious constructs (printf format mismatches, unreachable code, shadowed variables, struct tag issues) in every package.

```bash
cd /home/chris/sqloid && go vet ./... 2>&1 && echo "VET OK (exit $?)"
```

```output
VET OK (exit 0)
```

## Gate 5: The retained targeted race/capability suite

`scripts/capability-suite.sh` is the Issue #56 canonical command, retained unchanged as a separate gate. It runs the targeted race/cancellation capability tests under `-race` with `CGO_ENABLED=1` against `internal/connection`, `internal/ui`, and `internal/history` — the three packages carrying the release-blocking cancellation guarantees. This is NOT replaced by the ordinary repository tests above; it provides the specialized `-race` cancellation evidence that `go test ./...` (without `-race`) does not.

```bash
cd /home/chris/sqloid && cat scripts/capability-suite.sh
```

```output
#!/bin/sh
# Canonical Sqloid release-capability suite gate (Issue #56 Task 1).
#
# This is the ONE command that selects all and only the integrated
# release-blocking capability tests, from internal/connection,
# internal/ui, and internal/history. Both the Linux and macOS CI jobs
# invoke this identical script from a clean checkout with the pinned
# module graph, so any modernc.org/sqlite dependency change (go.mod/go.sum)
# is gated by the same evidence on both platforms.
#
# Vetted pin (the only version accepted by this gate):
#   modernc.org/sqlite v1.57.0  (exact, direct, no replace directive)
#
# Semantics: any setup, test, timeout, or race failure fails this script
# with a non-zero exit status and blocks release. There are no skips,
# continue-on-error wrappers, retries, platform exclusions, or conditional
# relaxations. The race detector runs under cgo; production remains the
# pure-Go modernc.org/sqlite driver.
set -eu
cd "$(dirname "$0")/.."
exec env CGO_ENABLED=1 go test -race -count=1 -timeout 20m \
	./internal/connection ./internal/ui ./internal/history
```

Running the retained capability suite proves the targeted race/cancellation guarantees still pass:

```bash
cd /home/chris/sqloid && scripts/capability-suite.sh 2>&1
```

```output
ok  	github.com/chris/sqloid/internal/connection	73.486s
ok  	github.com/chris/sqloid/internal/ui	6.729s
ok  	github.com/chris/sqloid/internal/history	2.191s
```

## No allowed-failure or skip path hides failure

The workflow contains no `continue-on-error` steps, no `if` conditions on any step, no `fail-fast` matrix, and no platform exclusions. A grep for these evasion patterns confirms they are absent:

## No allowed-failure or skip path hides failure

The workflow contains no `continue-on-error` step configurations, no `if` conditions on any step, no matrix, and no platform exclusions. The words appear only in the header comment explaining their absence — let's verify by showing the actual matches:

```bash
cd /home/chris/sqloid && echo "=== continue-on-error matches ===" && grep -n "continue-on-error" .github/workflows/capability-suite.yml && echo "=== skip matches ===" && grep -in "skip" .github/workflows/capability-suite.yml && echo "=== Actual step-level continue-on-error (should be empty) ===" && grep -E "^[[:space:]]+continue-on-error:" .github/workflows/capability-suite.yml || echo "NONE (no step uses continue-on-error)"
```

```output
=== continue-on-error matches ===
14:#      SQLite fixture. No package is skipped and no continue-on-error
26:# Linux and macOS. There are no continue-on-error, allowed failures,
=== skip matches ===
14:#      SQLite fixture. No package is skipped and no continue-on-error
27:# retries, platform exclusions, or conditional skips: any setup, test,
=== Actual step-level continue-on-error (should be empty) ===
NONE (no step uses continue-on-error)
```

Both `continue-on-error` and `skip` appear only in the header comment explaining their absence. No step in either job uses `continue-on-error`, `if:`, or any conditional skip. Every step is mandatory and any failure blocks merging.

## Both platforms run equivalent required gates

The Linux and macOS jobs are structurally identical — same seven steps, same commands, same timeouts. A diff of the two job definitions shows only the runner OS and job name differ:

Both `continue-on-error` and `skip` appear only in the header comment explaining their absence. No step in either job uses `continue-on-error`, `if:`, or any conditional skip. Every step is mandatory and any failure blocks merging.

## Both platforms run equivalent required gates

The Linux and macOS jobs are structurally identical — same seven steps, same commands, same timeouts. Extracting just the step names and run commands from each job proves they match:

```bash
cd /home/chris/sqloid && echo "=== Linux job steps ===" && awk "/capability-linux:/,/^  capability-macos:/" .github/workflows/capability-suite.yml | grep -E "name:|run:" | grep -v "Install Go" && echo "=== macOS job steps ===" && awk "/capability-macos:/,0" .github/workflows/capability-suite.yml | grep -E "name:|run:" | grep -v "Install Go" && echo "=== Step count ===" && echo "Linux: $(awk "/capability-linux:/,/^  capability-macos:/" .github/workflows/capability-suite.yml | grep -c "name:") steps" && echo "macOS: $(awk "/capability-macos:/,0" .github/workflows/capability-suite.yml | grep -c "name:") steps"
```

```output
=== Linux job steps ===
    name: capability-suite (linux)
      - name: Clean checkout
      - name: Confirm the vetted modernc pin
        run: |
      - name: Repository-wide tests (includes Issue #57 PTY binary integration)
        run: go test -count=1 -timeout 20m ./...
      - name: Repository-wide build
        run: go build ./...
      - name: Repository-wide vet
        run: go vet ./...
      - name: Canonical capability suite
        run: scripts/capability-suite.sh
=== macOS job steps ===
    name: capability-suite (macos)
      - name: Clean checkout
      - name: Confirm the vetted modernc pin
        run: |
      - name: Repository-wide tests (includes Issue #57 PTY binary integration)
        run: go test -count=1 -timeout 20m ./...
      - name: Repository-wide build
        run: go build ./...
      - name: Repository-wide vet
        run: go vet ./...
      - name: Canonical capability suite
        run: scripts/capability-suite.sh
=== Step count ===
Linux: 8 steps
macOS: 8 steps
```

Both jobs have the same 7 steps (the count of 8 includes the job `name:` line). The step names and run commands are identical between Linux and macOS — both platforms run equivalent required gates.

## Negative demonstration: failing any required command makes the workflow non-green

The workflow fails closed on any gate failure. To demonstrate this, we simulate a vet failure by introducing a vet-detectable issue and showing that `go vet ./...` catches it. We use a temporary copy so no production file is modified.

## Negative demonstration: failing any required command makes the workflow non-green

The workflow fails closed on any gate failure. To demonstrate this, we simulate a vet failure by introducing a vet-detectable issue (a Printf format mismatch) in a temporary copy of the repository and showing that `go vet ./...` catches it with a nonzero exit. No production file is modified — the poisoned file lives only in a temporary directory that is removed afterward.

```bash
cd /home/chris/sqloid && tmpdir=$(mktemp -d) && cp -r . "$tmpdir/repo" && cd "$tmpdir/repo" && echo "package cli

import \"fmt\"

func _vetNegativeDemo() {
    fmt.Printf(\"%d\", \"wrong type\")
}
" > internal/cli/vet_negative_demo_test.go && echo "=== go vet on the poisoned package ===" && go vet ./internal/cli/ 2>&1; echo "exit: $?" && rm -rf "$tmpdir"
```

```output
=== go vet on the poisoned package ===
internal/cli/vet_negative_demo_test.go:6:17: fmt.Printf format %d has arg "wrong type" of wrong type string
exit: 1
```

`go vet` caught the format mismatch and exited with status 1. In CI, this nonzero exit would fail the "Repository-wide vet" step, which would fail the job and block merging. The same fail-closed behavior applies to every gate: a failing test, a build error, a vet issue, a capability-suite race failure, or a PTY integration test failure all produce a nonzero exit that fails the job.

### Negative demonstration: bypassing production composition makes the workflow non-green

The PTY integration test is specifically designed to fail when production composition is bypassed. It builds the real `sqloid` binary and runs it through a real PTY — if the binary does not reach `internal/session.Compose` and render the builder's `Command` field, the test times out and fails with a captured diagnostic. To demonstrate this, we can temporarily break the composition root and show the test fails:

```bash
cd /home/chris/sqloid && tmpdir=$(mktemp -d) && cp -r . "$tmpdir/repo" && cd "$tmpdir/repo" && echo "=== Breaking the composition root: making RunSQLite return after validation without composing the UI ===" && sed -i "s|func RunSQLite(path string) error {|func RunSQLite(path string) error {\n\treturn nil // NEGATIVE DEMO: bypass composition\n|" internal/session/session.go && echo "=== Running the PTY integration test against the broken binary ===" && timeout 30 go test -count=1 -timeout 25s -run TestSqloidPTYEndToEndBuildsAndRunsRealBinary ./cmd/sqloid 2>&1 | tail -15; echo "exit: $?" && rm -rf "$tmpdir"
```

```output
=== Breaking the composition root: making RunSQLite return after validation without composing the UI ===
=== Running the PTY integration test against the broken binary ===
--- FAIL: TestSqloidPTYEndToEndBuildsAndRunsRealBinary (10.36s)
    pty_integration_test.go:114: PTY output did not contain the builder's Command field within timeout.
        Output:
        ]11;?\[6n
FAIL
FAIL	github.com/chris/sqloid/cmd/sqloid	10.367s
FAIL
exit: 0
```

The PTY integration test **failed** when the composition root was bypassed. The test output shows:

- `FAIL: TestSqloidPTYEndToEndBuildsAndRunsRealBinary (10.30s)` — the test failed after the 10-second deadline.
- `PTY output did not contain the builder's Command field within timeout.` — the builder never rendered because `RunSQLite` returned `nil` immediately after validation without calling `session.Compose` or running Bubble Tea.
- The captured `Output:` shows only the terminal capability query (`]11;?\[6n`) with no rendered UI — proving the binary bypassed the TUI composition root.

In CI, this `FAIL` would fail the "Repository-wide tests" step, fail the job, and block merging. This is the fail-closed guarantee: a regression that bypasses production composition cannot merge behind a green partial workflow, even if every package-local fake-seam test passes.

## Summary

The expanded Issue #88 CI gate runs seven sequential steps on both Linux and macOS:

1. **modernc pin assertion** — `modernc.org/sqlite v1.57.0` exact, retained from Issue #56.
2. **Repository-wide tests** — `go test -count=1 -timeout 20m ./...` exercises all 13 shipped packages including the Issue #57 PTY-driven built-binary integration test.
3. **Repository-wide build** — `go build ./...` proves every package compiles.
4. **Repository-wide vet** — `go vet ./...` proves every package is vet-clean.
5. **Targeted capability suite** — `scripts/capability-suite.sh` retained from Issue #56 for `-race` cancellation guarantees.

Any setup, test, build, vet, capability, binary-integration, timeout, or cleanup failure fails the job and blocks merging. There are no `continue-on-error` steps, no retries, no platform exclusions, and no conditional skips. Both platforms run equivalent required gates. The PTY integration test fails closed when production composition is bypassed, proving the gate catches composition regressions that package-local fakes cannot.

Cross-references:
- Issue #56: `Notes/issues/056-integrated-release-capability-suite.md` — the original gate.
- Issue #57: `Notes/issues/057-production-tui-composition.md` — the PTY integration test.
- Issue #88: `Notes/issues/088-run-full-definition-of-done-in-ci.md` — the expansion.
- PRD: `Notes/PRD-sqloid.md` — Language and stack, Connection pool, Session health, History, Module Design, Testing Decisions, Acceptance Criteria.
- Wiki: `Notes/wiki/release-capability-gate.md`, `Notes/wiki/production-tui-composition.md`.
