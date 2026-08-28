// Request-boundary tests for pre-execution schema-version revalidation
// (Issue #21, Task 1): PRAGMA schema_version reads are cancellable requests,
// an unchanged version reuses the cached catalog without issuing a catalog
// refresh, a changed version refreshes the selected object and columns from
// main.sqlite_master through the established ReadCatalog seam, an ordinary
// corruption refresh failure retains the prior cache without partial
// replacement, and DDL after successful validation is a later ordinary
// event that never retroactively mutates the settled revalidation outcome.

package connection

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/chris/sqloid/internal/schema"
)

// TestReadSchemaVersionReturnsCurrentVersion pins that the version read is a
// real request whose value matches the version recorded with a catalog read.
func TestReadSchemaVersionReturnsCurrentVersion(t *testing.T) {
	path := t.TempDir() + "/version.db"
	createDatabase(t, path)

	db := mustOpen(t, path)
	version, res := db.ReadSchemaVersion(context.Background())
	if res.Outcome != OutcomeSuccess || res.Err != nil {
		t.Fatalf("ReadSchemaVersion result = %+v, want success without error", res)
	}
	cat, res := db.ReadCatalog(context.Background())
	if res.Outcome != OutcomeSuccess {
		t.Fatalf("ReadCatalog result = %+v, want success", res)
	}
	if version != cat.Version {
		t.Errorf("ReadSchemaVersion = %d, want the catalog version %d", version, cat.Version)
	}
}

// TestReadSchemaVersionCancelledContextFailsWithCancellation proves the
// version read honours its context like any other request.
func TestReadSchemaVersionCancelledContextFailsWithCancellation(t *testing.T) {
	path := t.TempDir() + "/version-cancelled.db"
	createDatabase(t, path)

	db := mustOpen(t, path)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, res := db.ReadSchemaVersion(ctx)
	if res.Outcome != OutcomeFailed {
		t.Fatalf("ReadSchemaVersion outcome = %v, want failed", res.Outcome)
	}
	if !errors.Is(res.Err, context.Canceled) {
		t.Errorf("error = %v, want a context.Canceled cause", res.Err)
	}
}

// TestRevalidateUnchangedVersionSkipsCatalogRefresh proves the full
// Connection-backed flow: after reading the current version, a revalidation
// against a prior catalog of the same version reuses the exact cached
// catalog and never issues the catalog-refresh request.
func TestRevalidateUnchangedVersionSkipsCatalogRefresh(t *testing.T) {
	path := t.TempDir() + "/unchanged.db"
	createDatabase(t, path)

	db := mustOpen(t, path)
	prior, res := db.ReadCatalog(context.Background())
	if res.Outcome != OutcomeSuccess {
		t.Fatalf("initial ReadCatalog result = %+v, want success", res)
	}
	priorObject := prior.Objects[0]

	version, res := db.ReadSchemaVersion(context.Background())
	if res.Outcome != OutcomeSuccess {
		t.Fatalf("ReadSchemaVersion result = %+v, want success", res)
	}

	refreshCalls := 0
	refresh := func() schema.Attempt {
		refreshCalls++
		cat, res := db.ReadCatalog(context.Background())
		if res.Outcome != OutcomeSuccess {
			return schema.NewFailure(res.Err)
		}
		return schema.NewSuccess(cat)
	}
	got := schema.Revalidate(prior, version, refresh)

	if got.Status != schema.RevalidateUnchanged {
		t.Errorf("revalidation status = %v, want RevalidateUnchanged", got.Status)
	}
	if refreshCalls != 0 {
		t.Errorf("catalog refresh issued %d times on an unchanged version, want 0", refreshCalls)
	}
	if got.Catalog != prior {
		t.Error("unchanged revalidation did not reuse the exact cached catalog")
	}
	if got.Catalog.Objects[0] != priorObject {
		t.Error("unchanged revalidation did not reuse the exact cached object metadata")
	}
}

