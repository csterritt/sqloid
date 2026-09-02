// Task 1 (RED) for Issue #57: test-only composition coverage around the
// production composition root. These tests open a real temporary SQLite
// database, read its initial schema.Catalog through the production
// composition seam, construct the initial ui.Model, and inspect or exercise
// every resulting database/filesystem seam. They require the model to retain
// the loaded catalog and wire schema-version reads, catalog refresh,
// first-page SELECT, complete-result count, later paging, destructive
// estimates, transactional writes, destination picking, and atomic saves to
// the same owned *connection.DB and real filesystem implementations. They
// prove connection outcomes map to the existing typed UI result,
// cancellation, health-terminal, and write-phase contracts rather than
// leaking driver types into internal/ui, and require an initial catalog
// failure to stop before Bubble Tea starts. They follow the adapters implied
// by internal/ui/model.go, first_select.go, count.go, paging.go,
// schema_validation.go, schema_refresh.go, destructive_prep.go,
// write_exec.go, filepicker.go, and save_write.go, and add no second startup
// or database-opening path.

package session_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chris/sqloid/internal/connection"
	"github.com/chris/sqloid/internal/export"
	"github.com/chris/sqloid/internal/filepicker"
	"github.com/chris/sqloid/internal/result"
	"github.com/chris/sqloid/internal/schema"
	"github.com/chris/sqloid/internal/session"
	"github.com/chris/sqloid/internal/ui"

	_ "modernc.org/sqlite"
)

// createTestDB builds a real SQLite database at path with one table t(id
// INTEGER PRIMARY KEY, name TEXT) and three seeded rows.
func createTestDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open(%q): %v", path, err)
	}
	defer db.Close()
	for _, stmt := range []string{
		"CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)",
		"INSERT INTO t (id, name) VALUES (1, 'alpha'), (2, 'beta'), (3, 'gamma')",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
}

// openTestDB creates a fixture and opens it through the production opener,
// returning the *connection.DB and the path. The caller closes the DB.
func openTestDB(t *testing.T) (*connection.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	createTestDB(t, path)
	db, err := connection.Open(path)
	if err != nil {
		t.Fatalf("connection.Open(%q): %v", path, err)
	}
	return db, path
}

// realCatalog reads the catalog directly from the DB through the existing
// connection boundary so tests can compare it against the composition's
// retained catalog without re-implementing schema decoding.
func realCatalog(t *testing.T, db *connection.DB) *schema.Catalog {
	t.Helper()
	cat, res := db.ReadCatalog(context.Background())
	if res.Outcome != connection.OutcomeSuccess {
		t.Fatalf("realCatalog: %v", res.Err)
	}
	if cat == nil {
		t.Fatal("realCatalog: nil catalog on success")
	}
	return cat
}

// TestComposeRetainsCatalogAndWiresEverySeam opens a real database, composes
// the production session, and requires the retained catalog plus every
// database/filesystem seam to be wired to the real implementations. No driver
// type may leak into internal/ui: the seams are the established typed
// function/interface values.
func TestComposeRetainsCatalogAndWiresEverySeam(t *testing.T) {
	db, _ := openTestDB(t)
	defer db.Close()

	s, err := session.Compose(db)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	defer s.Close()

	want := realCatalog(t, db)
	got := s.Catalog()
	if got == nil {
		t.Fatal("Compose retained a nil catalog")
	}
	if got.Version != want.Version {
		t.Errorf("retained catalog version = %d, want %d", got.Version, want.Version)
	}
	if len(got.Objects) != len(want.Objects) {
		t.Fatalf("retained catalog objects = %d, want %d", len(got.Objects), len(want.Objects))
	}
	for i, obj := range got.Objects {
		if obj.Name != want.Objects[i].Name {
			t.Errorf("object %d name = %q, want %q", i, obj.Name, want.Objects[i].Name)
		}
	}

	m := s.Model()
	// The model's QueryBuilder must carry the same schema so the field bar
	// reflects the real database.
	if tables := m.QB.EligibleTables(); len(tables) == 0 {
		t.Error("model QueryBuilder has no eligible tables from the retained catalog")
	} else if tables[0].Name != "t" {
		t.Errorf("first eligible table = %q, want %q", tables[0].Name, "t")
	}

	// Every database seam must be wired to a real adapter (non-nil). A nil
	// seam would mean the composition left the model unable to issue real
	// database work, defeating the production path.
	if m.Select == nil {
		t.Error("model.Select is nil, want a real first-page adapter")
	}
	if m.Count == nil {
		t.Error("model.Count is nil, want a real count adapter")
	}
	if m.Page == nil {
		t.Error("model.Page is nil, want a real paging adapter")
	}
	if m.VersionReader == nil {
		t.Error("model.VersionReader is nil, want a real schema-version adapter")
	}
	if m.Refresher == nil {
		t.Error("model.Refresher is nil, want a real catalog-refresh adapter")
	}
	if m.Estimator == nil {
		t.Error("model.Estimator is nil, want a real estimate adapter")
	}
	if m.Write == nil {
		t.Error("model.Write is nil, want a real transactional-write adapter")
	}
	if m.History == nil {
		t.Error("model.History is nil, want a fresh session query-history store")
	}
	if m.ResultHistory == nil {
		t.Error("model.ResultHistory is nil, want a fresh session result-history store")
	}

	// The filesystem seams must be nil so the model uses the real
	// filepicker.OSFS and export.OSSaveFS implementations in production.
	if m.PickerFS != nil {
		t.Errorf("model.PickerFS = %T, want nil so the real OSFS is used", m.PickerFS)
	}
	if m.SaveFS != nil {
		t.Errorf("model.SaveFS = %T, want nil so the real OSSaveFS is used", m.SaveFS)
	}
}

