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
