package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/chris/sqloid/internal/cli"
)

// The real binary's exact stream and exit-status contracts are asserted by
// re-executing this test binary as the CLI. TestMain detects the re-execution
// through SQLOID_CLI_UNDER_TEST, runs the CLI shell with recording handlers,
// and exits with the CLI's status.
func TestMain(m *testing.M) {
	if args, ok := os.LookupEnv("SQLOID_CLI_UNDER_TEST"); ok {
		record := os.Getenv("SQLOID_CLI_RECORD")
		handlers := cli.Handlers{
			SQLite: func(path string) error { return appendRecord(record, "sqlite "+path) },
			D1:     func() error { return appendRecord(record, "d1") },
		}
		os.Exit(cli.Main(append([]string{"sqloid"}, strings.Fields(args)...), handlers))
	}
	os.Exit(m.Run())
}

func appendRecord(path, line string) error {
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line + "\n")
	return err
}

// runCLI invokes the test binary as the Sqloid CLI and returns its streams
// and exit status, so assertions cover exactly what the process emits.
func runCLI(t *testing.T, record string, args ...string) (stdout, stderr string, status int) {
	t.Helper()

	cmd := exec.Command(os.Args[0])
	cmd.Args[0] = "sqloid"
	cmd.Env = append(os.Environ(),
		"SQLOID_CLI_UNDER_TEST="+strings.Join(args, " "),
		"SQLOID_CLI_RECORD="+record,
	)
	var out, errOut strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	err := cmd.Run()
	if err == nil {
		status = 0
	} else if exitErr, ok := err.(*exec.ExitError); ok {
		status = exitErr.ExitCode()
	} else {
		t.Fatalf("running CLI: %v", err)
	}
	return out.String(), errOut.String(), status
}

// readRecord returns the dispatch lines recorded by the injected handlers.
func readRecord(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading dispatch record: %v", err)
	}
	return string(data)
}

func TestCLIStreamsAndStatuses(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStatus int
		wantStdout string
	}{
		{
			name:       "missing sqlite argument is a usage failure on stderr with status 2",
			args:       []string{"sqlite"},
			wantStatus: 2,
		},
		{
			name:       "unexpected sqlite argument is a usage failure with status 2",
			args:       []string{"sqlite", "one.db", "two.db"},
			wantStatus: 2,
		},
		{
			name:       "unexpected d1 argument is a usage failure with status 2",
			args:       []string{"d1", "extra"},
			wantStatus: 2,
		},
		{
			name:       "unknown command is a usage failure with status 2",
			args:       []string{"bogus"},
			wantStatus: 2,
		},
		{
			name:       "help exits successfully",
			args:       []string{"--help"},
			wantStatus: 0,
		},
		{
			name:       "short help exits successfully",
			args:       []string{"-h"},
			wantStatus: 0,
		},
		{
			name:       "version writes exactly the version line to stdout",
			args:       []string{"--version"},
			wantStatus: 0,
			wantStdout: "sqloid " + cli.Version + "\n",
		},
		{
			name:       "short version writes exactly the version line to stdout",
			args:       []string{"-v"},
			wantStatus: 0,
			wantStdout: "sqloid " + cli.Version + "\n",
		},
		{
			name:       "successful sqlite dispatch is silent",
			args:       []string{"sqlite", "example.db"},
			wantStatus: 0,
		},
		{
			name:       "successful d1 dispatch is silent",
			args:       []string{"d1"},
			wantStatus: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := t.TempDir() + "/record"
			stdout, stderr, status := runCLI(t, record, tt.args...)

			if status != tt.wantStatus {
				t.Errorf("%v status = %d, want %d (stderr: %q)", tt.args, status, tt.wantStatus, stderr)
			}
			if stdout != tt.wantStdout {
				t.Errorf("%v stdout = %q, want %q", tt.args, stdout, tt.wantStdout)
			}
		})
	}
}

func TestUsageFailureWritesErrorAndUsageToStderr(t *testing.T) {
	stdout, stderr, status := runCLI(t, "", "sqlite")
	if status != 2 {
		t.Fatalf("missing sqlite argument status = %d, want 2", status)
	}
	if stdout != "" {
		t.Errorf("usage failure stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "Usage: sqloid sqlite FILE") {
		t.Errorf("usage failure stderr %q does not contain usage", stderr)
	}
	if !strings.Contains(stderr, "Error:") {
		t.Errorf("usage failure stderr %q does not contain an error message", stderr)
	}
}

func TestHelpGoesToStderr(t *testing.T) {
	stdout, stderr, status := runCLI(t, "", "--help")
	if status != 0 {
		t.Fatalf("--help status = %d, want 0", status)
	}
	if !strings.Contains(stderr, "Usage: sqloid [OPTIONS] COMMAND [arg...]") {
		t.Errorf("--help stderr %q does not contain usage", stderr)
	}
	if !strings.Contains(stderr, "sqlite") || !strings.Contains(stderr, "d1") {
		t.Errorf("--help stderr %q does not list commands", stderr)
	}
	if stdout != "" {
		t.Errorf("--help stdout = %q, want empty", stdout)
	}
}

func TestSuccessfulDispatchRunsHandlersSilently(t *testing.T) {
	record := t.TempDir() + "/record"
	stdout, stderr, status := runCLI(t, record, "sqlite", "example.db")
	if status != 0 || stdout != "" || stderr != "" {
		t.Fatalf("sqlite dispatch: status=%d stdout=%q stderr=%q, want silent success", status, stdout, stderr)
	}
	if got := readRecord(t, record); got != "sqlite example.db\n" {
		t.Errorf("sqlite dispatch record = %q, want %q", got, "sqlite example.db\n")
	}

	record = t.TempDir() + "/record"
	stdout, stderr, status = runCLI(t, record, "d1")
	if status != 0 || stdout != "" || stderr != "" {
		t.Fatalf("d1 dispatch: status=%d stdout=%q stderr=%q, want silent success", status, stdout, stderr)
	}
	if got := readRecord(t, record); got != "d1\n" {
		t.Errorf("d1 dispatch record = %q, want %q", got, "d1\n")
	}
}