// TestComposeSelectExecutorRunsRealFirstPageAndMapsTypedResult exercises the
// wired Select seam directly against the real database and requires the typed
// FirstPageResult to carry real rows without leaking driver types.
func TestComposeSelectExecutorRunsRealFirstPageAndMapsTypedResult(t *testing.T) {
	db, _ := openTestDB(t)
	defer db.Close()

	s, err := session.Compose(db)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	defer s.Close()

	res := s.Model().Select(context.Background(), "SELECT id, name FROM t ORDER BY id", nil)
	if res.Err != nil || res.Cancelled {
		t.Fatalf("Select: err=%v cancelled=%v", res.Err, res.Cancelled)
	}
	if res.Page == nil {
		t.Fatal("Select returned nil page on success")
	}
	if len(res.Page.Rows) != 3 {
		t.Fatalf("Select rows = %d, want 3", len(res.Page.Rows))
	}
	if got := res.Page.Rows[0][1].Display(); got != "alpha" {
		t.Errorf("first row name = %q, want %q", got, "alpha")
	}
}

// TestComposeCountExecutorRunsRealCount exercises the wired Count seam
// directly against the real database and requires the typed CountResult to
// carry the real total.
func TestComposeCountExecutorRunsRealCount(t *testing.T) {
	db, _ := openTestDB(t)
	defer db.Close()

	s, err := session.Compose(db)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	defer s.Close()

	res := s.Model().Count(context.Background(), "SELECT COUNT(*) FROM (SELECT id FROM t)", nil)
	if res.Err != nil || res.Cancelled {
		t.Fatalf("Count: err=%v cancelled=%v", res.Err, res.Cancelled)
	}
	if res.Total != 3 {
		t.Errorf("Count total = %d, want 3", res.Total)
	}
}

// TestComposePageExecutorRunsRealPagedPage exercises the wired Page seam
// directly against the real database with an exact LIMIT/OFFSET range and
// requires the typed FirstPageResult to carry the real paged rows.
func TestComposePageExecutorRunsRealPagedPage(t *testing.T) {
	db, _ := openTestDB(t)
	defer db.Close()

	s, err := session.Compose(db)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	defer s.Close()

	res := s.Model().Page(context.Background(), "SELECT id, name FROM t ORDER BY id LIMIT 1 OFFSET 1", nil, 1)
	if res.Err != nil || res.Cancelled {
		t.Fatalf("Page: err=%v cancelled=%v", res.Err, res.Cancelled)
	}
	if res.Page == nil || len(res.Page.Rows) != 1 {
		t.Fatalf("Page rows = %d, want 1", len(res.Page.Rows))
	}
	if got := res.Page.Rows[0][1].Display(); got != "beta" {
		t.Errorf("paged row name = %q, want %q", got, "beta")
	}
}

