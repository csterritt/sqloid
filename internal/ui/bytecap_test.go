// Model coverage for the Issue #31 byte-cap disclosure: the results header
// renders the single shared Issue #31 definition — internal/result's
// ByteCapWarning — exactly once, row-cap-only eviction metadata never shows
// it, and the disclosure persists through subsequent page traversal and
// result finalization because the typed metadata stays on the result view.

package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chris/sqloid/internal/result"
)

// byteCapModel returns a settled-result model whose result view carries the
// given byte-truncation metadata.
func byteCapModel(t *testing.T, truncated bool) Model {
	t.Helper()
	m := resultModel(t, threeRowPage(), nil)
	if truncated {
		m.Result.ByteTruncated = true
	}
	return m
}

func TestByteCapWarningRenderedFromSharedDefinition(t *testing.T) {
	m := byteCapModel(t, true)
	view := m.View()
	if !strings.Contains(view, result.ByteCapWarning) {
		t.Fatalf("view does not contain the shared byte-cap warning:\n%s", view)
	}
	if got := strings.Count(view, result.ByteCapWarning); got != 1 {
		t.Fatalf("view shows the byte-cap warning %d times, want exactly once:\n%s", got, view)
	}
	if strings.Contains(view, "Result truncated:") && !strings.Contains(view, result.ByteCapWarning) {
		t.Fatal("a second truncation literal is rendered instead of the shared definition")
	}
}

func TestNoByteCapWarningWithoutByteEviction(t *testing.T) {
	m := byteCapModel(t, false)
	if strings.Contains(m.View(), result.ByteCapWarning) {
		t.Fatal("row-cap-only result rendered the byte-cap warning")
	}
}

func TestByteCapDisclosurePersistsThroughTraversal(t *testing.T) {
	pageExec := &fakePageExecutor{rowsShown: 2}
	exec := &fakeSelectExecutor{page: threeRowPage()}
	m := pagingModel(exec, pageExec)
	execModel, execCmd := driveToExecutionStart(t, m)
	m = settleFirstPage(t, execModel, execCmd)
	m.Result.ByteTruncated = true

	// Page Down: the next page settles while the byte-cap disclosure stays.
	cmd := m.handlePageKey(false)
	m = settlePage(t, m, cmd)
	if m.Result == nil || !m.Result.ByteTruncated {
		t.Fatal("page traversal replaced the result view and lost byte-cap disclosure")
	}
	if !strings.Contains(m.View(), result.ByteCapWarning) {
		t.Fatal("view after traversal lost the shared byte-cap warning")
	}

	// A subsequent failed page settlement keeps the disclosure as well.
	if cmd = m.handlePageKey(false); cmd != nil {
		if msg, ok := cmd().(PageSettledMsg); ok {
			msg.Result = FirstPageResult{Err: contextErr()}
			next, _ := m.Update(msg)
			m = asModel(next, nil)
		}
	}
	if m.Result == nil || !m.Result.ByteTruncated {
		t.Fatal("a failed page settlement lost byte-cap disclosure")
	}
}

// prodGoFiles returns every non-test .go file under dir as raw source, so the
// single-literal architecture assertion can scan production source only.
func prodGoFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	files := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		files[name] = string(src)
	}
	return files
}

// TestByteCapWarningSingleLiteral pins that the exact warning literal is
// owned by internal/result only: internal/ui and cmd must not contain their
// own copy of the string, which would fork the presentation contract.
func TestByteCapWarningSingleLiteral(t *testing.T) {
	literal := "Result truncated: 64 MiB cache limit"
	for _, dir := range []string{"../ui", "../../cmd/sqloid"} {
		for name, src := range prodGoFiles(t, dir) {
			if strings.Contains(src, literal) {
				t.Errorf("%s/%s contains a private copy of the byte-cap warning literal; use result.ByteCapWarning", dir, name)
			}
		}
	}
}

func TestTypedLimitFailureMessagesRendered(t *testing.T) {
	m := resultModel(t, threeRowPage(), nil)
	m.Result.LimitFailure = &result.LimitFailure{Kind: result.KindValue, Position: 7}
	m.Result.Page.Rows = [][]result.Value{{result.NewInteger(9)}}
	view := m.View()
	if got := "result value exceeds the 64 MiB v1 limit at row 7"; !strings.Contains(view, got) {
		t.Fatalf("view does not contain the exact value-limit message %q:\n%s", got, view)
	}

	m2 := resultModel(t, threeRowPage(), nil)
	m2.Result.LimitFailure = &result.LimitFailure{Kind: result.KindPage, Position: 12}
	if got := "result page exceeds the 64 MiB v1 limit at row 12"; !strings.Contains(m2.View(), got) {
		t.Fatalf("view does not contain the exact page-limit message %q:\n%s", got, m2.View())
	}
}

func TestTypedLimitFailureSingleLiteral(t *testing.T) {
	for _, literal := range []string{
		"result page exceeds the 64 MiB v1 limit at row",
		"result value exceeds the 64 MiB v1 limit at row",
	} {
		for name, src := range prodGoFiles(t, "../ui") {
			if strings.Contains(src, literal) {
				t.Errorf("internal/ui/%s contains a private copy of %q; use result.LimitFailure.Error", name, literal)
			}
		}
	}
}