// TestRevalidateChangedVersionRefreshesSelectedObjectAndColumns proves a
// changed PRAGMA schema_version refreshes the object and its columns from
// main.sqlite_master through the established seam, and that the prior
// catalog cache is never partially replaced.
func TestRevalidateChangedVersionRefreshesSelectedObjectAndColumns(t *testing.T) {
	path := t.TempDir() + "/changed.db"
	createDatabase(t, path)

	db := mustOpen(t, path)
	prior, res := db.ReadCatalog(context.Background())
	if res.Outcome != OutcomeSuccess {
		t.Fatalf("initial ReadCatalog result = %+v, want success", res)
	}

	if _, err := db.SQL.Exec("ALTER TABLE t ADD COLUMN extra TEXT"); err != nil {
		t.Fatalf("adding a column: %v", err)
	}
	version, res := db.ReadSchemaVersion(context.Background())
	if res.Outcome != OutcomeSuccess {
		t.Fatalf("ReadSchemaVersion result = %+v, want success", res)
	}
	if version == prior.Version {
		t.Fatalf("version = %d, want it changed after DDL", version)
	}

	got := schema.Revalidate(prior, version, func() schema.Attempt {
		cat, res := db.ReadCatalog(context.Background())
		if res.Outcome != OutcomeSuccess {
			return schema.NewFailure(res.Err)
		}
		return schema.NewSuccess(cat)
	})

	if got.Status != schema.RevalidateRefreshed {
		t.Fatalf("revalidation status = %v, want RevalidateRefreshed", got.Status)
	}
	var refreshed *schema.Object
	for _, o := range got.Catalog.Objects {
		if o.Name == "t" {
			refreshed = o
		}
	}
	if refreshed == nil {
		t.Fatal("refreshed catalog lost the selected object t")
	}
	found := false
	for _, col := range refreshed.Columns {
		if col.Name == "extra" {
			found = true
		}
	}
	if !found {
		t.Error("refreshed columns do not include the DDL-added extra column")
	}
	// The prior cache must stand untouched as its own immutable snapshot.
	if len(prior.Objects) != 1 || len(prior.Objects[0].Columns) != 2 {
		t.Errorf("prior catalog was partially replaced: %+v", prior.Objects[0].Columns)
	}
	if prior.Version != got.Catalog.Version-0 && prior.Version >= got.Catalog.Version {
		t.Errorf("prior version %d not below refreshed version %d", prior.Version, got.Catalog.Version)
	}
}

// TestRevalidateOrdinaryCorruptionRefreshFailureRetainsPriorCache proves an
// ordinary refresh failure carries only its cause: the caller keeps the
// complete prior catalog with no partial replacement, exactly as the
// stale-retention contract requires.
func TestRevalidateOrdinaryCorruptionRefreshFailureRetainsPriorCache(t *testing.T) {
	path := t.TempDir() + "/corrupt.db"
	createDatabase(t, path)

	db := mustOpen(t, path)
	prior, res := db.ReadCatalog(context.Background())
	if res.Outcome != OutcomeSuccess {
		t.Fatalf("initial ReadCatalog result = %+v, want success", res)
	}

	// Corrupt the file in place: the device/inode identity still matches, so
	// the request passes the boundary check and fails with an ordinary
	// database error during the catalog read.
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %q: %v", path, err)
	}
	copy(body[0:], []byte("corrupted sqlite headerxxxxxxxxxxxxxxx"))
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("writing corrupted body: %v", err)
	}

	got := schema.Revalidate(prior, prior.Version+1, func() schema.Attempt {
		cat, res := db.ReadCatalog(context.Background())
		if res.Outcome != OutcomeSuccess {
			return schema.NewFailure(res.Err)
		}
		return schema.NewSuccess(cat)
	})

	if got.Status != schema.RevalidateRefreshFailed {
		t.Fatalf("revalidation status = %v, want RevalidateRefreshFailed", got.Status)
	}
	if got.Cause == nil {
		t.Fatal("ordinary refresh failure carried no cause")
	}
	if got.Catalog != nil {
		t.Errorf("failed revalidation catalog = %v, want nil so the prior cache stands", got.Catalog)
	}
	if len(prior.Objects) != 1 || len(prior.Objects[0].Columns) != 2 {
		t.Errorf("prior catalog was partially replaced: %+v", prior.Objects[0])
	}
}

// TestPostValidationDDLRaceDoesNotMutateValidationOutcome proves that DDL
// racing in after a successful validation is a later ordinary event: the
// settled revalidation outcome stays exactly as returned, and only a
// subsequent revalidation observes the change through a fresh version read.
func TestPostValidationDDLRaceDoesNotMutateValidationOutcome(t *testing.T) {
	path := t.TempDir() + "/race.db"
	createDatabase(t, path)

	db := mustOpen(t, path)
	prior, _ := db.ReadCatalog(context.Background())
	version, _ := db.ReadSchemaVersion(context.Background())
	validated := schema.Revalidate(prior, version, func() schema.Attempt {
		t.Fatal("refresh must not be issued for an unchanged version")
		return schema.Attempt{}
	})
	if validated.Status != schema.RevalidateUnchanged {
		t.Fatalf("validation status = %v, want RevalidateUnchanged", validated.Status)
	}

	// DDL after successful validation succeeds; the post-validation race.
	if _, err := db.SQL.Exec("ALTER TABLE t ADD COLUMN raced TEXT"); err != nil {
		t.Fatalf("post-validation DDL: %v", err)
	}

	if validated.Status != schema.RevalidateUnchanged || validated.Catalog != prior {
		t.Fatalf("settled validation outcome mutated: %+v", validated)
	}
	nextVersion, res := db.ReadSchemaVersion(context.Background())
	if res.Outcome != OutcomeSuccess {
		t.Fatalf("post-race ReadSchemaVersion result = %+v, want success", res)
	}
	next := schema.Revalidate(validated.Catalog, nextVersion, func() schema.Attempt {
		cat, res := db.ReadCatalog(context.Background())
		if res.Outcome != OutcomeSuccess {
			return schema.NewFailure(res.Err)
		}
		return schema.NewSuccess(cat)
	})
	if next.Status != schema.RevalidateRefreshed {
		t.Errorf("subsequent revalidation status = %v, want RevalidateRefreshed", next.Status)
	}
}