// TestComposePageExecutorPassesOffsetToConnection proves Issue #71's adapter
// contract: the wired Page seam passes the supplied logical offset to
// connection.DB.StartPage (via ExecutePage) so an oversized value on a later
// page fails at the one-based absolute logical position, not a page-relative
// position. The fixture has three small rows followed by one oversized BLOB
// at absolute position 4; a page request with offset 3 and limit 1 hits the
// oversized row at page-relative index 0, so the failure must name row 4.
func TestComposePageExecutorPassesOffsetToConnection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.db")
	// Build the fixture through a separate unlimited session.
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("fixture open: %v", err)
	}
	defer fixture.Close()
	for _, stmt := range []string{
		"CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)",
		"INSERT INTO t (id, name) VALUES (1, 'alpha'), (2, 'beta'), (3, 'gamma')",
	} {
		if _, err := fixture.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	oversized := make([]byte, 64*1024*1024+1)
	if _, err := fixture.Exec("INSERT INTO t (id, name) VALUES (4, ?)", oversized); err != nil {
		t.Fatalf("insert oversized row: %v", err)
	}

	db, err := connection.Open(path)
	if err != nil {
		t.Fatalf("connection.Open: %v", err)
	}
	defer db.Close()

	s, err := session.Compose(db)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	defer s.Close()

	res := s.Model().Page(context.Background(),
		"SELECT id, name FROM t ORDER BY id LIMIT 1 OFFSET 3", nil, 3)
	if res.Err == nil {
		t.Fatal("Page: expected a typed value-limit failure, got nil err")
	}
	if res.LimitFailure == nil {
		t.Fatalf("Page: err = %v, want a typed *result.LimitFailure", res.Err)
	}
	if res.LimitFailure.Kind != result.KindValue || res.LimitFailure.Position != 4 {
		t.Fatalf("LimitFailure = %+v, want {KindValue, position 4 (offset 3 + relative 0 + 1)}", res.LimitFailure)
	}
	wantMsg := "result value exceeds the 64 MiB v1 limit at row 4"
	if got := res.LimitFailure.Error(); got != wantMsg {
		t.Fatalf("LimitFailure message = %q, want %q", got, wantMsg)
	}
}

// TestComposeVersionReaderRunsRealSchemaVersion exercises the wired
// VersionReader seam directly against the real database and requires the
// typed schema.VersionAttempt to carry the real PRAGMA schema_version.
func TestComposeVersionReaderRunsRealSchemaVersion(t *testing.T) {
	db, _ := openTestDB(t)
	defer db.Close()

	s, err := session.Compose(db)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	defer s.Close()

	att := s.Model().VersionReader.ReadSchemaVersion()
	if att.Status != schema.RefreshOK {
		t.Fatalf("ReadSchemaVersion status = %v, want RefreshOK", att.Status)
	}
	if att.Version != s.Catalog().Version {
		t.Errorf("ReadSchemaVersion = %d, want retained catalog version %d", att.Version, s.Catalog().Version)
	}
}

// TestComposeRefresherRunsRealCatalogRefresh exercises the wired Refresher
// seam directly against the real database and requires the typed
// schema.Attempt to carry a refreshed catalog matching the real schema.
func TestComposeRefresherRunsRealCatalogRefresh(t *testing.T) {
	db, _ := openTestDB(t)
	defer db.Close()

	s, err := session.Compose(db)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	defer s.Close()

	att := s.Model().Refresher.RefreshCatalog()
	if att.Status != schema.RefreshOK {
		t.Fatalf("RefreshCatalog status = %v, want RefreshOK", att.Status)
	}
	if att.Catalog == nil || len(att.Catalog.Objects) != len(s.Catalog().Objects) {
		t.Errorf("RefreshCatalog catalog objects = %v, want %d", att.Catalog, len(s.Catalog().Objects))
	}
}

// TestComposeEstimatorRunsRealEstimate exercises the wired Estimator seam
// directly against the real database and requires the typed EstimateResult
// to carry the real matching-target count.
func TestComposeEstimatorRunsRealEstimate(t *testing.T) {
	db, _ := openTestDB(t)
	defer db.Close()

	s, err := session.Compose(db)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	defer s.Close()

	res := s.Model().Estimator(context.Background(), "SELECT COUNT(*) FROM t WHERE id >= 2", nil)
	if res.Err != nil || res.Cancelled {
		t.Fatalf("Estimator: err=%v cancelled=%v", res.Err, res.Cancelled)
	}
	if res.Total != 2 {
		t.Errorf("Estimator total = %d, want 2", res.Total)
	}
}

