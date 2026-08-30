// Architecture assertions for Issue #22 Task 7: internal/result stays the
// single, UI-independent result representation and no second production
// execution or formatting route appears outside it. These checks parse
// repository source rather than behavior, so a violation fails the build
// before any grid or exporter can start copying representation logic.

package result

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// goFiles returns every non-test .go file under dir, as parsed source.
func goFiles(t *testing.T, dir string) map[string]string {
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

// TestResultPackageStaysUIIndependent forbids Bubble Tea or driver imports in
// the shared result package: its seam must remain plain data plus policy.
func TestResultPackageStaysUIIndependent(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse result package: %v", err)
	}
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			for _, imp := range f.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				if path == "github.com/charmbracelet/bubbletea" || path == "database/sql" || strings.Contains(path, "modernc.org") {
					t.Errorf("%s imports %s: result package must stay UI- and driver-independent", f.Name.Name, path)
				}
			}
		}
	}
}

// TestNoUIPrivateResultRepresentation forbids the UI package from owning
// private copies of the shared representation: REAL token generation, BLOB
// display text, UTF-8 replacement, and name deduplication may exist only in
// internal/result.
func TestNoUIPrivateResultRepresentation(t *testing.T) {
	banned := []string{"FormatFloat", "[BLOB ", "RuneError", "FormatInt"}
	for name, src := range goFiles(t, "../ui") {
		for _, b := range banned {
			if strings.Contains(src, b) {
				t.Errorf("internal/ui/%s contains %q: result representation must come from internal/result", name, b)
			}
		}
	}
}

// TestNoExporterPrivateResultRepresentation forbids the exporter-facing
// internal/export boundary from owning private copies of the shared
// representation (Issue #47 Task 5): name collision suffixing, numeric token
// formatting, and UTF-8 replacement may exist only in internal/result, while
// format-specific CSV/JSON escaping stays out until Issues #50/#51 own it.
func TestNoExporterPrivateResultRepresentation(t *testing.T) {
	banned := []string{"FormatFloat", "FormatInt", "RuneError", "[BLOB ", "strconv", "_\"", "encoding/csv", "encoding/json", "unicode/utf8"}
	for name, src := range goFiles(t, "../export") {
		for _, b := range banned {
			if strings.Contains(src, b) {
				t.Errorf("internal/export/%s contains %q: shared result policies must come from internal/result", name, b)
			}
		}
	}
}

// TestSingleProductionExecutionRoute pins that no removed Issue #10 tracer
// execution route survives anywhere in production source: the UI model keeps
// no Trace state, and only the builder→validation→ExecutionStartedMsg→Select
// path exists.
func TestSingleProductionExecutionRoute(t *testing.T) {
	for dir := range map[string]bool{"../ui": true, "../../cmd/sqloid": true} {
		for name, src := range goFiles(t, dir) {
			if strings.Contains(strings.ToLower(src), "tracer") || strings.Contains(src, "TraceResult") || strings.Contains(src, "StartTraceMsg") {
				t.Errorf("%s/%s still references the removed tracer route", dir, name)
			}
		}
	}
}