// TestComposeWriteExecutorRunsRealTransactionalWrite exercises the wired
// Write seam directly against the real database and requires the typed
// connection.WriteResult to carry the committed outcome, the real
// RowsAffected, and the relayed phase stream — then verifies the row
// persisted in the database.
func TestComposeWriteExecutorRunsRealTransactionalWrite(t *testing.T) {
	db, path := openTestDB(t)
	defer db.Close()

	s, err := session.Compose(db)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	defer s.Close()

	var phases []connection.WritePhaseMsg
	phase := func(p connection.WritePhaseMsg) { phases = append(phases, p) }
	res := s.Model().Write(context.Background(), 1, "INSERT INTO t (id, name) VALUES (?, ?)", []any{42, "delta"}, phase)
	if res.Outcome != connection.WriteCommitted {
		t.Fatalf("Write outcome = %v err=%v, want WriteCommitted", res.Outcome, res.Err)
	}
	if res.RowsAffected != 1 {
		t.Errorf("Write RowsAffected = %d, want 1", res.RowsAffected)
	}
	if res.Health != nil {
		t.Errorf("Write Health = %v, want nil on a healthy commit", res.Health)
	}
	if len(phases) < 2 {
		t.Errorf("relayed phases = %v, want at least Beginning and Committing", phases)
	}
	var sawCommit bool
	for _, p := range phases {
		if p.Phase == connection.WritePhaseCommitting {
			sawCommit = true
		}
	}
	if !sawCommit {
		t.Errorf("relayed phases %v never included Committing", phases)
	}

	// Verify the write persisted in the real database through an independent
	// connection.
	verify, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer verify.Close()
	var name string
	if err := verify.QueryRow("SELECT name FROM t WHERE id = 42").Scan(&name); err != nil {
		t.Fatalf("persisted row read: %v", err)
	}
	if name != "delta" {
		t.Errorf("persisted name = %q, want %q", name, "delta")
	}
}

// TestComposeSelectExecutorMapsCancellationToTypedResult requires a cancelled
// context to settle the wired Select seam as Cancelled in the typed
// FirstPageResult, proving the adapter maps the connection outcome rather
// than leaking driver cancellation errors.
func TestComposeSelectExecutorMapsCancellationToTypedResult(t *testing.T) {
	db, _ := openTestDB(t)
	defer db.Close()

	s, err := session.Compose(db)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := s.Model().Select(ctx, "SELECT id FROM t", nil)
	if !res.Cancelled {
		t.Errorf("Select on cancelled context: Cancelled = false, want true (err=%v)", res.Err)
	}
	if res.Page != nil {
		t.Errorf("Select on cancelled context returned rows, want nil page")
	}
}

// TestComposeSelectExecutorMapsHealthClassificationToTypedResult requires a
// request-boundary deletion (the file vanishing after open) to surface as a
// typed *connection.HealthError inside FirstPageResult.Err so the UI's
// health-terminal mapping (errors.As) classifies it without parsing driver
// text.
func TestComposeSelectExecutorMapsHealthClassificationToTypedResult(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; file-removal fixtures cannot be exercised")
	}
	db, path := openTestDB(t)
	defer db.Close()

	s, err := session.Compose(db)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	defer s.Close()

	if err := os.Remove(path); err != nil {
		t.Fatalf("removing database file: %v", err)
	}
	res := s.Model().Select(context.Background(), "SELECT id FROM t", nil)
	if res.Err == nil {
		t.Fatal("Select after file removal: nil Err, want a typed health classification")
	}
	var he *connection.HealthError
	if !errors.As(res.Err, &he) {
		t.Fatalf("Select Err = %T, want a *connection.HealthError in the chain", res.Err)
	}
	if he.Kind != connection.HealthDeleted {
		t.Errorf("health kind = %v, want HealthDeleted", he.Kind)
	}
}

// TestComposeInitialCatalogFailureStopsBeforeBubbleTea requires a catalog
// load that cannot succeed (the file vanishing after a successful open) to
// stop the composition with an error before any Bubble Tea program could
// start.
func TestComposeInitialCatalogFailureStopsBeforeBubbleTea(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; file-removal fixtures cannot be exercised")
	}
	db, path := openTestDB(t)
	defer db.Close()

	if err := os.Remove(path); err != nil {
		t.Fatalf("removing database file: %v", err)
	}
	if _, err := session.Compose(db); err == nil {
		t.Fatal("Compose with an unreadable catalog returned nil, want an error that stops before Bubble Tea")
	}
}

// TestComposeDoesNotLeakDriverTypesIntoUI requires the wired seams to use only
// the established UI typed values (result.Value, result.Page,
// connection.WriteResult, schema.VersionAttempt, schema.Attempt) and never the
// raw modernc driver types. This guards the boundary the PRD Module Design
// section sets for internal/ui.
func TestComposeDoesNotLeakDriverTypesIntoUI(t *testing.T) {
	db, _ := openTestDB(t)
	defer db.Close()

	s, err := session.Compose(db)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	defer s.Close()

	res := s.Model().Select(context.Background(), "SELECT id, name FROM t", nil)
	if res.Err != nil {
		t.Fatalf("Select: %v", res.Err)
	}
	if res.Page == nil {
		t.Fatal("Select returned nil page")
	}
	// result.Value is the only typed cell representation the UI accepts; the
	// adapter must not hand back []any driver cells.
	for _, row := range res.Page.Rows {
		for _, v := range row {
			if v.Kind == 0 {
				t.Errorf("row cell has zero Kind, want a typed result.Value")
			}
		}
	}
}

// TestComposeSaveFlowResolvesToRealOSSaveFS requires the model's SaveFS to be
// nil so the save seam resolves to the real export.OSSaveFS, and proves the
// real atomic write persists bytes to a real temporary file.
func TestComposeSaveFlowResolvesToRealOSSaveFS(t *testing.T) {
	db, _ := openTestDB(t)
	defer db.Close()

	s, err := session.Compose(db)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	defer s.Close()

	if m := s.Model(); m.SaveFS != nil {
		t.Fatalf("model.SaveFS = %T, want nil for the real OSSaveFS", m.SaveFS)
	}
	dest := filepath.Join(t.TempDir(), "saved.sql")
	fs := export.OSSaveFS{}
	state, err := export.InspectDestination(fs, dest)
	if err != nil {
		t.Fatalf("InspectDestination: %v", err)
	}
	if err := export.WriteAtomic(fs, dest, []byte("SELECT 1;\n"), state, export.IntentNoReplace); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading saved file: %v", err)
	}
	if string(got) != "SELECT 1;\n" {
		t.Errorf("saved file = %q, want %q", got, "SELECT 1;\n")
	}
}

// TestComposePickerFSResolvesToRealOSFS requires the model's PickerFS to be
// nil so the picker uses the real filepicker.OSFS, and proves the real
// boundary can list a temporary directory.
func TestComposePickerFSResolvesToRealOSFS(t *testing.T) {
	db, _ := openTestDB(t)
	defer db.Close()

	s, err := session.Compose(db)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	defer s.Close()

	if s.Model().PickerFS != nil {
		t.Fatalf("model.PickerFS = %T, want nil for the real OSFS", s.Model().PickerFS)
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := filepicker.OSFS{}.ReadDir(dir)
	if err != nil {
		t.Fatalf("OSFS.ReadDir: %v", err)
	}
	if len(entries) != 1 || !entries[0].IsDir() || entries[0].Name() != "sub" {
		t.Errorf("OSFS.ReadDir = %v, want one entry %q", entries, "sub")
	}
}

// TestComposeCloseReleasesDatabase requires Close to release the owned
// database pool so the composition owns the *connection.DB lifecycle exactly
// once.
func TestComposeCloseReleasesDatabase(t *testing.T) {
	db, _ := openTestDB(t)

	s, err := session.Compose(db)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	// A second Close must be a safe no-op; the pool is already released.
	if err := s.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestComposeModelAcceptsWindowSizeAndRendersBuilder proves the composed
// model is a normal ui.Model: a WindowSizeMsg sizes it and its View renders
// the full-screen builder rather than returning after validation.
func TestComposeModelAcceptsWindowSizeAndRendersBuilder(t *testing.T) {
	db, _ := openTestDB(t)
	defer db.Close()

	s, err := session.Compose(db)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	defer s.Close()

	m := s.Model()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(ui.Model)
	if m.Width != 100 || m.Height != 30 {
		t.Fatalf("model dimensions = %dx%d, want 100x30", m.Width, m.Height)
	}
	view := m.View()
	if view == "" {
		t.Fatal("model View is empty after sizing, want the full-screen builder")
	}
}

// Ensure the ui.Model zero value's exported fields used above exist at
// compile time as a static guard against accidental seam renaming.
var _ ui.Model
